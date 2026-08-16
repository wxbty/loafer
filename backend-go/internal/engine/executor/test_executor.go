package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// MaxTestRounds 业务模块「测试→修复」自动循环的最大轮数。
const MaxTestRounds = 3

// ModuleTestFailure 单条测试失败记录（结果 JSON 契约中 failures 的元素）。
type ModuleTestFailure struct {
	Kind string `json:"kind"` // integration / e2e / build / agent
	Name string `json:"name"`
	Log  string `json:"log"`
}

// ScenarioStepResult 场景内单步骤的执行结果（单场景手动运行时回写 lastSteps 用）。
type ScenarioStepResult struct {
	Action  string `json:"action"`
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Output  string `json:"output"`
	Error   string `json:"error"`
}

// ScenarioResult 单个测试场景的执行结果（结果 JSON 契约中 scenarios 的元素）。
// Name 与模块用例 JSON（api_integration_test / web_integration_test）中场景的 name 精确对应。
type ScenarioResult struct {
	Kind            string               `json:"kind"` // api / e2e
	Name            string               `json:"name"` // 场景名，回写用例时按此精确匹配
	Passed          bool                 `json:"passed"`
	Log             string               `json:"log"`             // 关键日志摘要
	Screenshot      string               `json:"screenshot"`      // 终态截图（相对 workDir 路径），仅 e2e 场景有
	ErrorScreenshot string               `json:"errorScreenshot"` // 失败时 Playwright 出错截图，可为空
	Steps           []ScenarioStepResult `json:"steps"`           // 步骤明细，仅单场景手动运行产出
}

// ModuleTestResult 测试 agent 写入 tests/results/module-<id>.json 的结构化结果。
type ModuleTestResult struct {
	ModuleID  int64               `json:"module_id"`
	Passed    bool                `json:"passed"`
	Summary   string              `json:"summary"`
	Failures  []ModuleTestFailure `json:"failures"`
	Scenarios []ScenarioResult    `json:"scenarios"`
}

// resultFilePath 返回模块测试结果 JSON 的绝对路径。
func resultFilePath(workDir string, moduleID int64) string {
	return filepath.Join(workDir, "tests", "results", fmt.Sprintf("module-%d.json", moduleID))
}

// parseModuleTestResult 解析结果 JSON 内容；空内容、畸形 JSON、全零值结果均视为无效。
func parseModuleTestResult(data []byte) (*ModuleTestResult, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("结果文件为空")
	}
	var result ModuleTestResult
	if err := json.Unmarshal(trimmed, &result); err != nil {
		return nil, fmt.Errorf("结果文件不是合法 JSON: %w", err)
	}
	if !result.Passed && result.Summary == "" && len(result.Failures) == 0 {
		return nil, fmt.Errorf("结果文件缺少有效字段（passed/summary/failures 均为零值）")
	}
	return &result, nil
}

// readModuleTestResult 从磁盘读取并解析结果文件。
func readModuleTestResult(path string) (*ModuleTestResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取结果文件失败: %w", err)
	}
	return parseModuleTestResult(data)
}

// tailString 取字符串尾部 maxLen 个字符（按 rune 计，避免截断多字节字符）。
func tailString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[len(runes)-maxLen:])
}

// TestExecutor 模块自动测试/修复执行器，通过 Claude Code CLI（--print 模式）驱动测试 agent 与修复 agent。
type TestExecutor struct {
	db       *gorm.DB
	executor *cli.OfflineExecutor
}

// NewTestExecutor 构造模块自动测试执行器。
func NewTestExecutor(db *gorm.DB, executor *cli.OfflineExecutor) *TestExecutor {
	return &TestExecutor{db: db, executor: executor}
}

// scenarioNamesFromSpec 从用例 spec JSON 提取场景名列表；空/非法返回 nil。
func scenarioNamesFromSpec(specJSON string) []string {
	var spec struct {
		TestScenarios []struct {
			Name string `json:"name"`
		} `json:"testScenarios"`
	}
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil
	}
	var names []string
	for _, s := range spec.TestScenarios {
		if s.Name != "" {
			names = append(names, s.Name)
		}
	}
	return names
}

// buildScenarioDrivenSection 有用例时生成「按场景逐条执行」提示词段落。
// 截图文件名由后端按 slugifyScenarioName 预先推导并写死进提示词，agent 不需自行计算。
func buildScenarioDrivenSection(mod *model.Module, accessURL string) string {
	var b strings.Builder
	b.WriteString("请按以下已落库的场景逐条执行测试（不要自由发挥新场景）：\n")

	if names := scenarioNamesFromSpec(mod.APIIntegrationTest); len(names) > 0 {
		b.WriteString(fmt.Sprintf("【API 集成测试场景】共 %d 个：%s\n", len(names), strings.Join(names, "、")))
		b.WriteString("用例定义见模块的 api_integration_test（curl 命令中的 $BASE_URL 替换为上述访问地址）。逐场景执行并把每步实际输出与 expected 子串比对。\n")
	}
	if names := scenarioNamesFromSpec(mod.WebIntegrationTest); len(names) > 0 {
		b.WriteString(fmt.Sprintf("【Playwright UI 场景】共 %d 个：%s\n", len(names), strings.Join(names, "、")))
		b.WriteString(fmt.Sprintf("可执行用例在 tests/e2e/module-%d.spec.ts，用 BASE_URL=%s npx playwright test module-%d.spec.ts 运行。\n", mod.ID, accessURL, mod.ID))
		dir := fmt.Sprintf("tests/results/screenshots/module-%d", mod.ID)
		b.WriteString(fmt.Sprintf("每个 Playwright 场景跑完（无论成败）必须用 page.screenshot 截一张终态图，保存为 %s/<场景名转文件名>.png；失败的场景额外保存出错图 <场景名转文件名>-error.png。", dir))
		b.WriteString("文件名推导规则已为你算好，对照如下：\n")
		for _, n := range names {
			b.WriteString(fmt.Sprintf("  - 场景「%s」→ %s/%s.png（失败加 %s-error.png）\n", n, dir, slugifyScenarioName(n), slugifyScenarioName(n)))
		}
	}

	b.WriteString(fmt.Sprintf(`全部执行完毕后，无论通过与否，必须把结构化结果写入文件 tests/results/module-%d.json，格式严格如下（合法 JSON，不要包裹 Markdown 代码块标记）：
{"module_id": %d, "passed": true或false, "summary": "一句话总结，如：API 2/2 通过；Playwright 1/2 通过", "failures": [{"kind": "integration或e2e或build", "name": "失败测试名", "log": "关键失败日志摘要"}], "scenarios": [{"kind": "api或e2e", "name": "场景名（与上述清单完全一致）", "passed": true或false, "log": "关键日志摘要", "screenshot": "终态截图相对路径或空串", "errorScreenshot": "出错截图相对路径或空串"}]}
要求：所有场景通过时 passed 才为 true 且 failures 为空数组；任一场景失败时 passed 为 false。scenarios 数组必须覆盖上述全部场景，name 不得改动。`, mod.ID, mod.ID))
	return b.String()
}

// buildTestPrompt 构造测试 agent 的提示词。
// 模块已落库用例时按场景逐条执行并截图回传；否则退化为现场编写并运行测试。
func buildTestPrompt(project *model.Project, mod *model.Module, accessURL string, round int) string {
	if strings.TrimSpace(mod.APIIntegrationTest) != "" || strings.TrimSpace(mod.WebIntegrationTest) != "" {
		return fmt.Sprintf(`你是测试工程师 agent。项目「%s」的模块「%s」（序号 %s）刚完成开发，服务已部署，访问地址：%s。

%s

模块需求描述：%s

这是第 %d 轮测试。现在开始。`,
			project.Name, mod.Name, mod.SequenceNumber, accessURL,
			buildScenarioDrivenSection(mod, accessURL), mod.Description, round)
	}
	return fmt.Sprintf(`你是测试工程师 agent。项目「%s」的模块「%s」（序号 %s）刚完成开发，服务已部署，访问地址：%s。

请为该模块现场编写并运行测试：
1. 集成测试：优先编写/运行 Go 测试（go test ./...），并用 HTTP 请求（curl 或测试脚本）验证本模块核心 API 的正常与边界情况。
2. Playwright UI 测试：若模块涉及前端页面，在 tests/e2e 目录下编写 Playwright 用例（@playwright/test 未安装则先安装，无 playwright.config 则创建，baseURL 使用上述访问地址），然后运行 npx playwright test。
3. 全部测试执行完毕后，无论通过与否，必须把结构化结果写入文件 tests/results/module-%d.json，格式严格如下（合法 JSON，不要包裹 Markdown 代码块标记）：
{"module_id": %d, "passed": true或false, "summary": "一句话总结，如：集成测试 8/10 通过；Playwright 3/4 通过", "failures": [{"kind": "integration或e2e或build", "name": "失败测试名", "log": "关键失败日志摘要"}]}
要求：所有测试通过时 passed 才为 true 且 failures 为空数组；任一测试失败时 passed 为 false 且 failures 逐项列出。

模块需求描述：%s

这是第 %d 轮测试。现在开始。`,
		project.Name, mod.Name, mod.SequenceNumber, accessURL,
		mod.ID, mod.ID, mod.Description, round)
}

// applyResultToRun 把测试结果映射到 TestRun 记录字段（不写库）。
func applyResultToRun(run *model.TestRun, testResult *ModuleTestResult, cliResponse string) {
	run.Status = "failed"
	run.FailCount = len(testResult.Failures)
	if testResult.Passed {
		run.Status = "passed"
		run.FailCount = 0
		// 从 summary 中统计通过数，如「集成测试 8/10 通过；Playwright 3/4 通过」→ 11。
		run.PassCount = parsePassCountFromSummary(testResult.Summary)
	}
	output := testResult.Summary + "\n\n" + tailString(cliResponse, 8000)
	run.Output = truncateUTF8Bytes(output, 60000)
}

// parsePassCountFromSummary 从测试总结中提取所有「x/y 通过」的分子并求和；
// 未匹配到时返回 1，至少表示存在通过的测试。
func parsePassCountFromSummary(summary string) int {
	re := regexp.MustCompile(`(\d+)/(\d+)\s*通过`)
	matches := re.FindAllStringSubmatch(summary, -1)
	if len(matches) == 0 {
		return 1
	}
	total := 0
	for _, m := range matches {
		if len(m) >= 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				total += n
			}
		}
	}
	if total == 0 {
		return 1
	}
	return total
}

// apiTestTimeout / webTestTimeout 脚本执行超时：API 多步 curl，UI 场景含浏览器启动。
const (
	apiTestTimeout = 5 * time.Minute
	webTestTimeout = 15 * time.Minute
)

// fileExists 判断文件是否存在（用于探测测试脚本/spec 是否已生成）。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// RunModuleTest 运行一轮模块自动测试（API + Web 全量）。
// 优先直接执行已生成的测试脚本（API shell 脚本 + Playwright spec）：
// 后端 exec 脚本、解析输出，不再驱动 agent 执行。脚本缺失时降级为
// agent 自由测试模式（保留 buildTestPrompt 路径）。
// 任何异常都归一化为 Passed=false 的 ModuleTestResult 返回，
// error 仅在记录创建失败等不可继续的场景返回非 nil。
func (e *TestExecutor) RunModuleTest(project *model.Project, mod *model.Module, accessURL string, round int, onOutput func(string)) (*ModuleTestResult, error) {
	return e.runModuleTestScoped(project, mod, "", accessURL, round, onOutput)
}

// RunModuleScenariosTest 运行一轮「指定类型」的全量测试（api 或 web）。
// 复用 runModuleTestScoped 的脚本直执行路径，但只跑该类型场景并只回写该类型结果。
// 访问地址由内部从最近部署记录 / tests/.base_url 解析（与 RunSingleScenario 一致），
// 未部署时返回明确错误。供「全量测试」按钮调用。
func (e *TestExecutor) RunModuleScenariosTest(project *model.Project, mod *model.Module, scenarioType string, round int, onOutput func(string)) (*ModuleTestResult, error) {
	if scenarioType != "api" && scenarioType != "web" {
		return nil, fmt.Errorf("不支持的场景类型: %s（仅支持 api/web）", scenarioType)
	}
	accessURL, err := e.resolveAccessURL(project.ID)
	if err != nil || accessURL == "" {
		return nil, fmt.Errorf("项目未部署或无法获取访问地址: %v（请先部署项目后再全量测试）", err)
	}
	return e.runModuleTestScoped(project, mod, scenarioType, accessURL, round, onOutput)
}

// runModuleTestScoped 运行一轮模块自动测试。
// scenarioType 取值 ""（API+Web 全量，原 RunModuleTest 行为）/ "api" / "web"。
// 类型化运行时脚本/spec 缺失会明确报错，不降级为 agent 自由测试（避免
// 「点了 Web 全量测试却跑自由测试」的困惑）。
func (e *TestExecutor) runModuleTestScoped(project *model.Project, mod *model.Module, scenarioType, accessURL string, round int, onOutput func(string)) (*ModuleTestResult, error) {
	testType := "module-auto"
	switch scenarioType {
	case "api":
		testType = "module-api"
	case "web":
		testType = "module-web"
	}
	now := time.Now()
	run := &model.TestRun{
		ProjectID: project.ID,
		ModuleID:  &mod.ID,
		TestType:  testType,
		Status:    "running",
		StartedAt: &now,
	}
	if err := e.db.Create(run).Error; err != nil {
		return nil, fmt.Errorf("创建测试运行记录失败: %w", err)
	}

	// 删除上一轮结果文件，防止误读旧结果
	_ = os.Remove(resultFilePath(project.WorkDir, mod.ID))
	// 清空上一轮截图目录并重建，只保留最新一轮截图
	shotDir := ScreenshotDir(project.WorkDir, mod.ID)
	_ = os.RemoveAll(shotDir)
	_ = os.MkdirAll(shotDir, 0o755)

	// 每次运行前用最新用例重新生成 API 脚本：保证脚本包含 BASE_URL 自解析逻辑
	//（未注入时回退 tests/.base_url），且与前端编辑后的用例同步；生成失败不阻断
	//（脚本缺失时仍降级为 agent 自由测试模式）。web 单独运行时无需刷新 API 脚本。
	if scenarioType != "web" && strings.TrimSpace(mod.APIIntegrationTest) != "" {
		if script, genErr := GenerateAPITestScript(mod.APIIntegrationTest); genErr == nil {
			if writeErr := writeAPITestScript(project.WorkDir, mod.ID, script); writeErr != nil {
				log.Printf("刷新 API 测试脚本失败(module=%d): %v", mod.ID, writeErr)
			}
		}
	}

	// 自愈固定存储：把本轮解析到的访问地址同步写入 tests/.base_url，
	// 保证生成的测试脚本未注入 BASE_URL 时也能回退自解析。
	_ = writeBaseURLFile(project.WorkDir, accessURL)

	apiScriptPath := apiTestScriptPath(project.WorkDir, mod.ID)
	e2eSpecPath := e2eSpecFilePath(project.WorkDir, mod.ID)
	hasAPIScript := fileExists(apiScriptPath) && (scenarioType == "" || scenarioType == "api")
	hasE2ESpec := fileExists(e2eSpecPath) && (scenarioType == "" || scenarioType == "web")

	var testResult *ModuleTestResult
	var detail string
	if !hasAPIScript && !hasE2ESpec {
		if scenarioType != "" {
			// 类型化全量测试：脚本/spec 缺失时明确报错，不降级为 agent 自由测试
			return nil, fmt.Errorf("模块 %s 没有可执行的 %s 测试（脚本/spec 未生成，请先通过「AI 生成用例」生成 %s 用例）", mod.Name, scenarioType, scenarioType)
		}
		// 无脚本：降级为 agent 自由测试模式
		onOutput("  ⚠ 未找到可执行测试脚本，降级为 agent 自由测试模式\n")
		testResult = e.runAgentTest(project, mod, accessURL, round, onOutput)
		detail = testResult.Summary
	} else {
		testResult, detail = e.runScriptTests(project, mod, accessURL, hasAPIScript, hasE2ESpec, onOutput)
	}

	completedAt := time.Now()
	run.CompletedAt = &completedAt
	run.Duration = int(completedAt.Sub(now).Seconds())
	applyResultToRun(run, testResult, detail)
	if err := e.db.Save(run).Error; err != nil {
		log.Printf("更新测试运行记录失败: %v", err)
	}

	// 场景级结果回写模块用例字段（面板「上次通过/失败」标签与截图的数据源）。
	// 按 name 精确匹配；匹配不上的场景结果只留在 failures 里，不回写。
	if len(testResult.Scenarios) > 0 {
		now := time.Now()
		updates := map[string]interface{}{}
		if apiResults := filterScenariosByKind(testResult.Scenarios, "api"); len(apiResults) > 0 && strings.TrimSpace(mod.APIIntegrationTest) != "" {
			updates["api_integration_test"] = applyScenarioResults(mod.APIIntegrationTest, apiResults, now)
		}
		if webResults := filterScenariosByKind(testResult.Scenarios, "e2e"); len(webResults) > 0 && strings.TrimSpace(mod.WebIntegrationTest) != "" {
			updates["web_integration_test"] = applyScenarioResults(mod.WebIntegrationTest, webResults, now)
		}
		if len(updates) > 0 {
			if err := e.db.Model(&model.Module{}).Where("id = ?", mod.ID).Updates(updates).Error; err != nil {
				log.Printf("场景结果回写模块用例失败(module=%d): %v", mod.ID, err)
			}
		}
	}
	return testResult, nil
}

// runAgentTest 降级路径：驱动测试 agent 自由测试（无预生成脚本时使用）。
// 保留原有 buildTestPrompt 契约：agent 现场编写并运行测试，结果写入结果文件。
func (e *TestExecutor) runAgentTest(project *model.Project, mod *model.Module, accessURL string, round int, onOutput func(string)) *ModuleTestResult {
	resultPath := resultFilePath(project.WorkDir, mod.ID)
	prompt := buildTestPrompt(project, mod, accessURL, round)
	cliResult := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	cli.RecordCall(e.db, "module_test", &project.ID, nil, prompt, cliResult, project.WorkDir)

	var testResult *ModuleTestResult
	switch parsed, readErr := readModuleTestResult(resultPath); {
	case cliResult.ExitCode != 0:
		testResult = &ModuleTestResult{
			ModuleID: mod.ID,
			Passed:   false,
			Summary:  fmt.Sprintf("测试 agent 执行异常（退出码 %d）: %s", cliResult.ExitCode, cliResult.Error),
			Failures: []ModuleTestFailure{{Kind: "agent", Name: "test-agent", Log: tailString(cliResult.Response, 4000)}},
		}
	case readErr != nil:
		testResult = &ModuleTestResult{
			ModuleID: mod.ID,
			Passed:   false,
			Summary:  fmt.Sprintf("测试 agent 未产出有效结果文件: %v", readErr),
			Failures: []ModuleTestFailure{{Kind: "agent", Name: "test-agent", Log: tailString(cliResult.Response, 4000)}},
		}
	default:
		testResult = parsed
	}
	testResult.ModuleID = mod.ID
	return testResult
}

// webScenarioByTitle 解析 Web 用例 spec，构建 playwrightTest 标题 → 场景定义映射。
// Playwright --reporter=json 结果按 test 标题（playwrightTest）返回，而模块用例按
// 显示名 name 记录；回写用例与合成步骤明细都需要按标题定位到对应场景定义。
func webScenarioByTitle(specJSON string) map[string]*scenarioDef {
	var spec struct {
		TestScenarios []scenarioDef `json:"testScenarios"`
	}
	if json.Unmarshal([]byte(specJSON), &spec) != nil {
		return nil
	}
	m := make(map[string]*scenarioDef, len(spec.TestScenarios))
	for i := range spec.TestScenarios {
		s := &spec.TestScenarios[i]
		title := s.PlaywrightTest
		if title == "" {
			title = s.Name
		}
		m[title] = s
	}
	return m
}

// synthE2EScenarioSteps 用用例定义的步骤合成 e2e 步骤明细。
// Playwright JSON reporter 不含逐步骤数据，无法拿到真实分步执行轨迹；
// 按定义步骤回填：passed 时全部 ok，失败时末步携带真实错误（截断 300），
// 保证前端「步骤明细」可复现输入与最终结果。
func synthE2EScenarioSteps(steps []scenarioStepDef, passed bool, log string) []ScenarioStepResult {
	if len(steps) == 0 {
		return nil
	}
	out := make([]ScenarioStepResult, 0, len(steps))
	for _, st := range steps {
		out = append(out, ScenarioStepResult{
			Action:  st.Action,
			Command: st.Command,
			OK:      passed,
		})
	}
	if !passed {
		out[len(out)-1].Error = tailString(log, 300)
	}
	return out
}

// runScriptTests 直接执行已生成的测试脚本：API shell 脚本 + Playwright spec。
// 返回合并后的测试结果与可展示的执行明细（脚本 stderr）。
func (e *TestExecutor) runScriptTests(project *model.Project, mod *model.Module, accessURL string, hasAPIScript, hasE2ESpec bool, onOutput func(string)) (*ModuleTestResult, string) {
	var apiResult *ModuleTestResult
	var stderrParts []string

	if hasAPIScript {
		onOutput(fmt.Sprintf("  ▶ 执行 API 测试脚本 tests/scripts/module-%d-api.sh ...\n", mod.ID))
		scriptResult, err := RunScript(project.WorkDir, apiTestScriptPath(project.WorkDir, mod.ID), map[string]string{"BASE_URL": accessURL}, apiTestTimeout)
		if err != nil {
			onOutput(fmt.Sprintf("  ✗ API 测试脚本执行失败: %v\n", err))
			apiResult = &ModuleTestResult{
				Passed:   false,
				Summary:  "API 脚本执行失败: " + err.Error(),
				Failures: []ModuleTestFailure{{Kind: "api", Name: "api-script", Log: err.Error()}},
			}
		} else {
			if scriptResult.Stderr != "" {
				onOutput(scriptResult.Stderr)
				stderrParts = append(stderrParts, scriptResult.Stderr)
			}
			apiResult = ParseModuleTestResultFromOutput(scriptResult.Stdout, scriptResult.ExitCode)
			apiResult.ModuleID = mod.ID
		}
	}

	var webScenarios []ScenarioResult
	var webScriptResult *ScriptResult
	if hasE2ESpec {
		onOutput(fmt.Sprintf("  ▶ 执行 Playwright UI 测试 tests/e2e/module-%d.spec.ts ...\n", mod.ID))
		pwResult, err := RunPlaywright(project.WorkDir, e2eSpecFilePath(project.WorkDir, mod.ID), accessURL, "", webTestTimeout)
		if err != nil {
			// 保留 stdout/stderr，避免把真实原因（如收集期错误、超时）吞成一句泛化文案
			onOutput(fmt.Sprintf("  ✗ Playwright 执行失败: %v\n", err))
			if pwResult == nil {
				pwResult = &ScriptResult{ExitCode: -1, Stderr: err.Error()}
			}
		}
		webScriptResult = pwResult
		webScenarios = ParsePlaywrightResultFromOutput(pwResult.Stdout, pwResult.ExitCode)
		// playwright 标题 → 场景定义（显示名+步骤）：reporter 结果按 test 标题返回，
		// 回写用例需按显示名精确匹配，步骤明细由定义合成。
		var webDefs map[string]*scenarioDef
		if strings.TrimSpace(mod.WebIntegrationTest) != "" {
			webDefs = webScenarioByTitle(mod.WebIntegrationTest)
		}
		// spec 按约定以 playwright 标题命名终态/出错截图，先按标题回填相对路径供前端展示，
		// 再把 Name 换算回用例显示名（否则 applyScenarioResults 按 name 匹配永不命中）。
		for i := range webScenarios {
			title := webScenarios[i].Name
			// 截图可能按 playwright 标题命名，也可能按用例显示名命名（两种 spec 生成风格），
			// 两 slug 都查一遍：命不中会回写空截图并清除旧路径，避免旧「Failed」截图残留。
			fillScenarioScreenshotPath(project.WorkDir, mod.ID, title, &webScenarios[i])
			if webDefs != nil {
				if def, ok := webDefs[title]; ok {
					fillScenarioScreenshotPath(project.WorkDir, mod.ID, def.Name, &webScenarios[i])
					webScenarios[i].Name = def.Name
					webScenarios[i].Steps = synthE2EScenarioSteps(def.Steps, webScenarios[i].Passed, webScenarios[i].Log)
				}
			}
		}
	}

	result := mergeScriptResults(mod.ID, apiResult, webScenarios, hasAPIScript, hasE2ESpec)
	// Playwright 无任何场景结果时，用真实执行输出覆盖「Playwright 无测试结果输出」的泛化文案
	if hasE2ESpec && len(webScenarios) == 0 && webScriptResult != nil {
		if detail := summarizePWScriptOutput(webScriptResult); detail != "" {
			for i := range result.Failures {
				if result.Failures[i].Kind == "e2e" && result.Failures[i].Name == "playwright" {
					result.Failures[i].Log = detail
				}
			}
		}
	}
	return result, strings.Join(stderrParts, "\n")
}

// mergeScriptResults 合并 API 脚本结果与 Playwright 场景结果为一个 ModuleTestResult。
// 任一有脚本的一侧失败整体即失败；两侧都无场景时保持失败并给出提示。
func mergeScriptResults(moduleID int64, apiResult *ModuleTestResult, webScenarios []ScenarioResult, hasAPI, hasWeb bool) *ModuleTestResult {
	result := &ModuleTestResult{
		ModuleID:  moduleID,
		Passed:    true,
		Failures:  []ModuleTestFailure{},
		Scenarios: []ScenarioResult{},
	}
	summaryParts := []string{}
	passed := true

	if hasAPI {
		if apiResult == nil {
			passed = false
			result.Failures = append(result.Failures, ModuleTestFailure{Kind: "api", Name: "api-script", Log: "API 测试脚本无结果"})
			summaryParts = append(summaryParts, "API 无结果")
		} else {
			passed = passed && apiResult.Passed
			result.Scenarios = append(result.Scenarios, apiResult.Scenarios...)
			if !apiResult.Passed {
				result.Failures = append(result.Failures, apiResult.Failures...)
			}
			summaryParts = append(summaryParts, apiResult.Summary)
		}
	}
	if hasWeb {
		if len(webScenarios) == 0 {
			passed = false
			result.Failures = append(result.Failures, ModuleTestFailure{Kind: "e2e", Name: "playwright", Log: "Playwright 无测试结果输出"})
			summaryParts = append(summaryParts, "Playwright 执行无结果")
		} else {
			webPass := 0
			for _, s := range webScenarios {
				result.Scenarios = append(result.Scenarios, s)
				if s.Passed {
					webPass++
				} else {
					passed = false
					result.Failures = append(result.Failures, ModuleTestFailure{Kind: "e2e", Name: s.Name, Log: s.Log})
				}
			}
			summaryParts = append(summaryParts, fmt.Sprintf("Playwright %d/%d 通过", webPass, len(webScenarios)))
		}
	}

	if len(summaryParts) == 0 {
		result.Summary = "无测试脚本执行"
	} else {
		result.Summary = strings.Join(summaryParts, "；")
	}
	result.Passed = passed
	return result
}

// buildFixPrompt 构造修复 agent 的提示词，携带上一轮失败明细。
func buildFixPrompt(project *model.Project, mod *model.Module, testResult *ModuleTestResult, round int) string {
	failuresJSON, _ := json.MarshalIndent(testResult.Failures, "", "  ")
	return fmt.Sprintf(`你是开发工程师 agent。项目「%s」模块「%s」的自动测试未通过。

测试总结：%s

失败明细（JSON）：
%s

请定位并修复代码中的问题。要求：
1. 只修业务代码或确实有误的测试代码，不得通过删除/弱化测试来让测试通过。
2. 修复完成后确保 go build ./... 与前端构建（npm run build，若有前端目录）均通过。
3. 不要重新运行完整测试套件，下一轮会由测试 agent 统一执行。

这是第 %d 轮修复。现在开始。`,
		project.Name, mod.Name, testResult.Summary, string(failuresJSON), round)
}

// RunModuleFix 运行一轮修复 agent。修复结果不由本方法判定，下一轮测试自然验证。
func (e *TestExecutor) RunModuleFix(project *model.Project, mod *model.Module, testResult *ModuleTestResult, round int, onOutput func(string)) error {
	prompt := buildFixPrompt(project, mod, testResult, round)
	cliResult := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	cli.RecordCall(e.db, "module_fix", &project.ID, nil, prompt, cliResult, project.WorkDir)
	if cliResult.ExitCode != 0 {
		return fmt.Errorf("修复 agent 执行失败（退出码 %d）: %s", cliResult.ExitCode, cliResult.Error)
	}
	return nil
}
