// Package skill 提供 Loafer 多 Agent 系统的 Skill 能力抽象层。
//
// Skill 是任务能力抽象层：每个 Skill 描述一个可复用、有清晰输入输出与失败
// 处理契约的 Agent 能力。当前第一层落地为「能力契约 + 注册表」，能力的
// 具体执行仍由各 executor 承载（plan / decomposer / task_executor /
// test_designer / test_executor / deploy_service / infra）；将 Skill 与执行器
// 绑定调用、以 MCP 形式对外暴露，列为后续迭代路线图。
package skill

import (
	"fmt"
	"sort"
)

// Skill 定义可复用 Agent 能力的标准契约，字段对齐大赛对 Skill 的要求：
// 名称、用途、输入输出、调用条件、依赖工具、失败处理、验证方式、安全边界、
// 复用价值、版本演进、开源分发设计、与多 Agent 协同流程的关系。
type Skill struct {
	// Name 是 Skill 的唯一标识，如 "PlanSkill"。
	Name string `json:"name"`
	// Description 说明 Skill 的用途。
	Description string `json:"description"`
	// Category 能力分类：plan/decompose/code/test-design/test-execute/fix/deploy/infra。
	Category string `json:"category"`
	// Inputs 描述 Skill 的输入。
	Inputs []string `json:"inputs"`
	// Outputs 描述 Skill 的输出。
	Outputs []string `json:"outputs"`
	// CallConditions 说明 Skill 的调用条件（何时可被 Agent 调用）。
	CallConditions string `json:"callConditions"`
	// Dependencies 列出依赖的工具（Claude CLI、SSH、Playwright、Shell 等）。
	Dependencies []string `json:"dependencies"`
	// FailureHandling 说明失败处理机制。
	FailureHandling string `json:"failureHandling"`
	// VerificationMethod 说明 Skill 产出的验证方式（如何判定正确）。
	VerificationMethod string `json:"verificationMethod"`
	// SecurityBoundary 说明安全边界。
	SecurityBoundary string `json:"securityBoundary"`
	// ReuseValue 说明复用价值。
	ReuseValue string `json:"reuseValue"`
	// Version 是当前契约版本（语义化版本）。
	Version string `json:"version"`
	// VersionEvolution 说明版本演进策略。
	VersionEvolution string `json:"versionEvolution"`
	// OpenSourceDistribution 说明开源分发设计（协议/载体/文档）。
	OpenSourceDistribution string `json:"openSourceDistribution"`
	// CollaborationFlow 说明与多 Agent 协同流程的关系。
	CollaborationFlow string `json:"collaborationFlow"`
	// Implementation 标注能力在代码中的承载位置（供审计追踪）。
	Implementation string `json:"implementation"`
	// RelativePath / Content 兼容前端「项目级 Claude 配置」的 skill 字段（当前占位）。
	RelativePath string `json:"relativePath"`
	Content      string `json:"content"`
}

// Registry 是 Skill 的注册表，按名称去重管理全部可用 Skill。
type Registry struct {
	skills map[string]*Skill
	order  []string
}

// NewRegistry 构造一个空注册表。
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Skill)}
}

// Register 注册一个 Skill。名称重复或为空时返回错误，避免契约冲突。
func (r *Registry) Register(s *Skill) error {
	if s == nil || s.Name == "" {
		return fmt.Errorf("skill 名称不能为空")
	}
	if _, ok := r.skills[s.Name]; ok {
		return fmt.Errorf("skill %q 已注册", s.Name)
	}
	r.skills[s.Name] = s
	r.order = append(r.order, s.Name)
	return nil
}

// Get 按名称获取 Skill。
func (r *Registry) Get(name string) (*Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// List 按注册顺序返回全部 Skill。
func (r *Registry) List() []*Skill {
	list := make([]*Skill, 0, len(r.order))
	for _, name := range r.order {
		list = append(list, r.skills[name])
	}
	return list
}

// Names 返回全部 Skill 名称（排序，便于展示与测试）。
func (r *Registry) Names() []string {
	names := append([]string(nil), r.order...)
	sort.Strings(names)
	return names
}

// global 是进程级默认 Skill 注册表。
var global *Registry

func init() {
	global = NewRegistry()
	for _, s := range builtinSkills() {
		if err := global.Register(s); err != nil {
			panic(fmt.Sprintf("注册内置 Skill 失败: %v", err))
		}
	}
}

// Global 返回进程级默认 Skill 注册表。
func Global() *Registry { return global }

// builtinSkills 返回 Loafer 内置的 8 个职能 Skill。
// Implementation 字段标注该能力在代码中的承载位置。
func builtinSkills() []*Skill {
	return []*Skill{
		{
			Name:                   "PlanSkill",
			Description:            "从自然语言需求生成项目级执行计划（plan.md）",
			Category:               "plan",
			Inputs:                 []string{"自然语言需求描述", "技术栈约束"},
			Outputs:                []string{"Markdown 执行计划（落库 ExecutionPlan）"},
			CallConditions:         "项目创建后、尚无已有计划时",
			Dependencies:           []string{"Claude Code CLI"},
			FailureHandling:        "CLI 不可用或退出码非 0 时快速失败，流水线终止",
			VerificationMethod:     "计划落库后进入分解阶段，由分解 Agent 校验可执行性；人工可在 UI 预览确认",
			SecurityBoundary:       "仅读取需求并生成文本，不执行外部命令",
			ReuseValue:             "同类项目复用计划模板与拆分策略",
			Version:                "v1.0",
			VersionEvolution:       "计划模板与拆分策略迭代时升级契约，变更记入 changelog",
			OpenSourceDistribution: "以计划提示词模板 + 计划 Schema（plan.md）分发于仓库 docs/ 下，协议 Apache-2.0",
			CollaborationFlow:      "Manager Agent 流水线阶段 1（计划生成）的入口能力",
			Implementation:         "internal/engine/plan/generator.go:199 buildPlanPrompt",
		},
		{
			Name:                   "DecomposeSkill",
			Description:            "将已确认的计划拆分为带依赖的模块与任务",
			Category:               "decompose",
			Inputs:                 []string{"已确认的执行计划（status=confirmed）"},
			Outputs:                []string{"Module + Task 结构化 JSON，落库"},
			CallConditions:         "计划状态为 confirmed",
			Dependencies:           []string{"Claude Code CLI"},
			FailureHandling:        "解析失败返回错误，Manager 终止流水线",
			VerificationMethod:     "输出经 JSON Schema 校验 + blocked_by 依赖一致性检查通过后落库",
			SecurityBoundary:       "输出受 JSON Schema 校验，仅写模块/任务表",
			ReuseValue:             "复用模块拆分策略与依赖编排规则",
			Version:                "v1.0",
			VersionEvolution:       "模块/任务 Schema（sequence_number/blocked_by）变更需升级契约",
			OpenSourceDistribution: "以模块/任务 JSON Schema + 分解提示词模板分发，协议 Apache-2.0",
			CollaborationFlow:      "Manager Agent 流水线阶段 2（任务分解）",
			Implementation:         "internal/engine/executor/decomposer.go:282 buildDecomposePrompt",
		},
		{
			Name:                   "CodeSkill",
			Description:            "按任务步骤编写/修改代码",
			Category:               "code",
			Inputs:                 []string{"任务描述", "项目代码上下文", "依赖检查结果"},
			Outputs:                []string{"源码变更", "任务摘要", "SliceHistory 分片历史"},
			CallConditions:         "任务依赖（blockedBy）全部满足",
			Dependencies:           []string{"Claude Code CLI", "本地文件系统"},
			FailureHandling:        "任务失败标记状态，继续无依赖模块；基础设施模块失败终止流水线",
			VerificationMethod:     "代码正确性由自动测试门禁验证（部署 + API/Playwright UI 测试），通过才置为完成",
			SecurityBoundary:       "仅在项目工作目录内操作；运行中任务防重复执行",
			ReuseValue:             "跨项目复用编码模式与恢复策略",
			Version:                "v1.0",
			VersionEvolution:       "任务执行上下文构建策略迭代时升级契约",
			OpenSourceDistribution: "以任务执行提示词模板 + 上下文契约分发，协议 Apache-2.0",
			CollaborationFlow:      "Worker 开发 Agent，受业务模块质量门禁约束串行执行",
			Implementation:         "internal/engine/executor/task_executor.go:545 buildExecutionPrompt",
		},
		{
			Name:                   "TestDesignSkill",
			Description:            "根据已实现代码自动生成 API + Playwright UI 测试用例",
			Category:               "test-design",
			Inputs:                 []string{"模块需求", "Go handler 路由", "前端 views/api 目录"},
			Outputs:                []string{"api_integration_test / web_integration_test JSON", "Playwright spec 文件，落库"},
			CallConditions:         "业务模块任务完成且用例为空（幂等）",
			Dependencies:           []string{"Claude Code CLI"},
			FailureHandling:        "生成失败降级为自由测试模式，不阻断流水线",
			VerificationMethod:     "后端校验 JSON 合法性后落库；生成的用例可被 TestExecuteSkill 执行验证",
			SecurityBoundary:       "Agent 不直接写库，后端校验 JSON 后落库（Omit created_at）",
			ReuseValue:             "用例模板与场景命名规范沉淀",
			Version:                "v1.0",
			VersionEvolution:       "用例 Schema 与场景命名规范演进时升级契约",
			OpenSourceDistribution: "以用例 JSON Schema + Playwright spec 模板分发，协议 Apache-2.0",
			CollaborationFlow:      "门禁第 0 步（测试设计 Agent），幂等补生成",
			Implementation:         "internal/engine/executor/test_designer.go:52 buildDesignPrompt",
		},
		{
			Name:                   "TestExecuteSkill",
			Description:            "运行 API + Playwright UI 集成测试，判定模块质量",
			Category:               "test-execute",
			Inputs:                 []string{"落库用例 JSON", "当轮部署 URL", "Playwright spec 文件"},
			Outputs:                []string{"tests/results/module-<id>.json 场景级结果", "终态/失败截图"},
			CallConditions:         "业务模块部署成功后",
			Dependencies:           []string{"Shell/curl", "Playwright"},
			FailureHandling:        "每轮失败记录 failures 进入修复循环；3 轮耗尽置测试失败并暂停",
			VerificationMethod:     "结果 JSON + 截图作为通过/失败判定依据，按场景回写模块用例与状态",
			SecurityBoundary:       "截图目录严格校验防路径穿越；仅访问部署 URL",
			ReuseValue:             "标准化测试协议与结果格式，跨项目复用",
			Version:                "v1.0",
			VersionEvolution:       "结果契约扩展需保持向后兼容（无 scenarios 时回退模块级判定）",
			OpenSourceDistribution: "以测试结果 JSON 契约 + 截图命名规范分发，协议 Apache-2.0",
			CollaborationFlow:      "门禁第 1~3 轮（测试执行 Agent）",
			Implementation:         "internal/engine/executor/test_executor.go:162 buildTestPrompt",
		},
		{
			Name:                   "FixSkill",
			Description:            "读取失败明细，自动修改代码修复缺陷",
			Category:               "fix",
			Inputs:                 []string{"上一轮 failures 全文", "项目源码"},
			Outputs:                []string{"修复后的源码"},
			CallConditions:         "测试未通过且轮次未耗尽",
			Dependencies:           []string{"Claude Code CLI"},
			FailureHandling:        "修复异常继续下一轮测试，由测试自然判定",
			VerificationMethod:     "修复后重新部署测试，由下一轮测试结果验证修复是否有效",
			SecurityBoundary:       "仅在项目工作目录内修改代码",
			ReuseValue:             "修复策略与失败模式沉淀",
			Version:                "v1.0",
			VersionEvolution:       "失败报告解析与修复提示词策略迭代时升级契约",
			OpenSourceDistribution: "以修复提示词模板 + failures 数据契约分发，协议 Apache-2.0",
			CollaborationFlow:      "门禁失败分支（修复 Agent），与测试 Agent 交替直至通过或 3 轮耗尽",
			Implementation:         "internal/engine/executor/test_executor.go:565 buildFixPrompt",
		},
		{
			Name:                   "DeploySkill",
			Description:            "构建、上传、启动后端并配置 Nginx 反向代理",
			Category:               "deploy",
			Inputs:                 []string{"项目工作目录", "部署目标（远程/本地）"},
			Outputs:                []string{"访问 URL", "部署记录（端口/PID/Nginx 配置）"},
			CallConditions:         "模块任务完成或全局交付",
			Dependencies:           []string{"本地 Go/Node 工具链", "SSH", "tar", "Nginx"},
			FailureHandling:        "部署失败视为一轮测试失败，计入 3 轮额度",
			VerificationMethod:     "构建成功 + 后端启动校验 + 访问 URL 可访问性检查",
			SecurityBoundary:       "SSH 最小权限 key；端口隔离（40410-40500）；force 参数控制覆盖式部署",
			ReuseValue:             "标准化部署契约，环境变量注入（PORT/JWT_SECRET/APP_ENV_VARS/DB_PATH）",
			Version:                "v1.0",
			VersionEvolution:       "部署契约（env 注入 / 端口 / Nginx 配置）演进时升级契约",
			OpenSourceDistribution: "以部署脚本（deploy-local.sh）+ 运行时环境变量契约分发，协议 Apache-2.0",
			CollaborationFlow:      "部署 Agent，支撑业务模块门禁与全局阶段 4（部署上线）",
			Implementation:         "internal/service/deploy_service.go",
		},
		{
			Name:                   "InfraVerifySkill",
			Description:            "验证基础设施模块可构建并启动",
			Category:               "infra",
			Inputs:                 []string{"基础设施模块代码"},
			Outputs:                []string{"构建/启动校验结果"},
			CallConditions:         "基础设施模块任务完成后",
			Dependencies:           []string{"go build", "Shell"},
			FailureHandling:        "失败立即终止流水线（后续业务模块依赖基础设施）",
			VerificationMethod:     "编译（go build）+ 启动校验，通过才置为完成",
			SecurityBoundary:       "仅本地编译与启动校验，不写入生产环境",
			ReuseValue:             "环境基线与依赖版本校验复用",
			Version:                "v1.0",
			VersionEvolution:       "校验步骤随技术栈与依赖基线扩展时升级契约",
			OpenSourceDistribution: "以构建/校验脚本 + 环境基线说明分发，协议 Apache-2.0",
			CollaborationFlow:      "基础设施模块分流分支，构建通过后置为完成",
			Implementation:         "internal/handler/infra_verify.go / infra.go",
		},
	}
}
