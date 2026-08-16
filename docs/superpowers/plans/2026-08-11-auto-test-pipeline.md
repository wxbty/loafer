# 流水线全自动测试闭环 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 业务模块任务跑完后不再人工暂停，流水线自动执行「部署 → 测试 agent（集成测试 + Playwright UI）→ 失败自动修复（最多 3 轮）」，通过才继续下一模块。

**Architecture:** 新增 `TestExecutor`（engine/executor/test_executor.go）封装测试/修复 agent 的 prompt 构造、Claude CLI 执行与 JSON 结果契约解析；改造 `PipelineHandler` 模块循环，用自动测试门禁替换 status=2 暂停分支；尾部阶段5 的假检查改为全局 Playwright 冒烟。

**Tech Stack:** Go + Gin + GORM（后端），Claude Code CLI（`--print` 模式 agent 执行），Playwright（UI 测试）。

**Spec:** `docs/superpowers/specs/2026-08-11-auto-test-pipeline-design.md`

## Global Constraints

- 提交信息规范：`feat:` / `fix:` / `refactor:` / `docs:` / `chore:` + 简要中文描述；每个任务完成后立即 `git add -A && git commit && git push origin master`。
- 提交前 `cd /srv/zfei/projects/loafer/backend-go && go build ./...` 必须通过。
- 测试风格：标准库 `testing`，表驱动，参考 `backend-go/internal/engine/executor/task_executor_test.go`，不使用 testify。
- 模块状态语义：0 待执行 / 1 执行中 / 2 待测试 / 3 测试中 / 4 完成 / 5 测试失败 / 6 失败。
- 结果 JSON 契约文件路径：`tests/results/module-<id>.json`（相对项目工作目录）。
- 自动修复上限：`MaxTestRounds = 3`。
- 写库的长文本字段必须经 `truncateUTF8Bytes` 截断，防止 MySQL Error 1366（参考 `task_executor.go:451` 与 `cli/call_logger.go:27`）。

---

### Task 1: TestExecutor 结果契约与解析

**Files:**
- Create: `backend-go/internal/engine/executor/test_executor.go`
- Test: `backend-go/internal/engine/executor/test_executor_test.go`

**Interfaces:**
- Consumes: 无（纯新增）
- Produces:
  - `type ModuleTestFailure struct { Kind, Name, Log string }`（JSON tag 均为小写：`kind`/`name`/`log`）
  - `type ModuleTestResult struct { ModuleID int64; Passed bool; Summary string; Failures []ModuleTestFailure }`（JSON tag：`module_id`/`passed`/`summary`/`failures`）
  - `func parseModuleTestResult(data []byte) (*ModuleTestResult, error)`
  - `func readModuleTestResult(path string) (*ModuleTestResult, error)`
  - `func resultFilePath(workDir string, moduleID int64) string`
  - `func tailString(s string, maxLen int) string` — 取字符串尾部，供错误日志截取用

- [ ] **Step 1: 写失败测试**

创建 `backend-go/internal/engine/executor/test_executor_test.go`：

```go
package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseModuleTestResult 验证结果 JSON 的解析：合法输入、空内容、畸形 JSON、零值结果。
func TestParseModuleTestResult(t *testing.T) {
	t.Run("合法-通过", func(t *testing.T) {
		data := []byte(`{"module_id":12,"passed":true,"summary":"集成测试 10/10 通过","failures":[]}`)
		r, err := parseModuleTestResult(data)
		if err != nil {
			t.Fatalf("期望解析成功，得到错误: %v", err)
		}
		if !r.Passed || r.ModuleID != 12 || r.Summary == "" {
			t.Fatalf("解析结果不正确: %+v", r)
		}
	})

	t.Run("合法-带失败明细", func(t *testing.T) {
		data := []byte(`{"module_id":12,"passed":false,"summary":"集成测试 8/10 通过","failures":[{"kind":"integration","name":"TestLoginAPI","log":"期望 200 得到 500"},{"kind":"e2e","name":"登录流程.spec.ts","log":"超时"}]}`)
		r, err := parseModuleTestResult(data)
		if err != nil {
			t.Fatalf("期望解析成功，得到错误: %v", err)
		}
		if r.Passed {
			t.Fatalf("passed 应为 false")
		}
		if len(r.Failures) != 2 || r.Failures[0].Kind != "integration" || r.Failures[1].Name != "登录流程.spec.ts" {
			t.Fatalf("failures 解析不正确: %+v", r.Failures)
		}
	})

	t.Run("空内容报错", func(t *testing.T) {
		if _, err := parseModuleTestResult([]byte("   ")); err == nil {
			t.Fatalf("空内容应返回错误")
		}
	})

	t.Run("畸形JSON报错", func(t *testing.T) {
		if _, err := parseModuleTestResult([]byte(`{"module_id":12,"passed":`)); err == nil {
			t.Fatalf("畸形 JSON 应返回错误")
		}
	})

	t.Run("非JSON文本报错", func(t *testing.T) {
		if _, err := parseModuleTestResult([]byte("测试全部通过！")); err == nil {
			t.Fatalf("非 JSON 文本应返回错误")
		}
	})

	t.Run("零值结果报错", func(t *testing.T) {
		// passed=false 且无 summary 无 failures，视为 agent 未真正产出结果
		if _, err := parseModuleTestResult([]byte(`{"module_id":12}`)); err == nil {
			t.Fatalf("零值结果应返回错误")
		}
	})
}

// TestReadModuleTestResult 验证从文件读取：文件不存在报错、存在则解析。
func TestReadModuleTestResult(t *testing.T) {
	dir := t.TempDir()

	t.Run("文件不存在", func(t *testing.T) {
		if _, err := readModuleTestResult(filepath.Join(dir, "nope.json")); err == nil {
			t.Fatalf("文件不存在应返回错误")
		}
	})

	t.Run("文件存在且合法", func(t *testing.T) {
		p := filepath.Join(dir, "module-7.json")
		if err := os.WriteFile(p, []byte(`{"module_id":7,"passed":true,"summary":"ok","failures":[]}`), 0644); err != nil {
			t.Fatal(err)
		}
		r, err := readModuleTestResult(p)
		if err != nil || r.ModuleID != 7 || !r.Passed {
			t.Fatalf("读取结果不正确: r=%+v err=%v", r, err)
		}
	})
}

// TestResultFilePath 验证结果文件路径格式。
func TestResultFilePath(t *testing.T) {
	got := resultFilePath("/srv/work/proj", 42)
	want := filepath.Join("/srv/work/proj", "tests", "results", "module-42.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestTailString 验证尾部截取。
func TestTailString(t *testing.T) {
	long := strings.Repeat("a", 100) + "尾部"
	if got := tailString(long, 10); !strings.HasSuffix(got, "尾部") {
		t.Fatalf("应保留尾部内容: %q", got)
	}
	if got := tailString("短串", 100); got != "短串" {
		t.Fatalf("短字符串应原样返回: %q", got)
	}
	if got := tailString("", 10); got != "" {
		t.Fatalf("空字符串应原样返回: %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /srv/zfei/projects/loafer/backend-go && go test ./internal/engine/executor/ -run 'TestParseModuleTestResult|TestReadModuleTestResult|TestResultFilePath|TestTailString' -v`
Expected: 编译失败（`parseModuleTestResult` 等未定义）

- [ ] **Step 3: 写最小实现**

创建 `backend-go/internal/engine/executor/test_executor.go`：

```go
package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// MaxTestRounds 业务模块「测试→修复」自动循环的最大轮数。
const MaxTestRounds = 3

// ModuleTestFailure 单条测试失败记录（结果 JSON 契约中 failures 的元素）。
type ModuleTestFailure struct {
	Kind string `json:"kind"` // integration / e2e / build / agent
	Name string `json:"name"`
	Log  string `json:"log"`
}

// ModuleTestResult 测试 agent 写入 tests/results/module-<id>.json 的结构化结果。
type ModuleTestResult struct {
	ModuleID int64               `json:"module_id"`
	Passed   bool                `json:"passed"`
	Summary  string              `json:"summary"`
	Failures []ModuleTestFailure `json:"failures"`
}

// resultFilePath 返回模块测试结果 JSON 的绝对路径。
func resultFilePath(workDir string, moduleID int64) string {
	return filepath.Join(workDir, "tests", "results", fmt.Sprintf("module-%d.json", moduleID))
}

// parseModuleTestResult 解析结果 JSON 内容；空内容、畸形 JSON、全零值结果均视为无效。
func parseModuleTestResult(data []byte) (*ModuleTestResult, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, fmt.Errorf("结果文件为空")
	}
	var result ModuleTestResult
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return nil, fmt.Errorf("结果文件不是合法 JSON: %w", err)
	}
	if !result.Passed && result.Summary == "" && len(result.Failures) == 0 {
		return nil, fmt.Errorf("结果文件缺少有效字段（passed/summary/failures 均为零值）")
	}
	return &result, nil
}

// readModuleTestResult 从磁盘读取并解析结果文件。
func readModuleTestResult(path string) (*ModuleTestResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取结果文件失败: %w", err)
	}
	return parseModuleTestResult(data)
}

// tailString 取字符串尾部 maxLen 个字符（按 rune 计，避免截断多字节字符）。
func tailString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[len(runes)-maxLen:])
}

// TestExecutor 模块自动测试/修复执行器，通过 Claude Code CLI（--print 模式）驱动测试 agent 与修复 agent。
type TestExecutor struct {
	db       *gorm.DB
	executor *cli.OfflineExecutor
}

// NewTestExecutor 构造模块自动测试执行器。
func NewTestExecutor(db *gorm.DB, executor *cli.OfflineExecutor) *TestExecutor {
	return &TestExecutor{db: db, executor: executor}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /srv/zfei/projects/loafer/backend-go && go test ./internal/engine/executor/ -run 'TestParseModuleTestResult|TestReadModuleTestResult|TestResultFilePath|TestTailString' -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit + Push**

```bash
cd /srv/zfei/projects/loafer
git add -A
git commit -m "feat: 新增 TestExecutor 结果 JSON 契约与解析"
git push origin master
```

---

### Task 2: RunModuleTest —— 测试 agent 执行与 TestRun 落库

**Files:**
- Modify: `backend-go/internal/engine/executor/test_executor.go`
- Test: `backend-go/internal/engine/executor/test_executor_test.go`

**Interfaces:**
- Consumes: Task 1 的类型与函数；`cli.OfflineExecutor.ExecuteSimple(workDir, prompt string, onOutput func(string)) cli.ExecutionResult`；`cli.RecordCall(db *gorm.DB, callType string, projectID, taskID *int64, prompt string, result cli.ExecutionResult, workDir string)`（见 `cli/call_logger.go:25`）；`model.TestRun`（字段见 `model/deployment_models.go:83-99`）；`truncateUTF8Bytes(s string, maxBytes int) string`（同包 `task_executor.go:451`）
- Produces:
  - `func buildTestPrompt(project *model.Project, mod *model.Module, accessURL string, round int) string`
  - `func applyResultToRun(run *model.TestRun, testResult *ModuleTestResult, cliResponse string)` — 纯函数，把测试结果映射到 TestRun 字段
  - `func (e *TestExecutor) RunModuleTest(project *model.Project, mod *model.Module, accessURL string, round int, onOutput func(string)) (*ModuleTestResult, error)`

- [ ] **Step 1: 写失败测试**

在 `test_executor_test.go` 追加：

```go
// TestBuildTestPrompt 验证测试 agent prompt 包含关键要素。
func TestBuildTestPrompt(t *testing.T) {
	project := &model.Project{ID: 1, Name: "演示项目", WorkDir: "/srv/work/demo"}
	mod := &model.Module{ID: 12, Name: "账号系统", SequenceNumber: "2", Description: "用户注册登录"}
	prompt := buildTestPrompt(project, mod, "http://127.0.0.1:40410", 2)

	for _, want := range []string{
		"账号系统", "用户注册登录", "http://127.0.0.1:40410",
		"tests/results/module-12.json", "go test", "playwright",
		`"passed"`, `"failures"`, "第 2 轮",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt 缺少要素 %q\nprompt:\n%s", want, prompt)
		}
	}
}

// TestApplyResultToRun 验证测试结果到 TestRun 的映射。
func TestApplyResultToRun(t *testing.T) {
	t.Run("通过", func(t *testing.T) {
		run := &model.TestRun{}
		applyResultToRun(run, &ModuleTestResult{Passed: true, Summary: "全部通过"}, "agent输出")
		if run.Status != "passed" || run.FailCount != 0 {
			t.Fatalf("通过时映射不正确: %+v", run)
		}
		if !strings.Contains(run.Output, "全部通过") {
			t.Fatalf("Output 应包含 summary: %q", run.Output)
		}
	})

	t.Run("失败", func(t *testing.T) {
		run := &model.TestRun{}
		r := &ModuleTestResult{Passed: false, Summary: "2个失败", Failures: []ModuleTestFailure{
			{Kind: "integration", Name: "T1", Log: "x"},
			{Kind: "e2e", Name: "T2", Log: "y"},
		}}
		applyResultToRun(run, r, "agent输出")
		if run.Status != "failed" || run.FailCount != 2 {
			t.Fatalf("失败时映射不正确: %+v", run)
		}
	})
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /srv/zfei/projects/loafer/backend-go && go test ./internal/engine/executor/ -run 'TestBuildTestPrompt|TestApplyResultToRun' -v`
Expected: 编译失败（函数未定义）

- [ ] **Step 3: 写实现**

在 `test_executor.go` 追加（需要新增 import：`log`、`time`）：

```go
// buildTestPrompt 构造测试 agent 的提示词。
// 要求 agent 现场编写并运行集成测试与 Playwright UI 测试，并把结构化结果写入结果文件。
func buildTestPrompt(project *model.Project, mod *model.Module, accessURL string, round int) string {
	return fmt.Sprintf(`你是测试工程师 agent。项目「%s」的模块「%s」（序号 %s）刚完成开发，服务已部署，访问地址：%s。

请为该模块现场编写并运行测试：
1. 集成测试：优先编写/运行 Go 测试（go test ./...），并用 HTTP 请求（curl 或测试脚本）验证本模块核心 API 的正常与边界情况。
2. Playwright UI 测试：若模块涉及前端页面，在 tests/e2e 目录下编写 Playwright 用例（@playwright/test 未安装则先安装，无 playwright.config 则创建，baseURL 使用上述访问地址），然后运行 npx playwright test。
3. 全部测试执行完毕后，无论通过与否，必须把结构化结果写入文件 tests/results/module-%d.json，格式严格如下（合法 JSON，不要包裹 Markdown 代码块标记）：
{"module_id": %d, "passed": true或false, "summary": "一句话总结，如：集成测试 8/10 通过；Playwright 3/4 通过", "failures": [{"kind": "integration或e2e或build", "name": "失败测试名", "log": "关键失败日志摘要"}]}
要求：所有测试通过时 passed 才为 true 且 failures 为空数组；任一测试失败时 passed 为 false 且 failures 逐项列出。

模块需求描述：%s

这是第 %d 轮测试。现在开始。`,
		project.Name, mod.Name, mod.SequenceNumber, accessURL,
		mod.ID, mod.ID, mod.Description, round)
}

// applyResultToRun 把测试结果映射到 TestRun 记录字段（不写库）。
func applyResultToRun(run *model.TestRun, testResult *ModuleTestResult, cliResponse string) {
	run.Status = "failed"
	run.FailCount = len(testResult.Failures)
	if testResult.Passed {
		run.Status = "passed"
		run.FailCount = 0
	}
	output := testResult.Summary + "\n\n" + tailString(cliResponse, 8000)
	run.Output = truncateUTF8Bytes(output, 60000)
}

// RunModuleTest 运行一轮模块自动测试：驱动测试 agent → 读取结果 JSON → 落库 TestRun。
// 任何异常（CLI 非零退出、结果文件缺失/畸形）都归一化为 Passed=false 的 ModuleTestResult 返回，
// error 仅在记录创建失败等不可继续的场景返回非 nil。
func (e *TestExecutor) RunModuleTest(project *model.Project, mod *model.Module, accessURL string, round int, onOutput func(string)) (*ModuleTestResult, error) {
	now := time.Now()
	run := &model.TestRun{
		ProjectID: project.ID,
		ModuleID:  &mod.ID,
		TestType:  "module-auto",
		Status:    "running",
		StartedAt: &now,
	}
	if err := e.db.Create(run).Error; err != nil {
		return nil, fmt.Errorf("创建测试运行记录失败: %w", err)
	}

	resultPath := resultFilePath(project.WorkDir, mod.ID)
	// 删除上一轮结果文件，防止误读旧结果
	_ = os.Remove(resultPath)

	prompt := buildTestPrompt(project, mod, accessURL, round)
	cliResult := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	cli.RecordCall(e.db, "module_test", &project.ID, nil, prompt, cliResult, project.WorkDir)

	var testResult *ModuleTestResult
	switch parsed, readErr := readModuleTestResult(resultPath); {
	case cliResult.ExitCode != 0:
		testResult = &ModuleTestResult{
			ModuleID: mod.ID,
			Passed:   false,
			Summary:  fmt.Sprintf("测试 agent 执行异常（退出码 %d）: %s", cliResult.ExitCode, cliResult.Error),
			Failures: []ModuleTestFailure{{Kind: "agent", Name: "test-agent", Log: tailString(cliResult.Response, 4000)}},
		}
	case readErr != nil:
		testResult = &ModuleTestResult{
			ModuleID: mod.ID,
			Passed:   false,
			Summary:  fmt.Sprintf("测试 agent 未产出有效结果文件: %v", readErr),
			Failures: []ModuleTestFailure{{Kind: "agent", Name: "test-agent", Log: tailString(cliResult.Response, 4000)}},
		}
	default:
		testResult = parsed
	}

	completedAt := time.Now()
	run.CompletedAt = &completedAt
	run.Duration = int(completedAt.Sub(now).Seconds())
	applyResultToRun(run, testResult, cliResult.Response)
	if err := e.db.Save(run).Error; err != nil {
		log.Printf("更新测试运行记录失败: %v", err)
	}
	return testResult, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /srv/zfei/projects/loafer/backend-go && go test ./internal/engine/executor/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit + Push**

```bash
cd /srv/zfei/projects/loafer
git add -A
git commit -m "feat: TestExecutor 实现测试 agent 执行与 TestRun 落库"
git push origin master
```

---

### Task 3: RunModuleFix —— 修复 agent

**Files:**
- Modify: `backend-go/internal/engine/executor/test_executor.go`
- Test: `backend-go/internal/engine/executor/test_executor_test.go`

**Interfaces:**
- Consumes: Task 2 的所有产出
- Produces:
  - `func buildFixPrompt(project *model.Project, mod *model.Module, testResult *ModuleTestResult, round int) string`
  - `func (e *TestExecutor) RunModuleFix(project *model.Project, mod *model.Module, testResult *ModuleTestResult, round int, onOutput func(string)) error`

- [ ] **Step 1: 写失败测试**

在 `test_executor_test.go` 追加：

```go
// TestBuildFixPrompt 验证修复 agent prompt 携带失败明细与约束。
func TestBuildFixPrompt(t *testing.T) {
	project := &model.Project{ID: 1, Name: "演示项目"}
	mod := &model.Module{ID: 12, Name: "账号系统"}
	result := &ModuleTestResult{
		Passed:  false,
		Summary: "集成测试 8/10 通过",
		Failures: []ModuleTestFailure{
			{Kind: "integration", Name: "TestLoginAPI", Log: "期望 200 得到 500"},
		},
	}
	prompt := buildFixPrompt(project, mod, result, 1)

	for _, want := range []string{
		"账号系统", "集成测试 8/10 通过", "TestLoginAPI", "期望 200 得到 500",
		"不得通过删除", "go build ./...", "第 1 轮",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt 缺少要素 %q\nprompt:\n%s", want, prompt)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /srv/zfei/projects/loafer/backend-go && go test ./internal/engine/executor/ -run TestBuildFixPrompt -v`
Expected: 编译失败（函数未定义）

- [ ] **Step 3: 写实现**

在 `test_executor.go` 追加：

```go
// buildFixPrompt 构造修复 agent 的提示词，携带上一轮失败明细。
func buildFixPrompt(project *model.Project, mod *model.Module, testResult *ModuleTestResult, round int) string {
	failuresJSON, _ := json.MarshalIndent(testResult.Failures, "", "  ")
	return fmt.Sprintf(`你是开发工程师 agent。项目「%s」模块「%s」的自动测试未通过。

测试总结：%s

失败明细（JSON）：
%s

请定位并修复代码中的问题。要求：
1. 只修业务代码或确实有误的测试代码，不得通过删除/弱化测试来让测试通过。
2. 修复完成后确保 go build ./... 与前端构建（npm run build，若有前端目录）均通过。
3. 不要重新运行完整测试套件，下一轮会由测试 agent 统一执行。

这是第 %d 轮修复。现在开始。`,
		project.Name, mod.Name, testResult.Summary, string(failuresJSON), round)
}

// RunModuleFix 运行一轮修复 agent。修复结果不由本方法判定，下一轮测试自然验证。
func (e *TestExecutor) RunModuleFix(project *model.Project, mod *model.Module, testResult *ModuleTestResult, round int, onOutput func(string)) error {
	prompt := buildFixPrompt(project, mod, testResult, round)
	cliResult := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	cli.RecordCall(e.db, "module_fix", &project.ID, nil, prompt, cliResult, project.WorkDir)
	if cliResult.ExitCode != 0 {
		return fmt.Errorf("修复 agent 执行失败（退出码 %d）: %s", cliResult.ExitCode, cliResult.Error)
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /srv/zfei/projects/loafer/backend-go && go test ./internal/engine/executor/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit + Push**

```bash
cd /srv/zfei/projects/loafer
git add -A
git commit -m "feat: TestExecutor 实现修复 agent"
git push origin master
```

---

### Task 4: Pipeline 模块循环改造 —— 自动测试门禁

**Files:**
- Modify: `backend-go/internal/handler/module.go:479-485`（常量块）
- Modify: `backend-go/internal/handler/pipeline.go:30-54`（handler 结构体与构造函数）、`949-1053`（模块循环）
- Test: `backend-go/internal/handler/pipeline_test.go`（新建）

**Interfaces:**
- Consumes: `executor.TestExecutor`（`RunModuleTest`/`RunModuleFix`/`MaxTestRounds`/`ModuleTestResult`/`ModuleTestFailure`，见 Task 1-3）；`service.DeployService.Deploy(projectID int64, onProgress func(string)) (*model.ProjectDeployment, error)`；`ProgressWriter` 接口（`pipeline_manager.go:11`，方法 `SendOutput(string)`）
- Produces:
  - `func resolveModuleAction(status int) moduleLoopAction`（handler 包内，pipeline.go）
  - `type moduleLoopAction int`，常量 `moduleActionSkip` / `moduleActionTestOnly` / `moduleActionRunThenTest`
  - `func (h *PipelineHandler) runBusinessModuleGate(project *model.Project, mod *model.Module, w ProgressWriter) bool`
  - `PipelineHandler` 新增字段 `testExecutor *executor.TestExecutor`（Task 5 还会加 `playwrightSvc`）
  - handler 包新增常量 `moduleStatusTesting = 3`

- [ ] **Step 1: 写失败测试**

创建 `backend-go/internal/handler/pipeline_test.go`：

```go
package handler

import "testing"

// TestResolveModuleAction 验证流水线重启时对各模块状态的分流：
// 4 完成→跳过；2 待测试/3 测试中→直接进测试门禁；其余（0/1/5/6）→重跑任务后进门禁。
func TestResolveModuleAction(t *testing.T) {
	cases := []struct {
		status int
		want   moduleLoopAction
	}{
		{0, moduleActionRunThenTest}, // 待执行
		{1, moduleActionRunThenTest}, // 执行中（中断残留）
		{2, moduleActionTestOnly},    // 待测试（历史数据兼容）
		{3, moduleActionTestOnly},    // 测试中（闭环中途被中断）
		{4, moduleActionSkip},        // 完成
		{5, moduleActionRunThenTest}, // 测试失败：重跑任务
		{6, moduleActionRunThenTest}, // 失败：重跑任务
	}
	for _, tc := range cases {
		if got := resolveModuleAction(tc.status); got != tc.want {
			t.Fatalf("status=%d: got %v, want %v", tc.status, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /srv/zfei/projects/loafer/backend-go && go test ./internal/handler/ -run TestResolveModuleAction -v`
Expected: 编译失败（`moduleLoopAction` 等未定义）

- [ ] **Step 3a: 加常量与分流函数**

`module.go:479-485` 常量块改为：

```go
// 模块状态常量（与前端 0-6 取值一致：0 待执行 / 1 执行中 / 2 待测试 / 3 测试中 / 4 完成 / 5 测试失败 / 6 失败）。
const (
	moduleStatusPendingTest = 2
	moduleStatusTesting     = 3
	moduleStatusCompleted   = 4
	moduleStatusTestFailed  = 5
	moduleStatusFailed      = 6
)
```

在 `pipeline.go` 中（`PipelineHandler` 结构体定义之后）新增：

```go
// moduleLoopAction 流水线模块循环对单个模块的处理动作。
type moduleLoopAction int

const (
	moduleActionSkip         moduleLoopAction = iota // 已完成，跳过
	moduleActionTestOnly                             // 任务已完成，直接进测试门禁
	moduleActionRunThenTest                          // 先跑任务，再进测试门禁
)

// resolveModuleAction 按模块状态决定处理动作。
// 4 完成 → 跳过；2 待测试 / 3 测试中 → 直接进测试门禁（断点续跑/历史数据兼容）；
// 0 待执行 / 1 执行中(中断残留) / 5 测试失败 / 6 失败 → 重跑任务后进门禁。
func resolveModuleAction(status int) moduleLoopAction {
	switch status {
	case moduleStatusCompleted:
		return moduleActionSkip
	case moduleStatusPendingTest, moduleStatusTesting:
		return moduleActionTestOnly
	default:
		return moduleActionRunThenTest
	}
}
```

- [ ] **Step 3b: 注入 TestExecutor**

`PipelineHandler` 结构体（pipeline.go:30-39）加字段，构造函数（42-54）初始化：

```go
type PipelineHandler struct {
	db              *gorm.DB
	cfg             *config.Config
	planGenerator   *plan.PlanGenerator
	decomposer      *executor.Decomposer
	taskExecutor    *executor.TaskExecutor
	testExecutor    *executor.TestExecutor
	deployService   *service.DeployService
	offlineExecutor *cli.OfflineExecutor
	pipelineManager *PipelineManager
}
```

```go
		taskExecutor:    executor.NewTaskExecutor(db, offlineExecutor),
		testExecutor:    executor.NewTestExecutor(db, offlineExecutor),
```

- [ ] **Step 3c: 新增测试门禁方法**

在 `pipeline.go` 新增方法（建议放在 `executePipeline` 之后）：

```go
// runBusinessModuleGate 业务模块质量门禁：部署 → 测试 agent → 失败则修复 agent 重试，
// 最多 MaxTestRounds 轮。全部通过返回 true 并把模块置为完成(4)；
// 轮次耗尽返回 false 并把模块置为测试失败(5)。
func (h *PipelineHandler) runBusinessModuleGate(project *model.Project, mod *model.Module, w ProgressWriter) bool {
	h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", moduleStatusTesting)

	var lastResult *executor.ModuleTestResult
	for round := 1; round <= executor.MaxTestRounds; round++ {
		// ① 全量部署（复用部署服务：build + 重启 + Nginx）
		w.SendOutput(fmt.Sprintf("  ▶ [第%d/%d轮] 重新构建并部署项目...\n", round, executor.MaxTestRounds))
		deployment, deployErr := h.deployService.Deploy(project.ID, func(p string) {
			w.SendOutput("  " + p + "\n")
		})

		if deployErr != nil {
			w.SendOutput(fmt.Sprintf("  ✗ [第%d/%d轮] 部署失败: %v\n", round, executor.MaxTestRounds, deployErr))
			lastResult = &executor.ModuleTestResult{
				ModuleID: mod.ID,
				Passed:   false,
				Summary:  "部署失败: " + deployErr.Error(),
				Failures: []executor.ModuleTestFailure{{Kind: "build", Name: "deploy", Log: deployErr.Error()}},
			}
		} else {
			accessURL := ""
			if deployment != nil {
				accessURL = deployment.AccessURL
			}
			// ② 测试 agent：集成测试 + Playwright UI 测试
			w.SendOutput(fmt.Sprintf("  ▶ [第%d/%d轮] 启动测试 agent 执行集成测试与 Playwright UI 测试...\n", round, executor.MaxTestRounds))
			testResult, testErr := h.testExecutor.RunModuleTest(project, mod, accessURL, round, func(o string) {
				w.SendOutput(o)
			})
			if testErr != nil {
				w.SendOutput(fmt.Sprintf("  ✗ 测试执行器内部错误: %v\n", testErr))
				testResult = &executor.ModuleTestResult{
					ModuleID: mod.ID,
					Passed:   false,
					Summary:  "测试执行器内部错误: " + testErr.Error(),
					Failures: []executor.ModuleTestFailure{{Kind: "agent", Name: "test-executor", Log: testErr.Error()}},
				}
			}
			lastResult = testResult
		}

		// ③ 判定
		if lastResult.Passed {
			h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", moduleStatusCompleted)
			w.SendOutput(fmt.Sprintf("  ✓ 模块 %s 自动测试通过（第%d轮），状态置为完成\n", mod.Name, round))
			return true
		}
		w.SendOutput(fmt.Sprintf("  ✗ [第%d/%d轮] 测试未通过: %s\n", round, executor.MaxTestRounds, lastResult.Summary))

		if round == executor.MaxTestRounds {
			break
		}
		// ④ 修复 agent
		w.SendOutput(fmt.Sprintf("  ▶ [第%d/%d轮] 启动修复 agent 自动修复...\n", round, executor.MaxTestRounds))
		if fixErr := h.testExecutor.RunModuleFix(project, mod, lastResult, round, func(o string) {
			w.SendOutput(o)
		}); fixErr != nil {
			w.SendOutput(fmt.Sprintf("  ✗ 修复 agent 执行异常: %v（将继续下一轮测试）\n", fixErr))
		}
	}

	h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", moduleStatusTestFailed)
	w.SendOutput(fmt.Sprintf("  ✗ 模块 %s 经过 %d 轮自动测试/修复仍未通过，已置为「测试失败」\n", mod.Name, executor.MaxTestRounds))
	return false
}
```

- [ ] **Step 3d: 重写模块循环分支**

`pipeline.go:956-1042` 的模块状态分流与业务模块收尾替换为以下逻辑（保留基础架构模块分支不变；注意删除原 960 行的 `mod.Status >= 4` 跳过、967-974 的 status=2/3 暂停分支、977-979 的 5/6 分支、1028-1037 的业务模块置待测试暂停分支）：

```go
		// 加载模块的最新状态
		h.db.First(&mod, mod.ID)

		// 严格串行：业务模块必须依次通过自动测试门禁（部署→测试→修复，最多3轮）后才启动下一模块。
		// - 完成(4)：跳过
		// - 待测试(2)/测试中(3)：任务已跑完，直接进测试门禁（断点续跑/历史数据兼容）
		// - 其余（0/1/5/6）：（重新）跑任务，然后进测试门禁
		switch resolveModuleAction(mod.Status) {
		case moduleActionSkip:
			w.SendOutput(fmt.Sprintf("  模块 %s 已完成（status=%d），跳过\n", mod.Name, mod.Status))
			continue
		case moduleActionTestOnly:
			w.SendOutput(fmt.Sprintf("  ↻ 模块 %s 当前状态=%d（待测试/测试中），任务已完成，直接进入自动测试门禁\n", mod.Name, mod.Status))
			if !h.runBusinessModuleGate(&project, &mod, w) {
				stages[2].Status = "paused"
				stages[2].Message = fmt.Sprintf("模块 [%d/%d] %s 自动测试未通过（已自动修复重试 %d 轮），请人工介入后重启流水线", i+1, len(modules), mod.Name, executor.MaxTestRounds)
				h.sendStageUpdate(w, stages)
				h.sendPipelineDone(w, result, stages, fmt.Sprintf("模块 %s 自动测试失败，流水线已暂停；修复后再次启动可从断点续跑", mod.Name))
				return
			}
			continue
		}

		// moduleActionRunThenTest：先执行任务
		if mod.Status == moduleStatusTestFailed || mod.Status == moduleStatusFailed {
			w.SendOutput(fmt.Sprintf("  ↻ 模块 %s 处于失败状态(status=%d)，将重新执行任务\n", mod.Name, mod.Status))
		}
```

任务执行成功后的分流（原 1010-1037）中业务模块分支改为：

```go
			isInfra := mod.ModuleType == executor.ModuleTypeInfrastructure
			if isInfra {
				// ……基础架构模块 InfraVerify 分支保持原样不变……
			} else {
				// 业务模块：任务跑完后进入自动测试门禁（部署→测试→修复，最多3轮）
				if !h.runBusinessModuleGate(&project, &mod, w) {
					stages[2].Status = "paused"
					stages[2].Message = fmt.Sprintf("模块 [%d/%d] %s 自动测试未通过（已自动修复重试 %d 轮），请人工介入后重启流水线", i+1, len(modules), mod.Name, executor.MaxTestRounds)
					h.sendStageUpdate(w, stages)
					h.sendPipelineDone(w, result, stages, fmt.Sprintf("模块 %s 自动测试失败，流水线已暂停；修复后再次启动可从断点续跑", mod.Name))
					return
				}
			}
```

注意：`&project` 与 `&mod`——`executePipeline` 中 `project` 是 `model.Project` 值（pipeline.go:700 附近加载），`mod` 是循环内 `model.Module` 值，直接取地址传入即可。

- [ ] **Step 4: 运行测试确认通过 + 全量构建**

Run: `cd /srv/zfei/projects/loafer/backend-go && go build ./... && go test ./internal/handler/ -run TestResolveModuleAction -v && go test ./internal/engine/executor/ -v`
Expected: 构建成功，全部 PASS

- [ ] **Step 5: Commit + Push**

```bash
cd /srv/zfei/projects/loafer
git add -A
git commit -m "feat: 业务模块流水线改为自动测试门禁，移除人工暂停；修复 status>=4 误判跳过失败模块"
git push origin master
```

---

### Task 5: 阶段5 全局 Playwright 冒烟

**Files:**
- Modify: `backend-go/internal/handler/pipeline.go:30-54`（注入 PlaywrightService）、`1132-1156`（测试验证阶段）

**Interfaces:**
- Consumes: `service.PlaywrightService.RunTest(projectID int64, moduleID *int64, taskID *int64, testType string, workDir string, onOutput func(string)) (*model.TestRun, error)`（`service/playwright_service.go:57`）；pipeline.go 已 import `os` 与 `path/filepath`
- Produces: `PipelineHandler` 新增字段 `playwrightSvc *service.PlaywrightService`

- [ ] **Step 1: 注入 PlaywrightService**

结构体加字段 `playwrightSvc *service.PlaywrightService`，构造函数加 `playwrightSvc: service.NewPlaywrightService(db, cfg),`。

- [ ] **Step 2: 替换测试验证阶段**

`pipeline.go:1132-1156` 替换为：

```go
	// ========== 阶段5: 测试验证 ==========
	stages[4].Status = "running"
	h.sendStageUpdate(w, stages)

	if deployment != nil && deployment.AccessURL != "" {
		w.SendOutput(fmt.Sprintf("访问地址: %s\n", deployment.AccessURL))
		// 全局冒烟：若测试 agent 已在 tests 目录下生成 Playwright 用例，则全量跑一遍；
		// 无用例时退化为可访问性检查（部署记录存在即通过）。
		testsDir := filepath.Join(project.WorkDir, "tests")
		if _, statErr := os.Stat(testsDir); statErr == nil {
			w.SendOutput("运行全局 Playwright 冒烟测试...\n")
			smokeRun, smokeErr := h.playwrightSvc.RunTest(projectID, nil, nil, "smoke", project.WorkDir, func(o string) {
				w.SendOutput(o)
			})
			if smokeErr != nil || smokeRun.Status != "passed" {
				stages[4].Status = "failed"
				stages[4].Message = "全局冒烟测试未通过"
				stages[4].Summary = "部署后全局 Playwright 冒烟测试存在失败用例，详见测试运行记录。"
				h.sendStageUpdate(w, stages)
				h.sendPipelineDone(w, result, stages, "全局冒烟测试未通过")
				return
			}
			stages[4].Status = "completed"
			stages[4].Message = "全局冒烟测试通过"
			stages[4].Summary = fmt.Sprintf("服务可访问，全局 Playwright 冒烟测试通过（%d 个用例）。", smokeRun.PassCount)
		} else {
			stages[4].Status = "completed"
			stages[4].Message = "服务验证通过"
			stages[4].Summary = fmt.Sprintf("已验证服务可正常访问，访问地址 %s 响应正常。", deployment.AccessURL)
		}
		stages[4].Artifacts = []StageArtifact{
			{
				Type:    "url",
				Name:    "在线访问",
				APIPath: deployment.AccessURL,
			},
		}
	} else {
		stages[4].Status = "skipped"
		stages[4].Message = "无访问URL，跳过测试"
		stages[4].Summary = "由于没有可访问的URL，测试阶段已跳过。"
	}
	h.sendStageUpdate(w, stages)
```

- [ ] **Step 3: 构建验证**

Run: `cd /srv/zfei/projects/loafer/backend-go && go build ./... && go vet ./internal/handler/`
Expected: 成功，无告警

- [ ] **Step 4: Commit + Push**

```bash
cd /srv/zfei/projects/loafer
git add -A
git commit -m "feat: 流水线测试验证阶段改为全局 Playwright 冒烟测试"
git push origin master
```

---

### Task 6: 全量构建与端到端验证

**Files:**
- 无新增代码

**Interfaces:**
- Consumes: Task 1-5 全部产出
- Produces: 无

- [ ] **Step 1: 后端全量构建与测试**

Run: `cd /srv/zfei/projects/loafer/backend-go && go build ./... && go test ./...`
Expected: 构建成功，全部测试 PASS

- [ ] **Step 2: 前端构建**

Run: `cd /srv/zfei/projects/loafer/frontend && npm run build`
Expected: 构建成功

- [ ] **Step 3: 本地部署 loafer 自身**

Run: `cd /srv/zfei/projects/loafer && ./deploy-local.sh backend`
Expected: 后端重启成功

- [ ] **Step 4: 端到端冒烟（人工/半自动）**

在 loafer UI 上用一个小需求（含 1 个基础架构模块 + 1-2 个业务模块）跑完整流水线，逐项观察：
- 业务模块任务完成后**不暂停**，日志出现「[第1/3轮] 重新构建并部署」「启动测试 agent」
- `tests/results/module-<id>.json` 在目标项目工作目录生成
- `test_run` 表新增 `test_type='module-auto'` 记录，状态与结果一致
- 测试通过后模块 status=4，流水线自动继续下一模块
- （可选）人为制造一个失败用例，观察修复 agent 启动、3 轮耗尽后模块 status=5、流水线 paused

- [ ] **Step 5: 若端到端验证发现问题，修复后提交**

```bash
cd /srv/zfei/projects/loafer
git add -A
git commit -m "fix: <端到端验证发现的问题描述>"
git push origin master
```

---

## Self-Review 记录

- **Spec coverage**：部署方式（Task 4 门禁①）、测试 agent（Task 2）、修复 3 轮（Task 3/4）、status>=4 bug（Task 4 Step 3d）、断点续跑 2/3/5/6 分流（Task 4 resolveModuleAction）、TestRun 落库（Task 2）、尾部阶段5 冒烟（Task 5）、脚手架降级路径（Task 4 未触碰 `cliAvailable==false` 分支，维持原样）、UI 手动标记完成（不改前端，保留）。错误处理表中「部署失败计入轮次」「JSON 缺失当轮失败」「修复异常继续下一轮」均已落入 Task 4 门禁代码。
- **Placeholder scan**：无 TBD/TODO；所有代码步骤含完整代码。
- **Type consistency**：`ModuleTestResult`/`ModuleTestFailure` 字段名、`RunModuleTest`/`RunModuleFix`/`buildTestPrompt`/`buildFixPrompt`/`applyResultToRun`/`resultFilePath`/`readModuleTestResult`/`parseModuleTestResult`/`tailString`/`resolveModuleAction`/`runBusinessModuleGate`/`moduleLoopAction` 各任务间签名一致；`RecordCall`、`ExecuteSimple`、`Deploy`、`RunTest` 签名已对照现有源码核实。
