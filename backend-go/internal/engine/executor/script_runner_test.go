package executor

import (
	"strings"
	"testing"
)

// TestMergeScriptResults_AllPass API + Web 全通过 → passed=true。
func TestMergeScriptResults_AllPass(t *testing.T) {
	api := &ModuleTestResult{
		ModuleID: 1,
		Passed:   true,
		Summary:  "API 2/2 通过",
		Scenarios: []ScenarioResult{
			{Kind: "api", Name: "健康检查", Passed: true},
			{Kind: "api", Name: "登录", Passed: true},
		},
	}
	web := []ScenarioResult{
		{Kind: "e2e", Name: "首页渲染", Passed: true},
		{Kind: "e2e", Name: "表单提交", Passed: true},
	}
	r := mergeScriptResults(1, api, web, true, true)
	if !r.Passed {
		t.Fatalf("预期 passed=true，实际 false: %s", r.Summary)
	}
	if len(r.Failures) != 0 {
		t.Fatalf("预期无失败，实际 %+v", r.Failures)
	}
	if len(r.Scenarios) != 4 {
		t.Fatalf("预期 4 个场景，实际 %d", len(r.Scenarios))
	}
	if !strings.Contains(r.Summary, "API 2/2") || !strings.Contains(r.Summary, "Playwright 2/2") {
		t.Fatalf("summary 缺失计数: %s", r.Summary)
	}
}

// TestMergeScriptResults_PartialFail Web 一侧失败 → 整体失败且 failures 含 e2e 条目。
func TestMergeScriptResults_PartialFail(t *testing.T) {
	api := &ModuleTestResult{
		ModuleID: 1,
		Passed:   true,
		Summary:  "API 1/1 通过",
		Scenarios: []ScenarioResult{
			{Kind: "api", Name: "健康检查", Passed: true},
		},
	}
	web := []ScenarioResult{
		{Kind: "e2e", Name: "首页渲染", Passed: true},
		{Kind: "e2e", Name: "表单提交", Passed: false, Log: "element not found"},
	}
	r := mergeScriptResults(1, api, web, true, true)
	if r.Passed {
		t.Fatalf("预期 passed=false，实际 true: %s", r.Summary)
	}
	if len(r.Failures) != 1 || r.Failures[0].Kind != "e2e" || r.Failures[0].Name != "表单提交" {
		t.Fatalf("预期 1 条 e2e 失败「表单提交」，实际 %+v", r.Failures)
	}
}

// TestMergeScriptResults_MissingWeb Playwright 无结果 → 失败并提示。
func TestMergeScriptResults_MissingWeb(t *testing.T) {
	api := &ModuleTestResult{ModuleID: 1, Passed: true, Summary: "API 1/1 通过"}
	r := mergeScriptResults(1, api, nil, true, true)
	if r.Passed {
		t.Fatalf("预期 passed=false（Web 无结果），实际 true")
	}
	if len(r.Failures) != 1 || r.Failures[0].Kind != "e2e" {
		t.Fatalf("预期 1 条 e2e 失败提示，实际 %+v", r.Failures)
	}
}

// TestParsePlaywrightResultFromOutput 解析 playwright --reporter=json 输出。
func TestParsePlaywrightResultFromOutput(t *testing.T) {
	// status: passed/failed 各一，无 extra.stdout 干扰
	output := `{
  "suites": [{
    "specs": [
      {"title": "首页渲染", "tests": [{"results": [{"status": "passed", "error": {}, "duration": 100}]}]},
      {"title": "表单提交", "tests": [{"results": [{"status": "failed", "error": {"message": "element not found"}, "duration": 50}]}]}
    ]
  }]
}`
	results := ParsePlaywrightResultFromOutput(output, 1)
	if len(results) != 2 {
		t.Fatalf("预期 2 个结果，实际 %d", len(results))
	}
	byName := map[string]ScenarioResult{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if !byName["首页渲染"].Passed {
		t.Fatalf("首页渲染预期通过")
	}
	if byName["表单提交"].Passed {
		t.Fatalf("表单提交预期失败")
	}
	if !strings.Contains(byName["表单提交"].Log, "element not found") {
		t.Fatalf("失败日志缺失: %s", byName["表单提交"].Log)
	}
}

// TestParsePlaywrightResultFromOutput_Empty 无结果时返回 nil。
func TestParsePlaywrightResultFromOutput_Empty(t *testing.T) {
	if r := ParsePlaywrightResultFromOutput("not json", 1); r != nil {
		t.Fatalf("预期 nil，实际 %+v", r)
	}
}

// TestParsePlaywrightResultFromOutput_NestedSuite 递归解析嵌套 suite：
// spec 用 test.describe() 包裹时 specs 挂在第二层，只读顶层会漏掉全部场景。
func TestParsePlaywrightResultFromOutput_NestedSuite(t *testing.T) {
	output := `{
	  "suites": [{
	    "title": "module-84.spec.ts",
	    "specs": [],
	    "suites": [{
	      "title": "模块84 注册流程",
	      "specs": [
	        {"title": "E2E-84-01 注册成功", "tests": [{"results": [{"status": "failed", "error": {"message": "element not found"}, "duration": 100}]}]},
	        {"title": "E2E-84-02 注册校验", "tests": [{"results": [{"status": "passed", "error": {}, "duration": 50}]}]}
	      ]
	    }]
	  }]
	}`
	results := ParsePlaywrightResultFromOutput(output, 1)
	if len(results) != 2 {
		t.Fatalf("嵌套 suite 下预期解析出 2 个结果，实际 %d", len(results))
	}
	byName := map[string]ScenarioResult{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if byName["E2E-84-01 注册成功"].Passed {
		t.Fatalf("E2E-84-01 预期失败")
	}
	if !byName["E2E-84-02 注册校验"].Passed {
		t.Fatalf("E2E-84-02 预期通过")
	}
	if !strings.Contains(byName["E2E-84-01 注册成功"].Log, "element not found") {
		t.Fatalf("失败日志缺失: %s", byName["E2E-84-01 注册成功"].Log)
	}
}

// TestSummarizePWError_HookScreenshotMarker 验证截图/afterEach 钩子类失败被标注为非断言失败，
// 普通断言失败不加前缀。
func TestSummarizePWError_HookScreenshotMarker(t *testing.T) {
	t.Run("page.screenshot 抛错标注", func(t *testing.T) {
		got := summarizePWError(pwTestResult{Status: "failed", Error: pwError{Message: "page.screenshot: Target page, context or browser has been closed"}})
		if !strings.Contains(got, "非断言失败") || !strings.Contains(got, "page.screenshot") {
			t.Fatalf("截图异常应标注为非断言失败: %s", got)
		}
	})

	t.Run("afterEach 钩子异常标注", func(t *testing.T) {
		got := summarizePWError(pwTestResult{Status: "failed", Error: pwError{Message: "Error in afterEach: page crashed"}})
		if !strings.Contains(got, "非断言失败") {
			t.Fatalf("afterEach 异常应标注: %s", got)
		}
	})

	t.Run("普通断言失败不标注", func(t *testing.T) {
		got := summarizePWError(pwTestResult{Status: "failed", Error: pwError{Message: "expect(received).toBeVisible(): element not found, locator: getByText('暂无待办')"}})
		if strings.Contains(got, "非断言失败") {
			t.Fatalf("断言失败不应标注为钩子异常: %s", got)
		}
	})

	t.Run("空错误返回空串", func(t *testing.T) {
		if got := summarizePWError(pwTestResult{Status: "failed"}); got != "" {
			t.Fatalf("空错误应返回空串: %q", got)
		}
	})
}
