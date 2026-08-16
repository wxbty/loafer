package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loafer-agent/internal/model"
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

// TestApplyResultToRun 验证测试结果到 TestRun 的映射。
func TestApplyResultToRun(t *testing.T) {
	t.Run("通过-从 summary 统计 PassCount", func(t *testing.T) {
		run := &model.TestRun{}
		applyResultToRun(run, &ModuleTestResult{
			Passed:  true,
			Summary: "集成测试 8/10 通过；Playwright 3/4 通过",
		}, "agent输出")
		if run.Status != "passed" || run.FailCount != 0 {
			t.Fatalf("通过时映射不正确: %+v", run)
		}
		if run.PassCount != 11 {
			t.Fatalf("PassCount 应为 8+3=11，得到 %d", run.PassCount)
		}
	})

	t.Run("通过-summary 无匹配时 PassCount 为 1", func(t *testing.T) {
		run := &model.TestRun{}
		applyResultToRun(run, &ModuleTestResult{Passed: true, Summary: "全部通过"}, "agent输出")
		if run.Status != "passed" || run.FailCount != 0 {
			t.Fatalf("通过时映射不正确: %+v", run)
		}
		if run.PassCount != 1 {
			t.Fatalf("PassCount 应为 1，得到 %d", run.PassCount)
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

// TestRunModuleScenariosTestTypeGuard 验证类型化全量测试的类型校验：
// 不支持的场景类型在触碰 DB/执行前即报错（nil executor 下也不会 panic）。
func TestRunModuleScenariosTestTypeGuard(t *testing.T) {
	e := &TestExecutor{}
	if _, err := e.RunModuleScenariosTest(&model.Project{ID: 1}, &model.Module{ID: 2}, "bad", 1, func(string) {}); err == nil {
		t.Fatalf("不支持的场景类型应返回错误")
	}
	if _, err := e.RunModuleScenariosTest(&model.Project{ID: 1}, &model.Module{ID: 2}, "", 1, func(string) {}); err == nil {
		t.Fatalf("空场景类型应返回错误")
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
	if got := tailString("任意内容", 0); got != "" {
		t.Fatalf("maxLen=0 应返回空字符串: %q", got)
	}
	if got := tailString("任意内容", -5); got != "" {
		t.Fatalf("maxLen 为负数应返回空字符串: %q", got)
	}
}

// TestSynthE2EScenarioSteps 验证 e2e 步骤明细合成：
// Playwright JSON reporter 无逐步骤数据，按用例定义步骤回填；
// 通过时全部 ok=true；失败时末步携带真实错误、其余步骤也标记失败。
func TestSynthE2EScenarioSteps(t *testing.T) {
	steps := []scenarioStepDef{
		{Action: "打开注册页", Expected: "显示表单"},
		{Action: "填写表单并点击注册", Command: "fill + click", Expected: "跳转 /login"},
	}

	t.Run("通过-全部ok", func(t *testing.T) {
		out := synthE2EScenarioSteps(steps, true, "")
		if len(out) != 2 {
			t.Fatalf("应合成 2 步，得到 %d", len(out))
		}
		for i, s := range out {
			if !s.OK || s.Action != steps[i].Action || s.Command != steps[i].Command || s.Error != "" {
				t.Fatalf("通过场景步骤应全部 ok 且保留输入: %+v", s)
			}
		}
	})

	t.Run("失败-末步携带错误", func(t *testing.T) {
		out := synthE2EScenarioSteps(steps, false, "expect(received).toHaveURL() 超时 5000ms")
		if len(out) != 2 || out[0].OK || out[1].OK {
			t.Fatalf("失败场景所有步骤应 ok=false: %+v", out)
		}
		if out[0].Error != "" {
			t.Fatalf("非末步不应携带错误: %+v", out[0])
		}
		if out[1].Error != "expect(received).toHaveURL() 超时 5000ms" {
			t.Fatalf("末步应携带真实错误: %+v", out[1])
		}
	})

	t.Run("失败-错误超长截断", func(t *testing.T) {
		long := strings.Repeat("e", 400)
		out := synthE2EScenarioSteps(steps, false, long)
		if len([]rune(out[1].Error)) > 300 {
			t.Fatalf("错误应按 300 字符截断: %d", len([]rune(out[1].Error)))
		}
	})

	t.Run("空步骤返回nil", func(t *testing.T) {
		if out := synthE2EScenarioSteps(nil, true, ""); out != nil {
			t.Fatalf("空步骤应返回 nil，得到 %+v", out)
		}
	})
}

// TestWebScenarioByTitle 验证 playwright 标题 → 场景定义映射：
// playwrightTest 缺失时回退场景名；非法 JSON 返回 nil。
func TestWebScenarioByTitle(t *testing.T) {
	spec := `{"testScenarios":[
		{"name":"注册成功跳转登录页","playwrightTest":"注册成功：跳转 /login","steps":[{"action":"打开注册页","expected":"跳转 /login"}]},
		{"name":"密码过短提示","steps":[{"action":"填 5 位密码","expected":"提示错误"}]}
	]}`

	m := webScenarioByTitle(spec)
	if len(m) != 2 {
		t.Fatalf("应解析出 2 个场景，得到 %d", len(m))
	}
	reg, ok := m["注册成功：跳转 /login"]
	if !ok || reg.Name != "注册成功跳转登录页" || len(reg.Steps) != 1 || reg.Steps[0].Action == "" {
		t.Fatalf("标题→场景映射不正确: %+v", reg)
	}
	short, ok := m["密码过短提示"]
	if !ok || short.Name != "密码过短提示" {
		t.Fatalf("playwrightTest 缺失应回退场景名: %+v", short)
	}
	if got := webScenarioByTitle("{bad"); got != nil {
		t.Fatalf("非法 JSON 应返回 nil，得到 %+v", got)
	}
	if got := webScenarioByTitle(`{"testScenarios":[]}`); len(got) != 0 {
		t.Fatalf("空场景数组应返回空 map，得到 %d", len(got))
	}
}
