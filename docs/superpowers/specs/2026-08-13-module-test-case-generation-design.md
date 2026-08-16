# 业务模块测试用例自动生成 + 测试/修复/部署闭环 设计

日期：2026-08-13
状态：已确认（方案 A）

## 背景与问题

项目 22（轻量待办）所有模块任务显示完成，但模块 2/3 的「集成测试」面板为空。排查结论：

1. 自动测试门禁（`runBusinessModuleGate`）确实跑过并最终通过（`test_run` 表有 11 条 `module-auto` 记录），测试 agent 把用例临时写进了项目 `tests/` 目录；
2. 但「集成测试」面板读的是 `module.api_integration_test` / `module.web_integration_test` 字段，**后端没有任何代码写过这两个字段**（均为空）；
3. 面板「AI 生成 API+Web 用例」按钮调用的 `POST /modules/:id/generate-test-stream` 是桩接口（直接返回"暂未实现"）。

需求：编码实现完成一个业务模块（不含基础架构模块）后，自动生成 API 测试用例和 Playwright 测试用例并持久化到模块上，然后执行「测试 → 修复 → 部署」自动闭环；暂不做 TDD；Playwright 测试完需要截图，展示在模块的测试详情面板。

## 已确认的决策

| 决策点 | 结论 |
|---|---|
| 用例生成位置 | 门禁全自动：业务模块任务完成后先生成用例落库 → 部署 → 按用例执行 → 修复重试，无人工干预 |
| 历史模块（已完成但用例为空） | 流水线遇到时自动补跑「生成用例」步骤（不重跑任务、不重跑测试） |
| 执行粒度 | 按落库场景逐条执行，场景级通过/失败回写面板 |
| 截图粒度 | 每个 Playwright 场景跑完截一张终态图；失败场景额外保留出错图；只保留最新一轮 |

## 总体流程

`runBusinessModuleGate`（backend-go/internal/handler/pipeline.go）改造为四步门禁：

```
业务模块任务完成（或断点续跑进门禁）
  ↓
【新增·第0步】用例生成（幂等：仅当 api_integration_test / web_integration_test 为空时执行）
  「测试设计 agent」（CLI --print 模式）读取模块需求 + 已实现代码（Go 路由、前端页面），产出：
    a) API 测试场景 JSON      → 落库 module.api_integration_test
    b) Playwright 场景清单 JSON → 落库 module.web_integration_test
       可执行 spec 文件        → 写 tests/e2e/module-<id>.spec.ts
  生成失败（agent 异常/产出畸形）→ 记警告日志，门禁继续走现状的"自由测试"模式，不卡死流水线
  ↓
【现有循环】第1~3轮：全量部署 → 测试 agent（按落库场景逐条执行）→ 失败则修复 agent
  ↓
通过 → status=4；轮次耗尽 → status=5 暂停
```

**历史回填**：`moduleActionSkip` 分支（status=4）增加判断——业务模块且用例为空则只补跑第 0 步生成，然后照常跳过。

**断点续跑**：`resolveModuleAction` 语义不变；第 0 步幂等，任何中断后重启都安全。

## 数据契约

### API 用例（`api_integration_test`）

沿用 `IntegrationTestEditor` 已识别的格式，agent 直接产出：

```json
{"testScenarios": [{
  "name": "登录成功",
  "steps": [{"action": "调用登录接口", "command": "curl -s -X POST $BASE_URL/api/login -H 'Content-Type: application/json' -d '{...}'", "expected": "\"code\":0"}],
  "onFailure": "continue"
}]}
```

`$BASE_URL` 为占位符，执行时替换为当轮部署的 accessURL（避免端口变化导致用例失效）。

### Web 用例（`web_integration_test`）

同一信封，场景增加 `playwrightTest` / `specFile` 字段，与 `tests/e2e/module-<id>.spec.ts` 中的 `test()` 标题一一对应：

```json
{"testScenarios": [{"name": "注册流程", "playwrightTest": "注册流程", "specFile": "tests/e2e/module-84.spec.ts", "steps": [{"action": "打开注册页填写表单", "expected": "跳转到登录页"}]}]}
```

### 结果 JSON 扩展（`tests/results/module-<id>.json`）

向后兼容——无 `scenarios` 字段时回退现状的模块级判定：

```json
{"module_id": 84, "passed": false, "summary": "...", "failures": [...],
 "scenarios": [
   {"kind": "api", "name": "登录成功", "passed": true,  "log": "...", "screenshot": ""},
   {"kind": "e2e", "name": "注册流程", "passed": false, "log": "...", "screenshot": "tests/results/screenshots/module-84/register-error.png"}
 ]}
```

### 截图

- 每场景跑完截终态图 `tests/results/screenshots/module-<id>/<slug(name)>.png`，失败加截 `<slug>-error.png`
- 每轮测试开始清空该模块截图目录，只保留最新一轮
- 每轮测试结束后把场景结果（`lastRunAt` / `lastSuccess` / `lastSummary` / 截图路径）按场景 `name` 匹配回写进模块的两个用例字段——前端面板现有「上次通过/失败」标签直接点亮，无需新数据结构
- slug：场景名转安全文件名（保留中文，去除路径分隔符与空白转连字符）

## 后端组件改动

### ① 新增 `TestDesignExecutor`（`backend-go/internal/engine/executor/test_designer.go`）

与 `TestExecutor` 并列、复用同一个 `cli.OfflineExecutor`。

- `RunModuleTestDesign(project, mod, onOutput) error`：
  - 构造提示词：模块需求 + 要求 agent 先读代码（`router`、前端 `views`/`api` 目录）再产出两类用例
  - 契约：agent 把两个 JSON 分别写到 `tests/specs/module-<id>-api.json` 和 `tests/specs/module-<id>-web.json`，Playwright spec 写到 `tests/e2e/module-<id>.spec.ts`
  - **后端读文件、校验 JSON 合法性后负责落库**（`api_integration_test` / `web_integration_test`）——不让 agent 直接写库，与现有「结果文件」契约同构，可靠且可校验
- 落库用 `Omit("created_at").Updates`（避免零值 created_at 触发 MySQL Error 1292）

### ② `TestExecutor` 改造（test_executor.go）

- `buildTestPrompt`：模块有用例时，提示词携带两个场景清单 + spec 文件路径，要求逐场景执行、每场景结束截图、结果 JSON 带 `scenarios` 数组；无用例时退化为现状提示词（自由测试）
- 结果解析后新增回写步骤：按 `scenarios` 把场景结果合并进模块用例 JSON（按 `name` 匹配），`Updates` 落库
- `ModuleTestResult` 增加 `Scenarios []ScenarioResult` 字段；`ScenarioResult{Kind, Name string; Passed bool; Log, Screenshot string}`

### ③ 桩接口实现（module.go `GenerateTestStream`）

LEGACY 模式改为调 `TestDesignExecutor`，SSE 流式输出生成过程，结束后 `SendDone(module)` 返回更新后的模块。面板「AI 生成 API+Web 用例」按钮从此真正可用，与门禁第 0 步同源。

### ④ 截图静态路由

`GET /api/projects/:id/modules/:mid/screenshots/:file`

- 从项目 `workDir/tests/results/screenshots/module-<mid>/<file>` 读图
- 严格校验：`filepath.Clean` + 禁止 `..`、只允许 `.png/.jpg` 后缀、最终路径必须落在模块截图目录内
- `Cache-Control: no-cache`（截图同名覆盖）

### ⑤ 历史回填钩子（pipeline.go `moduleActionSkip` 分支）

业务模块且用例为空 → 调一次 `RunModuleTestDesign` 再跳过；生成失败仅记警告不阻断。

## 前端展示

1. **IntegrationTestEditor 场景卡片**：场景有截图字段时，在「上次运行」面板内嵌缩略图（`el-image`，点击放大预览）；失败场景同时展示终态图和 error 图。
2. **ModuleTaskTab 集成测试区**：现有结构不动；生成落库后走现有 `module-updated` 回写逻辑自动刷新。
3. **Web 用例 tab 补全**：LEGACY 模式目前只有「API集成测试」tab，增加「Web集成测试」tab，复用 `IntegrationTestEditor` 组件展示 `webIntegrationTestText`。

## 错误处理

| 场景 | 行为 |
|---|---|
| 用例生成 agent 异常/产出畸形 | 警告日志，门禁继续自由测试模式；模块字段保持空，下次进门禁重试生成 |
| 测试 agent 未输出 `scenarios` | 回退现状模块级判定（`parseModuleTestResult` 兼容） |
| 截图文件缺失 | 前端不渲染图片，不影响状态标签 |
| 场景名匹配不上（agent 改名） | 按 `name` 精确匹配；匹配不上的场景结果只进 `failures`，不回写用例 |
| 每轮测试开始 | 清空 `screenshots/module-<id>/`，避免旧图误导 |

## 测试

- **Go 单测**：
  - `parseModuleTestResult` 含 scenarios 的解析与无 scenarios 的兼容回退
  - 场景结果回写合并逻辑（含改名不匹配、字段缺失）
  - 截图路由路径穿越（`../`、`%2e%2e`、非图片后缀）
  - slug 生成（中文场景名 → 安全文件名）
- **端到端验证**：项目 22 重启流水线 → 模块 2/3 自动补用例 → 面板可见场景+截图；新建 demo 项目全链路跑一遍门禁闭环
- 提交前 `go build ./...` 与 `npm run build` 通过

## 非目标（YAGNI）

- TDD 验收标准模式（用户明确暂缓）
- 场景级单跑/重跑接口（`run-stream` 桩不在本期范围）
- 历史轮次截图归档（只保留最新一轮）
