package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"loafer-agent/internal/model"
	"loafer-agent/internal/service"

	"gorm.io/gorm"
)

// detectBuildSteps 检测 workDir 下可执行的构建步骤，返回 shell 命令列表。
// 支持两种项目布局：
//  1. 单工程：根目录直接包含 go.mod 或 package.json；
//  2. 前后端分离 monorepo：一级子目录分别包含 go.mod / package.json
//     （如 backend-go/ + frontend/），每个匹配子目录生成一条构建命令。
//
// 空切片表示未检测到任何可构建清单。
func detectBuildSteps(workDir string) []string {
	var steps []string
	if fileExists(filepath.Join(workDir, "go.mod")) {
		steps = append(steps, "go build ./...")
	}
	if fileExists(filepath.Join(workDir, "package.json")) {
		steps = append(steps, "npm run build")
	}
	if len(steps) > 0 {
		return steps
	}

	entries, err := os.ReadDir(workDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
			continue
		}
		sub := filepath.Join(workDir, e.Name())
		if fileExists(filepath.Join(sub, "go.mod")) {
			steps = append(steps, fmt.Sprintf("(cd %s && go build ./...)", e.Name()))
		} else if fileExists(filepath.Join(sub, "package.json")) {
			steps = append(steps, fmt.Sprintf("(cd %s && npm run build)", e.Name()))
		}
	}
	return steps
}

// runLocalBuildVerify 在本地 workDir 执行构建校验，通过 onOutput 输出过程信息，返回是否通过。
// 供 InfraVerifyStream 与流水线 runInfraVerificationForPipeline 共用。
func runLocalBuildVerify(workDir string, onOutput func(string)) bool {
	onOutput(fmt.Sprintf("▶ 开始构建校验（work_dir=%s）", workDir))
	steps := detectBuildSteps(workDir)
	if len(steps) == 0 {
		onOutput("✗ 未检测到可构建的清单文件（根目录或一级子目录下的 go.mod / package.json），无法执行构建校验")
		return false
	}
	for _, step := range steps {
		onOutput(fmt.Sprintf("  $ %s", step))
		out, err := runLocalCommand(buildEnvPrefix()+fmt.Sprintf("cd %s && %s 2>&1", workDir, step), 5*time.Minute)
		if strings.TrimSpace(out) != "" {
			onOutput(out)
		}
		if err != nil {
			onOutput(fmt.Sprintf("✗ 构建失败: %v", err))
			return false
		}
	}
	onOutput("✓ 构建校验通过")
	return true
}

// buildEnvPrefix 委托 service.BuildEnvPrefix（已迁移至 service 包以复用）。
func buildEnvPrefix() string {
	return service.BuildEnvPrefix()
}

// runStartupVerify 启动验证：确保项目存在可用部署，然后对 AccessURL 发起 HTTP 探活
// （最多 5 次，间隔 2 秒，HTTP < 500 视为通过）。
// 项目从未部署（无部署记录或记录无 AccessURL）时，先调用 DeployService 自动部署——
// 首跑项目在基础架构模块验证时不存在任何部署记录，部署记录原本只在业务模块门禁中创建。
// 供 InfraVerifyStream 与流水线 runInfraVerificationForPipeline 共用。
func runStartupVerify(db *gorm.DB, deployService *service.DeployService, projectID int64, onOutput func(string)) bool {
	onOutput("▶ 开始启动验证（探活部署访问地址）")

	accessURL := ""
	var deployment model.ProjectDeployment
	if err := db.Where("project_id = ?", projectID).Order("id DESC").First(&deployment).Error; err != nil {
		onOutput(fmt.Sprintf("⚠ 未找到部署记录（%v），将先执行部署", err))
	} else {
		accessURL = deployment.AccessURL
		if accessURL == "" {
			onOutput("⚠ 部署记录无 AccessURL，将重新部署")
		}
	}

	if accessURL == "" {
		dep, err := deployService.Deploy(projectID, false, onOutput)
		if err != nil {
			onOutput(fmt.Sprintf("✗ 启动验证失败：自动部署失败: %v", err))
			return false
		}
		if dep == nil || dep.AccessURL == "" {
			onOutput("✗ 启动验证失败：部署完成但未生成访问地址")
			return false
		}
		accessURL = dep.AccessURL
	}

	onOutput(fmt.Sprintf("探活地址: %s", accessURL))
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		resp, err := client.Get(accessURL)
		if err == nil {
			resp.Body.Close()
			onOutput(fmt.Sprintf("  第 %d 次探活：HTTP %d", attempt, resp.StatusCode))
			if resp.StatusCode < 500 {
				onOutput("✓ 启动验证通过")
				return true
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			lastErr = err
			onOutput(fmt.Sprintf("  第 %d 次探活失败: %v", attempt, err))
		}
		if attempt < 5 {
			time.Sleep(2 * time.Second)
		}
	}
	onOutput(fmt.Sprintf("✗ 启动验证失败: %v", lastErr))
	return false
}
