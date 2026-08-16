package handler

import (
	"strings"
	"testing"

	"loafer-agent/internal/engine/executor"
)

// TestScreenshotRouteContract 锁定路由与路径解析的契约：handler 委托
// executor.ResolveScreenshotPath，穿越/非图片一律 404（由 ResolveScreenshotPath 报错决定）。
func TestScreenshotRouteContract(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executor.ResolveScreenshotPath(workDir, 1, "../../etc/passwd"); err == nil {
		t.Fatalf("路径穿越必须报错")
	}
	p, err := executor.ResolveScreenshotPath(workDir, 1, "登录成功.png")
	if err != nil || !strings.Contains(p, "module-1") {
		t.Fatalf("合法截图路径解析失败: %v", err)
	}
}
