package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"
)

// scenarioDef 用例 spec JSON 中单个场景的解析中间态。
type scenarioDef struct {
	Name           string            `json:"name"`
	PlaywrightTest string            `json:"playwrightTest"`
	SpecFile       string            `json:"specFile"`
	Steps          []scenarioStepDef `json:"steps"`
	OnFailure      string            `json:"onFailure"`
}

// scenarioStepDef 用例 spec JSON 中单个步骤的解析中间态。
type scenarioStepDef struct {
	Action   string `json:"action"`
	Command  string `json:"command"`
	Expected string `json:"expected"`
}

// scenarioKind 映射前端 scenarioType → agent 结果 JSON 中的 kind 字段与 DB 列名。
func scenarioKind(scenarioType string) (kind, column string, err error) {
	switch scenarioType {
	case "api":
		return "api", "api_integration_test", nil
	case "web":
		return "e2e", "web_integration_test", nil
	default:
		return "", "", fmt.Errorf("不支持的场景类型: %s（仅支持 api/web）", scenarioType)
	}
}

// selectScenarioSpec 按 scenarioType 从模块选取用例 JSON、kind 与列名。
// 用例为空时返回 error。
func selectScenarioSpec(mod *model.Module, scenarioType string) (specJSON string, kind string, column string, err error) {
	kind, column, err = scenarioKind(scenarioType)
	if err != nil {
		return
	}
	switch scenarioType {
	case "api":
		specJSON = strings.TrimSpace(mod.APIIntegrationTest)
	case "web":
		specJSON = strings.TrimSpace(mod.WebIntegrationTest)
	}
	if specJSON == "" {
		err = fmt.Errorf("模块 %s 用例为空，请先生成用例", column)
		return
	}
	return
}

// parseScenarioAt 从用例 spec JSON 按索引提取单个场景。
func parseScenarioAt(specJSON string, index int) (*scenarioDef, error) {
	var spec struct {
		TestScenarios []scenarioDef `json:"testScenarios"`
	}
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil, fmt.Errorf("用例 JSON 解析失败: %w", err)
	}
	if len(spec.TestScenarios) == 0 {
		return nil, fmt.Errorf("用例 JSON 无任何场景（testScenarios 为空）")
	}
	if index < 0 || index >= len(spec.TestScenarios) {
		return nil, fmt.Errorf("场景索引 %d 越界（共 %d 个场景）", index, len(spec.TestScenarios))
	}
	return &spec.TestScenarios[index], nil
}

// manualResultFilePath 单场景手动运行的结果文件路径（与门禁的 module-<id>.json 隔离）。
func manualResultFilePath(workDir string, moduleID int64) string {
	return fmt.Sprintf("%s/tests/results/module-%d-manual.json", workDir, moduleID)
}

// buildSingleScenarioPrompt 构造单场景手动运行的 agent 提示词。
// api 场景：按 steps 中的 curl 命令逐条执行，$BASE_URL 替换为 accessURL。
// e2e 场景：npx playwright test -g "<场景名>"，截图保存到 ScreenshotDir。
func buildSingleScenarioPrompt(project *model.Project, mod *model.Module, sc *scenarioDef, kind string, accessURL string) string {
	resultPath := manualResultFilePath(project.WorkDir, mod.ID)

	var body strings.Builder
	body.WriteString(fmt.Sprintf("项目「%s」模块「%s」的单场景手动运行。\n场景名：%s\n类型：%s\n访问地址：%s\n\n",
		project.Name, mod.Name, sc.Name, kind, accessURL))

	// 步骤明细（让 agent 逐步执行并回报每步 pass/fail）
	if len(sc.Steps) > 0 {
		body.WriteString("请按以下步骤逐步执行，每步报告通过与否：\n")
		for i, s := range sc.Steps {
			body.WriteString(fmt.Sprintf("步骤 %d：%s\n", i+1, s.Action))
			if s.Command != "" {
				body.WriteString(fmt.Sprintf("  命令：%s\n", s.Command))
			}
			if s.Expected != "" {
				body.WriteString(fmt.Sprintf("  期望：%s\n", s.Expected))
			}
		}
		body.WriteString("\n")
	}

	if kind == "api" {
		body.WriteString("API 执行规则：\n")
		body.WriteString("- curl 命令中的 $BASE_URL 替换为上述访问地址\n")
		body.WriteString("- 每步对比实际输出与 expected 子串\n\n")
	} else {
		// e2e
		playwrightTest := sc.PlaywrightTest
		if playwrightTest == "" {
			playwrightTest = sc.Name
		}
		specFile := sc.SpecFile
		if specFile == "" {
			specFile = fmt.Sprintf("tests/e2e/module-%d.spec.ts", mod.ID)
		}
		shotDir := ScreenshotDir(project.WorkDir, mod.ID)
		slug := slugifyScenarioName(sc.Name)
		body.WriteString("Playwright 执行规则：\n")
		body.WriteString(fmt.Sprintf("运行命令：BASE_URL=%s npx playwright test %s -g \"%s\"\n", accessURL, specFile, playwrightTest))
		body.WriteString(fmt.Sprintf("无论成败必须用 page.screenshot 截终态图，保存为 %s/%s.png\n", shotDir, slug))
		body.WriteString(fmt.Sprintf("失败的场景额外保存出错图 %s/%s-error.png\n\n", shotDir, slug))
	}

	// 结构化结果文件契约
	body.WriteString(fmt.Sprintf(`全部步骤执行完毕后，把结构化结果写入文件 %s，格式严格如下（合法 JSON，不要包裹 Markdown 代码块标记）：
{"name":"%s","passed":true或false,"log":"一句话总结","screenshot":"终态截图相对路径或空串","errorScreenshot":"出错截图相对路径或空串","steps":[{"action":"步骤描述","command":"执行的命令或空串","ok":true或false,"output":"步骤输出","error":"错误信息或空串"}]}
要求：steps 数组覆盖上述全部步骤；所有步骤通过时 passed 才为 true。`, resultPath, sc.Name))
	return body.String()
}

// parseSingleScenarioResult 解析单场景结果 JSON；name 缺失时回填 expectedName。
func parseSingleScenarioResult(data []byte, expectedName string) (*ScenarioResult, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("结果文件为空")
	}
	var r ScenarioResult
	if err := json.Unmarshal(trimmed, &r); err != nil {
		return nil, fmt.Errorf("结果文件不是合法 JSON: %w", err)
	}
	if r.Name == "" {
		r.Name = expectedName
	}
	return &r, nil
}

// RunSingleScenario 运行单个场景并回写结果到模块用例字段。
// scenarioType: "api" | "web"；scenarioIndex: 用例 spec 中的场景索引。
// 优先直接执行已生成脚本（API shell 脚本 / Playwright spec）；脚本缺失时降级为 agent 运行。
// 返回更新后的模块指针供前端刷新。
func (e *TestExecutor) RunSingleScenario(project *model.Project, mod *model.Module, scenarioType string, scenarioIndex int, onOutput func(string)) (*model.Module, error) {
	specJSON, kind, _, err := selectScenarioSpec(mod, scenarioType)
	if err != nil {
		return nil, err
	}
	sc, err := parseScenarioAt(specJSON, scenarioIndex)
	if err != nil {
		return nil, err
	}

	// 从最近部署记录取 accessURL
	accessURL, err := e.resolveAccessURL(project.ID)
	if err != nil || accessURL == "" {
		return nil, fmt.Errorf("项目未部署或无法获取访问地址: %v", err)
	}

	// 自愈固定存储：把解析到的访问地址同步写入 tests/.base_url，
	// 即使部署后文件被清理，测试脚本也能回退自解析。
	_ = writeBaseURLFile(project.WorkDir, accessURL)

	// 每次运行前用最新用例重新生成 API 脚本：保证脚本包含 BASE_URL 自解析逻辑
	//（未注入时回退 tests/.base_url）且与前端编辑后的用例同步。
	if kind == "api" {
		if script, genErr := GenerateAPITestScript(specJSON); genErr == nil {
			if writeErr := writeAPITestScript(project.WorkDir, mod.ID, script); writeErr != nil {
				onOutput(fmt.Sprintf("[SSE] ⚠ API 测试脚本刷新失败（继续用旧脚本）: %v\n", writeErr))
			}
		}
	}

	// 清理截图目录（保留自动门禁的截图由门禁自己管，这里只清理手动运行的同名截图）
	// spec 截图按 playwright 标题命名，slug 需用标题而非用例显示名，否则删的是不存在的文件。
	if kind == "e2e" {
		shotDir := ScreenshotDir(project.WorkDir, mod.ID)
		title := sc.PlaywrightTest
		if title == "" {
			title = sc.Name
		}
		slug := slugifyScenarioName(title)
		_ = os.Remove(fmt.Sprintf("%s/%s.png", shotDir, slug))
		_ = os.Remove(fmt.Sprintf("%s/%s-error.png", shotDir, slug))
	}

	onOutput(fmt.Sprintf("[SSE] 开始运行场景「%s」(%s)...\n", sc.Name, scenarioType))
	scenarioResult, err := e.runSingleScenarioScript(project, mod, sc, kind, scenarioIndex, accessURL, onOutput)
	if err != nil {
		// 脚本缺失/执行异常 → 降级为 agent 运行（兼容历史无脚本数据）
		onOutput(fmt.Sprintf("[SSE] ⚠ 脚本执行不可用，降级为 agent 运行: %v\n", err))
		scenarioResult, err = e.runSingleScenarioAgent(project, mod, sc, kind, accessURL, onOutput)
		if err != nil {
			return nil, err
		}
	}

	// 回写模块用例字段
	now := time.Now()
	var column string
	switch scenarioType {
	case "api":
		column = "api_integration_test"
	case "web":
		column = "web_integration_test"
	}
	updatedSpec := applyScenarioResults(specJSON, []ScenarioResult{*scenarioResult}, now)
	if err := e.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update(column, updatedSpec).Error; err != nil {
		onOutput(fmt.Sprintf("[SSE] ⚠ 结果回写失败: %v\n", err))
	}

	status := "失败"
	if scenarioResult.Passed {
		status = "通过"
	}
	onOutput(fmt.Sprintf("[SSE] 场景「%s」%s\n", sc.Name, status))

	// 重载模块供前端刷新
	var updated model.Module
	if err := e.db.First(&updated, mod.ID).Error; err != nil {
		return mod, nil // 回写已成功，返回旧模块也不影响
	}
	return &updated, nil
}

// runSingleScenarioScript 直接执行脚本运行单个场景。
// API：运行 shell 脚本（SCENARIO_INDEX 指定场景），解析 stdout JSON。
// e2e：运行 Playwright（-g 过滤 test 标题），解析 JSON reporter。
// 返回场景结果；脚本缺失或真实执行异常（非测试失败）返回 error 触发降级。
func (e *TestExecutor) runSingleScenarioScript(project *model.Project, mod *model.Module, sc *scenarioDef, kind string, scenarioIndex int, accessURL string, onOutput func(string)) (*ScenarioResult, error) {
	switch kind {
	case "api":
		scriptPath := apiTestScriptPath(project.WorkDir, mod.ID)
		if !fileExists(scriptPath) {
			return nil, fmt.Errorf("API 测试脚本不存在")
		}
		env := map[string]string{"BASE_URL": accessURL, "SCENARIO_INDEX": strconv.Itoa(scenarioIndex)}
		sr, err := RunScript(project.WorkDir, scriptPath, env, apiTestTimeout)
		if err != nil {
			return nil, err
		}
		if sr.Stderr != "" {
			onOutput(sr.Stderr)
		}
		if result, ok := parseResultJSONFromStdout(sr.Stdout); ok && len(result.Scenarios) > 0 {
			r := result.Scenarios[0]
			r.Kind = "api"
			r.Name = sc.Name
			return &r, nil
		}
		// stdout 无有效 JSON：按退出码判定（0=通过）
		return &ScenarioResult{Kind: "api", Name: sc.Name, Passed: sr.ExitCode == 0, Log: tailString(sr.Stderr, 500)}, nil

	case "e2e":
		specPath := e2eSpecFilePath(project.WorkDir, mod.ID)
		if !fileExists(specPath) {
			return nil, fmt.Errorf("Playwright spec 不存在")
		}
		title := sc.PlaywrightTest
		if title == "" {
			title = sc.Name
		}
		pwResult, err := RunPlaywright(project.WorkDir, specPath, accessURL, title, webTestTimeout)
		if err != nil {
			return nil, err
		}
		scenarios := ParsePlaywrightResultFromOutput(pwResult.Stdout, pwResult.ExitCode)
		for _, s := range scenarios {
			if s.Name == title {
				s.Kind = "e2e"
				// 截图可能按 playwright 标题命名，也可能按用例显示名命名（两种 spec 生成风格），
				// 两 slug 都查一遍，必须在改名（换成用例显示名）之前用 title/显示名回填，
				// 否则 slug 对不上 spec 里实际生成的文件，回写不到截图导致旧「Failed」截图残留。
				fillScenarioScreenshotPath(project.WorkDir, mod.ID, title, &s)
				fillScenarioScreenshotPath(project.WorkDir, mod.ID, sc.Name, &s)
				s.Name = sc.Name
				s.Steps = synthE2EScenarioSteps(sc.Steps, s.Passed, s.Log)
				return &s, nil
			}
		}
		// 未匹配到对应场景：附上真实输出便于定位（收集期错误、标题变化等），
		// 避免用户只看到一句泛化文案。
		detail := "Playwright 未匹配到该场景结果"
		if len(scenarios) == 0 {
			if summarized := summarizePWScriptOutput(pwResult); summarized != "" {
				detail = summarized
			}
		} else {
			// 有场景执行但标题未精确匹配：附上首条失败场景的错误
			for _, s := range scenarios {
				if !s.Passed && s.Log != "" {
					detail = s.Name + ": " + s.Log
					break
				}
			}
		}
		passed := len(scenarios) > 0
		for _, s := range scenarios {
			if !s.Passed {
				passed = false
				break
			}
		}
		r := &ScenarioResult{Kind: "e2e", Name: sc.Name, Passed: passed, Log: tailString(detail, 500)}
		// 截图按 playwright 标题命名，回退分支同样用 title/显示名回填
		fillScenarioScreenshotPath(project.WorkDir, mod.ID, title, r)
		fillScenarioScreenshotPath(project.WorkDir, mod.ID, sc.Name, r)
		r.Steps = synthE2EScenarioSteps(sc.Steps, r.Passed, r.Log)
		return r, nil
	}
	return nil, fmt.Errorf("未知场景类型: %s", kind)
}

// fillScenarioScreenshotPath 若 spec 已按约定把终态/出错截图写入截图目录，回填相对路径。
// slugName 为截图文件名对应的名称——Playwright 场景必须传 test 标题（spec 按
// scenarioFileName(testInfo.title) 命名，与用例显示名不同），不能从 r.Name 推导。
func fillScenarioScreenshotPath(workDir string, moduleID int64, slugName string, r *ScenarioResult) {
	shotDir := ScreenshotDir(workDir, moduleID)
	slug := slugifyScenarioName(slugName)
	base := fmt.Sprintf("tests/results/screenshots/module-%d", moduleID)
	if fileExists(filepath.Join(shotDir, slug+".png")) {
		r.Screenshot = base + "/" + slug + ".png"
	}
	if fileExists(filepath.Join(shotDir, slug+"-error.png")) {
		r.ErrorScreenshot = base + "/" + slug + "-error.png"
	}
}

// runSingleScenarioAgent 降级路径：驱动 agent 运行单个场景（兼容无脚本的历史数据）。
func (e *TestExecutor) runSingleScenarioAgent(project *model.Project, mod *model.Module, sc *scenarioDef, kind, accessURL string, onOutput func(string)) (*ScenarioResult, error) {
	resultPath := manualResultFilePath(project.WorkDir, mod.ID)
	_ = os.Remove(resultPath)

	prompt := buildSingleScenarioPrompt(project, mod, sc, kind, accessURL)
	cliResult := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	cli.RecordCall(e.db, "single_scenario_run", &project.ID, nil, prompt, cliResult, project.WorkDir)

	switch parsed, readErr := readSingleResultFile(resultPath, sc.Name); {
	case cliResult.ExitCode != 0:
		onOutput(fmt.Sprintf("[SSE] agent 退出码 %d\n", cliResult.ExitCode))
		return &ScenarioResult{Kind: kind, Name: sc.Name, Passed: false, Log: fmt.Sprintf("agent 执行异常（退出码 %d）", cliResult.ExitCode)}, nil
	case readErr != nil:
		onOutput(fmt.Sprintf("[SSE] 结果文件无效: %v\n", readErr))
		return &ScenarioResult{Kind: kind, Name: sc.Name, Passed: false, Log: fmt.Sprintf("agent 未产出有效结果文件: %v", readErr)}, nil
	default:
		return parsed, nil
	}
}

// resolveAccessURL 从 project_deployment 表读最近一次部署的访问地址。
// 部署记录缺失或无访问地址时，回退读取部署时写入的固定存储 tests/.base_url，
// 保证已部署过但部署记录被清理/未命中时测试脚本仍能自解析目标地址。
func (e *TestExecutor) resolveAccessURL(projectID int64) (string, error) {
	var d model.ProjectDeployment
	if err := e.db.Where("project_id = ?", projectID).Order("id DESC").Limit(1).First(&d).Error; err == nil && d.AccessURL != "" {
		return d.AccessURL, nil
	}
	var project model.Project
	if err := e.db.First(&project, projectID).Error; err != nil {
		return "", err
	}
	url, err := readBaseURLFile(project.WorkDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("项目未部署（无部署记录且无访问地址固定存储）")
		}
		return "", err
	}
	return url, nil
}

// readSingleResultFile 读取并解析单场景结果文件。
func readSingleResultFile(path string, expectedName string) (*ScenarioResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取结果文件失败: %w", err)
	}
	return parseSingleScenarioResult(data, expectedName)
}
