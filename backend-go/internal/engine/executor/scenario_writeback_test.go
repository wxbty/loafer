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

	t.Run("新结果无截图时清除旧screenshot", func(t *testing.T) {
		// 本轮该场景没有产出终态截图（如 afterEach 截图失败），必须删除旧路径，
		// 否则旧一轮失败留下的「Failed」截图会与最新 lastSuccess=true 并存误导用户。
		dirty := `{"testScenarios":[{"name":"注册流程","screenshot":"old-failed.png","lastSuccess":false}]}`
		out := applyScenarioResults(dirty, []ScenarioResult{{Kind: "e2e", Name: "注册流程", Passed: true}}, now)
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("输出应为合法 JSON: %v", err)
		}
		sc := parsed["testScenarios"].([]interface{})[0].(map[string]interface{})
		if _, ok := sc["screenshot"]; ok {
			t.Fatalf("新结果无截图时应清除旧 screenshot: %+v", sc)
		}
		if sc["lastSuccess"] != true {
			t.Fatalf("lastSuccess 应按新结果更新为 true: %+v", sc)
		}
	})

	t.Run("新结果有截图时覆盖旧截图", func(t *testing.T) {
		dirty := `{"testScenarios":[{"name":"注册流程","screenshot":"old.png"}]}`
		out := applyScenarioResults(dirty, []ScenarioResult{{Kind: "e2e", Name: "注册流程", Passed: true, Screenshot: "new.png"}}, now)
		if !strings.Contains(out, "new.png") || strings.Contains(out, "old.png") {
			t.Fatalf("新截图应覆盖旧截图: %s", out)
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

	t.Run("回写lastSteps步骤明细", func(t *testing.T) {
		out := applyScenarioResults(spec, []ScenarioResult{
			{Kind: "api", Name: "登录成功", Passed: false, Log: "断言失败", Steps: []ScenarioStepResult{
				{Action: "登录", Command: "curl $BASE_URL/api/login", OK: true, Output: "200 OK"},
				{Action: "拉取列表", Command: "curl $BASE_URL/api/todos", OK: false, Error: "期望 code:0"},
			}},
		}, now)
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("输出应为合法 JSON: %v", err)
		}
		first := parsed["testScenarios"].([]interface{})[0].(map[string]interface{})
		steps, ok := first["lastSteps"].([]interface{})
		if !ok || len(steps) != 2 {
			t.Fatalf("lastSteps 应有 2 条: %+v", first)
		}
		s0 := steps[0].(map[string]interface{})
		s1 := steps[1].(map[string]interface{})
		if s0["ok"] != true || s0["action"] != "登录" || s1["ok"] != false || s1["error"] == "" {
			t.Fatalf("lastSteps 内容不正确: %+v / %+v", s0, s1)
		}
	})

	t.Run("lastSteps输出截断", func(t *testing.T) {
		longOutput := strings.Repeat("x", 2000)
		out := applyScenarioResults(spec, []ScenarioResult{
			{Kind: "api", Name: "登录成功", Passed: true, Steps: []ScenarioStepResult{
				{Action: "a", OK: true, Output: longOutput},
			}},
		}, now)
		if len(out) >= len(longOutput) {
			t.Fatalf("步骤输出应被截断，回写结果过长: %d", len(out))
		}
		if !strings.Contains(out, "lastSteps") {
			t.Fatalf("应包含 lastSteps: %s", out[:200])
		}
	})
}

func TestInjectCycleStepsToSpec(t *testing.T) {
	spec := `{"testScenarios":[{"name":"登录成功","steps":[],"lastSteps":[{"action":"登录","ok":true}]},{"name":"注册流程","steps":[]}],"retryStrategy":{"maxRetries":3}}`
	cycle := map[string][]ScenarioStepResult{
		"登录成功": {
			{Action: "第1轮API全量测试未通过", OK: false, Output: "步骤2失败", Error: "步骤2失败"},
			{Action: "开发agent修复", OK: true, Output: "修复了 xxx"},
			{Action: "重新部署", OK: true, Output: "部署完成"},
			{Action: "第2轮API全量测试通过", OK: true, Output: "通过"},
		},
	}

	t.Run("命中场景里程碑前置", func(t *testing.T) {
		out := InjectCycleStepsToSpec(spec, cycle)
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("输出应为合法 JSON: %v", err)
		}
		scenarios := parsed["testScenarios"].([]interface{})
		first := scenarios[0].(map[string]interface{})
		steps, ok := first["lastSteps"].([]interface{})
		if !ok {
			t.Fatalf("应有 lastSteps: %+v", first)
		}
		if len(steps) != 5 {
			t.Fatalf("lastSteps 应为 4 里程碑 + 1 原步骤 = 5，得到 %d", len(steps))
		}
		s0 := steps[0].(map[string]interface{})
		if s0["action"] != "第1轮API全量测试未通过" || s0["ok"] != false || s0["error"] == "" {
			t.Fatalf("里程碑应前置并携带失败信息: %+v", s0)
		}
		sLast := steps[4].(map[string]interface{})
		if sLast["action"] != "登录" || sLast["ok"] != true {
			t.Fatalf("原步骤应保留在尾部: %+v", sLast)
		}
		if parsed["retryStrategy"] == nil {
			t.Fatalf("无关字段应保留")
		}
	})

	t.Run("未命中场景不动", func(t *testing.T) {
		out := InjectCycleStepsToSpec(spec, map[string][]ScenarioStepResult{"不存在": {{Action: "x"}}})
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("输出应为合法 JSON: %v", err)
		}
		sc := parsed["testScenarios"].([]interface{})[1].(map[string]interface{})
		if _, ok := sc["lastSteps"]; ok {
			t.Fatalf("未命中场景不应新增 lastSteps")
		}
	})

	t.Run("空cycle原样返回", func(t *testing.T) {
		if out := InjectCycleStepsToSpec(spec, nil); out != spec {
			t.Fatalf("空 cycle 应原样返回")
		}
	})

	t.Run("非法spec原样返回", func(t *testing.T) {
		bad := `{not json`
		if out := InjectCycleStepsToSpec(bad, cycle); out != bad {
			t.Fatalf("非法 spec 应原样返回")
		}
	})

	t.Run("里程碑输出截断", func(t *testing.T) {
		long := strings.Repeat("x", 1000)
		out := InjectCycleStepsToSpec(spec, map[string][]ScenarioStepResult{"登录成功": {{Action: "a", Output: long}}})
		if len(out) >= len(spec)+len(long) {
			t.Fatalf("里程碑 output 应被截断")
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
