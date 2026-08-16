package executor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugifyScenarioName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"登录成功", "登录成功"},
		{"注册流程 e2e", "注册流程-e2e"},
		{"a/b\\c:d", "a-b-c-d"},
		{"  首尾空格  ", "首尾空格"},
		{"", "scenario"},
		{"---", "scenario"},
		{strings.Repeat("长", 50), strings.Repeat("长", 40)},
	}
	for _, c := range cases {
		if got := slugifyScenarioName(c.in); got != c.want {
			t.Errorf("slugifyScenarioName(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestResolveScreenshotPath(t *testing.T) {
	workDir := t.TempDir()
	dir := ScreenshotDir(workDir, 84)
	if !strings.HasSuffix(dir, filepath.Join("screenshots", "module-84")) {
		t.Fatalf("ScreenshotDir 后缀不正确: %s", dir)
	}

	t.Run("合法png", func(t *testing.T) {
		p, err := ResolveScreenshotPath(workDir, 84, "登录成功.png")
		if err != nil || !strings.HasPrefix(p, filepath.Clean(dir)+string(filepath.Separator)) {
			t.Fatalf("期望目录内路径，得到 %q, err=%v", p, err)
		}
	})

	t.Run("路径穿越拒绝", func(t *testing.T) {
		for _, bad := range []string{"../x.png", "..\\x.png", "a/b.png", "..%2e.png", "x..png"} {
			if _, err := ResolveScreenshotPath(workDir, 84, bad); err == nil {
				t.Errorf("应拒绝 %q", bad)
			}
		}
	})

	t.Run("非图片后缀拒绝", func(t *testing.T) {
		for _, bad := range []string{"x.html", "x", ".png", "x.PNG.exe"} {
			if _, err := ResolveScreenshotPath(workDir, 84, bad); err == nil {
				t.Errorf("应拒绝 %q", bad)
			}
		}
	})

	t.Run("大写PNG放行", func(t *testing.T) {
		if _, err := ResolveScreenshotPath(workDir, 84, "a.PNG"); err != nil {
			t.Errorf("大写 .PNG 应放行: %v", err)
		}
	})
}
