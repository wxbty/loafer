package executor

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateUTF8Bytes 验证按字节截断不会切断多字节 UTF-8 字符，
// 且结果严格不超过 maxBytes。
func TestTruncateUTF8Bytes(t *testing.T) {
	// 中文"配"占 3 字节（E9 85 8D）。
	threeByte := "配"
	if len(threeByte) != 3 {
		t.Fatalf("前置条件失败: len(%q)=%d, 期望 3", threeByte, len(threeByte))
	}

	cases := []struct {
		name     string
		in       string
		maxBytes int
	}{
		{"不超过上限原样返回", "hello", 100},
		{"ASCII 恰好截断", "hello world", 5},
		{"ASCII 截断边界", "hello", 3},
		{"空字符串", "", 10},
		{"中文字符内截断", strings.Repeat(threeByte, 100), 299},   // 落在第 100 个字的第 2 字节
		{"中文字符内截断2", strings.Repeat(threeByte, 100), 298}, // 落在第 100 个字的第 1 字节
		{"中文字符边界", strings.Repeat(threeByte, 100), 300},    // 恰好完整
		{"混合中英截断", "ab" + strings.Repeat(threeByte, 50) + "cd", 100},
		{"超大上限", strings.Repeat(threeByte, 100), 60000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateUTF8Bytes(tc.in, tc.maxBytes)
			if len(got) > tc.maxBytes {
				t.Fatalf("len(got)=%d 超过上限 %d", len(got), tc.maxBytes)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("结果不是合法 UTF-8: %q", got)
			}
			// 截断后仍应是原字符串的前缀。
			if !strings.HasPrefix(tc.in, got) {
				t.Fatalf("结果不是原字符串前缀: %q", got)
			}
		})
	}

	// 特别验证：截断恰好落在"配"的第二个字节时，应保留 99 个完整汉字。
	got := truncateUTF8Bytes(strings.Repeat(threeByte, 100), 299)
	if want := 99 * 3; len(got) != want {
		t.Fatalf("299 字节截断应保留 99 个汉字(%d 字节)，实际 %d 字节", want, len(got))
	}
	if got != strings.Repeat(threeByte, 99) {
		t.Fatalf("内容不符合预期: %q", got)
	}
}

// TestIsTaskRunning 验证运行表判定：新任务不在运行中，标记后返回运行中，清理后恢复。
// 对应 StartTask 的并发防御：执行中的任务不应被重复启动/重试。
func TestIsTaskRunning(t *testing.T) {
	e := NewTaskExecutor(nil, nil)

	if e.IsTaskRunning(1) {
		t.Fatal("新任务不应处于运行中")
	}

	// 模拟 ExecuteTask 标记开始
	e.mu.Lock()
	e.runningTask[1] = true
	e.mu.Unlock()
	if !e.IsTaskRunning(1) {
		t.Fatal("标记后应处于运行中")
	}

	// 模拟 ExecuteTask 的 defer 清理
	e.mu.Lock()
	delete(e.runningTask, 1)
	e.mu.Unlock()
	if e.IsTaskRunning(1) {
		t.Fatal("清理后不应处于运行中")
	}
}

// TestBuildCompletedStepStatusJSON 验证两种步骤格式都能生成全部 completed 的状态 JSON。
func TestBuildCompletedStepStatusJSON(t *testing.T) {
	// 字符串数组格式
	out1 := BuildCompletedStepStatusJSON(`["步骤一","步骤二"]`)
	if !strings.Contains(out1, `"status":"completed"`) || !strings.Contains(out1, `"action":"步骤一"`) {
		t.Fatalf("字符串数组格式输出异常: %s", out1)
	}
	// 对象数组格式
	out2 := BuildCompletedStepStatusJSON(`[{"seq":5,"action":"A"},{"seq":6,"name":"B"}]`)
	if !strings.Contains(out2, `"seq":5`) || !strings.Contains(out2, `"action":"A"`) {
		t.Fatalf("对象数组格式输出异常: %s", out2)
	}
	if out2 == "" {
		t.Fatal("对象数组格式输出为空")
	}
	// 空/非法输入
	if BuildCompletedStepStatusJSON("") != "" {
		t.Fatal("空输入应返回空串")
	}
	if BuildCompletedStepStatusJSON("not-json") != "" {
		t.Fatal("非法 JSON 应返回空串")
	}
}
