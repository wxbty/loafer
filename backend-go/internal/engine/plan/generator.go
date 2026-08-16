package plan

import (
	"fmt"
	"strings"
	"time"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"

	"gorm.io/gorm"
)

// ExecutionPlan 状态常量，对应 execution_plan.status 字段的取值。
// 状态流转：draft → confirmed → decomposed → executing → completed
const (
	PlanStatusDraft      = "draft"
	PlanStatusConfirmed  = "confirmed"
	PlanStatusDecomposed = "decomposed"
	PlanStatusExecuting  = "executing"
	PlanStatusCompleted  = "completed"
)

// extractPlanContent 从 Claude 输出中提取计划内容。
// 策略：查找首个 Markdown 标题作为计划起始；若未找到标题则返回完整输出。
func extractPlanContent(output string) string {
	lines := strings.Split(output, "\n")
	startIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			startIdx = i
			break
		}
	}
	if startIdx >= 0 {
		return strings.TrimSpace(strings.Join(lines[startIdx:], "\n"))
	}
	return strings.TrimSpace(output)
}

// PlanGenerator 计划生成引擎，通过 Claude Code CLI（--print 模式）从自然语言需求生成执行计划。
// 生成的计划存储在 ExecutionPlan 模型中，支持生成、细化、确认等操作。
type PlanGenerator struct {
	db          *gorm.DB
	executor    *cli.OfflineExecutor
	docsService *service.DocsArtifactService
}

// NewPlanGenerator 构造计划生成器。
// docsService 可为 nil（此时跳过 docs 目录产物写入）。
func NewPlanGenerator(db *gorm.DB, executor *cli.OfflineExecutor, docsService *service.DocsArtifactService) *PlanGenerator {
	return &PlanGenerator{db: db, executor: executor, docsService: docsService}
}

// GetExecutor 返回底层执行器，供其他模块复用 CLI 调用能力。
func (g *PlanGenerator) GetExecutor() *cli.OfflineExecutor {
	return g.executor
}

// GeneratePlan 从自然语言需求生成执行计划。
// 流程：构建提示词 → 通过 OfflineExecutor（claude --print）执行 → 解析输出 → 保存为 draft 状态。
// onOutput 回调用于实时推送 Claude 输出（如 SSE 推送）。
func (g *PlanGenerator) GeneratePlan(projectID int64, requirement string, onOutput func(string)) (*model.ExecutionPlan, error) {
	var project model.Project
	if err := g.db.First(&project, projectID).Error; err != nil {
		return nil, fmt.Errorf("加载项目失败: %w", err)
	}
	if project.WorkDir == "" {
		return nil, fmt.Errorf("项目工作目录未设置，无法启动 Claude 会话")
	}

	prompt := buildPlanPrompt(requirement, &project)
	if onOutput != nil {
		onOutput("正在向 Claude 发送计划生成请求...\n")
	}

	result := g.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	projectIDPtr := projectID
	cli.RecordCall(g.db, "plan_generate", &projectIDPtr, nil, prompt, result, project.WorkDir)
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("Claude CLI 执行失败（退出码 %d）: %s", result.ExitCode, result.Error)
	}

	planContent := extractPlanContent(result.Response)
	if planContent == "" {
		return nil, fmt.Errorf("Claude 输出为空，未能生成计划内容")
	}

	plan := &model.ExecutionPlan{
		ProjectID:       projectID,
		OriginalRequest: requirement,
		PlanContent:     planContent,
		Status:          PlanStatusDraft,
		SessionID:       result.ClaudeSessionUUID,
	}
	if err := g.db.Create(plan).Error; err != nil {
		return nil, fmt.Errorf("保存执行计划失败: %w", err)
	}

	// 将计划写入 docs/plans/ 目录并 git 提交推送（best-effort，失败不阻断流程）
	g.writePlanDocBestEffort(project.WorkDir, project.Name, project.GitURL, planContent, onOutput)

	if onOutput != nil {
		onOutput(fmt.Sprintf("\n执行计划已生成，ID: %d，状态: draft\n", plan.ID))
	}
	return plan, nil
}

// RefinePlan 根据用户反馈细化已有的执行计划。
// 使用 --resume 续跑上次 Claude 会话（若有 session UUID），否则创建新会话。
func (g *PlanGenerator) RefinePlan(planID int64, feedback string, onOutput func(string)) (*model.ExecutionPlan, error) {
	var plan model.ExecutionPlan
	if err := g.db.First(&plan, planID).Error; err != nil {
		return nil, fmt.Errorf("加载执行计划失败: %w", err)
	}

	var project model.Project
	if err := g.db.First(&project, plan.ProjectID).Error; err != nil {
		return nil, fmt.Errorf("加载项目失败: %w", err)
	}
	if project.WorkDir == "" {
		return nil, fmt.Errorf("项目工作目录未设置")
	}

	prompt := buildRefinePrompt(plan.OriginalRequest, plan.PlanContent, feedback)
	if onOutput != nil {
		onOutput("正在向 Claude 发送计划细化请求...\n")
	}

	result := g.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	projectIDPtr := plan.ProjectID
	cli.RecordCall(g.db, "plan_refine", &projectIDPtr, nil, prompt, result, project.WorkDir)
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("Claude CLI 执行失败（退出码 %d）: %s", result.ExitCode, result.Error)
	}

	planContent := extractPlanContent(result.Response)
	if planContent == "" {
		return nil, fmt.Errorf("Claude 输出为空，未能生成细化后的计划内容")
	}

	plan.PlanContent = planContent
	if result.ClaudeSessionUUID != "" {
		plan.SessionID = result.ClaudeSessionUUID
	}
	if err := g.db.Save(&plan).Error; err != nil {
		return nil, fmt.Errorf("更新执行计划失败: %w", err)
	}

	// 细化后的计划也写入 docs/plans/ 覆盖旧文件并 git 提交推送
	g.writePlanDocBestEffort(project.WorkDir, project.Name, project.GitURL, planContent, onOutput)

	if onOutput != nil {
		onOutput(fmt.Sprintf("\n执行计划已细化，ID: %d\n", plan.ID))
	}
	return &plan, nil
}

// ConfirmPlan 确认执行计划，将状态从 draft 更新为 confirmed。
func (g *PlanGenerator) ConfirmPlan(planID int64) error {
	now := time.Now()
	result := g.db.Model(&model.ExecutionPlan{}).Where("id = ?", planID).Updates(map[string]interface{}{
		"status":       PlanStatusConfirmed,
		"confirmed_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("确认执行计划失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("执行计划 %d 不存在", planID)
	}
	return nil
}

// GetPlan 获取项目最新的执行计划（按创建时间倒序）。
func (g *PlanGenerator) GetPlan(projectID int64) (*model.ExecutionPlan, error) {
	var plan model.ExecutionPlan
	if err := g.db.Where("project_id = ?", projectID).
		Order("created_at DESC").
		First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

// GetPlanByID 按 ID 获取执行计划。
func (g *PlanGenerator) GetPlanByID(planID int64) (*model.ExecutionPlan, error) {
	var plan model.ExecutionPlan
	if err := g.db.First(&plan, planID).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

// buildPlanPrompt 构建计划生成提示词，指导 Claude 产出包含项目概述、技术栈、
// 模块拆分、任务列表、测试策略和风险评估的结构化 Markdown 计划。
func buildPlanPrompt(requirement string, project *model.Project) string {
	var sb strings.Builder
	sb.WriteString("【语言要求 - 最高优先级】你必须使用中文输出所有内容，包括所有标题、描述、分析、技术名词解释。不要使用英文输出。\n\n")

	sb.WriteString("你是一位资深软件架构师和技术负责人。")
	sb.WriteString("你的任务是分析软件需求，制定一份详细、可执行的项目计划。\n\n")

	sb.WriteString("## 用户需求\n")
	sb.WriteString(requirement)
	sb.WriteString("\n\n")

	sb.WriteString("## 项目上下文\n")
	sb.WriteString(fmt.Sprintf("- 项目名称: %s\n", project.Name))
	sb.WriteString(fmt.Sprintf("- 需求描述: %s\n", project.Description))
	sb.WriteString(fmt.Sprintf("- 开发语言: %s\n", project.DevLanguage))
	sb.WriteString(fmt.Sprintf("- Git 仓库: %s\n", project.GitURL))
	sb.WriteString(fmt.Sprintf("- 工作目录: %s\n\n", project.WorkDir))

	sb.WriteString("## 要求\n")
	sb.WriteString("仔细分析需求，产出一份结构化的 Markdown 项目计划，必须包含以下章节：\n\n")

	sb.WriteString("### 1. 项目概述\n")
	sb.WriteString("简要说明将要构建什么，解决什么核心问题，目标用户是谁，主要目标是什么。列出关键假设和约束。\n\n")

	sb.WriteString("### 2. 技术栈\n")
	sb.WriteString("推荐使用的技术、框架、库和工具，并简要说明选择理由。考虑：\n")
	sb.WriteString("- 后端框架和语言\n")
	sb.WriteString("- 前端框架（如适用）\n")
	sb.WriteString("- 数据库和 ORM\n")
	sb.WriteString("- 认证/授权方案\n")
	sb.WriteString("- 测试框架\n")
	sb.WriteString("- DevOps 和部署工具\n\n")

	sb.WriteString("### 3. 模块拆分\n")
	sb.WriteString("将项目划分为逻辑内聚的模块。每个模块包含：\n")
	sb.WriteString("- **模块名称**: 描述性名称\n")
	sb.WriteString("- **描述**: 模块的职责和功能\n")
	sb.WriteString("- **序号**: 建议的执行顺序（如 \"1\", \"2\", \"3\"）\n")
	sb.WriteString("- **前置依赖**: 必须先完成的模块序号（逗号分隔，无则留空）\n")
	sb.WriteString("- **关键组件**: 需要实现的主要类/函数/组件\n\n")

	sb.WriteString("### 4. 任务列表\n")
	sb.WriteString("为每个模块定义具体的、可执行的任务。每个任务包含：\n")
	sb.WriteString("- **任务名称**: 清晰的描述性名称\n")
	sb.WriteString("- **描述**: 需要做什么\n")
	sb.WriteString("- **序号**: 模块内任务标识（如 \"1.1\", \"1.2\"）\n")
	sb.WriteString("- **步骤**: 描述实现步骤的 JSON 字符串数组\n")
	sb.WriteString("- **类别**: backend, frontend, database, testing, devops, documentation 之一\n")
	sb.WriteString("- **前置依赖**: 必须先完成的任务序号（逗号分隔，无则留空）\n")
	sb.WriteString("- **预估工作量**: small/medium/large\n\n")

	sb.WriteString("### 5. 测试策略\n")
	sb.WriteString("定义测试方案：\n")
	sb.WriteString("- 单元测试: 测试什么，用什么框架\n")
	sb.WriteString("- 集成测试: 关键集成点\n")
	sb.WriteString("- 端到端测试: 关键用户流程\n")
	sb.WriteString("- 测试数据管理策略\n\n")

	sb.WriteString("### 6. 风险评估\n")
	sb.WriteString("识别潜在风险并提供缓解策略：\n")
	sb.WriteString("- 技术风险\n")
	sb.WriteString("- 进度风险\n")
	sb.WriteString("- 依赖风险\n\n")

	sb.WriteString("## 输出格式\n")
	sb.WriteString("输出一份结构良好的 Markdown 文档，使用正确的标题、列表和代码块。")
	sb.WriteString("不要将整个输出包裹在单个代码块中。以一级标题（# 项目计划）开头。")
	sb.WriteString("【再次强调】所有标题、描述、分析内容、风险评估等必须全部使用中文。技术名词可保留英文原文，但解释说明必须用中文。")
	return sb.String()
}

// buildRefinePrompt 构建计划细化提示词，附带原始需求、当前计划和用户反馈。
func buildRefinePrompt(originalRequest, currentPlan, feedback string) string {
	var sb strings.Builder
	sb.WriteString("【语言要求 - 最高优先级】你必须使用中文输出所有内容，包括所有标题、描述、分析。不要使用英文输出。\n\n")

	sb.WriteString("你之前生成了一份项目执行计划。用户已审阅并提供了反馈意见，请据此修改和完善计划。\n\n")

	sb.WriteString("## 原始需求\n")
	sb.WriteString(originalRequest)
	sb.WriteString("\n\n")

	sb.WriteString("## 当前计划\n")
	sb.WriteString(currentPlan)
	sb.WriteString("\n\n")

	sb.WriteString("## 修改意见\n")
	sb.WriteString(feedback)
	sb.WriteString("\n\n")

	sb.WriteString("## 要求\n")
	sb.WriteString("根据反馈修改和完善计划。")
	sb.WriteString("保持相同的结构化 Markdown 格式，包含所有章节")
	sb.WriteString("（项目概述、技术栈、模块拆分、任务列表、测试策略、风险评估）。")
	sb.WriteString("针对反馈中提出的所有问题进行修改。输出完整的修改后计划，")
	sb.WriteString("以一级标题（# 项目计划）开头。所有内容必须使用中文。\n")
	sb.WriteString("【再次强调】所有输出必须使用中文，不要使用英文。\n")
	return sb.String()
}

// writePlanDocBestEffort 将计划内容写入 docs/plans/ 目录并 git 提交推送。
// best-effort 模式：失败时仅输出提示信息，不阻断计划生成主流程。
func (g *PlanGenerator) writePlanDocBestEffort(workDir, projectName, gitURL, planContent string, onOutput func(string)) {
	if g.docsService == nil || !g.docsService.IsAvailable() {
		return
	}
	if onOutput != nil {
		onOutput("\n[文档] 正在将执行计划写入 docs/plans/ 并提交到 Git...\n")
	}
	if err := g.docsService.WritePlanDoc(workDir, projectName, gitURL, planContent); err != nil {
		if onOutput != nil {
			onOutput(fmt.Sprintf("[文档] 写入执行计划文档失败（不影响流程）: %v\n", err))
		}
		return
	}
	if onOutput != nil {
		onOutput("[文档] 执行计划已写入 docs/plans/ 并推送到 Git 仓库\n")
	}
}
