package cli

import (
	"strings"
	"testing"
)

// TestExtractDisplayTextFiltersThinkingTokens 验证 thinking_tokens 这类高频 token 计数
// 系统事件不会被转成展示文本（否则 SSE 输出被噪声淹没，导致任务运行中白屏/页面无响应、
// 断线重连回放暴涨、点击续跑后状态丢失）。
func TestExtractDisplayTextFiltersThinkingTokens(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "thinking_tokens 被过滤",
			line: `{"type":"system","subtype":"thinking_tokens","estimated_tokens":1147,"estimated_tokens_delta":3}`,
			want: "",
		},
		{
			name: "普通 system 事件保留",
			line: `{"type":"system","subtype":"progress","message":"done"}`,
			want: "[系统] progress",
		},
		{
			name: "system init 显示模型",
			line: `{"type":"system","subtype":"init","model":"claude-opus-5"}`,
			want: "[系统] 模型: claude-opus-5",
		},
		{
			name: "非 JSON 行返回空",
			line: `plain text not json`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractDisplayText(tc.line)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("extractDisplayText(%q) = %q, want empty", tc.line, got)
				}
				return
			}
			if !strings.HasPrefix(got, tc.want) {
				t.Fatalf("extractDisplayText(%q) = %q, want prefix %q", tc.line, got, tc.want)
			}
		})
	}
}
