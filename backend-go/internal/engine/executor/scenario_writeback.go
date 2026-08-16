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

// sanitizeStepResults 截断步骤 output/error 防止 JSON 体积爆炸，保留结构完整性。
func sanitizeStepResults(steps []ScenarioStepResult) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(steps))
	for _, s := range steps {
		out = append(out, map[string]interface{}{
			"action":  s.Action,
			"command": s.Command,
			"ok":      s.OK,
			"output":  tailString(s.Output, 500),
			"error":   tailString(s.Error, 300),
		})
	}
	return out
}

// applyScenarioResults 把场景执行结果回写进用例 spec JSON（按场景 name 精确匹配）。
// 写回字段：lastRunAt(RFC3339) / lastSuccess / lastSummary(日志尾部 200 字符) /
// lastSteps(步骤明细，output 截断 500 字符、error 截断 300 字符) /
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
		if r.Steps != nil {
			m["lastSteps"] = sanitizeStepResults(r.Steps)
		}
		if r.Screenshot != "" {
			m["screenshot"] = r.Screenshot
		} else {
			// 本轮该场景没有产出终态截图时删除旧路径，避免上一轮失败留下的
			// 「Failed」错误页截图与最新 lastSuccess 并存造成误判。
			delete(m, "screenshot")
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

// InjectCycleStepsToSpec 把「全量测试→失败→开发修复→重新部署→重测」循环里程碑
// 注入模块用例 spec 中相关场景的 lastSteps 头部，让「步骤明细」能回放整个修复过程。
// cycleSteps 键为场景名，值为按时间顺序的里程碑步骤；仅对有条目的场景注入。
// spec 为空/非法/无命中时原样返回。output 截断 500、error 截断 300，与
// applyScenarioResults 保持一致。
func InjectCycleStepsToSpec(specJSON string, cycleSteps map[string][]ScenarioStepResult) string {
	if strings.TrimSpace(specJSON) == "" || len(cycleSteps) == 0 {
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
	changed := false
	for _, s := range scenarios {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		milestones, hit := cycleSteps[name]
		if !hit || len(milestones) == 0 {
			continue
		}
		var existing []interface{}
		if ls, ok := m["lastSteps"].([]interface{}); ok {
			existing = ls
		}
		newSteps := make([]interface{}, 0, len(milestones)+len(existing))
		for _, ms := range milestones {
			newSteps = append(newSteps, map[string]interface{}{
				"action":  ms.Action,
				"command": ms.Command,
				"ok":      ms.OK,
				"output":  tailString(ms.Output, 500),
				"error":   tailString(ms.Error, 300),
			})
		}
		newSteps = append(newSteps, existing...)
		m["lastSteps"] = newSteps
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
