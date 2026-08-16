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
	// Web 用例生成提示必须包含导航容错与终态截图防错误页指导（避免「终态截图只有 Failed」）
	for _, want := range []string{"safeGoto", "chrome-error://", "终态截图防错误页", "domcontentloaded"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Web 用例提示词缺少 %q（截图防错误页指导）", want)
		}
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
