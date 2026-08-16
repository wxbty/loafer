package executor

import "testing"

// TestSanitizeBlockedBy 验证入库前对 blockedBy 的清洗：
// LLM 分解结果可能把模块序号（如 "2"）误填进任务级 blockedBy，
// 这些序号在 task 表中查不到，会导致执行期依赖检查永远无法满足（流水线死锁）。
func TestSanitizeBlockedBy(t *testing.T) {
	validSeqs := map[string]bool{
		"1.1": true, "1.2": true, "1.5": true, "1.6": true,
		"2.1": true, "2.2": true,
	}

	cases := []struct {
		name      string
		blockedBy string
		self      string
		wantKept  string
		wantDrop  int
	}{
		{"空依赖", "", "1.7", "", 0},
		{"全部有效", "1.2,1.3", "1.4", "1.2", 1}, // 1.3 不在 validSeqs 中
		{"模块序号被剔除", "2,3", "1.7", "", 2},
		{"混合有效无效", "1.5,2", "1.7", "1.5", 1},
		{"自依赖被剔除", "1.7", "1.7", "", 1},
		{"空白与空项", " 1.5 , , 1.6 ", "1.7", "1.5,1.6", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kept, dropped := sanitizeBlockedBy(c.blockedBy, validSeqs, c.self)
			if kept != c.wantKept {
				t.Errorf("kept = %q, 期望 %q", kept, c.wantKept)
			}
			if len(dropped) != c.wantDrop {
				t.Errorf("dropped = %v, 期望 %d 项", dropped, c.wantDrop)
			}
		})
	}
}
