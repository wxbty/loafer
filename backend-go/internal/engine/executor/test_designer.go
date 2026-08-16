package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// TestDesignExecutor 模块测试用例设计执行器：驱动「测试设计 agent」生成
// API 集成测试场景与 Playwright 场景清单，校验后落库到模块的
// api_integration_test / web_integration_test 字段。
// agent 只写文件不碰数据库；后端读文件、校验、落库，与结果文件契约同构。
type TestDesignExecutor struct {
	db       *gorm.DB
	executor *cli.OfflineExecutor
}

// NewTestDesignExecutor 构造用例设计执行器。
func NewTestDesignExecutor(db *gorm.DB, executor *cli.OfflineExecutor) *TestDesignExecutor {
	return &TestDesignExecutor{db: db, executor: executor}
}

// designSpecPaths 返回用例生成 agent 的三个产物路径（绝对路径）。
func designSpecPaths(workDir string, moduleID int64) (apiPath, webPath, e2ePath string) {
	return filepath.Join(workDir, "tests", "specs", fmt.Sprintf("module-%d-api.json", moduleID)),
		filepath.Join(workDir, "tests", "specs", fmt.Sprintf("module-%d-web.json", moduleID)),
		filepath.Join(workDir, "tests", "e2e", fmt.Sprintf("module-%d.spec.ts", moduleID))
}

// validateTestSpec 校验用例 spec：合法 JSON 对象且 testScenarios 为非空数组。
func validateTestSpec(data []byte) error {
	var spec map[string]interface{}
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("不是合法 JSON: %w", err)
	}
	scenarios, ok := spec["testScenarios"].([]interface{})
	if !ok || len(scenarios) == 0 {
		return fmt.Errorf("缺少非空 testScenarios 数组")
	}
	return nil
}

// buildDesignPrompt 构造测试设计 agent 提示词。
// needAPI / needWeb 标记还缺哪类用例（只生成缺失的，已存在的不覆盖）。
func buildDesignPrompt(project *model.Project, mod *model.Module, needAPI, needWeb bool) string {
	apiRel := fmt.Sprintf("tests/specs/module-%d-api.json", mod.ID)
	webRel := fmt.Sprintf("tests/specs/module-%d-web.json", mod.ID)
	e2eRel := fmt.Sprintf("tests/e2e/module-%d.spec.ts", mod.ID)

	var products strings.Builder
	if needAPI {
		products.WriteString(fmt.Sprintf(`
【产物 1：API 集成测试场景】写入文件 %s，格式（合法 JSON，不要包裹 Markdown 代码块标记）：
{"testScenarios": [{"name": "场景名", "steps": [{"action": "操作描述", "command": "curl 命令", "expected": "响应中应包含的子串"}], "onFailure": "continue"}]}
要求：
- 覆盖该模块全部核心 API 的正常与边界情况（缺参、越权、非法值等），每个场景至少 1 步。
- command 中的服务地址一律用占位符 $BASE_URL（如 curl -s -X POST $BASE_URL/api/login ...），执行时由平台替换。
- expected 填响应体中稳定包含的子串（如 "\"code\":0"），不要写整段响应。`, apiRel))
	}
	if needWeb {
		products.WriteString(fmt.Sprintf(`
【产物 2：Playwright 场景清单】写入文件 %s，格式：
{"testScenarios": [{"name": "场景名", "playwrightTest": "与产物3中 test() 标题完全一致", "specFile": "%s", "steps": [{"action": "操作描述", "expected": "预期页面表现"}]}]}
【产物 3：Playwright 可执行用例】写入文件 %s，用 @playwright/test 编写。要求：
- 每个 test() 的标题必须与产物 2 中对应场景的 playwrightTest 字段完全一致；
- baseURL 通过 test.use({ baseURL: process.env.BASE_URL }) 读取，不要硬编码地址；
- 所有页面跳转必须用 safeGoto 封装：async function safeGoto(page, url){ try { await page.goto(url, { waitUntil: 'domcontentloaded' }) } catch { try { await page.goto(url, { waitUntil: 'domcontentloaded' }) } catch {} } }，禁止直接调用 page.goto，避免瞬时网络波动让测试停在浏览器「Failed」错误页；
- 每个 test() 无论成败，结束前必须用 page.screenshot 截取终态图，保存到 tests/results/screenshots/module-%d/<test标题转文件名>.png；失败的场景额外保存 <test标题转文件名>-error.png。截图目录需先创建（fs.mkdirSync(dir, { recursive: true })）。文件名必须由 test() 的完整标题（即与 playwrightTest 一致的那个）转换而来：保留中英文字母与数字，其余字符一律转连字符，去掉首尾连字符，最长 40 字符。禁止另建「显示名→文件名」映射，保证文件名可被平台按 test 标题推导，否则截图回写不上、旧「Failed」截图会残留。
- 终态截图防错误页：afterEach 截屏前先判断 page.url() 是否以 chrome-error:// 开头（导航失败错误页），若是先 await safeGoto(page, process.env.BASE_URL || '/') 回到应用首页再截屏，保证终态截图展示真实应用而非「Failed」错误页；截图用 try/catch 包裹，页面崩溃等极端情况不阻断测试。`, webRel, e2eRel, e2eRel, mod.ID))
	}

	return fmt.Sprintf(`你是测试设计 agent。项目「%s」的业务模块「%s」（序号 %s）刚完成编码实现。

请先阅读该模块的已实现代码（后端路由/handler、前端页面与 api 调用），再为其设计测试用例。

模块需求描述：%s

%s

现在开始。`, project.Name, mod.Name, mod.SequenceNumber, mod.Description, products.String())
}

// RunModuleTestDesign 为业务模块生成缺失的测试用例并落库。
// 幂等：api_integration_test 与 web_integration_test 均非空时直接返回 nil。
// agent 失败或全部产物缺失/畸形时返回 error（调用方降级为自由测试模式）；
// 部分产物缺失时只落库成功的一侧，返回 nil 并在 onOutput 中告警。
func (e *TestDesignExecutor) RunModuleTestDesign(project *model.Project, mod *model.Module, onOutput func(string)) error {
	needAPI := strings.TrimSpace(mod.APIIntegrationTest) == ""
	needWeb := strings.TrimSpace(mod.WebIntegrationTest) == ""
	if !needAPI && !needWeb {
		return nil
	}
	if project.WorkDir == "" {
		return fmt.Errorf("项目工作目录为空，无法生成用例")
	}

	prompt := buildDesignPrompt(project, mod, needAPI, needWeb)
	cliResult := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	cli.RecordCall(e.db, "module_test_design", &project.ID, nil, prompt, cliResult, project.WorkDir)
	if cliResult.ExitCode != 0 {
		return fmt.Errorf("测试设计 agent 执行失败（退出码 %d）: %s", cliResult.ExitCode, cliResult.Error)
	}

	apiPath, webPath, _ := designSpecPaths(project.WorkDir, mod.ID)
	updates := map[string]interface{}{}
	var problems []string

	if needAPI {
		if data, err := os.ReadFile(apiPath); err != nil {
			problems = append(problems, fmt.Sprintf("API 用例文件缺失: %v", err))
		} else if err := validateTestSpec(data); err != nil {
			problems = append(problems, fmt.Sprintf("API 用例校验失败: %v", err))
		} else {
			updates["api_integration_test"] = string(data)
			// 从 JSON spec 生成可执行 shell 脚本，供测试执行器直接运行（不再依赖 agent 执行）。
			// 生成失败不阻断：测试执行时会降级为 agent 自由测试模式。
			if script, genErr := GenerateAPITestScript(string(data)); genErr != nil {
				problems = append(problems, fmt.Sprintf("API 测试脚本生成失败: %v", genErr))
			} else if writeErr := writeAPITestScript(project.WorkDir, mod.ID, script); writeErr != nil {
				problems = append(problems, fmt.Sprintf("API 测试脚本写入失败: %v", writeErr))
			}
		}
	}
	if needWeb {
		if data, err := os.ReadFile(webPath); err != nil {
			problems = append(problems, fmt.Sprintf("Web 用例文件缺失: %v", err))
		} else if err := validateTestSpec(data); err != nil {
			problems = append(problems, fmt.Sprintf("Web 用例校验失败: %v", err))
		} else {
			updates["web_integration_test"] = string(data)
		}
	}

	if len(updates) == 0 {
		return fmt.Errorf("测试设计 agent 未产出任何有效用例: %s", strings.Join(problems, "；"))
	}
	// map 形式 Updates 只触达给定 key，不会写零值 created_at。
	if err := e.db.Model(&model.Module{}).Where("id = ?", mod.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("用例落库失败: %w", err)
	}
	for _, p := range problems {
		onOutput("  ⚠ " + p + "\n")
	}
	return nil
}
