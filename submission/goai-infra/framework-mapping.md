# Loafer × GOAI Agent Infra 赛道框架映射

> 本文档用于将 Loafer 的现有实现与比赛要求（AgentTeams / Skill / MCP / 可观测 / RAG / 多 Agent 闭环）对齐，作为作品简介与方案 PPT 的底层依据。

---

## 1. 项目定位

| 项目 | 说明 |
|------|------|
| 作品名称 | Loafer：全自动软件交付多 Agent 平台 |
| 赛道 | Agent Infra（新智基座） |
| 选题方向 | **方向三：软件研发全流程协同**（缺陷/需求聚合 → 代码定位 → 修复执行 → 测试验证 → 发布确认 → 复盘沉淀） |
| 核心命题 | 让 AI Agent 不只是“代码补全助手”，而是能完成从自然语言需求到可访问生产服务的端到端交付 |

---

## 2. 多 Agent 设计：以 AgentTeams（HiClaw）为协同基点

AgentTeams/HiClaw 的核心范式是 **Manager-Worker**：Manager Agent 负责任务拆解与编排，Worker Agents 在透明、可审计的协作空间中执行具体任务，人类可随时观察与介入。Loafer 的实现与该范式一一对应：

| AgentTeams 概念 | Loafer 映射 | 说明 |
|-----------------|-------------|------|
| Manager Agent | `PipelineHandler.executePipeline` + Plan Generator + Decomposer | 接收需求、生成计划、拆解模块、按依赖编排串行执行、全局状态决策 |
| Worker Agent | 计划 Agent、分解 Agent、开发 Agent、测试设计 Agent、测试执行 Agent、修复 Agent、部署 Agent、基础架构校验 Agent | 每个 Agent 职责单一、输入输出契约清晰 |
| Collaboration Hub（Matrix Room） | `modules` / `tasks` / `task_state` / `slice_history` 表 + SSE/WebSocket 事件流 | 所有 Agent 输出、中间结论、执行状态持久化并可回放 |
| Human Supervision | Web UI 实时日志、模块状态面板、一键暂停/重跑、手动标记完成 | 人类可在任意断点观察、修正、兜底 |
| Higress AI Gateway（安全与治理层） | `service` 层统一工具网关（SSH、Nginx、Git、Playwright、端口分配、数据库准备） | 敏感凭证不直接交给 Agent，统一由服务层注入；工具调用可审计 |

---

## 3. Agent Identity 清单（≥3 个不同职能 Agent）

| 序号 | Agent 名称 | 职能定义 | 输入 | 输出 | 依赖工具 | 失败处理 | 安全边界 |
|------|-----------|---------|------|------|---------|---------|---------|
| 1 | 计划生成 Agent | 从自然语言需求生成可执行的项目级计划 | 需求描述、技术栈约束 | `plan.md` / `ExecutionPlan` 记录 | Claude CLI | CLI 不可用时快速失败 | 仅读取需求，不执行外部命令 |
| 2 | 任务分解 Agent | 将计划拆分为带依赖的模块与任务 | 已确认的执行计划 | `Module` + `Task` 结构化记录 | Claude CLI | 解析失败时返回错误，Manager 终止流水线 | 输出限定 JSON 模式 |
| 3 | 开发 Agent | 按任务步骤编写/修改代码 | 任务描述、项目代码上下文 | 修改后的源码、任务摘要 | Claude CLI、本地文件系统 | 任务失败标记状态，继续后续无依赖模块 | 在项目工作目录内操作，不越权 |
| 4 | 测试设计 Agent | 根据已实现代码生成 API 与 Playwright UI 用例 | 模块需求、Go handler、前端视图 | `api_integration_test` / `web_integration_test` JSON + Playwright spec | Claude CLI | 生成失败降级为自由测试模式，不阻断流水线 | 不写库，通过后端校验后落库 |
| 5 | 测试执行 Agent | 运行集成测试与 UI 测试，判定通过/失败 | 用例 JSON、部署 URL | `tests/results/module-<id>.json` + 截图 | Shell、Playwright、curl | 内部异常记为单轮失败 | 截图目录严格校验，防止路径穿越 |
| 6 | 修复 Agent | 读取失败明细，自动修改代码 | 上一轮失败报告、项目源码 | 修复后的源码 | Claude CLI | 修复异常继续下一轮测试 | 仅在项目目录内修改 |
| 7 | 部署 Agent | 构建、打包、上传、启动、Nginx 反向代理 | 项目工作目录 | 访问 URL、部署记录 | SSH、tar、Nginx、本地 Go/Node 工具链 | 部署失败视为一轮测试失败 | 最小权限 SSH key、端口隔离 |
| 8 | 基础架构校验 Agent | 验证基础设施模块（DB、配置等）可构建启动 | 基础设施模块代码 | 构建/启动结果 | Shell、go build | 失败即终止流水线 | 仅做编译与启动校验 |

---

## 4. 多 Agent 闭环说明

### 4.1 任务输入
- 用户在 Web UI 提交自然语言需求（如“做一个带登录的轻量待办应用”）。
- 系统也可接收来自 Git Issue、用户反馈等外部渠道的半结构化输入。

### 4.2 任务拆解
- Manager Agent（Pipeline）调用计划生成 Agent → 产出项目计划。
- 计划自动确认后，调用任务分解 Agent → 产出模块（基础设施 / 业务）与任务，持久化到 `modules` / `tasks` 表，并标注 `sequence_number` 与 `blocked_by`。

### 4.3 上下文传递
- **结构化上下文**：Agent 间通过数据库表（`execution_plan`、`module`、`task`、`task_state`、`slice_history`、`test_run`）传递状态。
- **文件上下文**：代码产物、测试 spec、结果 JSON、截图保存在项目工作目录，后续 Agent 可读。
- **提示词上下文**：修复 Agent 的 prompt 携带上一轮 `failures` 数组全文；测试 Agent 的 prompt 携带落库用例 JSON。
- **实时上下文**：SSE/WebSocket 将 Agent 输出实时推送给前端，实现人类监督。

### 4.4 工具调用
- 所有外部工具调用统一通过 `service` 层网关，避免 Agent 直接接触密钥或执行危险命令。
- 当前实现：SSH（远程部署）、Git（仓库管理）、Nginx（反向代理）、Playwright（UI 测试）、Shell/curl（API 测试）、数据库准备服务。
- 未来映射：将上述工具抽象为 MCP Server，Agent 通过统一 MCP Client 调用。

### 4.5 结果验证
- 业务模块：API 测试 + Playwright UI 测试 + 全局冒烟测试。
- 基础设施模块：`go build` + 启动校验。
- 结果结构化写入 JSON，后端解析后落库并更新模块状态。

### 4.6 执行证据沉淀
- 每次 Agent 调用通过 `cli.RecordCall` 记录 prompt/output/exit_code/workdir。
- 测试运行记录 `test_run` 保存每轮结果。
- 测试截图 `tests/results/screenshots/` 按模块/场景归档。
- 部署记录 `project_deployment` 保存访问 URL、PID、Nginx 配置。

### 4.7 审批与回滚
- 高风险动作（如覆盖式部署）通过 `deployService.Deploy(force bool)` 控制，强制重部署需 UI 显式触发。
- 自动测试 3 轮耗尽后流水线暂停，等待人工确认后再重跑。
- 手动“标记完成”作为人工兜底。
- 回滚：可基于 Git 历史或上一版本部署目录恢复（当前已实现重新部署，完整回滚为后续增强）。

### 4.8 经验沉淀
- 成功案例的 `ExecutionPlan`、`Module`、`Task`、测试用例可作为模板复用。
- 失败案例的 `failures` 与修复过程可用于训练/优化修复 Agent 的 prompt。
- 后续计划沉淀为标准 Skill 与 Runbook。

---

## 5. Skill 工程体系（本赛题必选项）

**落地现状（如实说明）**：Loafer 的每个 Agent 能力都有清晰的输入/输出/失败处理契约，能力执行由各 executor 承载（`plan/generator.go`、`executor/decomposer.go`、`executor/task_executor.go`、`executor/test_designer.go`、`executor/test_executor.go`、`service/deploy_service.go`、`handler/infra*.go`）。在此基础上，Skill 能力契约层已落地为代码：

- 代码工件：`backend-go/internal/engine/skill`（Skill 结构体 + Registry 注册表 + 8 个内置 Skill 契约）
- API：`GET /projects/:id/available-skills` 返回全部 Skill 契约（原先为桩，现已接线）
- 单元测试：`skill_test.go` 校验注册/去重/契约字段完整性

**第一层=能力契约与注册**（已完成）；**第二层=Skill 与执行器绑定调用、以 MCP 形式对外暴露**（复赛路线图）。下表为 8 个 Skill 的契约定义，覆盖评审核验的「输入输出 / 调用条件 / 依赖工具 / 失败处理 / 验证方式 / 复用价值 / 版本演进 / 开源分发设计」8 个维度（前三表列 + §5.1 补充表）：

| Skill 名称 | 用途 | 输入 | 输出 | 调用条件 | 依赖工具 | 失败处理 | 安全边界 | 复用价值 |
|------------|------|------|------|---------|---------|---------|---------|---------|
| PlanSkill | 需求→计划 | 需求文本 | 执行计划 | 项目创建/重编 | Claude CLI | 失败终止 | 只读 | 同类项目复用计划模板 |
| DecomposeSkill | 计划→模块/任务 | 已确认计划 | 结构化模块任务 | 计划确认后 | Claude CLI | 失败终止 | JSON Schema 校验 | 复用拆分策略 |
| CodeSkill | 按任务写代码 | 任务描述+代码上下文 | 源码变更 | 任务待执行 | Claude CLI + FS | 任务失败 | 工作目录沙箱 | 跨项目复用编码模式 |
| TestDesignSkill | 自动生成测试用例 | 模块需求+已实现代码 | API/Web 用例 JSON + spec | 业务模块任务完成后 | Claude CLI | 降级自由测试 | 后端校验落库 | 用例模板沉淀 |
| TestExecuteSkill | 执行测试并输出结果 | 用例+部署 URL | 结果 JSON + 截图 | 部署后 | Shell + Playwright | 单轮失败 | 截图目录隔离 | 标准测试协议 |
| FixSkill | 按失败报告修复代码 | failures + 源码 | 修复后源码 | 测试失败后 | Claude CLI | 继续下一轮 | 同 CodeSkill | 修复策略沉淀 |
| DeploySkill | 构建、上传、启动、配置 Nginx | 项目源码 | 访问 URL | 模块/全局交付 | SSH + Nginx + tar | 视为测试失败 | 最小权限、端口隔离 | 标准化部署契约 |
| InfraVerifySkill | 基础设施模块构建启动校验 | 基础设施代码 | 校验结果 | 基础设施模块完成后 | go build | 终止流水线 | 本地编译 | 环境基线校验 |

**Skill 与多 Agent 协同流程的关系**：Skill 是任务能力抽象层，Agent 是执行者。当前每个 Agent 的能力与一个 Skill 契约一一对应（如 PlanAgent→PlanSkill、TestAgent→TestExecuteSkill），契约经注册表统一管理、可审计；未来 Skill 将以 MCP Tool 形式对外暴露，供第三方 Agent 复用。

### 5.1 验证方式 / 版本演进 / 开源分发设计（补充核验维度）

| Skill | 验证方式 | 版本演进 | 开源分发设计 |
|-------|---------|---------|-------------|
| PlanSkill | 计划落库后进入分解阶段，由分解 Agent 校验可执行性；人工可在 UI 预览确认 | v1.0；计划模板与拆分策略迭代时升级契约，变更记入 changelog | 计划提示词模板 + 计划 Schema（plan.md）分发于仓库 docs/，Apache-2.0 |
| DecomposeSkill | 输出经 JSON Schema 校验 + blocked_by 依赖一致性检查通过后落库 | v1.0；模块/任务 Schema（sequence_number/blocked_by）变更需升级契约 | 模块/任务 JSON Schema + 分解提示词模板分发，Apache-2.0 |
| CodeSkill | 代码正确性由自动测试门禁验证（部署 + API/Playwright UI 测试），通过才置为完成 | v1.0；任务执行上下文构建策略迭代时升级契约 | 任务执行提示词模板 + 上下文契约分发，Apache-2.0 |
| TestDesignSkill | 后端校验 JSON 合法性后落库；生成的用例可被 TestExecuteSkill 执行验证 | v1.0；用例 Schema 与场景命名规范演进时升级契约 | 用例 JSON Schema + Playwright spec 模板分发，Apache-2.0 |
| TestExecuteSkill | 结果 JSON + 截图作为通过/失败判定依据，按场景回写模块用例与状态 | v1.0；结果契约扩展需保持向后兼容（无 scenarios 时回退模块级判定） | 测试结果 JSON 契约 + 截图命名规范分发，Apache-2.0 |
| FixSkill | 修复后重新部署测试，由下一轮测试结果验证修复是否有效 | v1.0；失败报告解析与修复提示词策略迭代时升级契约 | 修复提示词模板 + failures 数据契约分发，Apache-2.0 |
| DeploySkill | 构建成功 + 后端启动校验 + 访问 URL 可访问性检查 | v1.0；部署契约（env 注入 / 端口 / Nginx 配置）演进时升级契约 | 部署脚本（deploy-local.sh）+ 运行时环境变量契约分发，Apache-2.0 |
| InfraVerifySkill | 编译（go build）+ 启动校验，通过才置为完成 | v1.0；校验步骤随技术栈与依赖基线扩展时升级契约 | 构建/校验脚本 + 环境基线说明分发，Apache-2.0 |

---

## 6. MCP 与工具集成（推荐可选项）

### 6.1 当前等价集成契约

Loafer 当前未直接接入 MCP Server，但已具备等价于 MCP 的工具抽象：

| 工具名 | 调用入口 | 参数 Schema | 返回结构 | 鉴权 | 失败重试 | 幂等控制 | 审计日志 | 降级方式 |
|--------|---------|------------|---------|------|---------|---------|---------|---------|
| SSHDeployTool | `deployService.Deploy` | `{projectID, force}` | `Deployment{AccessURL, BackendPID, ...}` | SSH Key（环境变量 `INFRA_SSH_KEY_PATH`） | 部署失败由修复 Agent 重试 | force 参数控制 | `project_deployment` 表 | 本地部署 |
| PlaywrightTool | `playwrightService.RunTest` | `{projectID, specFile, testName, kind}` | `TestRun{Status, PassCount, ...}` | 本地 npx | 单轮失败 | 每次清空旧截图 | `test_run` 表 | 可访问性检查 |
| APIScriptTool | `scriptRunner.RunAPIScenarios` | `{moduleID, scenarios, baseURL}` | `ModuleTestResult` | 无 | 单轮失败 | 每次新脚本 | `test_run` 表 | 自由测试模式 |
| GitTool | `giteeService.*` | `{repoURL, token}` | 操作结果 | Git Token | 手动重试 | commit 幂等 | Git 历史 | 本地工作目录 |
| NginxTool | `nginxManager.ApplyConfig` | `{port, buildDir}` | 配置文件路径 | root | 部署失败重试 | 配置覆盖 | `project_deployment` | 直接端口访问 |

### 6.2 迁移到 MCP 的路径

- 将每个工具封装为独立 MCP Server（`mcp-server-ssh-deploy`、`mcp-server-playwright`、`mcp-server-api-test` 等）。
- Agent 通过 MCP Client 调用，参数/返回遵循 MCP Schema。
- 当前等价契约已具备 Schema、鉴权、审计、降级设计，迁移时只需协议适配，无需重写调用链。

---

## 7. 可观测设计（推荐可选项）

| 数据类型 | 采集方式 | 存储 | 用途 |
|----------|---------|------|------|
| Trace | `cli.RecordCall` 记录每次 Agent 调用的 prompt/output/exit_code/session_uuid | `claude_call_records` 表 | 追踪 Agent 调用链、定位失败节点 |
| Log | SSE/WebSocket 实时推送 + `app.log` / `backend.log` | 文件 + 前端 | 实时监督与问题排查 |
| Metrics | `test_run` 表统计通过率/轮次/耗时；部署表统计成功率 | MySQL | 量化质量与效率 |
| 截图 | Playwright 终态/失败截图 | `tests/results/screenshots/` | UI 测试证据 |

后续将按 OpenTelemetry GenAI 语义规范补全 Span/Attribute，对接 LoongSuite / AgentScope Studio / AgentLoop。

---

## 8. RAG 与上下文增强（推荐可选项）

已实现的上下文增强能力（4 项中 ≥2 项）：

| 能力 | 实现 | 说明 |
|------|------|------|
| Agent 记忆存储 | `task_state` / `slice_history` 保存任务执行历史与分片 | Agent 续跑时可读取历史 |
| 共享状态管理 | `modules` / `tasks` 状态机 + `test_run` 运行记录 | 多 Agent 共享统一状态 |
| 轨迹可观测 | `claude_call_records` 全量调用记录 | 完整推理轨迹可回放 |
| 知识库 RAG | 暂未实现 | 后续将沉淀成功案例为 Runbook，通过 RAG 提升计划/测试设计质量 |

---

## 9. 与评分标准的对应关系

| 评分维度 | 权重 | Loafer 对应内容 |
|----------|------|-----------------|
| 场景价值与行业可复制性 | 25% | 软件研发全流程自动化是行业普遍痛点；Loafer 可复用到任何 Go+Vue/React 业务系统构建 |
| 多 Agent 协同与自主闭环能力 | 25% | 8 个职能 Agent、Manager-Worker 编排、结构化状态机、审批与回滚 |
| Skill 工程体系与生态复用 | 25% | 8 大 Skill 契约（输入输出/调用条件/依赖工具/失败处理/验证方式/复用价值/版本演进/开源分发）、MCP 迁移路径 |
| 工程落地、运行验证与安全可审计 | 20% | 已运行项目、自动测试门禁、截图证据、调用记录、部署日志、断点续跑 |
| 开放/开源贡献 | 5% | 项目基于开源技术栈；计划开放 Skill 契约、Agent Identity 模板、运行证据格式 |

---

## 10. 当前进展与验证证据

- **代码规模**：Go 后端 ~1.95 万行，Vue3 前端完整实现。
- **Git 提交**：137 次提交，近期集中在自动测试门禁、Playwright UI 测试、失败自动修复、截图回写。
- **已验证链路**：需求输入 → 计划生成 → 模块分解 → 任务执行 → 自动用例生成 → 部署 → API/Web 测试 → 失败修复 → 断点续跑。
- **Skill 契约层**：`internal/engine/skill` 注册表 + `GET /projects/:id/available-skills` 返回 8 个职能 Skill 契约（含验证方式/版本/版本演进/开源分发），单测校验 8 维契约完整性。
- **运行证据**：`integration-test-report.html`、模块测试截图、`test_run` 运行记录、`project_deployment` 部署记录。
