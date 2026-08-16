package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// PlaywrightService Playwright 测试运行服务。
// 负责执行测试、解析结果、查询历史记录与生成测试配置文件。
type PlaywrightService struct {
	db  *gorm.DB
	cfg *config.Config
}

// NewPlaywrightService 构造 Playwright 测试服务。
func NewPlaywrightService(db *gorm.DB, cfg *config.Config) *PlaywrightService {
	return &PlaywrightService{db: db, cfg: cfg}
}

// 测试结果正则：匹配 Playwright 输出中的 "5 passed" 和 "1 failed" 等格式。
var (
	passCountRegex = regexp.MustCompile(`(\d+)\s+passed`)
	failCountRegex = regexp.MustCompile(`(\d+)\s+failed`)
)

// streamWriter 同时写入 bytes.Buffer 并通过回调推送文本流的 io.Writer。
type streamWriter struct {
	buf      *bytes.Buffer
	callback func(string)
}

func (w *streamWriter) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	if w.callback != nil {
		w.callback(string(p))
	}
	return n, err
}

// RunTest 在指定工作目录执行 Playwright 测试。
// 通过 onOutput 回调实时推送 stdout/stderr 输出，解析测试通过/失败数量，
// 将结果持久化到 TestRun 记录（状态流转：running -> passed/failed）。
func (s *PlaywrightService) RunTest(projectID int64, moduleID *int64, taskID *int64, testType string, workDir string, onOutput func(string)) (*model.TestRun, error) {
	now := time.Now()
	run := &model.TestRun{
		ProjectID: projectID,
		ModuleID:  moduleID,
		TaskID:    taskID,
		TestType:  testType,
		Status:    "running",
		StartedAt: &now,
	}

	// 创建运行记录
	if err := s.db.Create(run).Error; err != nil {
		return nil, fmt.Errorf("创建测试运行记录失败: %w", err)
	}

	// 解析可执行文件路径
	binary := s.cfg.Playwright.BinaryPath
	if binary == "" {
		binary = "npx"
	}

	// 超时设置
	timeout := s.cfg.Playwright.Timeout
	if timeout <= 0 {
		timeout = 120
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// 构建命令：npx playwright test
	cmd := exec.CommandContext(ctx, binary, "playwright", "test")
	cmd.Dir = workDir

	// 环境变量
	env := os.Environ()
	if s.cfg.Playwright.Headless {
		env = append(env, "CI=true")
	}
	if s.cfg.Playwright.BaseURL != "" {
		env = append(env, "PLAYWRIGHT_BASE_URL="+s.cfg.Playwright.BaseURL)
	}
	cmd.Env = env

	// 使用 bytes.Buffer 捕获输出，通过 streamWriter 实时推送回调
	var outputBuf bytes.Buffer
	writer := &streamWriter{buf: &outputBuf, callback: onOutput}
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Start(); err != nil {
		s.failRun(run, fmt.Sprintf("启动测试命令失败: %v", err), &outputBuf, onOutput)
		return run, fmt.Errorf("启动测试命令失败: %w", err)
	}

	// 等待命令完成
	waitErr := cmd.Wait()

	// 计算耗时
	completedAt := time.Now()
	run.CompletedAt = &completedAt
	run.Duration = int(completedAt.Sub(now).Seconds())
	run.Output = outputBuf.String()

	// 解析测试结果
	passCount, failCount := parseTestResults(run.Output)
	run.PassCount = passCount
	run.FailCount = failCount

	// 判定最终状态
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		run.Status = "failed"
		run.Output += "\n[PlaywrightService] 测试执行超时\n"
	case failCount > 0:
		run.Status = "failed"
	case waitErr != nil:
		// 命令非零退出且未能解析到失败数，判定为失败
		run.Status = "failed"
	default:
		run.Status = "passed"
	}

	if err := s.db.Save(run).Error; err != nil {
		log.Printf("更新测试运行记录失败: %v", err)
	}

	return run, nil
}

// GetTestRun 按 ID 查询单条测试运行记录。
func (s *PlaywrightService) GetTestRun(runID int64) (*model.TestRun, error) {
	var run model.TestRun
	if err := s.db.First(&run, runID).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// ListProjectTests 查询项目下所有测试运行记录，按创建时间倒序排列。
func (s *PlaywrightService) ListProjectTests(projectID int64) ([]model.TestRun, error) {
	var runs []model.TestRun
	if err := s.db.Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

// GenerateTestSpec 为项目生成 playwright.config.ts 配置文件模板。
// 在项目工作目录下创建配置文件及 tests 目录，返回配置文件路径。
func (s *PlaywrightService) GenerateTestSpec(projectID int64, url string, description string) (string, error) {
	if url == "" {
		url = s.cfg.Playwright.BaseURL
	}

	var project model.Project
	if err := s.db.First(&project, projectID).Error; err != nil {
		return "", fmt.Errorf("查询项目失败: %w", err)
	}

	workDir := project.WorkDir
	if workDir == "" {
		return "", errors.New("项目工作目录未设置")
	}

	configContent := fmt.Sprintf(`import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright 配置文件 - %s
 * 由 Loafer 平台自动生成
 * 描述: %s
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['line'], ['html', { open: 'never' }]],
  use: {
    baseURL: '%s',
    trace: 'on-first-retry',
    headless: %v,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
  ],
});
`, project.Name, description, url, s.cfg.Playwright.Headless)

	configPath := filepath.Join(workDir, "playwright.config.ts")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 创建 tests 目录
	testsDir := filepath.Join(workDir, "tests")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		log.Printf("创建 tests 目录失败: %v", err)
	}

	return configPath, nil
}

// failRun 将测试运行记录标记为失败并保存，同时通过回调推送错误信息。
func (s *PlaywrightService) failRun(run *model.TestRun, msg string, buf *bytes.Buffer, onOutput func(string)) {
	completedAt := time.Now()
	run.CompletedAt = &completedAt
	run.Status = "failed"
	run.Output = buf.String() + "\n" + msg + "\n"
	if onOutput != nil {
		onOutput("\n" + msg + "\n")
	}
	if err := s.db.Save(run).Error; err != nil {
		log.Printf("更新测试运行记录失败: %v", err)
	}
}

// parseTestResults 从 Playwright 输出文本中解析通过/失败数量。
// 取最后一组匹配结果，因为 Playwright 的汇总行位于输出末尾。
func parseTestResults(output string) (passCount, failCount int) {
	matches := passCountRegex.FindAllStringSubmatch(output, -1)
	if len(matches) > 0 {
		last := matches[len(matches)-1]
		if n, err := strconv.Atoi(last[1]); err == nil {
			passCount = n
		}
	}

	matches = failCountRegex.FindAllStringSubmatch(output, -1)
	if len(matches) > 0 {
		last := matches[len(matches)-1]
		if n, err := strconv.Atoi(last[1]); err == nil {
			failCount = n
		}
	}

	return passCount, failCount
}
