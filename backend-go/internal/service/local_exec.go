package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BuildEnvPrefix 返回注入到构建命令前的 PATH 补充前缀。
// loafer 服务进程可能以最小 PATH（如 /usr/bin:/bin）运行，而 go 常安装在
// /usr/local/go/bin；当 LookPath 找不到工具时，按候选目录探测实际安装位置并补充。
// 找不到任何候选目录时返回空字符串（命令原样执行，报错信息照旧）。
func BuildEnvPrefix() string {
	candidates := []struct {
		tool string
		dirs []string
	}{
		{"go", []string{"/usr/local/go/bin", "/usr/lib/go/bin", "/usr/lib/golang/bin"}},
		{"npm", []string{"/usr/local/bin"}},
	}
	var extra []string
	for _, c := range candidates {
		if _, err := exec.LookPath(c.tool); err == nil {
			continue
		}
		for _, d := range c.dirs {
			if localFileExists(filepath.Join(d, c.tool)) {
				extra = append(extra, d)
				break
			}
		}
	}
	if len(extra) == 0 {
		return ""
	}
	return fmt.Sprintf("export PATH=\"$PATH:%s\"; ", strings.Join(extra, ":"))
}

// RunLocalCommand 在本地执行 shell 命令，返回 stdout+stderr 输出和错误。
func RunLocalCommand(command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// DetectFrontendDir 返回前端目录：workDir 根目录含 package.json 则返回根目录，
// 否则返回第一个含 package.json 的一级子目录（跳过隐藏目录与 node_modules）。
func DetectFrontendDir(workDir string) (string, error) {
	return detectSubDir(workDir, "package.json")
}

// DetectBackendDir 返回后端目录：workDir 根目录含 go.mod 则返回根目录，
// 否则返回第一个含 go.mod 的一级子目录（跳过隐藏目录与 node_modules）。
func DetectBackendDir(workDir string) (string, error) {
	return detectSubDir(workDir, "go.mod")
}

// detectSubDir 在 workDir 根目录或一级子目录中查找包含指定清单文件的目录。
func detectSubDir(workDir, manifest string) (string, error) {
	if localFileExists(filepath.Join(workDir, manifest)) {
		return workDir, nil
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return "", fmt.Errorf("读取工作目录 %s 失败: %w", workDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
			continue
		}
		sub := filepath.Join(workDir, e.Name())
		if localFileExists(filepath.Join(sub, manifest)) {
			return sub, nil
		}
	}
	return "", fmt.Errorf("未找到 %s（已检查 %s 及其一级子目录）", manifest, workDir)
}

// DetectGoMainPackage 探测 backendDir 内的 main 包构建目标：
//  1. 存在 ./cmd/server 目录时优先使用；
//  2. ./cmd/ 下恰有一个含 .go 文件的子目录时使用该目录；
//  3. 兜底通过 go list 查找 main 包，取第一个结果。
func DetectGoMainPackage(backendDir string) (string, error) {
	cmdDir := filepath.Join(backendDir, "cmd")
	if info, err := os.Stat(filepath.Join(cmdDir, "server")); err == nil && info.IsDir() {
		return "./cmd/server", nil
	}
	if entries, err := os.ReadDir(cmdDir); err == nil {
		var mains []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if dirHasGoFiles(filepath.Join(cmdDir, e.Name())) {
				mains = append(mains, "./cmd/"+e.Name())
			}
		}
		if len(mains) == 1 {
			return mains[0], nil
		}
	}
	out, err := RunLocalCommand(BuildEnvPrefix()+fmt.Sprintf(
		"cd %s && go list -f '{{if eq .Name \"main\"}}{{.ImportPath}}{{end}}' ./... 2>/dev/null",
		backendDir), time.Minute)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if p := strings.TrimSpace(line); p != "" {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("未找到 Go main 包（已检查 ./cmd/server、./cmd/* 及 go list）")
}

// dirHasGoFiles 判断目录下是否直接包含 .go 文件。
func dirHasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// localFileExists 判断本地路径是否为存在的文件。
func localFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
