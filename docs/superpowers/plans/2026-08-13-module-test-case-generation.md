# 业务模块测试用例自动生成 + 测试/修复/部署闭环 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 业务模块编码完成后自动生成 API/Playwright 测试用例并落库，门禁按场景逐条执行并截图回写，历史空用例模块自动补生成。

**Architecture:** 在现有 `runBusinessModuleGate` 前置幂等的「第0步·用例生成」（新 `TestDesignExecutor`，agent 写 spec 文件、后端校验落库）；`TestExecutor` 扩展场景级结果契约（`scenarios` 数组 + 截图路径）并回写模块用例字段；新增截图静态路由；前端场景卡片展示截图、补 Web 用例 tab。

**Tech Stack:** Go + Gin + GORM（后端），Vue 3 + TypeScript + Element Plus `<script setup>`（前端），Claude Code CLI --print 模式驱动 agent。

**Spec:** `docs/superpowers/specs/2026-08-13-module-test-case-generation-design.md`

**Spec 偏差说明（已确认的简化和扩展）：**
- 截图路由用 `GET /api/modules/:id/screenshots/:file`（替代 spec 中的 `/api/projects/:id/modules/:mid/...`，模块 ID 服务端反查项目，更简洁）
- `ScenarioResult` 增加 `errorScreenshot` 字段（spec 只提了 `screenshot`），区分终态图与出错图

## Global Constraints

- 后端注释、提交信息用中文，遵循现有代码风格（参考 `test_executor.go`）
- 所有模块字段更新用 **map 形式的 `Updates`**（map 只触达给定 key，天然避开零值 `created_at` 的 MySQL Error 1292 坑）；禁止结构体 `Save`
- `ModuleTestResult`/`parseModuleTestResult` 必须向后兼容：无 `scenarios` 字段的旧结果文件照常解析
- 每个 Task 结束：`go build ./...` 通过（涉及前端的还需 `npm run build` 通过）→ `git add -A && git commit && git push origin master`
- 测试文件放被测代码同包（`package executor` / `package handler`），风格参考 `test_executor_test.go`（t.Run 子测试、中文用例名）
- agent 提示词中的文件路径一律用**相对 workDir 的相对路径**（agent 工作目录即 project.WorkDir）

---

### Task 1: 场景结果契约 — `ScenarioResult` 类型与解析兼容

**Files:**
- Modify: `backend-go/internal/engine/executor/test_executor.go:23-36`
- Test: `backend-go/internal/engine/executor/test_executor_test.go`

**Interfaces:**
- Produces:
  - `type ScenarioResult struct { Kind string; Name string; Passed bool; Log string; Screenshot string; ErrorScreenshot string }`（json tag 分别为 `kind/name/passed/log/screenshot/errorScreenshot`）
  - `ModuleTestResult` 新增字段 `Scenarios []ScenarioResult`（json tag `scenarios`）
- Consumes: 无（纯类型扩展）

- [ ] **Step 1: 写失败测试**

在 `test_executor_test.go` 的 `TestParseModuleTestResult` 末尾追加子测试：

```go
	t.Run("合法-带场景结果", func(t *testing.T) {
		data := []byte(`{"module_id":12,"passed":false,"summary":"集成测试 1/2 通过","failures":[{"kind":"e2e","name":"注册流程","log":"超时"}],"scenarios":[{"kind":"api","name":"登录成功","passed":true,"log":"200 OK","screenshot":"","errorScreenshot":""},{"kind":"e2e","name":"注册流程","passed":false,"log":"超时","screenshot":"tests/results/screenshots/module-12/注册流程.png","errorScreenshot":"tests/results/screenshots/module-12/注册流程-error.png"}]}`)
		r, err := parseModuleTestResult(data)
		if err != nil {
			t.Fatalf("期望解析成功，得到错误: %v", err)
		}
		if len(r.Scenarios) != 2 {
			t.Fatalf("scenarios 应有 2 条，得到 %d", len(r.Scenarios))
		}
		api := r.Scenarios[0]
		if api.Kind != "api" || api.Name != "登录成功" || !api.Passed {
			t.Fatalf("api 场景解析不正确: %+v", api)
		}
		e2e := r.Scenarios[1]
		if e2e.ErrorScreenshot == "" || e2e.Passed {
			t.Fatalf("e2e 场景解析不正确: %+v", e2e)
		}
	})

	t.Run("兼容-无scenarios字段", func(t *testing.T) {
		data := []byte(`{"module_id":12,"passed":true,"summary":"集成测试 10/10 通过","failures":[]}`)
		r, err := parseModuleTestResult(data)
		if err != nil {
			t.Fatalf("期望解析成功，得到错误: %v", err)
		}
		if r.Scenarios != nil {
			t.Fatalf("无 scenarios 字段时应为 nil，得到 %+v", r.Scenarios)
		}
	})
```

- [ ] **Step 2: 跑测试确认通过**（该测试验证的是 JSON 反序列化兼容性，加字段后天然通过——目的是锁定契约防回归）

Run: `cd backend-go && go test ./internal/engine/executor/ -run TestParseModuleTestResult -v`
Expected: 两个新子测试 FAIL（`Scenarios` 字段尚不存在，编译错误）

- [ ] **Step 3: 实现类型扩展**

`test_executor.go` 中 `ModuleTestFailure` 定义之后插入：

```go
// ScenarioResult 单个测试场景的执行结果（结果 JSON 契约中 scenarios 的元素）。
// Name 与模块用例 JSON（api_integration_test / web_integration_test）中场景的 name 精确对应。
type ScenarioResult struct {
	Kind            string `json:"kind"`            // api / e2e
	Name            string `json:"name"`            // 场景名，回写用例时按此精确匹配
	Passed          bool   `json:"passed"`
	Log             string `json:"log"`             // 关键日志摘要
	Screenshot      string `json:"screenshot"`      // 终态截图（相对 workDir 路径），仅 e2e 场景有
	ErrorScreenshot string `json:"errorScreenshot"` // 失败时 Playwright 出错截图，可为空
}
```

`ModuleTestResult` 结构体追加字段：

```go
	Scenarios []ScenarioResult `json:"scenarios"`
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend-go && go test ./internal/engine/executor/ -run TestParseModuleTestResult -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
cd backend-go && go build ./... && cd ..
git add -A && git commit -m "feat: 测试结果契约扩展 scenarios 场景级结果

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 2: 截图工具 — slug、截图目录、路径安全解析

**Files:**
- Create: `backend-go/internal/engine/executor/screenshot.go`
- Test: `backend-go/internal/engine/executor/screenshot_test.go`

**Interfaces:**
- Produces:
  - `func slugifyScenarioName(name string) string` — 场景名→安全文件名（保留中英文数字，其余字符转 `-`，去首尾 `-`，最长 40 rune，空则返回 `"scenario"`）
  - `func ScreenshotDir(workDir string, moduleID int64) string` — 返回 `<workDir>/tests/results/screenshots/module-<id>`
  - `func ResolveScreenshotPath(workDir string, moduleID int64, file string) (string, error)` — 校验文件名（禁分隔符/`..`/非图片后缀）并返回目录内绝对路径
- Consumes: 无

- [ ] **Step 1: 写失败测试**

创建 `screenshot_test.go`：

```go
package executor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugifyScenarioName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"登录成功", "登录成功"},
		{"注册流程 e2e", "注册流程-e2e"},
		{"a/b\\c:d", "a-b-c-d"},
		{"  首尾空格  ", "首尾空格"},
		{"", "scenario"},
		{"---", "scenario"},
		{strings.Repeat("长", 50), strings.Repeat("长", 40)},
	}
	for _, c := range cases {
		if got := slugifyScenarioName(c.in); got != c.want {
			t.Errorf("slugifyScenarioName(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestResolveScreenshotPath(t *testing.T) {
	workDir := t.TempDir()
	dir := ScreenshotDir(workDir, 84)
	if !strings.HasSuffix(dir, filepath.Join("screenshots", "module-84")) {
		t.Fatalf("ScreenshotDir 后缀不正确: %s", dir)
	}

	t.Run("合法png", func(t *testing.T) {
		p, err := ResolveScreenshotPath(workDir, 84, "登录成功.png")
		if err != nil || !strings.HasPrefix(p, filepath.Clean(dir)+string(filepath.Separator)) {
			t.Fatalf("期望目录内路径，得到 %q, err=%v", p, err)
		}
	})

	t.Run("路径穿越拒绝", func(t *testing.T) {
		for _, bad := range []string{"../x.png", "..\\x.png", "a/b.png", "..%2e.png", "x..png"} {
			if _, err := ResolveScreenshotPath(workDir, 84, bad); err == nil {
				t.Errorf("应拒绝 %q", bad)
			}
		}
	})

	t.Run("非图片后缀拒绝", func(t *testing.T) {
		for _, bad := range []string{"x.html", "x", ".png", "x.PNG.exe"} {
			if _, err := ResolveScreenshotPath(workDir, 84, bad); err == nil {
				t.Errorf("应拒绝 %q", bad)
			}
		}
	})

	t.Run("大写PNG放行", func(t *testing.T) {
		if _, err := ResolveScreenshotPath(workDir, 84, "a.PNG"); err != nil {
			t.Errorf("大写 .PNG 应放行: %v", err)
		}
	})
}
```

注意：`"..%2e.png"` 含 `..` 子串被拒，`"x..png"` 同理（宁可误杀）。`".png"` 文件名主体为空，按非法处理（实现里 `file == ext` 时拒绝）。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend-go && go test ./internal/engine/executor/ -run 'TestSlugifyScenarioName|TestResolveScreenshotPath' -v`
Expected: 编译 FAIL（函数不存在）

- [ ] **Step 3: 实现 screenshot.go**

```go
package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// slugifyScenarioName 把场景名转为安全的文件名片段：保留中英文字母与数字，
// 其余字符一律转为连字符；去首尾连字符；最长 40 rune；结果为空时返回 "scenario"。
// 测试 agent 与本后端使用同一套规则，保证截图文件名可互相推导。
func slugifyScenarioName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "scenario"
	}
	runes := []rune(slug)
	if len(runes) > 40 {
		slug = string(runes[:40])
	}
	return slug
}

// ScreenshotDir 返回模块截图目录：<workDir>/tests/results/screenshots/module-<id>。
func ScreenshotDir(workDir string, moduleID int64) string {
	return filepath.Join(workDir, "tests", "results", "screenshots", fmt.Sprintf("module-%d", moduleID))
}

// ResolveScreenshotPath 把请求文件名解析为模块截图目录内的安全绝对路径。
// 拒绝：空名、含路径分隔符、含 ".."、非图片后缀、无主体的后缀名（如 ".png"）。
func ResolveScreenshotPath(workDir string, moduleID int64, file string) (string, error) {
	if file == "" || strings.ContainsAny(file, `/\`) || strings.Contains(file, "..") {
		return "", fmt.Errorf("非法截图文件名: %q", file)
	}
	ext := strings.ToLower(filepath.Ext(file))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return "", fmt.Errorf("截图仅支持 png/jpg: %q", file)
	}
	if file == ext { // ".png" 这类无主体文件名
		return "", fmt.Errorf("非法截图文件名: %q", file)
	}
	dir := ScreenshotDir(workDir, moduleID)
	full := filepath.Join(dir, file)
	cleanDir := filepath.Clean(dir)
	if full != cleanDir && !strings.HasPrefix(full, cleanDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("截图路径越界: %q", file)
	}
	return full, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend-go && go test ./internal/engine/executor/ -run 'TestSlugifyScenarioName|TestResolveScreenshotPath' -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
cd backend-go && go build ./... && cd ..
git add -A && git commit -m "feat: 截图工具——场景名 slug、截图目录与路径安全解析

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 3: 场景结果回写 — `applyScenarioResults`

**Files:**
- Create: `backend-go/internal/engine/executor/scenario_writeback.go`
- Test: `backend-go/internal/engine/executor/scenario_writeback_test.go`

**Interfaces:**
- Produces:
  - `func applyScenarioResults(specJSON string, results []ScenarioResult, now time.Time) string` — 按场景 `name` 精确匹配，把 `lastRunAt`(RFC3339)/`lastSuccess`/`lastSummary`(log 尾部 200 字符)/`screenshot`/`errorScreenshot` 写进 spec JSON 的 `testScenarios` 元素；spec 为空/非法/无匹配时**原样返回**
  - `func filterScenariosByKind(results []ScenarioResult, kind string) []ScenarioResult`
- Consumes: Task 1 的 `ScenarioResult`；`test_executor.go` 已有的 `tailString`

- [ ] **Step 1: 写失败测试**

创建 `scenario_writeback_test.go`：

```go
package executor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestApplyScenarioResults(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	spec := `{"testScenarios":[{"name":"登录成功","steps":[]},{"name":"注册流程","steps":[]}],"retryStrategy":{"maxRetries":3}}`
	results := []ScenarioResult{
		{Kind: "api", Name: "登录成功", Passed: true, Log: "200 OK"},
		{Kind: "e2e", Name: "注册流程", Passed: false, Log: "超时", Screenshot: "tests/results/screenshots/module-1/注册流程.png", ErrorScreenshot: "tests/results/screenshots/module-1/注册流程-error.png"},
	}

	t.Run("按名匹配回写", func(t *testing.T) {
		out := applyScenarioResults(spec, results, now)
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("输出应为合法 JSON: %v", err)
		}
		scenarios := parsed["testScenarios"].([]interface{})
		first := scenarios[0].(map[string]interface{})
		if first["lastSuccess"] != true || first["lastRunAt"] != "2026-08-13T12:00:00Z" || first["lastSummary"] != "200 OK" {
			t.Fatalf("登录成功场景回写不正确: %+v", first)
		}
		second := scenarios[1].(map[string]interface{})
		if second["lastSuccess"] != false || second["screenshot"] == "" || second["errorScreenshot"] == "" {
			t.Fatalf("注册流程场景回写不正确: %+v", second)
		}
		if parsed["retryStrategy"] == nil {
			t.Fatalf("无关字段应保留")
		}
	})

	t.Run("通过场景清除旧errorScreenshot", func(t *testing.T) {
		dirty := `{"testScenarios":[{"name":"注册流程","errorScreenshot":"old.png"}]}`
		out := applyScenarioResults(dirty, []ScenarioResult{{Kind: "e2e", Name: "注册流程", Passed: true, Screenshot: "new.png"}}, now)
		if strings.Contains(out, "old.png") {
			t.Fatalf("通过的场景应清除旧 errorScreenshot: %s", out)
		}
	})

	t.Run("名字不匹配不回写", func(t *testing.T) {
		out := applyScenarioResults(spec, []ScenarioResult{{Kind: "api", Name: "不存在的场景", Passed: true}}, now)
		if strings.Contains(out, "lastRunAt") {
			t.Fatalf("无匹配场景时不应写入任何结果: %s", out)
		}
	})

	t.Run("空spec原样返回", func(t *testing.T) {
		if out := applyScenarioResults("", results, now); out != "" {
			t.Fatalf("空 spec 应原样返回，得到 %q", out)
		}
	})

	t.Run("非法JSON原样返回", func(t *testing.T) {
		bad := `{not json`
		if out := applyScenarioResults(bad, results, now); out != bad {
			t.Fatalf("非法 JSON 应原样返回")
		}
	})

	t.Run("空results原样返回", func(t *testing.T) {
		if out := applyScenarioResults(spec, nil, now); out != spec {
			t.Fatalf("空 results 应原样返回")
		}
	})
}

func TestFilterScenariosByKind(t *testing.T) {
	results := []ScenarioResult{
		{Kind: "api", Name: "a"}, {Kind: "e2e", Name: "b"}, {Kind: "api", Name: "c"},
	}
	api := filterScenariosByKind(results, "api")
	if len(api) != 2 || api[0].Name != "a" || api[1].Name != "c" {
		t.Fatalf("过滤 api 不正确: %+v", api)
	}
	if got := filterScenariosByKind(results, "e2e"); len(got) != 1 {
		t.Fatalf("过滤 e2e 不正确: %+v", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend-go && go test ./internal/engine/executor/ -run 'TestApplyScenarioResults|TestFilterScenariosByKind' -v`
Expected: 编译 FAIL

- [ ] **Step 3: 实现 scenario_writeback.go**

```go
package executor

import (
	"encoding/json"
	"strings"
	"time"
)

// filterScenariosByKind 按 kind（api/e2e）过滤场景结果。
func filterScenariosByKind(results []ScenarioResult, kind string) []ScenarioResult {
	var out []ScenarioResult
	for _, r := range results {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// applyScenarioResults 把场景执行结果回写进用例 spec JSON（按场景 name 精确匹配）。
// 写回字段：lastRunAt(RFC3339) / lastSuccess / lastSummary(日志尾部 200 字符) /
// screenshot / errorScreenshot（通过场景会清除旧的 errorScreenshot）。
// spec 为空、非法 JSON、无 testScenarios 数组或无任何匹配时，原样返回 specJSON。
func applyScenarioResults(specJSON string, results []ScenarioResult, now time.Time) string {
	if strings.TrimSpace(specJSON) == "" || len(results) == 0 {
		return specJSON
	}
	var spec map[string]interface{}
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return specJSON
	}
	scenarios, ok := spec["testScenarios"].([]interface{})
	if !ok {
		return specJSON
	}
	byName := make(map[string]ScenarioResult, len(results))
	for _, r := range results {
		byName[r.Name] = r
	}
	changed := false
	for _, s := range scenarios {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		r, hit := byName[name]
		if !hit {
			continue
		}
		m["lastRunAt"] = now.Format(time.RFC3339)
		m["lastSuccess"] = r.Passed
		m["lastSummary"] = tailString(strings.TrimSpace(r.Log), 200)
		if r.Screenshot != "" {
			m["screenshot"] = r.Screenshot
		}
		if r.ErrorScreenshot != "" {
			m["errorScreenshot"] = r.ErrorScreenshot
		} else {
			delete(m, "errorScreenshot")
		}
		changed = true
	}
	if !changed {
		return specJSON
	}
	out, err := json.Marshal(spec)
	if err != nil {
		return specJSON
	}
	return string(out)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend-go && go test ./internal/engine/executor/ -run 'TestApplyScenarioResults|TestFilterScenariosByKind' -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
cd backend-go && go build ./... && cd ..
git add -A && git commit -m "feat: 场景测试结果按名回写模块用例 JSON

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 4: `TestDesignExecutor` — 用例生成 agent 编排与落库

**Files:**
- Create: `backend-go/internal/engine/executor/test_designer.go`
- Test: `backend-go/internal/engine/executor/test_designer_test.go`

**Interfaces:**
- Produces:
  - `type TestDesignExecutor struct` + `func NewTestDesignExecutor(db *gorm.DB, executor *cli.OfflineExecutor) *TestDesignExecutor`
  - `func (e *TestDesignExecutor) RunModuleTestDesign(project *model.Project, mod *model.Module, onOutput func(string)) error` — 幂等：两字段均非空直接返回 nil；否则驱动 agent 产出 spec 文件，校验后只填**当前为空**的字段
  - `func validateTestSpec(data []byte) error` — 合法 JSON 对象且 `testScenarios` 为非空数组
  - `func designSpecPaths(workDir string, moduleID int64) (apiPath, webPath, e2ePath string)`
- Consumes: `cli.OfflineExecutor.ExecuteSimple(workDir, prompt, onOutput)` 返回 `cli.ExecutionResult{ExitCode, Error, Response}`；`cli.RecordCall(db, callType, projectID, taskID, prompt, result, workDir)`

- [ ] **Step 1: 写失败测试**

创建 `test_designer_test.go`：

```go
package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loafer-agent/internal/model"
)

func TestValidateTestSpec(t *testing.T) {
	t.Run("合法", func(t *testing.T) {
		if err := validateTestSpec([]byte(`{"testScenarios":[{"name":"a","steps":[]}]}`)); err != nil {
			t.Fatalf("合法 spec 不应报错: %v", err)
		}
	})
	t.Run("空scenarios拒绝", func(t *testing.T) {
		if err := validateTestSpec([]byte(`{"testScenarios":[]}`)); err == nil {
			t.Fatalf("空 testScenarios 应报错")
		}
	})
	t.Run("缺scenarios拒绝", func(t *testing.T) {
		if err := validateTestSpec([]byte(`{"retryStrategy":{}}`)); err == nil {
			t.Fatalf("缺 testScenarios 应报错")
		}
	})
	t.Run("畸形JSON拒绝", func(t *testing.T) {
		if err := validateTestSpec([]byte(`{bad`)); err == nil {
			t.Fatalf("畸形 JSON 应报错")
		}
	})
}

func TestDesignSpecPaths(t *testing.T) {
	api, web, e2e := designSpecPaths("/work", 84)
	if !strings.HasSuffix(api, filepath.Join("tests", "specs", "module-84-api.json")) {
		t.Fatalf("api 路径不正确: %s", api)
	}
	if !strings.HasSuffix(web, filepath.Join("tests", "specs", "module-84-web.json")) {
		t.Fatalf("web 路径不正确: %s", web)
	}
	if !strings.HasSuffix(e2e, filepath.Join("tests", "e2e", "module-84.spec.ts")) {
		t.Fatalf("e2e 路径不正确: %s", e2e)
	}
}

func TestBuildDesignPrompt(t *testing.T) {
	project := &model.Project{ID: 22, Name: "轻量待办", WorkDir: "/work"}
	mod := &model.Module{ID: 84, Name: "账号系统", SequenceNumber: "2", Description: "用户注册登录"}
	prompt := buildDesignPrompt(project, mod, true, true)
	for _, want := range []string{"轻量待办", "账号系统", "module-84-api.json", "module-84-web.json", "module-84.spec.ts", "$BASE_URL", "testScenarios"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("提示词缺少 %q", want)
		}
	}
	// 只缺 web 用例时，提示词不应再要求 api 产物
	promptWebOnly := buildDesignPrompt(project, mod, false, true)
	if strings.Contains(promptWebOnly, "module-84-api.json") {
		t.Errorf("仅缺 web 用例时不应要求生成 api spec")
	}
}

// TestRunModuleTestDesign_Idempotent 验证幂等：两字段均非空时不调 agent 直接返回。
func TestRunModuleTestDesign_Idempotent(t *testing.T) {
	e := NewTestDesignExecutor(nil, nil) // 幂等分支不触达 db/executor
	project := &model.Project{ID: 1, WorkDir: t.TempDir()}
	mod := &model.Module{ID: 1, APIIntegrationTest: `{"testScenarios":[{"name":"a"}]}`, WebIntegrationTest: `{"testScenarios":[{"name":"b"}]}`}
	if err := e.RunModuleTestDesign(project, mod, func(string) {}); err != nil {
		t.Fatalf("幂等跳过不应报错: %v", err)
	}
}

// TestRunModuleTestDesign_MissingProject 防御：nil 安全检查（WorkDir 空时 agent 无法工作）。
func TestRunModuleTestDesign_EmptyWorkDir(t *testing.T) {
	e := NewTestDesignExecutor(nil, nil)
	project := &model.Project{ID: 1, WorkDir: ""}
	mod := &model.Module{ID: 1}
	if err := e.RunModuleTestDesign(project, mod, func(string) {}); err == nil {
		t.Fatalf("空 WorkDir 应报错")
	}
	_ = os.MkdirAll(filepath.Join(t.TempDir(), "x"), 0o755) // 仅占位，保持 imports 使用
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend-go && go test ./internal/engine/executor/ -run 'TestValidateTestSpec|TestDesignSpecPaths|TestBuildDesignPrompt|TestRunModuleTestDesign' -v`
Expected: 编译 FAIL

- [ ] **Step 3: 实现 test_designer.go**

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

// TestDesignExecutor 模块测试用例设计执行器：驱动「测试设计 agent」生成
// API 集成测试场景与 Playwright 场景清单，校验后落库到模块的
// api_integration_test / web_integration_test 字段。
// agent 只写文件不碰数据库；后端读文件、校验、落库，与结果文件契约同构。
type TestDesignExecutor struct {
	db       *gorm.DB
	executor *cli.OfflineExecutor
}

// NewTestDesignExecutor 构造用例设计执行器。
func NewTestDesignExecutor(db *gorm.DB, executor *cli.OfflineExecutor) *TestDesignExecutor {
	return &TestDesignExecutor{db: db, executor: executor}
}

// designSpecPaths 返回用例生成 agent 的三个产物路径（绝对路径）。
func designSpecPaths(workDir string, moduleID int64) (apiPath, webPath, e2ePath string) {
	return filepath.Join(workDir, "tests", "specs", fmt.Sprintf("module-%d-api.json", moduleID)),
		filepath.Join(workDir, "tests", "specs", fmt.Sprintf("module-%d-web.json", moduleID)),
		filepath.Join(workDir, "tests", "e2e", fmt.Sprintf("module-%d.spec.ts", moduleID))
}

// validateTestSpec 校验用例 spec：合法 JSON 对象且 testScenarios 为非空数组。
func validateTestSpec(data []byte) error {
	var spec map[string]interface{}
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("不是合法 JSON: %w", err)
	}
	scenarios, ok := spec["testScenarios"].([]interface{})
	if !ok || len(scenarios) == 0 {
		return fmt.Errorf("缺少非空 testScenarios 数组")
	}
	return nil
}

// buildDesignPrompt 构造测试设计 agent 提示词。
// needAPI / needWeb 标记还缺哪类用例（只生成缺失的，已存在的不覆盖）。
func buildDesignPrompt(project *model.Project, mod *model.Module, needAPI, needWeb bool) string {
	apiRel := fmt.Sprintf("tests/specs/module-%d-api.json", mod.ID)
	webRel := fmt.Sprintf("tests/specs/module-%d-web.json", mod.ID)
	e2eRel := fmt.Sprintf("tests/e2e/module-%d.spec.ts", mod.ID)

	var products strings.Builder
	if needAPI {
		products.WriteString(fmt.Sprintf(`
【产物 1：API 集成测试场景】写入文件 %s，格式（合法 JSON，不要包裹 Markdown 代码块标记）：
{"testScenarios": [{"name": "场景名", "steps": [{"action": "操作描述", "command": "curl 命令", "expected": "响应中应包含的子串"}], "onFailure": "continue"}]}
要求：
- 覆盖该模块全部核心 API 的正常与边界情况（缺参、越权、非法值等），每个场景至少 1 步。
- command 中的服务地址一律用占位符 $BASE_URL（如 curl -s -X POST $BASE_URL/api/login ...），执行时由平台替换。
- expected 填响应体中稳定包含的子串（如 "\"code\":0"），不要写整段响应。`, apiRel))
	}
	if needWeb {
		products.WriteString(fmt.Sprintf(`
【产物 2：Playwright 场景清单】写入文件 %s，格式：
{"testScenarios": [{"name": "场景名", "playwrightTest": "与产物3中 test() 标题完全一致", "specFile": "%s", "steps": [{"action": "操作描述", "expected": "预期页面表现"}]}]}
【产物 3：Playwright 可执行用例】写入文件 %s，用 @playwright/test 编写；每个 test() 的标题必须与产物 2 中对应场景的 playwrightTest 字段完全一致；baseURL 通过 test.use({ baseURL: process.env.BASE_URL }) 读取，不要硬编码地址。`, webRel, e2eRel, e2eRel))
	}

	return fmt.Sprintf(`你是测试设计 agent。项目「%s」的业务模块「%s」（序号 %s）刚完成编码实现。

请先阅读该模块的已实现代码（后端路由/handler、前端页面与 api 调用），再为其设计测试用例。

模块需求描述：%s

%s

现在开始。`, project.Name, mod.Name, mod.SequenceNumber, mod.Description, products.String())
}

// RunModuleTestDesign 为业务模块生成缺失的测试用例并落库。
// 幂等：api_integration_test 与 web_integration_test 均非空时直接返回 nil。
// agent 失败或全部产物缺失/畸形时返回 error（调用方降级为自由测试模式）；
// 部分产物缺失时只落库成功的一侧，返回 nil 并在 onOutput 中告警。
func (e *TestDesignExecutor) RunModuleTestDesign(project *model.Project, mod *model.Module, onOutput func(string)) error {
	needAPI := strings.TrimSpace(mod.APIIntegrationTest) == ""
	needWeb := strings.TrimSpace(mod.WebIntegrationTest) == ""
	if !needAPI && !needWeb {
		return nil
	}
	if project.WorkDir == "" {
		return fmt.Errorf("项目工作目录为空，无法生成用例")
	}

	prompt := buildDesignPrompt(project, mod, needAPI, needWeb)
	cliResult := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	cli.RecordCall(e.db, "module_test_design", &project.ID, nil, prompt, cliResult, project.WorkDir)
	if cliResult.ExitCode != 0 {
		return fmt.Errorf("测试设计 agent 执行失败（退出码 %d）: %s", cliResult.ExitCode, cliResult.Error)
	}

	apiPath, webPath, _ := designSpecPaths(project.WorkDir, mod.ID)
	updates := map[string]interface{}{}
	var problems []string

	if needAPI {
		if data, err := os.ReadFile(apiPath); err != nil {
			problems = append(problems, fmt.Sprintf("API 用例文件缺失: %v", err))
		} else if err := validateTestSpec(data); err != nil {
			problems = append(problems, fmt.Sprintf("API 用例校验失败: %v", err))
		} else {
			updates["api_integration_test"] = string(data)
		}
	}
	if needWeb {
		if data, err := os.ReadFile(webPath); err != nil {
			problems = append(problems, fmt.Sprintf("Web 用例文件缺失: %v", err))
		} else if err := validateTestSpec(data); err != nil {
			problems = append(problems, fmt.Sprintf("Web 用例校验失败: %v", err))
		} else {
			updates["web_integration_test"] = string(data)
		}
	}

	if len(updates) == 0 {
		return fmt.Errorf("测试设计 agent 未产出任何有效用例: %s", strings.Join(problems, "；"))
	}
	// map 形式 Updates 只触达给定 key，不会写零值 created_at。
	if err := e.db.Model(&model.Module{}).Where("id = ?", mod.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("用例落库失败: %w", err)
	}
	for _, p := range problems {
		onOutput("  ⚠ " + p + "\n")
	}
	return nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend-go && go test ./internal/engine/executor/ -run 'TestValidateTestSpec|TestDesignSpecPaths|TestBuildDesignPrompt|TestRunModuleTestDesign' -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
cd backend-go && go build ./... && cd ..
git add -A && git commit -m "feat: TestDesignExecutor——测试设计 agent 生成 API/Playwright 用例并落库

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 5: `TestExecutor` 场景化 — 提示词改造 + 截图清理 + 结果回写

**Files:**
- Modify: `backend-go/internal/engine/executor/test_executor.go:91-108`（buildTestPrompt）、`149-198`（RunModuleTest）
- Test: `backend-go/internal/engine/executor/test_executor_test.go`

**Interfaces:**
- Consumes: Task 1 `ScenarioResult`、Task 2 `slugifyScenarioName`/`ScreenshotDir`、Task 3 `applyScenarioResults`/`filterScenariosByKind`
- Produces: `buildTestPrompt(project, mod, accessURL, round)` 签名不变，内部按模块是否有用例分两支；`RunModuleTest` 行为扩展（调用方 pipeline.go 无签名变化）

- [ ] **Step 1: 写失败测试**

在 `test_executor_test.go` 追加：

```go
// TestBuildTestPrompt 验证场景驱动提示词：有用例时携带场景清单与截图文件名约定，
// 无用例时退化为自由测试提示词。
func TestBuildTestPrompt(t *testing.T) {
	project := &model.Project{ID: 22, Name: "轻量待办"}
	mod := &model.Module{ID: 84, Name: "账号系统", SequenceNumber: "2", Description: "用户注册登录"}

	t.Run("无用例-自由测试", func(t *testing.T) {
		prompt := buildTestPrompt(project, mod, "http://x:40410", 1)
		if !strings.Contains(prompt, "现场编写并运行测试") {
			t.Fatalf("无用例时应为自由测试提示词")
		}
		if strings.Contains(prompt, "按以下已落库的场景逐条执行") {
			t.Fatalf("无用例时不应出现场景清单指令")
		}
	})

	t.Run("有用例-场景驱动", func(t *testing.T) {
		modWithSpec := &model.Module{
			ID: 84, Name: "账号系统", SequenceNumber: "2", Description: "用户注册登录",
			APIIntegrationTest: `{"testScenarios":[{"name":"登录成功","steps":[{"action":"登录","command":"curl $BASE_URL/api/login","expected":"code"}]}]}`,
			WebIntegrationTest: `{"testScenarios":[{"name":"注册流程","playwrightTest":"注册流程","specFile":"tests/e2e/module-84.spec.ts"}]}`,
		}
		prompt := buildTestPrompt(project, modWithSpec, "http://x:40410", 1)
		for _, want := range []string{"按以下已落库的场景逐条执行", "登录成功", "注册流程", "tests/e2e/module-84.spec.ts", "tests/results/screenshots/module-84/注册流程.png", "scenarios", "$BASE_URL", "http://x:40410"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("场景驱动提示词缺少 %q", want)
			}
		}
	})
}

// TestScenarioNamesFromSpec 验证从用例 JSON 提取场景名。
func TestScenarioNamesFromSpec(t *testing.T) {
	names := scenarioNamesFromSpec(`{"testScenarios":[{"name":"a"},{"name":"b"}]}`)
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("提取不正确: %v", names)
	}
	if got := scenarioNamesFromSpec(""); got != nil {
		t.Fatalf("空 spec 应返回 nil")
	}
	if got := scenarioNamesFromSpec("{bad"); got != nil {
		t.Fatalf("非法 JSON 应返回 nil")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend-go && go test ./internal/engine/executor/ -run 'TestBuildTestPrompt|TestScenarioNamesFromSpec' -v`
Expected: 编译 FAIL（`scenarioNamesFromSpec` 不存在；「有用例」子测试失败）

- [ ] **Step 3: 实现提示词改造**

`test_executor.go` 中，`buildTestPrompt` 上方新增辅助函数，并改造 `buildTestPrompt`：

```go
// scenarioNamesFromSpec 从用例 spec JSON 提取场景名列表；空/非法返回 nil。
func scenarioNamesFromSpec(specJSON string) []string {
	var spec struct {
		TestScenarios []struct {
			Name string `json:"name"`
		} `json:"testScenarios"`
	}
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil
	}
	var names []string
	for _, s := range spec.TestScenarios {
		if s.Name != "" {
			names = append(names, s.Name)
		}
	}
	return names
}

// buildScenarioDrivenSection 有用例时生成「按场景逐条执行」提示词段落。
// 截图文件名由后端按 slugifyScenarioName 预先推导并写死进提示词，agent 不需自行计算。
func buildScenarioDrivenSection(mod *model.Module) string {
	var b strings.Builder
	b.WriteString("请按以下已落库的场景逐条执行测试（不要自由发挥新场景）：\n")

	if names := scenarioNamesFromSpec(mod.APIIntegrationTest); len(names) > 0 {
		b.WriteString(fmt.Sprintf("【API 集成测试场景】共 %d 个：%s\n", len(names), strings.Join(names, "、")))
		b.WriteString("用例定义见模块的 api_integration_test（curl 命令中的 $BASE_URL 替换为上述访问地址）。逐场景执行并把每步实际输出与 expected 子串比对。\n")
	}
	if names := scenarioNamesFromSpec(mod.WebIntegrationTest); len(names) > 0 {
		b.WriteString(fmt.Sprintf("【Playwright UI 场景】共 %d 个：%s\n", len(names), strings.Join(names, "、")))
		b.WriteString(fmt.Sprintf("可执行用例在 tests/e2e/module-%d.spec.ts，用 BASE_URL=%s npx playwright test module-%d.spec.ts 运行。\n", mod.ID, "%s", mod.ID))
		dir := fmt.Sprintf("tests/results/screenshots/module-%d", mod.ID)
		b.WriteString(fmt.Sprintf("每个 Playwright 场景跑完（无论成败）必须用 page.screenshot 截一张终态图，保存为 %s/<场景名转文件名>.png；失败的场景额外保存出错图 <场景名转文件名>-error.png。", dir))
		b.WriteString("文件名推导规则已为你算好，对照如下：\n")
		for _, n := range names {
			b.WriteString(fmt.Sprintf("  - 场景「%s」→ %s/%s.png（失败加 %s-error.png）\n", n, dir, slugifyScenarioName(n), slugifyScenarioName(n)))
		}
	}

	b.WriteString(fmt.Sprintf(`全部执行完毕后，无论通过与否，必须把结构化结果写入文件 tests/results/module-%d.json，格式严格如下（合法 JSON，不要包裹 Markdown 代码块标记）：
{"module_id": %d, "passed": true或false, "summary": "一句话总结，如：API 2/2 通过；Playwright 1/2 通过", "failures": [{"kind": "integration或e2e或build", "name": "失败测试名", "log": "关键失败日志摘要"}], "scenarios": [{"kind": "api或e2e", "name": "场景名（与上述清单完全一致）", "passed": true或false, "log": "关键日志摘要", "screenshot": "终态截图相对路径或空串", "errorScreenshot": "出错截图相对路径或空串"}]}
要求：所有场景通过时 passed 才为 true 且 failures 为空数组；任一场景失败时 passed 为 false。scenarios 数组必须覆盖上述全部场景，name 不得改动。`, mod.ID, mod.ID))
	return b.String()
}
```

`buildTestPrompt` 改为分两支（保留现有自由测试分支原文）：

```go
// buildTestPrompt 构造测试 agent 的提示词。
// 模块已落库用例时按场景逐条执行并截图回传；否则退化为现场编写并运行测试。
func buildTestPrompt(project *model.Project, mod *model.Module, accessURL string, round int) string {
	if strings.TrimSpace(mod.APIIntegrationTest) != "" || strings.TrimSpace(mod.WebIntegrationTest) != "" {
		return fmt.Sprintf(`你是测试工程师 agent。项目「%s」的模块「%s」（序号 %s）刚完成开发，服务已部署，访问地址：%s。

%s

模块需求描述：%s

这是第 %d 轮测试。现在开始。`,
			project.Name, mod.Name, mod.SequenceNumber, accessURL,
			buildScenarioDrivenSection(mod), mod.Description, round)
	}
	// ……保留现有自由测试提示词原文不变……
}
```

注意上面 `buildScenarioDrivenSection` 中 `%s` 占位（accessURL）的处理：为避免嵌套 Sprintf 混乱，把该函数的签名改为 `buildScenarioDrivenSection(mod *model.Module, accessURL string)`，内部直接插入 accessURL 而不是 `%s` 占位：

```go
		b.WriteString(fmt.Sprintf("可执行用例在 tests/e2e/module-%d.spec.ts，用 BASE_URL=%s npx playwright test module-%d.spec.ts 运行。\n", mod.ID, accessURL, mod.ID))
```

同步把 `buildTestPrompt` 中的调用改为 `buildScenarioDrivenSection(mod, accessURL)`。

- [ ] **Step 4: RunModuleTest 集成截图清理与结果回写**

`RunModuleTest` 中，删除旧结果文件之后插入截图目录清理：

```go
	resultPath := resultFilePath(project.WorkDir, mod.ID)
	// 删除上一轮结果文件，防止误读旧结果
	_ = os.Remove(resultPath)
	// 清空上一轮截图目录并重建，只保留最新一轮截图
	shotDir := ScreenshotDir(project.WorkDir, mod.ID)
	_ = os.RemoveAll(shotDir)
	_ = os.MkdirAll(shotDir, 0o755)
```

`run` 落库（`e.db.Save(run)`）之后、`return` 之前插入场景回写：

```go
	// 场景级结果回写模块用例字段（面板「上次通过/失败」标签与截图的数据源）。
	// 按 name 精确匹配；匹配不上的场景结果只留在 failures 里，不回写。
	if len(testResult.Scenarios) > 0 {
		now := time.Now()
		updates := map[string]interface{}{}
		if apiResults := filterScenariosByKind(testResult.Scenarios, "api"); len(apiResults) > 0 && strings.TrimSpace(mod.APIIntegrationTest) != "" {
			updates["api_integration_test"] = applyScenarioResults(mod.APIIntegrationTest, apiResults, now)
		}
		if webResults := filterScenariosByKind(testResult.Scenarios, "e2e"); len(webResults) > 0 && strings.TrimSpace(mod.WebIntegrationTest) != "" {
			updates["web_integration_test"] = applyScenarioResults(mod.WebIntegrationTest, webResults, now)
		}
		if len(updates) > 0 {
			if err := e.db.Model(&model.Module{}).Where("id = ?", mod.ID).Updates(updates).Error; err != nil {
				log.Printf("场景结果回写模块用例失败(module=%d): %v", mod.ID, err)
			}
		}
	}
```

注意：`RunModuleTest` 里的 `mod` 是门禁第 0 步落库后重新加载过的（Task 6 保证），所以这里能读到最新用例文本。

- [ ] **Step 5: 跑测试确认通过**

Run: `cd backend-go && go test ./internal/engine/executor/ -v`
Expected: 全部 PASS

- [ ] **Step 6: Commit**

```bash
cd backend-go && go build ./... && cd ..
git add -A && git commit -m "feat: 测试 agent 按落库场景逐条执行——截图清理+场景结果回写模块用例

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 6: 流水线接线 — 门禁第 0 步 + 历史回填

**Files:**
- Modify: `backend-go/internal/handler/pipeline.go:34-35`（结构体字段）、`64-75`（构造）、`985-998`（skip 分支）、`1233-1237`（门禁入口）
- Test: 无新单测（接线逻辑）；靠 Task 4/5 单测 + Task 11 端到端验证

**Interfaces:**
- Consumes: `executor.NewTestDesignExecutor(db, offlineExecutor)`、`RunModuleTestDesign(project, mod, onOutput) error`（Task 4）
- Produces: `PipelineHandler.testDesigner` 字段；`func (h *PipelineHandler) ensureModuleTestSpecs(project *model.Project, mod *model.Module, w ProgressWriter)`

- [ ] **Step 1: PipelineHandler 加字段并接线构造**

`PipelineHandler` 结构体 `testExecutor` 行后加：

```go
	testDesigner    *executor.TestDesignExecutor
```

`NewPipelineHandler` 中 `testExecutor:` 行后加：

```go
		testDesigner:    executor.NewTestDesignExecutor(db, offlineExecutor),
```

- [ ] **Step 2: 实现 ensureModuleTestSpecs**

`runBusinessModuleGate` 上方新增：

```go
// ensureModuleTestSpecs 门禁第 0 步：业务模块用例（API/Web）为空时自动调测试设计 agent 生成并落库。
// 幂等（两字段均非空直接跳过）；生成失败仅告警不阻断——门禁降级为自由测试模式继续，
// 避免用例生成成为流水线的单点故障。生成成功后重新加载模块，让后续测试提示词拿到用例。
func (h *PipelineHandler) ensureModuleTestSpecs(project *model.Project, mod *model.Module, w ProgressWriter) {
	if mod.ModuleType == executor.ModuleTypeInfrastructure {
		return
	}
	if strings.TrimSpace(mod.APIIntegrationTest) != "" && strings.TrimSpace(mod.WebIntegrationTest) != "" {
		return
	}
	w.SendOutput("  ▶ 模块测试用例为空，启动测试设计 agent 生成 API/Playwright 用例...\n")
	if err := h.testDesigner.RunModuleTestDesign(project, mod, func(o string) { w.SendOutput(o) }); err != nil {
		w.SendOutput(fmt.Sprintf("  ⚠ 用例生成失败: %v（门禁将以自由测试模式继续）\n", err))
		return
	}
	if err := h.db.First(mod, mod.ID).Error; err != nil {
		w.SendOutput(fmt.Sprintf("  ⚠ 用例落库后重新加载模块失败: %v（本轮测试提示词可能不含用例）\n", err))
		return
	}
	w.SendOutput("  ✓ 模块测试用例已生成并落库\n")
}
```

- [ ] **Step 3: 门禁入口调用**

`runBusinessModuleGate` 函数体最开头（`Update("status", ...Testing)` 之前）插入：

```go
	// 第 0 步：用例生成（幂等，仅业务模块且用例为空时真正执行）
	h.ensureModuleTestSpecs(project, mod, w)
```

- [ ] **Step 4: skip 分支历史回填**

pipeline.go 模块循环中 `case moduleActionSkip:` 分支（约 986 行）改为：

```go
			case moduleActionSkip:
				// 历史回填：已完成但用例为空的业务模块，补跑一次用例生成（不重跑任务/测试）
				h.ensureModuleTestSpecs(&project, &mod, w)
				w.SendOutput(fmt.Sprintf("  模块 %s 已完成（status=%d），跳过\n", mod.Name, mod.Status))
				continue
```

（`ensureModuleTestSpecs` 内部对基础架构模块和用例齐全的模块直接返回，此处无脑调用即可。）

- [ ] **Step 5: 编译 + 全量单测**

Run: `cd backend-go && go build ./... && go test ./...`
Expected: 编译通过，测试全 PASS

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: 门禁第0步自动生成用例+历史空用例模块重启自动补生成

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 7: 截图静态路由 + `GenerateTestStream` 桩实现

**Files:**
- Modify: `backend-go/internal/handler/module.go:23-42`（结构体/构造）、`44-82`（路由注册）、`379-396`（GenerateTestStream）
- Test: `backend-go/internal/handler/module_screenshot_test.go`（只测路径解析调用，handler 层薄）

**Interfaces:**
- Consumes: `executor.ResolveScreenshotPath(workDir, moduleID, file)`（Task 2）；`executor.NewTestDesignExecutor`（Task 4）；`util.NewSSEWriter` 的 `SendOutput/SendDone/SendError`
- Produces: `GET /api/modules/:id/screenshots/:file`；`ModuleHandler.testDesigner` 字段

- [ ] **Step 1: ModuleHandler 加字段并接线**

结构体加 `testDesigner *executor.TestDesignExecutor`；`NewModuleHandler` 初始化列表加：

```go
		testDesigner:  executor.NewTestDesignExecutor(db, offlineExecutor),
```

路由注册块中 `g.POST("/:id/generate-test-stream", h.GenerateTestStream)` 之后加：

```go
		// 模块测试截图（tests/results/screenshots/module-<id>/ 下的图片）
		g.GET("/:id/screenshots/:file", h.GetModuleScreenshot)
```

- [ ] **Step 2: 实现 GetModuleScreenshot**

`module.go` 中新增：

```go
// GetModuleScreenshot 对应 GET /modules/:id/screenshots/:file。
// 提供模块最新一轮测试的 Playwright 截图；文件名经 ResolveScreenshotPath 严格校验防路径穿越。
func (h *ModuleHandler) GetModuleScreenshot(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "模块不存在")
		return
	}
	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "项目不存在")
		return
	}
	full, err := executor.ResolveScreenshotPath(project.WorkDir, module.ID, c.Param("file"))
	if err != nil {
		util.Fail(c, http.StatusNotFound, "截图不存在")
		return
	}
	if info, statErr := os.Stat(full); statErr != nil || info.IsDir() {
		util.Fail(c, http.StatusNotFound, "截图不存在")
		return
	}
	// 截图同名覆盖（每轮只留最新），禁止缓存
	c.Header("Cache-Control", "no-cache")
	c.File(full)
}
```

`module.go` 头部 import 补 `"os"`。

- [ ] **Step 3: 实现 GenerateTestStream（LEGACY 模式）**

替换现有桩实现：

```go
// GenerateTestStream 对应 POST /modules/:id/generate-test-stream?mode=LEGACY|TDD。
// LEGACY：调测试设计 agent 生成 API+Playwright 用例并落库（与门禁第 0 步同源）；
// TDD：验收标准生成暂未实现，保持桩提示。
func (h *ModuleHandler) GenerateTestStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	mode := c.DefaultQuery("mode", "TDD")
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}

	if mode != "LEGACY" {
		sse.SendOutput("[SSE] TDD 验收标准生成暂未实现，模块 ID: " + strconv.FormatInt(id, 10))
		sse.SendDone(module)
		return
	}

	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		sse.SendError("项目不存在: " + err.Error())
		return
	}
	sse.SendOutput("[SSE] 启动测试设计 agent 生成 API/Playwright 用例...\n")
	if err := h.testDesigner.RunModuleTestDesign(&project, &module, func(o string) { sse.SendOutput(o) }); err != nil {
		sse.SendError("用例生成失败: " + err.Error())
		return
	}
	// 返回落库后的最新模块，前端据此刷新用例面板
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("重新加载模块失败: " + err.Error())
		return
	}
	sse.SendDone(module)
}
```

注意：`RunModuleTestDesign` 幂等——字段已有值时不重复生成。面板按钮用于「首次生成/补齐」；若后续需要强制重生成，用户先清空字段再点按钮（本期不做强制重生成，见 spec 非目标）。

- [ ] **Step 4: 写 handler 层薄测试**

创建 `module_screenshot_test.go`：

```go
package handler

import (
	"strings"
	"testing"

	"loafer-agent/internal/engine/executor"
)

// TestScreenshotRouteContract 锁定路由与路径解析的契约：handler 委托
// executor.ResolveScreenshotPath，穿越/非图片一律 404（由 ResolveScreenshotPath 报错决定）。
func TestScreenshotRouteContract(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executor.ResolveScreenshotPath(workDir, 1, "../../etc/passwd"); err == nil {
		t.Fatalf("路径穿越必须报错")
	}
	p, err := executor.ResolveScreenshotPath(workDir, 1, "登录成功.png")
	if err != nil || !strings.Contains(p, "module-1") {
		t.Fatalf("合法截图路径解析失败: %v", err)
	}
}
```

- [ ] **Step 5: 编译 + 全量单测 + 手动验证路由注册**

Run: `cd backend-go && go build ./... && go test ./...`
Expected: 全 PASS

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: 截图静态路由+generate-test-stream LEGACY 模式接入测试设计 agent

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 8: 前端 — 场景卡片截图展示

**Files:**
- Modify: `frontend/src/components/IntegrationTestEditor.vue`（last-run 面板约 96-130 行、script setup 约 249-262 行）

**Interfaces:**
- Consumes: 后端 `GET /api/modules/:id/screenshots/:file`（Task 7）；场景 JSON 上的 `screenshot`/`errorScreenshot` 相对路径字段（Task 3 回写）；组件已有 props `moduleId`
- Produces: `screenshotUrl(rel: string): string`

- [ ] **Step 1: script 加 URL 构造函数**

`const runningIndex = ref<number | null>(null)` 之后加：

```ts
const apiBase = import.meta.env.VITE_API_BASE_URL || '/api'
/** 把结果 JSON 里的相对截图路径转成静态路由 URL（取末段文件名，后端按模块截图目录解析）。 */
const screenshotUrl = (rel: string): string => {
  if (!rel) return ''
  const file = rel.split('/').pop() || ''
  if (!file) return ''
  return `${apiBase}/modules/${props.moduleId}/screenshots/${encodeURIComponent(file)}`
}
```

- [ ] **Step 2: last-run 面板加截图展示**

`<!-- 上次运行结果（折叠展示 step 明细） -->` 的 `<div v-if="scenario.lastRunAt" class="last-run-panel">` 内，时间/摘要那行 `</div>` 之后、`<el-collapse>` 之前插入：

```html
            <div v-if="scenario.screenshot || scenario.errorScreenshot" class="last-screenshots">
              <figure v-if="scenario.screenshot" class="shot-item">
                <el-image
                  :src="screenshotUrl(scenario.screenshot)"
                  :preview-src-list="[screenshotUrl(scenario.screenshot)]"
                  fit="contain"
                  class="shot-img"
                  lazy
                />
                <figcaption class="shot-caption">终态截图</figcaption>
              </figure>
              <figure v-if="scenario.errorScreenshot" class="shot-item">
                <el-image
                  :src="screenshotUrl(scenario.errorScreenshot)"
                  :preview-src-list="[screenshotUrl(scenario.errorScreenshot)]"
                  fit="contain"
                  class="shot-img"
                  lazy
                />
                <figcaption class="shot-caption">失败截图</figcaption>
              </figure>
            </div>
```

- [ ] **Step 3: 加样式**

组件 `<style>` 块内追加：

```css
.last-screenshots {
  display: flex;
  gap: 12px;
  margin: 6px 0;
  flex-wrap: wrap;
}
.shot-item {
  margin: 0;
}
.shot-img {
  width: 240px;
  height: 150px;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  background: #f8fafc;
}
.shot-caption {
  font-size: 12px;
  color: #94a3b8;
  text-align: center;
  margin-top: 2px;
}
```

- [ ] **Step 4: 构建验证**

Run: `cd frontend && npm run build`
Expected: 构建通过

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: 集成测试场景卡片展示 Playwright 终态/失败截图

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 9: 前端 — ModuleTaskTab 补「Web集成测试」tab

**Files:**
- Modify: `frontend/src/components/ModuleTaskTab.vue:317-327`（LEGACY tabs）

**Interfaces:**
- Consumes: 已有 `webIntegrationTestText` ref（1375 行）、`onModuleSpecUpdated`（3446 行附近）、`IntegrationTestEditor` 组件
- Produces: 无新接口

- [ ] **Step 1: 加 tab**

`modulePipelineModeDraft === 'LEGACY'` 的 `<template>` 内，「API集成测试」`el-tab-pane` 结束之后追加：

```html
              <el-tab-pane label="Web集成测试" name="web">
                <IntegrationTestEditor
                  v-model="webIntegrationTestText"
                  :module-id="selectedModule?.id"
                  :project-id="props.projectId"
                  @module-updated="onModuleSpecUpdated"
                />
              </el-tab-pane>
```

（`IntegrationTestEditor` 已在文件中导入并用于 API tab，无需新增 import。保存走现有 `saveTestSpec`，已包含 `webIntegrationTestText`。）

- [ ] **Step 2: 构建验证**

Run: `cd frontend && npm run build`
Expected: 构建通过

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat: 模块详情补 Web集成测试 tab（Playwright 用例展示）

Co-Authored-By: Claude <noreply@anthropic.com>" && git push origin master
```

---

### Task 10: 端到端验证 — 项目 22 回填 + 门禁闭环

**Files:** 无代码改动；部署与验证

- [ ] **Step 1: 全量构建与测试**

```bash
cd backend-go && go build ./... && go test ./... && cd ../frontend && npm run build && cd ..
```
Expected: 全绿

- [ ] **Step 2: 重新部署 loafer 后端**

按 [[loafer-auto-test-gate]] 的技巧复用运行中进程的环境（必须含 INFRA_SSH_KEY_PATH，否则门禁部署步失败）：

```bash
pid=$(pgrep -f './loafer-agent' | head -1)
APP_ENV_VARS="$(tr '\0' '\n' < /proc/$pid/environ | grep '^APP_ENV_VARS=' | cut -d= -f2-)" \
JVM_OPTS_ENV="$(tr '\0' '\n' < /proc/$pid/environ | grep '^JVM_OPTS_ENV=' | cut -d= -f2-)" \
./deploy-local.sh backend
```

部署后检查 `/srv/zfei/projects/loafer/app.log` 无 SSH 告警。

- [ ] **Step 3: 触发项目 22 流水线，验证历史回填**

通过 UI（http://124.223.33.130:9321/projects/22）重启流水线，或直接调启动接口。观察日志：模块 2/3 应出现「模块测试用例为空，启动测试设计 agent...」→「用例已生成并落库」。

验证落库：

```bash
mysql -h${DB_HOST} -P${DB_PORT} -uroot -p${DB_PASSWORD} loafer -e \
"SELECT id, LENGTH(api_integration_test), LENGTH(web_integration_test) FROM module WHERE project_id=22 AND module_type='business';"
```
Expected: 两字段长度 > 0

- [ ] **Step 4: 验证 UI 展示**

浏览器打开项目 22 → 模块 2 → 「集成测试」面板：API/Web tab 均有场景卡片。若模块随后重跑了门禁（非必须），场景卡片应显示「上次通过/失败」标签与截图。

- [ ] **Step 5: 发现问题则修复回归；全部通过后收尾**

如发现 agent 不遵守文件契约（不写 spec 文件/截图名不符），按 [[loafer-auto-test-gate]] 的失败模式迭代提示词，修复后回到 Step 1。

---

## Self-Review 记录

- **Spec 覆盖**：用例生成（T4）✓ 门禁第0步+历史回填（T6）✓ 场景级执行与回写（T1/T3/T5）✓ 截图契约/清理（T2/T5）✓ 截图路由（T7）✓ 桩接口实现（T7）✓ 前端截图展示+Web tab（T8/T9）✓ 错误处理表各项（T4 部分产物、T5 scenarios 缺失回退由 parseModuleTestResult 兼容、T2 路径穿越）✓ 非目标均未引入 ✓
- **类型一致性**：`ScenarioResult` 字段名在 T1/T3/T5 一致；`slugifyScenarioName`/`ScreenshotDir`/`ResolveScreenshotPath` 命名在 T2/T5/T7 一致；`buildDesignPrompt(project, mod, needAPI, needWeb)` 与 `buildScenarioDrivenSection(mod, accessURL)` 签名在 T4/T5 内部一致
- **已知取舍**：handler 层无 sqlite 驱动可用，截图路由只测路径解析契约（T7 Step 4），完整行为由 T10 端到端验证覆盖
