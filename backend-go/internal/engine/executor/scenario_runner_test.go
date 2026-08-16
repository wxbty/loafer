package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loafer-agent/internal/model"
)

// TestSelectScenarioSpec 验证按类型选择用例字段：api→api_integration_test(kind api)，
// web→web_integration_test(kind e2e)；未知类型与空 spec 均报错。
func TestSelectScenarioSpec(t *testing.T) {
	mod := &model.Module{
		APIIntegrationTest: `{"testScenarios":[{"name":"登录成功"}]}`,
		WebIntegrationTest: `{"testScenarios":[{"name":"注册流程"}]}`,
	}

	t.Run("api", func(t *testing.T) {
		spec, kind, column, err := selectScenarioSpec(mod, "api")
		if err != nil || kind != "api" || column != "api_integration_test" || !strings.Contains(spec, "登录成功") {
			t.Fatalf("api 选择不正确: kind=%s column=%s spec=%s err=%v", kind, column, spec, err)
		}
	})

	t.Run("web", func(t *testing.T) {
		spec, kind, column, err := selectScenarioSpec(mod, "web")
		if err != nil || kind != "e2e" || column != "web_integration_test" || !strings.Contains(spec, "注册流程") {
			t.Fatalf("web 选择不正确: kind=%s column=%s spec=%s err=%v", kind, column, spec, err)
		}
	})

	t.Run("未知类型报错", func(t *testing.T) {
		if _, _, _, err := selectScenarioSpec(mod, "tdd"); err == nil {
			t.Fatalf("未知类型应返回错误")
		}
	})

	t.Run("空spec报错", func(t *testing.T) {
		empty := &model.Module{}
		if _, _, _, err := selectScenarioSpec(empty, "api"); err == nil {
			t.Fatalf("空 spec 应返回错误")
		}
		if _, _, _, err := selectScenarioSpec(empty, "web"); err == nil {
			t.Fatalf("空 spec 应返回错误")
		}
	})
}

// TestParseScenarioAt 验证按索引取场景：合法索引、越界、负索引、非法 JSON、空场景数组。
func TestParseScenarioAt(t *testing.T) {
	spec := `{"testScenarios":[{"name":"场景A","steps":[{"action":"a1","command":"curl $BASE_URL/x","expected":"ok"}]},{"name":"场景B","playwrightTest":"注册流程","specFile":"tests/e2e/module-1.spec.ts"}]}`

	t.Run("取第一个", func(t *testing.T) {
		sc, err := parseScenarioAt(spec, 0)
		if err != nil || sc.Name != "场景A" || len(sc.Steps) != 1 || sc.Steps[0].Command == "" {
			t.Fatalf("解析不正确: %+v err=%v", sc, err)
		}
	})

	t.Run("取第二个-带playwright字段", func(t *testing.T) {
		sc, err := parseScenarioAt(spec, 1)
		if err != nil || sc.Name != "场景B" || sc.PlaywrightTest != "注册流程" || sc.SpecFile == "" {
			t.Fatalf("解析不正确: %+v err=%v", sc, err)
		}
	})

	t.Run("越界报错", func(t *testing.T) {
		if _, err := parseScenarioAt(spec, 2); err == nil {
			t.Fatalf("越界应返回错误")
		}
	})

	t.Run("负索引报错", func(t *testing.T) {
		if _, err := parseScenarioAt(spec, -1); err == nil {
			t.Fatalf("负索引应返回错误")
		}
	})

	t.Run("非法JSON报错", func(t *testing.T) {
		if _, err := parseScenarioAt("{bad", 0); err == nil {
			t.Fatalf("非法 JSON 应返回错误")
		}
	})

	t.Run("空场景数组报错", func(t *testing.T) {
		if _, err := parseScenarioAt(`{"testScenarios":[]}`, 0); err == nil {
			t.Fatalf("空场景数组应返回错误")
		}
	})
}

// TestBuildSingleScenarioPrompt 验证单场景提示词要素：
// api 场景含 $BASE_URL 替换说明与逐步比对要求；e2e 场景含 playwright 运行命令与截图路径约定；
// 两者都必须携带结果文件路径与场景名。
func TestBuildSingleScenarioPrompt(t *testing.T) {
	project := &model.Project{ID: 22, Name: "轻量待办", WorkDir: "/srv/work/proj"}
	mod := &model.Module{ID: 84, Name: "账号系统", SequenceNumber: "2", Description: "用户注册登录"}

	t.Run("api场景", func(t *testing.T) {
		sc := &scenarioDef{
			Name: "登录成功",
			Steps: []scenarioStepDef{
				{Action: "登录", Command: "curl -s -X POST $BASE_URL/api/login", Expected: "\"code\":0"},
			},
		}
		prompt := buildSingleScenarioPrompt(project, mod, sc, "api", "http://x:40410")
		for _, want := range []string{"登录成功", "$BASE_URL", "http://x:40410", "curl -s -X POST $BASE_URL/api/login", "tests/results/module-84-manual.json", "steps"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("api 单场景提示词缺少 %q", want)
			}
		}
	})

	t.Run("e2e场景", func(t *testing.T) {
		sc := &scenarioDef{
			Name:           "注册流程",
			PlaywrightTest: "注册流程",
			SpecFile:       "tests/e2e/module-84.spec.ts",
			Steps:          []scenarioStepDef{{Action: "打开注册页", Expected: "显示表单"}},
		}
		prompt := buildSingleScenarioPrompt(project, mod, sc, "e2e", "http://x:40410")
		for _, want := range []string{
			"注册流程", "tests/e2e/module-84.spec.ts", "npx playwright test", "BASE_URL=http://x:40410",
			"tests/results/screenshots/module-84/注册流程.png", "tests/results/screenshots/module-84/注册流程-error.png",
			"tests/results/module-84-manual.json", "打开注册页",
		} {
			if !strings.Contains(prompt, want) {
				t.Errorf("e2e 单场景提示词缺少 %q", want)
			}
		}
	})

	t.Run("e2e场景-缺省字段回退", func(t *testing.T) {
		sc := &scenarioDef{Name: "默认场景"}
		prompt := buildSingleScenarioPrompt(project, mod, sc, "e2e", "http://x:40410")
		// playwrightTest 缺省回退场景名；specFile 缺省回退 tests/e2e/module-<id>.spec.ts
		if !strings.Contains(prompt, "tests/e2e/module-84.spec.ts") {
			t.Errorf("specFile 缺省应回退到 module-<id>.spec.ts")
		}
		if !strings.Contains(prompt, "-g \"默认场景\"") && !strings.Contains(prompt, "「默认场景」") {
			t.Errorf("playwrightTest 缺省应回退为场景名")
		}
	})
}

// TestParseSingleScenarioResult 验证单场景结果文件解析：
// 合法 JSON（含 steps）、空/畸形报错、name 缺失时由调用方回填。
func TestParseSingleScenarioResult(t *testing.T) {
	t.Run("合法-含步骤明细", func(t *testing.T) {
		data := []byte(`{"name":"注册流程","passed":false,"log":"断言失败","screenshot":"tests/results/screenshots/module-1/注册流程.png","errorScreenshot":"tests/results/screenshots/module-1/注册流程-error.png","steps":[{"action":"打开注册页","command":"","ok":true,"output":"page loaded"},{"action":"提交表单","command":"","ok":false,"output":"","error":"timeout 5000ms"}]}`)
		r, err := parseSingleScenarioResult(data, "注册流程")
		if err != nil {
			t.Fatalf("期望解析成功: %v", err)
		}
		if r.Passed || r.Name != "注册流程" || len(r.Steps) != 2 || r.Steps[1].Error == "" {
			t.Fatalf("解析不正确: %+v", r)
		}
	})

	t.Run("name缺失-回填期望名", func(t *testing.T) {
		data := []byte(`{"passed":true,"log":"ok"}`)
		r, err := parseSingleScenarioResult(data, "登录成功")
		if err != nil || r.Name != "登录成功" {
			t.Fatalf("name 应回填为期望场景名: %+v err=%v", r, err)
		}
	})

	t.Run("空内容报错", func(t *testing.T) {
		if _, err := parseSingleScenarioResult([]byte("  "), "x"); err == nil {
			t.Fatalf("空内容应返回错误")
		}
	})

	t.Run("畸形JSON报错", func(t *testing.T) {
		if _, err := parseSingleScenarioResult([]byte(`{"passed":`), "x"); err == nil {
			t.Fatalf("畸形 JSON 应返回错误")
		}
	})
}

// TestFillScenarioScreenshotPath 验证截图回填按「命名 slug」而非 r.Name 匹配：
// spec 截图以 playwright 标题生成文件（与用例显示名 slug 不同），
// 传标题能命中、传显示名命中不到，这是修复「通过但无截图」的关键。
func TestFillScenarioScreenshotPath(t *testing.T) {
	workDir := t.TempDir()
	const moduleID int64 = 84
	title := "E2E-84-01 注册成功：跳转 /login 并显示「注册成功，请登录」"
	displayName := "E2E-84-01 注册成功跳转登录页并提示"
	// 与 spec scenarioFileName(testInfo.title) 一致的文件名
	fileBase := slugifyScenarioName(title)

	shotDir := filepath.Join(workDir, "tests", "results", "screenshots", "module-84")
	if err := os.MkdirAll(shotDir, 0o755); err != nil {
		t.Fatalf("创建截图目录失败: %v", err)
	}
	for _, f := range []string{fileBase + ".png", fileBase + "-error.png"} {
		if err := os.WriteFile(filepath.Join(shotDir, f), []byte("fake"), 0o644); err != nil {
			t.Fatalf("写入截图失败: %v", err)
		}
	}

	t.Run("按标题slug命中", func(t *testing.T) {
		r := &ScenarioResult{Name: displayName}
		fillScenarioScreenshotPath(workDir, moduleID, title, r)
		if !strings.Contains(r.Screenshot, fileBase+".png") {
			t.Fatalf("按标题 slug 应命中终态截图: %q", r.Screenshot)
		}
		if !strings.Contains(r.ErrorScreenshot, fileBase+"-error.png") {
			t.Fatalf("按标题 slug 应命中出错截图: %q", r.ErrorScreenshot)
		}
	})

	t.Run("按显示名slug不命中", func(t *testing.T) {
		r := &ScenarioResult{Name: displayName}
		fillScenarioScreenshotPath(workDir, moduleID, displayName, r)
		if r.Screenshot != "" || r.ErrorScreenshot != "" {
			t.Fatalf("显示名 slug 对应的文件不存在，不应回填: %+v", r)
		}
	})

	t.Run("无截图文件不回填", func(t *testing.T) {
		r := &ScenarioResult{}
		fillScenarioScreenshotPath(workDir, moduleID, "不存在的场景标题", r)
		if r.Screenshot != "" || r.ErrorScreenshot != "" {
			t.Fatalf("文件不存在不应回填: %+v", r)
		}
	})
}
