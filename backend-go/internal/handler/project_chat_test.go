package handler

import (
	"testing"
)

// TestSanitizeProjectName 验证项目名清洗逻辑，过滤低质量名称。
func TestSanitizeProjectName(t *testing.T) {
	original := "开发一个简易的个人待办mvp web系统"

	cases := []struct {
		name string
		want string // "" 表示应被过滤
	}{
		// 合格名称
		{"极简待办", "极简待办"},
		{"个人待办", "个人待办"},
		{"协作记事本", "协作记事本"},
		// 中英混杂应被过滤
		{"待办mvp", ""},
		{"商城web", ""},
		{"笔记app", ""},
		// 数字后缀应被过滤
		{"待办1", ""},
		{"待办2", ""},
		{"mvp3", ""},
		// 口语前缀应被过滤
		{"我想要待办", ""},
		{"帮我做笔记", ""},
		// 长度不符应被过滤
		{"记", ""},
		{"这是一个超长的项目名称超过八字", ""},
		// 与原始输入开头相同应被过滤（说明是截断）
		{"开发一个", ""},
	}

	for _, c := range cases {
		got := sanitizeProjectName(c.name, original)
		if got != c.want {
			t.Errorf("sanitizeProjectName(%q)=%q, 期望 %q", c.name, got, c.want)
		}
	}
}

// TestParseRequirementSummary_ValidJSON 验证正常 AI 输出能正确解析。
func TestParseRequirementSummary_ValidJSON(t *testing.T) {
	aiOutput := `{"projectName":"极简待办","projectNameOptions":["极简待办","个人待办","轻量待办","随身待办","今日待办"],"repoName":"simple_todo","repoNameOptions":["simple_todo","mini_todo"],"summary":"个人待办应用","keyFeatures":["注册登录","创建待办"],"techRequirements":["Go后端"],"userType":"个人用户"}`

	summary, err := parseRequirementSummary(aiOutput, "开发一个个人待办系统")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if summary.ProjectName != "极简待办" {
		t.Errorf("ProjectName=%q, 期望 \"极简待办\"", summary.ProjectName)
	}
	if len(summary.ProjectNameOptions) != 5 {
		t.Errorf("ProjectNameOptions 数量=%d, 期望 5", len(summary.ProjectNameOptions))
	}
}

// TestParseRequirementSummary_LowQualityNames 验证 AI 返回低质量名称时返回 error（触发重试）。
func TestParseRequirementSummary_LowQualityNames(t *testing.T) {
	// 所有名称都是中英混杂 + 数字后缀，应被全部过滤
	aiOutput := `{"projectName":"待办mvp1","projectNameOptions":["待办mvp1","待办mvp2","待办mvp3"],"repoName":"todo","summary":"待办","keyFeatures":["待办"],"techRequirements":["Go"],"userType":"个人"}`

	_, err := parseRequirementSummary(aiOutput, "开发一个待办mvp系统")
	if err == nil {
		t.Error("期望返回 error（低质量名称应被过滤并触发重试），但返回了 nil")
	}
}

// TestParseRequirementSummary_NoJSON 验证无 JSON 输出时返回 error。
func TestParseRequirementSummary_NoJSON(t *testing.T) {
	_, err := parseRequirementSummary("这不是JSON", "测试输入")
	if err == nil {
		t.Error("期望返回 error（无 JSON），但返回了 nil")
	}
}

// TestParseRequirementSummary_EmptyProjectName 验证 projectName 为空时返回 error。
func TestParseRequirementSummary_EmptyProjectName(t *testing.T) {
	aiOutput := `{"projectName":"","projectNameOptions":[],"repoName":"todo","summary":"待办"}`
	_, err := parseRequirementSummary(aiOutput, "测试输入")
	if err == nil {
		t.Error("期望返回 error（projectName 为空），但返回了 nil")
	}
}

// TestExtractFirstJSONObject 验证从含重复 JSON 的字符串中提取首个完整对象。
func TestExtractFirstJSONObject(t *testing.T) {
	// AI 输出中 JSON 可能出现两次（assistant 文本块 + result 事件）
	input := `{"projectName":"待办"} some text {"projectName":"待办"}`
	got := extractFirstJSONObject(input)
	want := `{"projectName":"待办"}`
	if got != want {
		t.Errorf("extractFirstJSONObject=%q, 期望 %q", got, want)
	}
}

// TestParseRequirementSummary_WithSystemEvents 验证 AI 输出中夹杂
// thinking_tokens 等系统事件 JSON 时，能跳过它们找到包含 projectName 的结果。
func TestParseRequirementSummary_WithSystemEvents(t *testing.T) {
	// 模拟真实的 AI 输出：系统事件 JSON + 格式化文本 + 实际结果 JSON
	aiOutput := `{"type":"system","subtype":"thinking_tokens","estimated_tokens":1147}` +
		`[系统] thinking_tokens {"type":"system","subtype":"thinking_tokens","estimated_tokens":1148}` +
		`[系统] thinking_tokens [思考] …` +
		`{"projectName":"极简待办","projectNameOptions":["极简待办","轻量待办","个人待办","今日待办","随身清单"],"repoName":"simple_todo","repoNameOptions":["simple_todo","lite_todo"],"summary":"个人待办应用","keyFeatures":["注册登录","创建待办"],"techRequirements":["Go后端"],"userType":"个人用户"}` +
		`[完成] success 耗时10671ms`

	summary, err := parseRequirementSummary(aiOutput, "开发一个个人待办系统")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if summary.ProjectName != "极简待办" {
		t.Errorf("ProjectName=%q, 期望 \"极简待办\"", summary.ProjectName)
	}
	if len(summary.ProjectNameOptions) != 5 {
		t.Errorf("ProjectNameOptions 数量=%d, 期望 5", len(summary.ProjectNameOptions))
	}
}
