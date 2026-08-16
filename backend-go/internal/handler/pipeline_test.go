package handler

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveModuleAction 验证流水线重启时对各模块状态的分流：
// 4 完成→跳过；2 待测试/3 测试中→直接进测试门禁；其余（0/1/5/6）→重跑任务后进门禁。
func TestResolveModuleAction(t *testing.T) {
	cases := []struct {
		status int
		want   moduleLoopAction
	}{
		{0, moduleActionRunThenTest}, // 待执行
		{1, moduleActionRunThenTest}, // 执行中（中断残留）
		{2, moduleActionTestOnly},    // 待测试（历史数据兼容）
		{3, moduleActionTestOnly},    // 测试中（闭环中途被中断）
		{4, moduleActionSkip},        // 完成
		{5, moduleActionRunThenTest}, // 测试失败：重跑任务
		{6, moduleActionRunThenTest}, // 失败：重跑任务
	}
	for _, tc := range cases {
		if got := resolveModuleAction(tc.status); got != tc.want {
			t.Fatalf("status=%d: got %v, want %v", tc.status, got, tc.want)
		}
	}
}

// TestHasPlaywrightSpecs 验证阶段5 全局冒烟前对 Playwright 用例文件的探测。
func TestHasPlaywrightSpecs(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(dir string)
		want    bool
	}{
		{
			name: "空目录无结果文件",
			prepare: func(dir string) {},
			want:    false,
		},
		{
			name: "只有测试 agent 生成的 results/module-1.json",
			prepare: func(dir string) {
				resultsDir := filepath.Join(dir, "results")
				if err := os.MkdirAll(resultsDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(resultsDir, "module-1.json"), []byte(`{"passed":true}`), 0644); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name: "tests/e2e 下存在 login.spec.ts",
			prepare: func(dir string) {
				e2eDir := filepath.Join(dir, "e2e")
				if err := os.MkdirAll(e2eDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(e2eDir, "login.spec.ts"), []byte("test"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.prepare(dir)
			if got := hasPlaywrightSpecs(dir); got != tc.want {
				t.Fatalf("hasPlaywrightSpecs(%q) = %v, want %v", dir, got, tc.want)
			}
		})
	}
}
