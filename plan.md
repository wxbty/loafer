# Loafer 项目开发计划

## 一、项目概述

Loafer 是 claude_sprint 项目的功能子集，目标是全自动项目开发平台。功能上只包含 CC终端和模块任务，通过 CC终端拆解一个复杂项目成 plan.md，然后通过模块任务拆解成具体任务后循环执行。

### 基本信息

- **来源**: Fork claude_sprint 后裁剪
- **Git**: 新仓库，不保留 claude_sprint 历史
- **Maven**: com.loafer:loafer-agent
- **Java 包名**: com.loafer.agent
- **数据库**: 同 MySQL 实例新建库 `loafer`，Liquibase 从零开始
- **端口**: 后端 9080，前端 9081
- **部署**: 独立部署，保留 Nginx
- **不用 Redis**

---

## 二、保留功能清单

### 2.1 项目管理 (Project)

只保留核心字段：id、name、description、status、work_dir、git_url、dev_language、created_at、updated_at、deleted。

删除字段：cli_os_user、test_cases_json、env_vars_json、prompt_vars_json、data_dir、multi_repo_json、frontend_url_override、nginx 相关、wechat 相关、ai_employee 相关、profile_id、init_deploy_script_template 等。

### 2.2 CC终端 (Session)

- ClaudeSessionController — 会话 CRUD、resume、会话池管理
- CliProcessManager — Claude Code CLI 进程管理（PTY）
- SessionPool — 会话池（创建/销毁/超时清理）
- SessionHandle — 会话句柄
- CheckpointManager — 检查点管理
- CachedContext — 上下文缓存
- TerminalWebSocketController — WebSocket 终端
- WebSocketService — WebSocket 服务
- XtermComponent (前端) — xterm.js 终端组件
- InteractiveClaudeTerminal (前端) — 交互式终端组件

### 2.3 模块任务 (Module + Task)

- ModuleController / ModuleService / ModuleMapper / Module — 模块 CRUD
- TaskController / TaskService / TaskMapper / Task — 任务 CRUD
- TaskStateController / TaskStateService / TaskStateMapper / TaskState — 任务状态
- ChecklistItemController / ChecklistItemService / ChecklistItemMapper / ChecklistItem — 子任务清单
- SliceHistoryController / SliceHistoryService / SliceHistoryMapper / SliceHistory — 分片历史
- DecisionLogController / DecisionLogService / DecisionLogMapper / DecisionLog — 决策日志
- DependencyGraphController / DependencyGraphService / DependencyGraphMapper / DependencyGraph — 依赖图谱
- EnvironmentStateController / EnvironmentStateService / EnvironmentStateMapper / EnvironmentState — 环境状态
- CheckpointController / CheckpointService / CheckpointMapper / Checkpoint — 检查点
- SliceExecutor / SliceManager / SliceScheduler — 分片执行引擎
- ModuleDecomposeService — 模块拆解（从 plan.md 识别模块和任务）
- TaskDecomposeService — 任务拆解
- TaskClaudeCodeExecuteService — 任务 Claude Code 执行
- TaskExecuteStreamService — 任务执行流
- TaskExecutionSummaryStreamService — 执行总结流
- ModuleSchedulerService — 模块调度

### 2.4 TDD 流水线（完整保留）

- ModuleTddExecutor — TDD 执行器
- ModuleTddManualRunService — TDD 手动运行
- TddPromptBuilder — TDD 提示词构建
- TddAssertionRunner — 断言运行器
- TddRetryChainService / TddRetryChain — TDD 重试链
- TddSelfHealingExecutor — TDD 自愈执行器
- ModuleFixService — 模块修复服务
- ModuleFixHistory / ModuleFixHistoryMapper — 修复历史
- IntegrationTestExecutor — API 集成测试执行器
- WebIntegrationTestExecutor — Web 集成测试执行器
- ProgrammaticAssertionExecutor — 编程断言执行器
- PreconditionValidator / PreconditionValidatorRegistry — 前置条件验证
- UserExistsValidator / UserNotExistsValidator — 用户存在性验证
- TestResourceCleaner / TestResourceNamespacer — 测试资源管理
- ScenarioRunService — 场景运行
- ModuleTestGenerateService — 模块测试生成

### 2.5 执行辅助服务（全部保留）

- ContextInjectionService — 上下文注入
- ExecutionStateManager — 执行状态管理
- SessionRecoveryService — 会话恢复
- TaskRecoveryService — 任务恢复
- TaskHandoffContextService — 任务交接上下文
- LogSummaryService — 日志摘要
- OutputStructuredParser — 输出解析
- EvidenceValidator — 证据验证
- StepByStepExecutor — 逐步执行器
- ParallelExecutionService — 并行执行

### 2.6 其他保留

- PromptTemplate / PromptTemplateService / PromptTemplateMapper / PromptTemplateController — 提示词模板
- SystemConfig / SystemConfigService / SystemConfigMapper / SystemConfigController — 系统配置
- DeployService / ProjectDeployLocalService / DeployController — 部署服务
- deploy-*.sh 脚本 — 部署脚本
- Nginx 配置 — 反向代理 + WebSocket
- FileController — 文件操作
- RuntimeStatusController — 运行时状态
- SpaController — SPA 路由
- AccessUrlService — 访问 URL 服务
- FailureAnalysisService — 失败分析
- MentionResolverService — @提及解析
- ProjectService — 项目服务（精简后）
- UserService — 用户服务（精简为配置文件校验）
- PlaceholderResolver — 占位符解析
- ProjectEnvVarsRenderer / ProjectEnvVarUtils — 环境变量渲染
- ProjectRepos — 项目仓库工具
- ClaudePathResolver / ClaudeProfileResolver / ClaudeSudoEnvArgs — Claude 路径工具
- AnsiUtils — ANSI 工具
- SudoPathHelper — Sudo 路径工具
- SqlMigrationExecutor — SQL 迁移执行器

---

## 三、删除功能清单

### 3.1 用户认证系统 → 简化为单用户

- 删除 User 实体、UserMapper、UserProject、UserProjectMapper
- 删除 user 表、user_project 表
- 删除 AuthInterceptor（或简化为 token 校验）
- 删除 AuthController 中的注册/多用户逻辑，只保留登录（校验 application.yml 中的硬编码账号密码）
- 删除 PasswordEncoderConfig（或保留用于密码比对）
- 删除 JwtUtil 中的多用户逻辑，简化为单用户 token 签发
- 账号密码写在 application.yml 中

### 3.2 微信小程序

- 删除 app/ 目录（uni-app 小程序前端）
- 删除 MiniProgramController、MiniProgramAuthService、MiniProgramSessionService、MiniProgramChatMessageService、MiniProgramProjectService、MiniProgramPublishService、MiniProgramPushService、MiniProgramDeployService、MiniProgramAgentExecutor
- 删除 MiniProgramSession、MiniProgramChatMessage、MiniProgramDeployTask 实体及 Mapper
- 删除 MiniProgramWebSocketHandler
- 删除 WeChatMiniProgramConfig
- 删除 pom.xml 中的 weixin-java-miniapp 依赖
- 删除 application.yml 中的 wechat.mini-program 配置

### 3.3 AI员工

- 删除 AiEmployeeController、AiEmployeeService、AiEmployeeTaskService、ProjectAiEmployeeService、GroupAgentDispatcherService
- 删除 AiEmployee、AiEmployeeTask、ProjectAiEmployee 实体及 Mapper
- 删除 ai_employee、ai_employee_task、project_ai_employee 表
- 删除 AgentAssignment、AgentMessage DTO
- 删除 .claude/agents/developer.md、.claude/agents/tester.md

### 3.4 服务器管理

- 删除 ServerController、ServerService、ProjectServerService、ProjectServerContextService、ServerProbeService
- 删除 Server、ProjectServer 实体及 Mapper
- 删除 ServerDTO、ServerVO、ProjectServerDTO、ProjectServerVO
- 删除 SshWebSocketHandler、SshWebSocketConfig
- 删除 AesGcmEncryptor、PasswordEncryptor
- 删除 SshCommandExecutor
- 删除 server、project_server 表
- 删除 pom.xml 中的 jsch 依赖
- 删除前端 SshTerminal 组件

### 3.5 聊天记录

- 删除 ChatRecordController
- 删除 chat_record 表相关代码

### 3.6 离线终端

- 删除 OfflineTerminalController、OfflineTerminalExecutor、OfflineTerminalJobRunner、OfflineTerminalJobService
- 删除 OfflineTerminalJob 实体及 Mapper
- 删除 offline_terminal_job 表
- 删除前端 OfflineTerminalDrawer 组件

### 3.7 工作流

- 删除 WorkflowOrchestratorService、ScenarioRunService（如仅工作流使用）
- 删除 WorkflowRun 实体及 Mapper
- 删除 workflow_run 表

### 3.8 视觉分析

- 删除 VisionController

### 3.9 语音识别

- 删除 SpeechController
- 删除 pom.xml 中的 aliyun-java-sdk-core 依赖（如仅语音使用）

### 3.10 登录日志

- 删除 LoginLogController、LoginLogService
- 删除 LoginLog 实体及 Mapper
- 删除 login_log 表
- 删除前端 LoginLogs 页面

### 3.11 LLM 调用日志

- 删除 LlmCallLogController、LlmCallLogService
- 删除 LlmCallLog 实体及 Mapper
- 删除 llm_call_log 表
- 删除 LlmCallTypes 常量
- 删除前端 LlmLogs 页面

### 3.12 ProjectMemo

- 删除 ProjectMemoController、ProjectMemoService
- 删除 ProjectMemo 实体及 Mapper
- 删除 project_memo 表
- 删除前端 MemoEditor 组件

### 3.13 ProjectPortAllocation

- 删除 PortAllocationController、ProjectPortAllocationService
- 删除 ProjectPortAllocation 实体及 Mapper
- 删除 project_port_allocation 表

### 3.14 TestConfig

- 删除 TestConfigController、TestConfigService
- 删除 TestConfig 实体及 Mapper
- 删除 test_config 表
- 删除前端 TestConfigForm 组件

### 3.15 TestUser

- 删除 TestUserController
- 删除 test_user 相关代码

### 3.16 防护栏系统

- 删除 GuardrailConfigController、GuardrailConfigService、GuardrailValidator、SensitiveOperationDetector、CommandInterceptor
- 删除 GuardrailConfig 实体及 Mapper
- 删除 guardrail_config 表

### 3.17 GitService

- 删除 GitService

### 3.18 ToolsController

- 删除 ToolsController

### 3.19 邮件通知

- 删除 NotificationService
- 删除 pom.xml 中的 spring-boot-starter-mail 依赖
- 删除 application.yml 中的 mail 配置
- 删除 UserNotificationSettings 前端组件

### 3.20 前端页面删除

- AiEmployees.vue
- ChatRecords.vue
- Servers.vue
- LlmLogs.vue
- DatabaseExplorer.vue
- TaskMonitor.vue
- Claude.vue
- TestDashboard.vue
- TestTerminal.vue
- TerminalDebug.vue
- LoginLogs.vue
- About.vue

### 3.21 前端组件删除

- AgentManagerModal.vue
- AssistanceRequestPanel.vue
- EnterpriseInfoCard.vue
- LogViewer.vue
- MemoEditor.vue
- NotificationCenter.vue
- OfflineTerminalDrawer.vue
- SshTerminal.vue
- UserNotificationSettings.vue
- WebIntegrationTestEditor.vue（如仅 TestConfig 使用）
- AddStepsDialog.vue（如仅 AI 员工使用）

### 3.22 前端 API 删除

- aiEmployee.ts
- chatRecords.ts
- claude.ts（如仅 AI 员工使用）
- llmCallLogs.ts
- loginLogs.ts
- offlineTerminal.ts
- projectServer.ts
- settings.ts（部分）
- testUser.ts
- tools.ts
- userSettings.ts
- monitoring.ts

---

## 四、数据库设计

### 4.1 新建数据库

```sql
CREATE DATABASE IF NOT EXISTS loafer CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

### 4.2 保留的表（精简后）

| 表名 | 用途 | 备注 |
|------|------|------|
| project | 项目表 | 只保留核心字段 |
| module | 模块表 | 完整保留 |
| task | 任务主表 | 完整保留 |
| task_state | 任务实时状态 | 完整保留 |
| checklist_item | 子任务清单项 | 完整保留 |
| slice_history | 分片执行历史 | 完整保留 |
| decision_log | 决策日志 | 完整保留 |
| dependency_graph | 依赖关系 | 完整保留 |
| environment_state | 环境状态 | 完整保留 |
| checkpoint | 检查点 | 完整保留 |
| prompt_template | 提示词模板 | 完整保留 |
| system_config | 系统配置 | 完整保留 |
| module_fix_history | 模块修复历史 | TDD 相关 |
| tdd_retry_chain | TDD 重试链 | TDD 相关 |

### 4.3 删除的表

user、user_project、server、project_server、project_port_allocation、project_memo、test_config、guardrail_config、llm_call_log、login_log、ai_employee、ai_employee_task、project_ai_employee、mini_program_session、mini_program_chat_message、mini_program_deploy_task、offline_terminal_job、workflow_run、chat_message

### 4.4 project 表精简后字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT AUTO_INCREMENT | 项目ID |
| name | VARCHAR(255) | 项目名称 |
| description | TEXT | 项目描述 |
| status | INT DEFAULT 0 | 状态：0-待启动，1-进行中，2-已暂停，3-已完成 |
| work_dir | VARCHAR(500) | 项目工作目录 |
| git_url | VARCHAR(255) | Git仓库地址 |
| dev_language | VARCHAR(100) | 开发语言 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |
| deleted | INT DEFAULT 0 | 逻辑删除 |

### 4.5 Liquibase 策略

- 从零开始，不继承 claude_sprint 的 changeSet
- master changelog: `src/main/resources/db/changelog/db.changelog-master.yaml`
- 变更文件放在 `src/main/resources/db/changelog/changes/`
- 第一个 changeSet 包含所有表的 CREATE TABLE 语句

---

## 五、核心流程

### 5.1 全自动项目开发流程

```
用户输入项目描述
    ↓
CC终端对话拆解 → 生成 plan.md
    ↓
用户手动触发"识别 plan.md"
    ↓
ModuleDecomposeService 解析 plan.md → 创建模块和任务
    ↓
模块任务自动循环执行（分片执行机制）
    ├─ 默认全自动执行
    └─ 关键决策点暂停等用户确认
    ↓
TDD 流水线验证
    ↓
项目完成
```

### 5.2 分片执行循环

复用 claude_sprint 的 SliceExecutor + SliceScheduler + SliceManager 机制：

1. 进入仪式：查询 task_state 获取当前状态，找到 pending 的 checklist_item
2. 执行开发：Claude Code 执行任务
3. 自测验证
4. Git 提交
5. 退出仪式：数据库事务更新 checklist_item、task_state、slice_history
6. 判断是否继续下一片

### 5.3 TDD 流水线

完整保留 5 阶段断言驱动流程：

1. 阶段1：需求分析 → 生成验收标准
2. 阶段2：编写测试 → 生成测试代码
3. 阶段3：实现功能 → 编写生产代码
4. 阶段4：运行测试 → 验证通过
5. 阶段5：重构优化 → 代码质量提升

包含 API 集成测试（curl）和 Web 集成测试（Playwright）。

---

## 六、配置变更

### 6.1 application.yml 变更

```yaml
spring:
  application:
    name: loafer-agent
  datasource:
    url: jdbc:mysql://${DB_HOST:127.0.0.1}:${DB_PORT:3306}/loafer?useUnicode=true&characterEncoding=utf-8&useSSL=false&serverTimezone=Asia/Shanghai&allowPublicKeyRetrieval=true&sessionVariables=character_set_client=utf8mb4,character_set_connection=utf8mb4,character_set_results=utf8mb4
    username: ${DB_USERNAME:root}
    password: ${DB_PASSWORD:}
  liquibase:
    enabled: true
    change-log: classpath:/db/changelog/db.changelog-master.yaml

server:
  port: 9080

# 单用户认证配置
app:
  auth:
    username: ${APP_AUTH_USERNAME:admin}
    password: ${APP_AUTH_PASSWORD:your_password}
  session-pool:
    max-size: ${SESSION_POOL_MAX_SIZE:5}
    idle-timeout: ${SESSION_POOL_IDLE_TIMEOUT:40}
```

### 6.2 pom.xml 变更

- groupId: com.loafer
- artifactId: loafer-agent
- 删除依赖：weixin-java-miniapp、jsch、aliyun-java-sdk-core、spring-boot-starter-mail
- 保留依赖：spring-boot-starter-web、spring-boot-starter-websocket、mysql-connector、liquibase-core、mybatis-plus、lombok、pty4j、spring-security-crypto、jjwt、jackson-datatype-jsr310

### 6.3 前端配置变更

- vite.config.ts: proxy target 改为 http://localhost:9080
- .env.development: API 地址改为 9080
- router: 只保留 Projects、ProjectDetail、Settings、Login 路由

---

## 七、实施步骤

### Phase 1: 项目初始化

1. 从 claude_sprint 复制完整代码到 loafer
2. 修改 Maven 坐标（pom.xml）
3. 修改 Java 包名（com.claude.agent → com.loafer.agent）
4. 修改 application.yml（数据库名、端口、认证配置）
5. 修改前端配置（端口、路由）
6. 删除不需要的 Maven 依赖
7. 创建 loafer 数据库
8. 编写 Liquibase 初始 changeSet（所有保留表的 CREATE TABLE）
9. 验证项目能编译启动

### Phase 2: 后端裁剪

1. 删除微信小程序相关代码（MiniProgram*）
2. 删除 AI 员工相关代码（AiEmployee*、GroupAgentDispatcher）
3. 删除服务器管理相关代码（Server*、ProjectServer*、Ssh*、AesGcm*）
4. 删除聊天记录相关代码（ChatRecord*）
5. 删除离线终端相关代码（OfflineTerminal*）
6. 删除工作流相关代码（Workflow*）
7. 删除视觉/语音相关代码（Vision*、Speech*）
8. 删除登录日志相关代码（LoginLog*）
9. 删除 LLM 调用日志相关代码（LlmCallLog*）
10. 删除 ProjectMemo 相关代码
11. 删除 ProjectPortAllocation 相关代码
12. 删除 TestConfig 相关代码
13. 删除 TestUser 相关代码
14. 删除 Guardrails 相关代码
15. 删除 GitService
16. 删除 ToolsController
17. 删除邮件通知相关代码
18. 简化用户认证（删除 User 表，配置文件存储账号密码）
19. 精简 Project 实体（只保留核心字段）
20. 修复所有编译错误（删除代码引起的引用断裂）

### Phase 3: 前端裁剪

1. 删除不需要的页面（AiEmployees、ChatRecords、Servers 等）
2. 删除不需要的组件（AgentManagerModal、SshTerminal 等）
3. 删除不需要的 API 模块
4. 精简路由配置
5. 精简侧边栏/导航菜单
6. 修改登录页（简化为单用户登录）
7. 修改 ProjectDetail 页面（删除服务器/AI员工相关 tab）
8. 修复所有编译错误

### Phase 4: 数据库与迁移

1. 创建 loafer 数据库
2. 编写 Liquibase master changelog
3. 编写初始 changeSet（所有保留表的 CREATE TABLE + 初始数据）
4. 验证 Liquibase 迁移能正确执行
5. 删除历史 SQL 脚本（db/init.sql、db/001~024 等）

### Phase 5: 集成测试

1. 启动后端验证所有 API 可用
2. 启动前端验证页面正常
3. 测试 CC终端功能（创建会话、WebSocket 连接、命令注入）
4. 测试模块任务流程（创建项目 → 拆解 → 执行）
5. 测试 TDD 流水线
6. 测试部署脚本

---

## 八、风险与注意事项

1. **包名重命名工作量大**: com.claude.agent → com.loafer.agent 涉及所有 Java 文件，需全局替换，注意字符串引用（如 mapper XML 中的全限定类名）
2. **代码依赖链**: 删除某个 Service 可能导致其他 Service 编译失败，需逐个修复
3. **数据库兼容**: loafer 的表结构是 claude_sprint 的子集，但字段可能有差异（project 表精简），需确保 SQL 正确
4. **前端组件依赖**: 删除某个组件可能导致其他组件引用断裂，需检查 import
5. **Liquibase 从零开始**: 不继承 claude_sprint 的 changeSet，需确保所有表结构完整
6. **JWT 简化**: 删除 user 表后，JWT 签发和校验逻辑需适配配置文件方式
