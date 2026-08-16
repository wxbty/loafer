# 方案 PPT 内容大纲

> 以下内容为 GOAI Agent Infra 赛道初赛方案 PPT 的逐页脚本，可直接用于制作 PPT/PDF。建议总页数 14-16 页，每页 3-5 个要点。

---

## Slide 1：封面

**Loafer 多 Agent 软件交付平台**

- 赛道：Agent Infra（新智基座）
- 选题：软件研发全流程协同
- 团队：[待填写]
- 日期：2026-08

---

## Slide 2：问题与场景

**企业软件研发的“最后一公里”困境**

- AI 编码助手只能补全片段，无法独立完成需求→上线全链路
- 计划、编码、测试、部署、修复跨多个工具与角色，人工切换成本高
- “Demo 可用、生产难用”：缺少自动验证、执行证据与风险边界
- 行业普遍存在：重复业务系统构建、内部工具开发、MVP 快速验证等场景

---

## Slide 3：价值主张

**从一句话需求到可访问 URL 的端到端自主闭环**

- 输入：自然语言需求（如“做一个带登录的轻量待办应用”）
- 输出：可直接访问的生产服务 URL
- 中间过程：计划→模块→任务→代码→测试→修复→部署→验证，全部自动完成
- 人类角色：设定目标、观察过程、在关键节点确认或兜底

---

## Slide 4：方案总览

**以 AgentTeams（HiClaw）Manager-Worker 为协同基点**

- Manager Agent：需求理解、计划生成、任务拆解、编排调度、全局决策
- Worker Agents：8 个职能 Agent 各司其职，按依赖串/并行执行
- 协作空间：数据库状态机 + SSE 实时事件流，所有过程透明可审计
- 工具网关：统一 SSH/Git/Nginx/Playwright/Shell 接入，敏感凭证不暴露给 Agent
- 人类监督：Web UI 实时日志、状态面板、暂停/重跑/手动完成

---

## Slide 5：Agent 身份清单

**8 个职能 Agent，覆盖软件交付全生命周期**

| Agent | 核心职责 |
|-------|---------|
| 计划生成 Agent | 需求 → 项目级执行计划 |
| 任务分解 Agent | 计划 → 带依赖的模块与任务 |
| 开发 Agent | 按任务步骤编写/修改代码 |
| 测试设计 Agent | 根据已实现代码生成 API + Web 用例 |
| 测试执行 Agent | 运行 API / Playwright UI 测试 |
| 修复 Agent | 读取失败明细，自动改代码 |
| 部署 Agent | 构建、上传、启动、配置 Nginx |
| 基础架构校验 Agent | 验证基础设施模块可构建启动 |

---

## Slide 6：任务拆解与上下文传递

**结构化状态机驱动多 Agent 协同**

- 三层拆解：ExecutionPlan → Module → Task → Step
- 依赖编排：`sequence_number` + `blocked_by`，严格串行处理业务模块质量门禁
- 上下文载体：
  - 数据库：`modules` / `tasks` / `task_state` / `slice_history` / `test_run`
  - 文件系统：`plan.md`、测试 spec、结果 JSON、截图
  - 提示词：修复 Agent 携带 `failures` 全文，测试 Agent 携带落库用例
- 状态流转：待执行 → 执行中 → 待测试 → 测试中 → 完成 / 测试失败 / 失败

---

## Slide 7：Skill 工程体系

**契约维度完整覆盖评审核验点（表格页）**

表格：契约维度 | 定义 / 核验点

| 契约维度 | 定义 / 核验点 |
|---------|--------------|
| 输入输出 | Inputs / Outputs 明确，8 个 Skill 均有 |
| 调用条件 | 何时可被 Agent 调用（如业务模块任务完成后） |
| 依赖工具 | Claude CLI / SSH / Playwright / Shell / Nginx 等 |
| 失败处理 | 快速失败 / 降级自由测试 / 修复循环 / 熔断 |
| 验证方式 | 自动测试门禁 / JSON Schema 校验 / 构建启动校验 |
| 复用价值 | 跨项目复用模板、Schema 与执行证据格式 |
| 版本演进 | 语义化版本 v1.0，变更记入 changelog，向后兼容 |
| 开源分发设计 | Schema + 提示词模板 + 部署脚本分发，Apache-2.0 |

副标题：已落地 `internal/engine/skill` 注册表 + `GET /projects/:id/available-skills` API + 单测校验契约完整性。能力执行由各 executor 承载；Skill 与执行器绑定调用、MCP Server 对外暴露列为复赛路线图。

---

## Slide 8：MCP 与工具集成

**当前等价契约 + 未来 MCP 迁移路径**

- 当前工具：SSHDeployTool、PlaywrightTool、APIScriptTool、GitTool、NginxTool
- 等价契约已明确：调用入口、参数 Schema、返回结构、鉴权、失败重试、幂等控制、审计日志、降级方式
- 迁移到 MCP：将每个工具封装为独立 MCP Server，Agent 通过 MCP Client 调用
- 优势：当前设计已完成 Schema 与审计，迁移只需协议适配，无需重写调用链

---

## Slide 9：核心闭环——自动测试门禁

**业务模块任务完成后触发：部署 → 测试 → 修复，最多 3 轮**

1. 第 0 步：测试设计 Agent 生成 API / Playwright 用例（幂等）
2. 第 1-3 轮：部署 Agent 全量部署 → 测试 Agent 执行用例
3. 失败 → 修复 Agent 读取失败明细改代码 → 回到第 2 步
4. 通过 → 模块置为完成，继续下一模块
5. 3 轮耗尽 → 流水线暂停，等待人工确认后断点续跑

---

## Slide 10：可观测、审计与断点续跑

**让多 Agent 系统从 Demo 走向 Production**

- Trace：`claude_call_records` 记录每次 Agent 调用的 prompt/output/exit_code/session
- Log：SSE/WebSocket 实时推送 + `app.log` / `backend.log`
- Metrics：`test_run` 统计通过率/轮次/耗时；部署表统计成功率
- 证据：Playwright 终态/失败截图、部署记录、测试 JSON
- 断点续跑：基于模块/任务状态机，重启流水线时自动从最近完成点恢复

---

## Slide 11：安全边界与风险控制

**高风险动作必须可审计、可暂停、可兜底**

- 敏感凭证：SSH Key、数据库密码、JWT Secret 由服务层注入，不交给 Agent
- 部署安全：`force` 参数显式控制覆盖式重部署
- 失败熔断：基础架构模块失败立即终止流水线；业务模块失败进入自动修复循环
- 人工兜底：自动测试 3 轮耗尽后暂停；UI 提供“标记完成”跳过异常
- 审计：所有 Agent 调用、工具执行、状态变更持久化到数据库

---

## Slide 12：工程落地可行性

**技术栈与运行状态**

- 后端：Go 1.21 + Gin + GORM + MySQL（约 1.95 万行 Go 代码）
- 前端：Vue 3 + TypeScript + Element Plus + Vite
- Agent 执行层：Claude Code CLI（--print 离线模式）
- 部署：本地构建 + tar 流上传远程服务器 + Nginx 反向代理
- 当前状态：已实现全链路自动化，完成多个 Demo 项目端到端交付

---

## Slide 13：当前进展与验证证据

**137 次提交，近期聚焦自动测试门禁与质量闭环**

- 自动用例生成：API + Playwright 场景由测试设计 Agent 自动生成并落库
- 场景级执行：按名匹配回写模块用例，前端面板展示通过/失败与截图
- 失败自动修复：开发 Agent 读取失败明细，修复后重部署重测
- 截图与报告：终态图、错误图、JSON 结果报告、`integration-test-report.html`
- 断点续跑：历史空用例模块重启自动补生成，失败模块修复后可恢复

---

## Slide 14：开放与开源计划

**从内部平台到可复用 Agent Infra**

- 开源 Skill 契约：Agent Identity 模板、输入输出 Schema、失败处理规范
- 开源运行证据格式：测试结果 JSON、截图命名规范、调用记录结构
- 贡献 MCP Server：将部署、测试、Git、Nginx 工具封装为标准 MCP Server
- 社区场景：支持更多技术栈（Python/Java）、更多测试框架、CI/CD 集成
- 许可证：计划采用 Apache 2.0 或 MIT

---

## Slide 15：总结

**Loafer = 软件研发领域的 Agent Infra 实践**

- 以 AgentTeams Manager-Worker 为设计基点，8 个 Agent 完成端到端交付
- Skill 抽象 + 等价 MCP 契约，兼顾当前落地与未来生态兼容
- 自动测试门禁、可观测、审计、断点续跑，具备 Production 潜力
- 已验证可行，计划开源，目标成为企业软件交付的多 Agent 基座

---

## 附录页（可选）：与评分标准对照

| 评分维度 | 权重 | Loafer 对应内容 |
|----------|------|-----------------|
| 场景价值与行业可复制性 | 25% | 软件研发全流程自动化，Go+Vue 业务系统快速构建 |
| 多 Agent 协同与自主闭环能力 | 25% | Manager-Worker 架构、状态机、上下文传递、审批与回滚 |
| Skill 工程体系与生态复用 | 25% | 8 大 Skill 清单、输入输出契约、MCP 迁移路径 |
| 工程落地、运行验证与安全可审计 | 20% | 已运行 Demo、测试门禁、截图证据、调用记录、部署日志 |
| 开放/开源贡献 | 5% | 计划开源 Skill 契约、运行证据格式、MCP Server |
