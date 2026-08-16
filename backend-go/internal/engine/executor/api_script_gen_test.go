package executor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sampleAPISpec = `{"testScenarios":[
  {"name":"健康检查","steps":[{"action":"GET 健康检查","command":"curl -s -o /dev/null -w %{http_code} $BASE_URL/api/health","expected":"200"}]},
  {"name":"登录成功","steps":[{"action":"POST 登录","command":"curl -s -X POST $BASE_URL/api/login -H \"Content-Type: application/json\" -d '{\"username\":\"admin\"}'","expected":"\"code\":0"}]},
  {"name":"登录失败","steps":[{"action":"POST 错误密码","command":"curl -s -X POST $BASE_URL/api/login -H \"Content-Type: application/json\" -d '{\"username\":\"admin\",\"password\":\"bad\"}'","expected":"\"code\":1"}]}
]}`

// startMockAPI 启动返回固定 JSON 的 mock 服务。
func startMockAPI() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"status":"ok"}`))
	})
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		req := string(body[:n])
		if strings.Contains(req, `"password":"bad"`) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":1,"msg":"密码错误"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"token":"abc"}`))
	})
	// /api/echo 原样回显请求体，用于校验命令内联变量的运行时展开
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"received":` + string(body[:n]) + `}`))
	})
	return httptest.NewServer(mux)
}

// writeTempScript 把生成脚本写入临时目录并返回路径。
func writeTempScript(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-api.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("写脚本失败: %v", err)
	}
	return path
}

// TestGenerateAPITestScript_BaseURLFallback 未注入 BASE_URL 时，脚本回退读取
// 部署固定存储 tests/.base_url（位于 $(dirname $0)/../.base_url）自解析目标地址。
func TestGenerateAPITestScript_BaseURLFallback(t *testing.T) {
	ts := startMockAPI()
	defer ts.Close()

	script, err := GenerateAPITestScript(sampleAPISpec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}

	// 按真实布局写入：脚本在 tests/scripts/，访问地址固定存储在 tests/.base_url
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "tests", "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("创建 scripts 目录失败: %v", err)
	}
	path := filepath.Join(scriptsDir, "test-api.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("写脚本失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", ".base_url"), []byte(ts.URL+"\n"), 0o644); err != nil {
		t.Fatalf("写 .base_url 失败: %v", err)
	}

	// 不注入 BASE_URL，仅靠固定存储
	cmd := exec.Command("bash", path)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("脚本应回退 .base_url 并全部通过（exit=0）: %v\nstderr: %s", err, stderr.String())
	}
	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("未从 stdout 解析到结果 JSON\nstdout: %s", stdout.String())
	}
	if !result.Passed {
		t.Fatalf("预期 passed=true（回退固定存储地址），实际 false\nsummary=%s", result.Summary)
	}
}

// TestGenerateAPITestScript_NoBaseURL 无 BASE_URL 且无固定存储时脚本给出明确报错。
func TestGenerateAPITestScript_NoBaseURL(t *testing.T) {
	script, err := GenerateAPITestScript(sampleAPISpec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test-api.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("写脚本失败: %v", err)
	}

	cmd := exec.Command("bash", path)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatalf("预期无 BASE_URL 时脚本失败，实际 exit=0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("预期 exit code=1，实际: %v", err)
	}
	if !strings.Contains(stderr.String(), "未设置 BASE_URL") {
		t.Fatalf("预期报错含「未设置 BASE_URL」，实际: %s", stderr.String())
	}
}

// TestGenerateAPITestScript_InlineVars 命令内联变量（先赋值再用）在 eval 时正确展开，
// 不被外层参数构造的双引号提前展开成空串（历史 bug：U="$(date...)" 后 -d "$U" 的 $U 为空）。
func TestGenerateAPITestScript_InlineVars(t *testing.T) {
	ts := startMockAPI()
	defer ts.Close()

	spec := `{"testScenarios":[
	  {"name":"内联变量","steps":[
	    {"action":"变量赋值后用于 body","command":"U=\"unique$(date +%s%N)\"; curl -s -X POST $BASE_URL/api/echo -H 'Content-Type: application/json' -d \"{\\\"u\\\":\\\"$U\\\"}\"","expected":"\"u\":\"unique"},
	    {"action":"单引号 header 保留","command":"curl -s -X POST $BASE_URL/api/login -H 'Content-Type: application/json' -d '{\"username\":\"admin\",\"password\":\"bad\"}'","expected":"\"code\":1"}
	  ]}
	]}`
	script, err := GenerateAPITestScript(spec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)

	cmd := exec.Command("bash", path)
	cmd.Env = append(os.Environ(), "BASE_URL="+ts.URL)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("脚本执行失败（预期全部通过，exit=0）: %v\nstderr: %s", err, stderr.String())
	}
	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("未解析到结果 JSON\nstdout: %s", stdout.String())
	}
	if !result.Passed {
		t.Fatalf("预期 passed=true，实际 false\nsummary=%s\nstderr: %s", result.Summary, stderr.String())
	}
}

// TestGenerateAPITestScript_BashSyntax 生成脚本并校验 bash -n 语法合法。
func TestGenerateAPITestScript_BashSyntax(t *testing.T) {
	script, err := GenerateAPITestScript(sampleAPISpec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)
	out, err := exec.Command("bash", "-n", path).CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n 语法校验失败: %v\n%s", err, out)
	}
}

// TestGenerateAPITestScript_Run 生成脚本并用 mock 服务执行，校验结果 JSON 解析。
func TestGenerateAPITestScript_Run(t *testing.T) {
	ts := startMockAPI()
	defer ts.Close()

	script, err := GenerateAPITestScript(sampleAPISpec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)

	cmd := exec.Command("bash", path)
	cmd.Env = append(os.Environ(), "BASE_URL="+ts.URL)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("脚本执行失败（预期全部通过，exit=0）: %v\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("未从 stdout 解析到结果 JSON\nstdout: %s", stdout.String())
	}
	if !result.Passed {
		t.Fatalf("预期 passed=true，实际 false\nsummary=%s", result.Summary)
	}
	if len(result.Scenarios) != 3 {
		t.Fatalf("预期 3 个场景，实际 %d", len(result.Scenarios))
	}
	for _, s := range result.Scenarios {
		if !s.Passed {
			t.Fatalf("场景 %s 预期通过，实际失败: %s", s.Name, s.Log)
		}
	}
}

// TestGenerateAPITestScript_SingleScenario 单场景模式只跑指定索引。
func TestGenerateAPITestScript_SingleScenario(t *testing.T) {
	ts := startMockAPI()
	defer ts.Close()

	script, err := GenerateAPITestScript(sampleAPISpec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)

	cmd := exec.Command("bash", path)
	cmd.Env = append(os.Environ(), "BASE_URL="+ts.URL, "SCENARIO_INDEX=1")
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("单场景执行失败: %v", err)
	}
	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("未解析到结果 JSON\nstdout: %s", stdout.String())
	}
	if len(result.Scenarios) != 1 || result.Scenarios[0].Name != "登录成功" {
		t.Fatalf("单场景模式应只返回 1 个「登录成功」，实际: %+v", result.Scenarios)
	}
}

// TestGenerateAPITestScript_Failure 场景失败时 exit code = 失败数，passed=false。
func TestGenerateAPITestScript_Failure(t *testing.T) {
	ts := startMockAPI()
	defer ts.Close()

	// 期望「登录失败」场景要求 code=999（mock 永远返回 code=1）→ 必然失败
	spec := `{"testScenarios":[
  {"name":"必然失败","steps":[{"action":"POST","command":"curl -s -X POST $BASE_URL/api/login -H \"Content-Type: application/json\" -d '{\"username\":\"admin\"}'","expected":"\"code\":999"}]},
  {"name":"必然通过","steps":[{"action":"GET","command":"curl -s $BASE_URL/api/health","expected":"\"code\":0"}]}
]}`
	script, err := GenerateAPITestScript(spec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)

	cmd := exec.Command("bash", path)
	cmd.Env = append(os.Environ(), "BASE_URL="+ts.URL)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatalf("预期失败场景使 exit 非 0，实际 0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("预期 exit code = 1（失败场景数），实际: %v\nstderr: %s", err, stderr.String())
	}

	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("未解析到结果 JSON\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
	if result.Passed {
		t.Fatalf("预期 passed=false，实际 true")
	}
	if len(result.Failures) != 1 {
		t.Fatalf("预期 1 条失败，实际 %d: %+v", len(result.Failures), result.Failures)
	}
}

// TestGenerateAPITestScript_StepsDetail 每个场景的 steps 数组应包含可复现的
// action/command/ok/output/error 明细，供前端「步骤明细」面板展示（输入与输出）。
func TestGenerateAPITestScript_StepsDetail(t *testing.T) {
	ts := startMockAPI()
	defer ts.Close()

	// 一个通过场景 + 一个失败场景（expected=code 999 必然不匹配）
	spec := `{"testScenarios":[
	  {"name":"混合结果","steps":[
	    {"action":"GET 健康检查","command":"curl -s $BASE_URL/api/health","expected":"\"code\":0"},
	    {"action":"POST 错误密码","command":"curl -s -X POST $BASE_URL/api/login -H \"Content-Type: application/json\" -d '{\"username\":\"admin\",\"password\":\"bad\"}'","expected":"\"code\":999"}
	  ]}
	]}`
	script, err := GenerateAPITestScript(spec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)

	cmd := exec.Command("bash", path)
	cmd.Env = append(os.Environ(), "BASE_URL="+ts.URL)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("预期含失败步骤时 exit 非 0，实际 0")
	}

	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("未解析到结果 JSON\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
	if len(result.Scenarios) != 1 {
		t.Fatalf("预期 1 个场景，实际 %d", len(result.Scenarios))
	}
	sc := result.Scenarios[0]
	if len(sc.Steps) != 2 {
		t.Fatalf("预期 2 条步骤明细，实际 %d: %+v", len(sc.Steps), sc.Steps)
	}

	// 第 1 步应通过，且 action/command/output 完整可复现
	s1 := sc.Steps[0]
	if !s1.OK {
		t.Fatalf("步骤 1 预期通过，实际失败: %+v", s1)
	}
	if s1.Action != "GET 健康检查" {
		t.Fatalf("步骤 1 action 不符: %q", s1.Action)
	}
	// command 应为实际执行的命令：$BASE_URL 展开为真实 mock 地址，不得保留占位符
	if strings.Contains(s1.Command, "$BASE_URL") {
		t.Fatalf("步骤 1 command 不应保留 $BASE_URL 占位符，应为实际值: %q", s1.Command)
	}
	if !strings.Contains(s1.Command, ts.URL) {
		t.Fatalf("步骤 1 command 应包含实际服务地址 %s，实际: %q", ts.URL, s1.Command)
	}
	if !strings.Contains(s1.Output, `"code":0`) {
		t.Fatalf("步骤 1 output 应包含实际响应 code:0，实际: %q", s1.Output)
	}

	// 第 2 步应失败，error 说明未匹配 expected
	s2 := sc.Steps[1]
	if s2.OK {
		t.Fatalf("步骤 2 预期失败，实际通过: %+v", s2)
	}
	if s2.Action != "POST 错误密码" {
		t.Fatalf("步骤 2 action 不符: %q", s2.Action)
	}
	if !strings.Contains(s2.Error, "未匹配 expected") {
		t.Fatalf("步骤 2 error 应说明未匹配 expected，实际: %q", s2.Error)
	}
	if !strings.Contains(s2.Output, `"code":1`) {
		t.Fatalf("步骤 2 output 应包含实际响应 code:1，实际: %q", s2.Output)
	}
}

// TestGenerateAPITestScript_StepsInlineVars 内联变量（U=reg$(date ...) 后用于 body）
// 在步骤明细的 command 中应显示展开后的真实值，而不是占位符 $U/$(date ...)。
func TestGenerateAPITestScript_StepsInlineVars(t *testing.T) {
	ts := startMockAPI()
	defer ts.Close()

	spec := `{"testScenarios":[
	  {"name":"注册唯一用户","steps":[
	    {"action":"POST 注册","command":"U=\"reg$(date +%s%N)\"; curl -s -X POST $BASE_URL/api/login -H \"Content-Type: application/json\" -d \"{\\\"username\\\":\\\"$U\\\"}\"","expected":"\"code\":0"}
	  ]}
	]}`
	script, err := GenerateAPITestScript(spec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)

	cmd := exec.Command("bash", path)
	cmd.Env = append(os.Environ(), "BASE_URL="+ts.URL)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("脚本执行失败: %v", err)
	}
	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("未解析到结果 JSON\nstdout: %s", stdout.String())
	}
	if len(result.Scenarios) != 1 || len(result.Scenarios[0].Steps) != 1 {
		t.Fatalf("预期 1 场景 1 步骤，实际: %+v", result.Scenarios)
	}
	step := result.Scenarios[0].Steps[0]
	// 不得保留 $U / $(date 占位符；应能看到展开后的唯一用户名与真实地址
	if strings.Contains(step.Command, "$U") || strings.Contains(step.Command, "$(date") {
		t.Fatalf("command 不应保留占位符 $U/$(date)，应为实际值: %q", step.Command)
	}
	if !strings.Contains(step.Command, ts.URL) {
		t.Fatalf("command 应包含实际服务地址 %s，实际: %q", ts.URL, step.Command)
	}
	if !strings.Contains(step.Command, "username") {
		t.Fatalf("command 应包含展开后的 body，实际: %q", step.Command)
	}
	if !step.OK {
		t.Fatalf("步骤预期通过，实际失败: %+v", step)
	}
}

// TestGenerateAPITestScript_StepsDetailJSON 步骤 output 含换行/引号时仍应输出单行合法 JSON。
func TestGenerateAPITestScript_StepsDetailJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("line1\nline2 \"quoted\" \\ backslash"))
	}))
	defer ts.Close()

	spec := `{"testScenarios":[
	  {"name":"多行输出","steps":[
	    {"action":"GET 文本","command":"curl -s $BASE_URL/","expected":"line2"}
	  ]}
	]}`
	script, err := GenerateAPITestScript(spec)
	if err != nil {
		t.Fatalf("GenerateAPITestScript 失败: %v", err)
	}
	path := writeTempScript(t, script)

	cmd := exec.Command("bash", path)
	cmd.Env = append(os.Environ(), "BASE_URL="+ts.URL)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("脚本执行失败: %v\nstderr: %s", err, stderr.String())
	}

	result, ok := parseResultJSONFromStdout(stdout.String())
	if !ok {
		t.Fatalf("输出含换行/引号/反斜杠时仍应解析到结果 JSON\nstdout: %s", stdout.String())
	}
	if len(result.Scenarios) != 1 || len(result.Scenarios[0].Steps) != 1 {
		t.Fatalf("预期 1 场景 1 步骤，实际: %+v", result.Scenarios)
	}
	s := result.Scenarios[0].Steps[0]
	if !strings.Contains(s.Output, "line1") || !strings.Contains(s.Output, "line2") {
		t.Fatalf("output 应保留原始换行内容，实际: %q", s.Output)
	}
	if !s.OK {
		t.Fatalf("步骤预期通过（expected=line2 应匹配），实际: %+v", s)
	}
}
