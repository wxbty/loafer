package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// slugifyScenarioName 把场景名转为安全的文件名片段：保留中英文字母与数字，
// 其余字符一律转为连字符；去首尾连字符；最长 40 rune；结果为空时返回 "scenario"。
// 测试 agent 与本后端使用同一套规则，保证截图文件名可互相推导。
func slugifyScenarioName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "scenario"
	}
	runes := []rune(slug)
	if len(runes) > 40 {
		slug = string(runes[:40])
	}
	return slug
}

// ScreenshotDir 返回模块截图目录：<workDir>/tests/results/screenshots/module-<id>。
func ScreenshotDir(workDir string, moduleID int64) string {
	return filepath.Join(workDir, "tests", "results", "screenshots", fmt.Sprintf("module-%d", moduleID))
}

// ResolveScreenshotPath 把请求文件名解析为模块截图目录内的安全绝对路径。
// 拒绝：空名、含路径分隔符、含 ".."、非图片后缀、无主体的后缀名（如 ".png"）。
func ResolveScreenshotPath(workDir string, moduleID int64, file string) (string, error) {
	if file == "" || strings.ContainsAny(file, `/\`) || strings.Contains(file, "..") {
		return "", fmt.Errorf("非法截图文件名: %q", file)
	}
	ext := strings.ToLower(filepath.Ext(file))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return "", fmt.Errorf("截图仅支持 png/jpg: %q", file)
	}
	if file == ext { // ".png" 这类无主体文件名
		return "", fmt.Errorf("非法截图文件名: %q", file)
	}
	dir := ScreenshotDir(workDir, moduleID)
	full := filepath.Join(dir, file)
	cleanDir := filepath.Clean(dir)
	if full != cleanDir && !strings.HasPrefix(full, cleanDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("截图路径越界: %q", file)
	}
	return full, nil
}
