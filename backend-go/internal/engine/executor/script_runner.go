package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ScriptResult 脚本执行结果。
type ScriptResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// baseURLFilePath 返回部署时写入的访问地址固定存储文件路径（相对 workdir）。
// 生成的 API 测试脚本（tests/scripts/ 目录）以 $(dirname $0)/../.base_url 定位该文件。
func baseURLFilePath(workDir string) string {
	return filepath.Join(workDir, "tests", ".base_url")
}

// writeBaseURLFile 把访问地址写入固定存储 tests/.base_url（部署成功后由 DeployService 调用）。
// accessURL 为空时不做任何事。
func writeBaseURLFile(workDir, accessURL string) error {
	if strings.TrimSpace(accessURL) == "" {
		return nil
	}
	dir := filepath.Join(workDir, "tests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".base_url"), []byte(strings.TrimSpace(accessURL)+"\n"), 0o644)
}

// readBaseURLFile 读取部署时写入的访问地址；文件缺失或内容为空返回 error。
func readBaseURLFile(workDir string) (string, error) {
	data, err := os.ReadFile(baseURLFilePath(workDir))
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(string(data))
	if url == "" {
		return "", fmt.Errorf("访问地址固定存储为空: %s", baseURLFilePath(workDir))
	}
	return url, nil
}

// RunScript 执行 bash 脚本，返回退出码、stdout、stderr 与耗时。
// env 为额外注入的环境变量（如 BASE_URL），合并进当前进程环境。
func RunScript(workDir, scriptPath string, env map[string]string, timeout time.Duration) (*ScriptResult, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("测试脚本不存在: %v", err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = mergeEnv(env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := runWithTimeout(cmd, timeout)
	duration := time.Since(start)

	rc := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			rc = exitErr.ExitCode()
		} else {
			return &ScriptResult{ExitCode: -1, Stdout: stdout.String(), Stderr: stderr.String(), Duration: duration}, fmt.Errorf("执行测试脚本失败: %w", err)
		}
	}
	return &ScriptResult{ExitCode: rc, Stdout: stdout.String(), Stderr: stderr.String(), Duration: duration}, nil
}

// RunPlaywright 执行 Playwright 测试 spec。
// baseURL 通过 BASE_URL 环境变量注入（spec 中 test.use({ baseURL: process.env.BASE_URL })）。
// grepPattern 非空时用 -g 过滤单个 test 标题（单场景运行）。
// 使用 --reporter=json 让 Playwright 输出结构化 JSON 到 stdout。
// 必须在 workDir/tests 目录内执行：@playwright/test 安装在 tests/node_modules
//（通常 symlink 到 frontend/node_modules），spec 也 import 本地 @playwright/test。
// 项目根往往没有 node_modules，若在根目录跑 `npx playwright`，npx 会解析到全局缓存里
// 独立的 playwright/@playwright/test 实例，与 spec 用到的本地实例不一致，收集期
// test.use() 报「did not expect test.use() to be called here」，整个 spec 无法收集。
// 在 tests/ 内执行则 npx 命中本地二进制，runner 与 spec 同实例。spec 路径相对 tests/ 换算。
func RunPlaywright(workDir, specFile, baseURL, grepPattern string, timeout time.Duration) (*ScriptResult, error) {
	args := []string{"playwright", "test"}
	if grepPattern != "" {
		args = append(args, "-g", grepPattern)
	}
	args = append(args, "--reporter=json")

	runDir := workDir
	relSpec := specFile
	testsDir := filepath.Join(workDir, "tests")
	if rel, err := filepath.Rel(testsDir, specFile); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		runDir = testsDir
		relSpec = rel
	}
	args = append(args, relSpec)

	cmd := exec.Command("npx", args...)
	cmd.Dir = runDir
	env := map[string]string{"BASE_URL": baseURL}
	cmd.Env = mergeEnv(env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := runWithTimeout(cmd, timeout)
	duration := time.Since(start)

	rc := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			rc = exitErr.ExitCode()
		} else {
			return &ScriptResult{ExitCode: -1, Stdout: stdout.String(), Stderr: stderr.String(), Duration: duration}, fmt.Errorf("执行 Playwright 失败: %w", err)
		}
	}
	return &ScriptResult{ExitCode: rc, Stdout: stdout.String(), Stderr: stderr.String(), Duration: duration}, nil
}

// runWithTimeout 带超时执行命令，超时则杀掉进程树并返回错误。
func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		// 杀掉整个进程组，避免 npx/playwright 派生的子进程残留
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("执行超时（>%s）", timeout)
	}
}

// mergeEnv 合并当前进程环境与注入变量，并把已知工具目录补充进 PATH。
// 注：loafer 服务进程 PATH 可能不含 /usr/local/bin（npx/node/go），
// 脚本执行依赖这些工具，需显式补全，否则 exit 127。
func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	if len(extra) == 0 {
		return augmentPath(env)
	}
	// 覆盖同名变量：先构建 map，剔除将被覆盖的项，再追加。
	seen := make(map[string]bool, len(extra))
	for k := range extra {
		seen[k] = true
	}
	out := make([]string, 0, len(env)+len(extra))
	for _, e := range env {
		key := e
		if idx := strings.IndexByte(e, '='); idx > 0 {
			key = e[:idx]
		}
		if seen[key] {
			continue
		}
		out = append(out, e)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return augmentPath(out)
}

// augmentPath 把 /usr/local/bin、/usr/local/go/bin 等工具目录补进 PATH（存在才补）。
func augmentPath(env []string) []string {
	extraDirs := []string{"/usr/local/bin", "/usr/local/go/bin"}
	pathVal := ""
	var out []string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			pathVal = strings.TrimPrefix(e, "PATH=")
			continue
		}
		out = append(out, e)
	}
	pathSet := make(map[string]bool)
	for _, d := range strings.Split(pathVal, ":") {
		if d != "" {
			pathSet[d] = true
		}
	}
	var add []string
	for _, d := range extraDirs {
		if pathSet[d] {
			continue
		}
		if _, err := os.Stat(d); err == nil {
			add = append(add, d)
			pathSet[d] = true
		}
	}
	if len(add) > 0 {
		pathVal = strings.Join(add, ":") + ":" + pathVal
	}
	out = append(out, "PATH="+pathVal)
	return out
}

// ParseModuleTestResultFromOutput 从脚本 stdout 解析 ModuleTestResult。
// 兼容策略：stdout 尾部为结构化 JSON 时直接解析；否则从退出码与摘要文本构造。
func ParseModuleTestResultFromOutput(stdout string, exitCode int) *ModuleTestResult {
	if r, ok := parseResultJSONFromStdout(stdout); ok {
		return r
	}
	// 降级：无法解析 JSON 时，从退出码与输出尾部构造
	passed := exitCode == 0
	log := strings.TrimSpace(tailString(stdout, 3000))
	return &ModuleTestResult{
		Passed:   passed,
		Summary:  fmt.Sprintf("API 测试退出码 %d，共 %d 条输出行", exitCode, strings.Count(stdout, "\n")),
		Failures: []ModuleTestFailure{{Kind: "api", Name: "api-script", Log: log}},
	}
}

// parseResultJSONFromStdout 从 stdout 中提取并解析结果 JSON。
// 脚本把 JSON 打印到 stdout 末尾；stdout 可能混有杂音，先取最后一行 JSON。
func parseResultJSONFromStdout(stdout string) (*ModuleTestResult, bool) {
	lines := strings.Split(stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r ModuleTestResult
		if err := json.Unmarshal([]byte(line), &r); err == nil && (r.Summary != "" || len(r.Scenarios) > 0) {
			return &r, true
		}
	}
	return nil, false
}

// pwSuite Playwright JSON reporter 中 suite 的结构。
// suite 可嵌套（test.describe() 产生的层级），specs 可能挂在任意层。
type pwSuite struct {
	Specs []struct {
		Title string `json:"title"`
		Tests []struct {
			Results []pwTestResult `json:"results"`
		} `json:"tests"`
	} `json:"specs"`
	Suites []pwSuite `json:"suites"`
}

// ParsePlaywrightResultFromOutput 从 Playwright --reporter=json 输出解析结果。
// 返回所有 test 的通过/失败明细，映射为 []ScenarioResult。
// 递归遍历嵌套 suite：spec 用 test.describe() 包裹时，specs 挂在第二层及更深层，
// 只读顶层会漏掉全部场景，导致上层误判为「Playwright 无测试结果输出」。
func ParsePlaywrightResultFromOutput(stdout string, exitCode int) []ScenarioResult {
	var doc struct {
		Suites []pwSuite `json:"suites"`
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&doc); err != nil {
		return nil
	}

	var results []ScenarioResult
	var walk func(suites []pwSuite)
	walk = func(suites []pwSuite) {
		for _, suite := range suites {
			for _, spec := range suite.Specs {
				for _, test := range spec.Tests {
					if len(test.Results) == 0 {
						continue
					}
					last := test.Results[len(test.Results)-1]
					passed := last.Status == "passed" || last.Status == "skipped"
					r := ScenarioResult{
						Kind:   "e2e",
						Name:   spec.Title,
						Passed: passed,
						Log:    summarizePWError(last),
					}
					results = append(results, r)
				}
			}
			walk(suite.Suites)
		}
	}
	walk(doc.Suites)
	if len(results) == 0 {
		return nil
	}
	return results
}

// pwTestResult Playwright JSON reporter 中单个 test result 的结构。
type pwTestResult struct {
	Status   string   `json:"status"`
	Error    pwError  `json:"error"`
	Stdout   []string `json:"stdout"`
	Stderr   []string `json:"stderr"`
	Duration int64    `json:"duration"`
}

// pwError Playwright JSON reporter 的错误结构。
type pwError struct {
	Message string `json:"message"`
}

// summarizePWError 从 Playwright 结果提取错误摘要。
// 若错误来自截图/afterEach 钩子（如 page.screenshot 抛错、浏览器已关闭），
// 加前缀标注「非断言失败」：这类错误通常是 spec 漏包 try/catch 导致截图动作
// 把断言全过的测试拖成 failed，需与真正的断言失败区分开。
func summarizePWError(res pwTestResult) string {
	msg := ""
	if res.Error.Message != "" {
		msg = strings.TrimSpace(res.Error.Message)
	} else {
		msg = strings.TrimSpace(strings.Join(res.Stderr, "\n"))
	}
	if msg == "" {
		return ""
	}
	if isPWHookOrScreenshotError(msg) {
		return "【截图/钩子异常导致失败，非断言失败】" + tailString(msg, 450)
	}
	return tailString(msg, 500)
}

// pwHookOrScreenshotMarkers 用于识别「截图/afterEach 钩子」类失败的特征短语。
// 命中即说明失败源是截图动作或页面已关闭等钩子期异常，而不是 expect 断言失败。
var pwHookOrScreenshotMarkers = []string{
	"page.screenshot",
	"aftereach",
	"after each",
	"target page, context or browser has been closed",
	"page crashed",
	"cannot take screenshot",
	"browser has been closed",
}

// isPWHookOrScreenshotError 判断错误消息是否属于截图/afterEach 钩子类失败。
func isPWHookOrScreenshotError(msg string) bool {
	m := strings.ToLower(strings.TrimSpace(msg))
	if m == "" {
		return false
	}
	for _, mk := range pwHookOrScreenshotMarkers {
		if strings.Contains(m, mk) {
			return true
		}
	}
	return false
}

// summarizePWScriptOutput 从 Playwright 执行结果中提取最能说明问题的输出片段。
// 优先取 --reporter=json 输出里 errors[].message（收集期错误，如 spec 加载失败、
// test.use 位置错误等），其次取 stdout/stderr 尾部。完全无输出时给出退出码提示。
// 用于替代「Playwright 无测试结果输出」这类掩盖真实原因的泛化文案。
func summarizePWScriptOutput(sr *ScriptResult) string {
	if sr == nil {
		return "Playwright 无输出"
	}
	if strings.TrimSpace(sr.Stdout) != "" {
		var doc struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal([]byte(sr.Stdout), &doc); err == nil && len(doc.Errors) > 0 {
			return tailString(strings.TrimSpace(doc.Errors[0].Message), 2000)
		}
	}
	combined := strings.TrimSpace(strings.TrimSpace(sr.Stdout) + "\n" + strings.TrimSpace(sr.Stderr))
	if combined != "" {
		return tailString(combined, 2000)
	}
	return fmt.Sprintf("Playwright 无测试结果输出（退出码 %d）", sr.ExitCode)
}

// e2eSpecFilePath 返回模块 Playwright spec 的绝对路径。
func e2eSpecFilePath(workDir string, moduleID int64) string {
	return fmt.Sprintf("%s/tests/e2e/module-%d.spec.ts", workDir, moduleID)
}
