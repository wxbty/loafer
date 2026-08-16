package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/engine/executor"
	"loafer-agent/internal/engine/plan"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PipelineHandler 端到端全链路流水线处理器。
// 在项目创建后，自动串联：计划生成 → 计划确认 → 计划分解 → 模块任务执行 → 部署 → 测试。
// 通过单一 SSE 连接推送全链路进度，前端可实时展示每个阶段状态和日志。
type PipelineHandler struct {
	db              *gorm.DB
	cfg             *config.Config
	planGenerator   *plan.PlanGenerator
	decomposer      *executor.Decomposer
	taskExecutor    *executor.TaskExecutor
	testExecutor    *executor.TestExecutor
	testDesigner    *executor.TestDesignExecutor
	deployService   *service.DeployService
	playwrightSvc   *service.PlaywrightService
	offlineExecutor *cli.OfflineExecutor
	pipelineManager *PipelineManager
}

// moduleLoopAction 流水线模块循环对单个模块的处理动作。
type moduleLoopAction int

const (
	moduleActionSkip         moduleLoopAction = iota // 已完成，跳过
	moduleActionTestOnly                             // 任务已完成，直接进测试门禁
	moduleActionRunThenTest                          // 先跑任务，再进测试门禁
)

// resolveModuleAction 按模块状态决定处理动作。
// 4 完成 → 跳过；2 待测试 / 3 测试中 → 直接进测试门禁（断点续跑/历史数据兼容）；
// 0 待执行 / 1 执行中(中断残留) / 5 测试失败 / 6 失败 → 重跑任务后进门禁。
func resolveModuleAction(status int) moduleLoopAction {
	switch status {
	case executor.ModuleStatusCompleted:
		return moduleActionSkip
	case executor.ModuleStatusPendingTest, executor.ModuleStatusTesting:
		return moduleActionTestOnly
	default:
		return moduleActionRunThenTest
	}
}

// NewPipelineHandler 构造全链路流水线处理器。
func NewPipelineHandler(db *gorm.DB, cfg *config.Config, offlineExecutor *cli.OfflineExecutor) *PipelineHandler {
	docsService := service.NewDocsArtifactService(cfg)
	return &PipelineHandler{
		db:              db,
		cfg:             cfg,
		planGenerator:   plan.NewPlanGenerator(db, offlineExecutor, docsService),
		decomposer:      executor.NewDecomposer(db, offlineExecutor, docsService),
		taskExecutor:    executor.NewTaskExecutor(db, offlineExecutor),
		testExecutor:    executor.NewTestExecutor(db, offlineExecutor),
		testDesigner:    executor.NewTestDesignExecutor(db, offlineExecutor),
		deployService:   service.NewDeployService(db, cfg),
		playwrightSvc:   service.NewPlaywrightService(db, cfg),
		offlineExecutor: offlineExecutor,
		pipelineManager: NewPipelineManager(),
	}
}

// RegisterRoutes 注册流水线路由。
func (h *PipelineHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/pipeline")
	{
		g.POST("/project/:projectId/run", h.RunPipeline)
		g.GET("/project/:projectId/status", h.GetPipelineStatus)
		g.GET("/project/:projectId/artifact/:type", h.GetArtifact)
		g.POST("/project/:projectId/clarify", h.GenerateClarifyQuestions)
		g.POST("/project/:projectId/clarify/answers", h.SubmitClarifyAnswers)
		// 独立需求澄清：不依赖项目ID，直接传入需求文本生成问题（不写DB）
		g.POST("/clarify-standalone", h.GenerateClarifyQuestionsStandalone)
		// 一键停止：杀死所有正在运行的 Claude Code CLI 进程
		g.POST("/project/:projectId/abort", h.AbortPipeline)
	}
}

// ClarifyQuestion 需求澄清问题
type ClarifyQuestion struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Category    string   `json:"category"`    // scope, tech, data, ui, security, performance
	Options     []string `json:"options"`     // 单选选项列表
	AllowCustom bool     `json:"allowCustom"` // 是否允许自定义输入（最后一个选项为"其他"）
	Required    bool     `json:"required"`    // 是否必答
}

// ClarifyAnswer 用户提交的澄清答案
type ClarifyAnswer struct {
	QuestionID string `json:"questionId"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

// GenerateClarifyQuestions 对应 POST /pipeline/project/:projectId/clarify。
// 基于项目需求描述，由 AI 分析需求细节并生成针对性的澄清问题（类似 Claude Code 的 /grill-me）。
// 当 Claude CLI 不可用或调用失败时，降级为基于需求文本的规则生成。
func (h *PipelineHandler) GenerateClarifyQuestions(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	var project model.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "项目不存在: "+err.Error())
		return
	}

	requirement := project.Description
	if requirement == "" {
		requirement = project.Name
	}

	// 优先使用 AI 生成针对性问题，失败时降级为规则生成
	var questions []ClarifyQuestion
	aiUsed := false
	cliAvailable := cli.IsCLIAvailable()

	if cliAvailable && project.WorkDir != "" {
		aiQuestions, err := h.generateAIQuestions(project.WorkDir, requirement)
		if err != nil {
			// AI 调用失败，记录错误但继续降级
			fmt.Printf("[clarify] AI 生成失败，降级为规则生成: %v\n", err)
		} else if len(aiQuestions) > 0 {
			questions = aiQuestions
			aiUsed = true
		}
	}

	// 降级或直接使用规则生成
	if !aiUsed {
		questions = h.generateRuleBasedQuestions(requirement)
	}

	// 兜底：确保至少有问题返回
	if len(questions) == 0 {
		questions = h.generateFallbackQuestions()
	}

	util.OKWithData(c, gin.H{
		"projectId":   projectID,
		"questions":   questions,
		"requirement": requirement,
		"aiGenerated": aiUsed,
	})
}

// GenerateClarifyQuestionsStandalone 对应 POST /pipeline/clarify-standalone。
// 独立需求澄清：不依赖项目ID，直接传入需求文本生成澄清问题。
// 不会进行任何数据库读写操作，适用于项目创建前的需求确认阶段。
func (h *PipelineHandler) GenerateClarifyQuestionsStandalone(c *gin.Context) {
	var body struct {
		Requirement string `json:"requirement"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "参数解析失败: "+err.Error())
		return
	}

	requirement := strings.TrimSpace(body.Requirement)
	if requirement == "" {
		util.Fail(c, http.StatusBadRequest, "需求描述不能为空")
		return
	}

	// 优先使用 AI 生成针对性问题，失败时降级为规则生成
	var questions []ClarifyQuestion
	aiUsed := false
	cliAvailable := cli.IsCLIAvailable()

	if cliAvailable {
		// 独立模式下无项目工作目录，使用系统临时目录执行 CLI
		aiQuestions, err := h.generateAIQuestionsStandalone(requirement)
		if err != nil {
			fmt.Printf("[clarify-standalone] AI 生成失败，降级为规则生成: %v\n", err)
		} else if len(aiQuestions) > 0 {
			questions = aiQuestions
			aiUsed = true
		}
	}

	// 降级或直接使用规则生成
	if !aiUsed {
		questions = h.generateRuleBasedQuestions(requirement)
	}

	// 兜底：确保至少有问题返回
	if len(questions) == 0 {
		questions = h.generateFallbackQuestions()
	}

	util.OKWithData(c, gin.H{
		"questions":   questions,
		"requirement": requirement,
		"aiGenerated": aiUsed,
	})
}

// generateAIQuestionsStandalone 使用 Claude CLI 分析需求并生成澄清问题（独立模式，不写DB）。
// 使用系统临时目录作为工作目录，不记录 CLI 调用日志到数据库。
func (h *PipelineHandler) generateAIQuestionsStandalone(requirement string) ([]ClarifyQuestion, error) {
	prompt := buildClarifyPrompt(requirement)

	// 使用系统临时目录作为 CLI 执行目录
	tmpDir := "/tmp"
	result := h.planGenerator.GetExecutor().ExecuteSimple(tmpDir, prompt, nil)

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("Claude CLI 执行失败（退出码 %d）: %s", result.ExitCode, result.Error)
	}

	// 从 AI 输出中提取 JSON
	jsonStr := extractJSONBlock(result.Response)
	if jsonStr == "" {
		return nil, fmt.Errorf("AI 输出中未找到有效的 JSON")
	}

	return parseClarifyQuestionsJSON(jsonStr)
}

// generateAIQuestions 使用 Claude CLI 分析需求并生成针对性的澄清问题。
// AI 会基于需求中的模糊点、缺失信息和技术决策点生成单选问题。
func (h *PipelineHandler) generateAIQuestions(workDir, requirement string) ([]ClarifyQuestion, error) {
	prompt := buildClarifyPrompt(requirement)

	result := h.planGenerator.GetExecutor().ExecuteSimple(workDir, prompt, nil)
	pid := int64(0)
	cli.RecordCall(h.db, "clarify_questions", &pid, nil, prompt, result, workDir)

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("Claude CLI 执行失败（退出码 %d）: %s", result.ExitCode, result.Error)
	}

	// 从 AI 输出中提取 JSON
	jsonStr := extractJSONBlock(result.Response)
	if jsonStr == "" {
		return nil, fmt.Errorf("AI 输出中未找到有效的 JSON")
	}

	return parseClarifyQuestionsJSON(jsonStr)
}

// buildClarifyPrompt 构建需求澄清的 AI 提示词。
// 指导 AI 分析需求中的模糊点和缺失信息，生成面向非技术用户的单选确认问题。
func buildClarifyPrompt(requirement string) string {
	var sb strings.Builder
	sb.WriteString("你是一位资深产品经理。请分析以下用户需求，找出需求中模糊、缺失或需要确认的细节，")
	sb.WriteString("生成 4-6 个针对性的确认问题，帮助用户明确需求。\n\n")
	sb.WriteString("## 用户需求\n")
	sb.WriteString(requirement)
	sb.WriteString("\n\n")
	sb.WriteString("## 要求\n")
	sb.WriteString("1. 问题必须基于需求内容，不要问与需求无关的通用问题\n")
	sb.WriteString("2. 每个问题提供 2-4 个选项供用户单选\n")
	sb.WriteString("3. 选项用非技术用户能理解的通俗语言描述\n")
	sb.WriteString("4. 问题的关注点示例：功能范围确认、业务流程选择、数据展示方式、交互方式偏好等\n")
	sb.WriteString("5. 如果需求中已经明确了某个方面，不要再问\n")
	sb.WriteString("6. 不要在选项中放\"其他\"或\"以上都不是\"，系统会自动为每个问题追加手动填写选项\n\n")
	sb.WriteString("## 输出格式\n")
	sb.WriteString("只输出一个 JSON 数组，不要输出其他内容。格式如下：\n")
	sb.WriteString("```json\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\n")
	sb.WriteString("    \"id\": \"q1\",\n")
	sb.WriteString("    \"question\": \"问题文本\",\n")
	sb.WriteString("    \"category\": \"scope\",\n")
	sb.WriteString("    \"options\": [\"选项1\", \"选项2\", \"选项3\"],\n")
	sb.WriteString("    \"allowCustom\": true,\n")
	sb.WriteString("    \"required\": true\n")
	sb.WriteString("  }\n")
	sb.WriteString("]\n")
	sb.WriteString("```\n")
	sb.WriteString("category 可选值: scope(功能范围), tech(技术选择), data(数据相关), ui(界面交互), security(权限安全), performance(性能规模)\n")
	return sb.String()
}

// extractJSONBlock 从文本中提取 JSON 代码块或裸 JSON 数组。
func extractJSONBlock(text string) string {
	// 尝试提取 ```json ... ``` 代码块
	if idx := strings.Index(text, "```json"); idx >= 0 {
		start := idx + len("```json")
		end := strings.Index(text[start:], "```")
		if end > 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	// 尝试提取 ``` ... ``` 代码块
	if idx := strings.Index(text, "```"); idx >= 0 {
		start := idx + len("```")
		// 跳过可能的语言标识行
		if nl := strings.IndexByte(text[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		end := strings.Index(text[start:], "```")
		if end > 0 {
			content := strings.TrimSpace(text[start : start+end])
			if strings.HasPrefix(content, "[") || strings.HasPrefix(content, "{") {
				return content
			}
		}
	}
	// 尝试直接查找 JSON 数组的起始
	start := strings.Index(text, "[")
	if start >= 0 {
		end := strings.LastIndex(text, "]")
		if end > start {
			return strings.TrimSpace(text[start : end+1])
		}
	}
	// 尝试查找 JSON 对象
	start = strings.Index(text, "{")
	if start >= 0 {
		end := strings.LastIndex(text, "}")
		if end > start {
			return strings.TrimSpace(text[start : end+1])
		}
	}
	return ""
}

// parseClarifyQuestionsJSON 将 AI 输出的 JSON 解析为 ClarifyQuestion 列表。
func parseClarifyQuestionsJSON(jsonStr string) ([]ClarifyQuestion, error) {
	type rawQuestion struct {
		ID          string   `json:"id"`
		Question    string   `json:"question"`
		Category    string   `json:"category"`
		Options     []string `json:"options"`
		AllowCustom *bool    `json:"allowCustom"`
		Required    *bool    `json:"required"`
	}

	var rawQuestions []rawQuestion
	if err := json.Unmarshal([]byte(jsonStr), &rawQuestions); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	questions := make([]ClarifyQuestion, 0, len(rawQuestions))
	for i, rq := range rawQuestions {
		id := rq.ID
		if id == "" {
			id = fmt.Sprintf("q%d", i+1)
		}
		// 强制每个问题的最后一个选项为用户手动填写
		allowCustom := true
		required := false
		if rq.Required != nil {
			required = *rq.Required
		}
		category := rq.Category
		if category == "" {
			category = "scope"
		}
		if len(rq.Options) < 2 {
			continue // 至少需要2个选项
		}
		questions = append(questions, ClarifyQuestion{
			ID:          id,
			Question:    rq.Question,
			Category:    category,
			Options:     rq.Options,
			AllowCustom: allowCustom,
			Required:    required,
		})
	}

	return questions, nil
}

// generateFallbackQuestions 兜底问题列表，确保始终有问题可展示。
func (h *PipelineHandler) generateFallbackQuestions() []ClarifyQuestion {
	return []ClarifyQuestion{
		{
			ID:       "core_scope",
			Question: "核心功能范围是否需要调整？",
			Category: "scope",
			Options: []string{
				"当前描述已涵盖全部功能，不需要增加",
				"需要增加数据管理功能（增删改查）",
				"需要增加用户管理功能",
			},
			AllowCustom: true,
			Required:    true,
		},
		{
			ID:       "auth",
			Question: "用户需要登录才能使用吗？",
			Category: "security",
			Options: []string{
				"需要登录，不同用户有不同权限",
				"需要登录，但所有用户权限相同",
				"不需要登录，直接使用",
			},
			AllowCustom: true,
			Required:    true,
		},
		{
			ID:       "scale",
			Question: "预计多少人使用这个系统？",
			Category: "performance",
			Options: []string{
				"小规模（几个人到几十人）",
				"中等规模（上百人）",
				"大规模（上千人或更多）",
			},
			AllowCustom: true,
			Required:    false,
		},
	}
}

// generateRuleBasedQuestions 基于需求文本的规则生成澄清问题（降级方案）。
// 当 Claude CLI 不可用时使用，通过分析需求关键词生成相对针对性的问题。
func (h *PipelineHandler) generateRuleBasedQuestions(requirement string) []ClarifyQuestion {
	questions := []ClarifyQuestion{}
	features := extractFeaturesFromRequirement(requirement)

	// 基于需求关键词分析，生成针对性问题

	// Q1: 针对已识别功能进行确认
	q1Options := []string{}
	if len(features) > 0 {
		q1Options = append(q1Options,
			fmt.Sprintf("就这些功能，不需要再增加"),
			fmt.Sprintf("还需要其他功能（请在自定义中描述）"),
		)
		// 将识别到的功能作为选项供确认
		for _, f := range features {
			if len(q1Options) < 4 {
				q1Options = append(q1Options, fmt.Sprintf("重点实现「%s」", f))
			}
		}
	} else {
		q1Options = append(q1Options,
			"主要是数据管理（增删改查）",
			"主要是信息展示（列表、详情）",
			"主要是流程审批（多步骤操作）",
		)
	}
	questions = append(questions, ClarifyQuestion{
		ID:          "core_scope",
		Question:    "根据你的需求，核心功能范围是否正确？",
		Category:    "scope",
		Options:     q1Options,
		AllowCustom: true,
		Required:    true,
	})

	// Q2: 登录与权限
	questions = append(questions, ClarifyQuestion{
		ID:       "auth",
		Question: "用户需要登录才能使用吗？",
		Category: "security",
		Options: []string{
			"需要登录，不同用户有不同权限",
			"需要登录，但所有用户权限相同",
			"不需要登录，直接使用",
		},
		AllowCustom: true,
		Required:    true,
	})

	// Q3: 数据展示方式（基于需求中的关键词判断）
	if strings.Contains(requirement, "列表") || strings.Contains(requirement, "表格") || strings.Contains(requirement, "统计") {
		questions = append(questions, ClarifyQuestion{
			ID:       "data_display",
			Question: "数据主要怎么展示？",
			Category: "ui",
			Options: []string{
				"表格列表展示，支持搜索和筛选",
				"卡片式展示，更美观",
				"图表统计为主（柱状图、饼图等）",
			},
			AllowCustom: true,
			Required:    false,
		})
	}

	// Q4: 关键交互确认
	if len(features) > 0 {
		// 针对第一个识别到的功能进行交互确认
		primaryFeature := features[0]
		questions = append(questions, ClarifyQuestion{
			ID:       "interaction",
			Question: fmt.Sprintf("「%s」这个功能的主要操作方式？", primaryFeature),
			Category: "ui",
			Options: []string{
				"表单填写后提交",
				"列表中选择后操作",
				"搜索后查看详情",
			},
			AllowCustom: true,
			Required:    false,
		})
	}

	// Q5: 使用规模
	questions = append(questions, ClarifyQuestion{
		ID:       "scale",
		Question: "预计多少人使用这个系统？",
		Category: "performance",
		Options: []string{
			"小规模（几个人到几十人）",
			"中等规模（上百人）",
			"大规模（上千人或更多）",
		},
		AllowCustom: true,
		Required:    false,
	})

	return questions
}

// SubmitClarifyAnswers 对应 POST /pipeline/project/:projectId/clarify/answers。
// 接收用户对澄清问题的答案，将答案合并到项目需求中，供后续流水线计划生成使用。
func (h *PipelineHandler) SubmitClarifyAnswers(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	var body struct {
		Answers []ClarifyAnswer `json:"answers"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "参数解析失败: "+err.Error())
		return
	}

	var project model.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "项目不存在: "+err.Error())
		return
	}

	// 将澄清答案追加到项目描述中，作为计划生成的上下文
	if len(body.Answers) > 0 {
		var sb strings.Builder
		sb.WriteString(project.Description)
		sb.WriteString("\n\n## 需求澄清记录\n")
		for _, ans := range body.Answers {
			if strings.TrimSpace(ans.Answer) != "" {
				sb.WriteString(fmt.Sprintf("\n**Q: %s**\nA: %s\n", ans.Question, ans.Answer))
			}
		}
		project.Description = sb.String()
		h.db.Model(&project).Update("description", project.Description)
	}

	util.OKWithData(c, gin.H{
		"projectId":          projectID,
		"updatedDescription": project.Description,
		"answerCount":        len(body.Answers),
	})
}

// PipelineStage 流水线阶段定义
type PipelineStage struct {
	Name      string          `json:"name"`      // 阶段标识：plan, decompose, code, deploy, test
	Label     string          `json:"label"`     // 阶段中文名
	Status    string          `json:"status"`    // pending, running, completed, failed, skipped, paused
	Message   string          `json:"message"`   // 状态说明或错误信息
	Summary   string          `json:"summary"`   // 阶段完成后的概要总结
	Artifacts []StageArtifact `json:"artifacts"` // 阶段产出的中间产物
}

// StageArtifact 阶段中间产物描述
type StageArtifact struct {
	Type        string `json:"type"`        // 产物类型：markdown, json, url, text
	Name        string `json:"name"`        // 产物名称（展示用）
	Filename    string `json:"filename"`    // 下载时使用的文件名
	APIPath     string `json:"apiPath"`     // 获取产物内容的 API 路径（相对 /api）
	PreviewText string `json:"previewText"` // 预览文本（前 N 行，用于前端内联展示）
}

// PipelineResult 流水线最终结果
type PipelineResult struct {
	ProjectID  int64                    `json:"projectId"`
	Stages     []PipelineStage          `json:"stages"`
	AccessURLs []string                 `json:"accessUrls,omitempty"`
	Deployment *model.ProjectDeployment `json:"deployment,omitempty"`
}

// AbortPipeline 对应 POST /pipeline/project/:projectId/abort。
// 一键停止：杀死所有正在运行的 Claude Code CLI 进程。
// 前端在用户点击"停止"按钮时调用此接口，确保服务端 CLI 子进程被真正终止，
// 而非仅断开 SSE 连接（断开 SSE 不会终止已启动的 CLI 子进程）。
func (h *PipelineHandler) AbortPipeline(c *gin.Context) {
	killed := h.offlineExecutor.StopAll()
	msg := fmt.Sprintf("已停止 %d 个 Claude Code CLI 进程", killed)
	if killed == 0 {
		msg = "当前没有正在运行的 CLI 进程"
	}
	fmt.Printf("[pipeline-abort] %s\n", msg)
	util.OKWithData(c, gin.H{
		"killed":        killed,
		"message":       msg,
		"runningBefore": killed,
	})
}

// RunPipeline 对应 POST /pipeline/project/:projectId/run（SSE 流式）。
// 端到端执行：计划 → 分解 → 编码 → 部署 → 测试
//
// 架构设计：后台执行 + 事件订阅
//   - 流水线在后台 goroutine 中执行，与 SSE 连接解耦
//   - 客户端断开后流水线继续执行，中间数据入库
//   - 客户端重连后回放历史事件 + 实时推送新事件
func (h *PipelineHandler) RunPipeline(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	// 校验项目存在
	var project model.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "项目不存在: "+err.Error())
		return
	}

	// 获取或创建流水线运行器
	runner := h.pipelineManager.GetOrCreate(projectID)

	// 如果流水线尚未启动，在后台 goroutine 中启动
	if !runner.IsStarted() {
		runner.SetStarted()
		go func() {
			defer func() {
				if r := recover(); r != nil {
					runner.Publish(PipelineEvent{Type: "error", Payload: fmt.Sprintf("pipeline panic: %v", r)})
					runner.MarkDone(fmt.Errorf("pipeline panic: %v", r))
				}
			}()
			w := &RunnerWriter{runner: runner}
			defer w.Close()
			h.executePipeline(projectID, w)
		}()
	}

	// 订阅运行器事件并通过 SSE 推送给客户端
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	subID, eventCount, done, _ := runner.Subscribe()
	defer runner.Unsubscribe(subID)

	// 回放历史事件（客户端重连场景）
	index := 0
	if eventCount > 0 {
		events, newIndex, _, _ := runner.GetEventsSince(0)
		for _, evt := range events {
			replayEvent(sse, evt)
		}
		index = newIndex
	}
	if done {
		return
	}

	// 实时推送新事件
	notifyCh := runner.GetNotifyCh(subID)
	for {
		// 检查新事件
		events, newIndex, done, _ := runner.GetEventsSince(index)
		for _, evt := range events {
			replayEvent(sse, evt)
		}
		index = newIndex
		if done {
			return
		}

		// 排空通知 channel
		select {
		case <-notifyCh:
			continue
		default:
		}

		// 等待新事件通知或客户端断开
		select {
		case <-notifyCh:
			continue
		case <-c.Request.Context().Done():
			return // 客户端断开，流水线在后台继续执行
		}
	}
}

// replayEvent 将历史事件通过 SSE 重放给客户端。
func replayEvent(out *util.SSEWriter, evt PipelineEvent) {
	switch evt.Type {
	case "output":
		out.SendOutput(evt.Payload)
	case "error":
		out.SendError(evt.Payload)
	case "done":
		out.SendDoneRaw(evt.Payload)
	}
}

// executePipeline 在后台执行全链路流水线。
// 通过 ProgressWriter 发布进度事件，与 SSE 连接解耦。
func (h *PipelineHandler) executePipeline(projectID int64, w ProgressWriter) {
	var project model.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		w.SendError(fmt.Sprintf("项目不存在: %v", err))
		w.SendDone(&PipelineResult{ProjectID: projectID})
		return
	}

	// 检测 Claude CLI 是否可用
	cliAvailable := cli.IsCLIAvailable()
	if !cliAvailable {
		w.SendOutput("⚠ Claude Code CLI 未安装，计划/分解阶段必须走 AI，将在生成计划阶段快速失败\n")
		w.SendOutput("  安装 CLI 后重试：npm install -g @anthropic-ai/claude-code\n\n")
	} else {
		cliPath := cli.GetCLIPath()
		w.SendOutput(fmt.Sprintf("✓ Claude Code CLI 已检测到: %s\n", cliPath))
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			w.SendOutput("⚠ 环境变量 ANTHROPIC_API_KEY 未设置，CLI 将依赖 ~/.claude/credentials.json 中的凭证\n")
		}
		w.SendOutput("\n")
	}

	stages := []PipelineStage{
		{Name: "plan", Label: "生成计划", Status: "pending"},
		{Name: "decompose", Label: "分解任务", Status: "pending"},
		{Name: "code", Label: "编码实现", Status: "pending"},
		{Name: "deploy", Label: "部署上线", Status: "pending"},
		{Name: "test", Label: "测试验证", Status: "pending"},
	}

	result := &PipelineResult{
		ProjectID: projectID,
		Stages:    stages,
	}

	// 发送初始状态
	w.SendOutputJSON(map[string]interface{}{
		"type":   "pipeline_start",
		"stages": stages,
	})

	// 检查项目是否已有模块（流水线重跑场景）：如果已有模块，跳过计划生成和分解，直接进入编码阶段。
	// 这样避免重复创建模块，保留各模块的执行/测试状态，实现真正的「断点续跑」。
	var existingModules []model.Module
	h.db.Where("project_id = ?", projectID).Order("id ASC").Find(&existingModules)
	util.SortBySequenceNumber(existingModules, func(m model.Module) string { return m.SequenceNumber })

	var executionPlan *model.ExecutionPlan
	var modules []model.Module
	var err error

	if len(existingModules) > 0 {
		// ===== 断点续跑：跳过计划生成和分解 =====
		w.SendOutput(fmt.Sprintf("检测到项目已有 %d 个模块，跳过计划生成和分解阶段，直接进入编码阶段（断点续跑）\n", len(existingModules)))
		modules = existingModules

		// 加载已有计划（仅用于产物展示）
		var plan model.ExecutionPlan
		if err := h.db.Where("project_id = ?", projectID).Order("id DESC").First(&plan).Error; err == nil {
			executionPlan = &plan
		}

		// 标记阶段0和阶段1为已完成
		stages[0].Status = "completed"
		stages[0].Message = "断点续跑：已有计划，跳过"
		if executionPlan != nil {
			planPreview := executionPlan.PlanContent
			if len([]rune(planPreview)) > 500 {
				planPreview = string([]rune(planPreview)[:500]) + "\n...(完整内容请点击查看)"
			}
			stages[0].Summary = "复用已有执行计划（断点续跑）"
			stages[0].Artifacts = []StageArtifact{
				{
					Type:        "markdown",
					Name:        "执行计划 (Plan.md)",
					Filename:    "plan.md",
					APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/plan", projectID),
					PreviewText: planPreview,
				},
			}
		}
		h.sendStageUpdate(w, stages)

		stages[1].Status = "completed"
		stages[1].Message = fmt.Sprintf("断点续跑：复用已有 %d 个模块", len(modules))
		var modulePreviewLines []string
		for i, m := range modules {
			taskCount := 0
			var tasks []model.Task
			h.db.Where("module_id = ?", m.ID).Find(&tasks)
			taskCount = len(tasks)
			modulePreviewLines = append(modulePreviewLines, fmt.Sprintf("模块%d: %s（%d个任务, status=%d）", i+1, m.Name, taskCount, m.Status))
		}
		modulePreviewText := strings.Join(modulePreviewLines, "\n")
		if len([]rune(modulePreviewText)) > 500 {
			modulePreviewText = string([]rune(modulePreviewText)[:500]) + "\n...(完整内容请点击查看)"
		}
		stages[1].Summary = fmt.Sprintf("复用已有 %d 个模块，继续按序执行。", len(modules))
		stages[1].Artifacts = []StageArtifact{
			{
				Type:        "markdown",
				Name:        "模块分解详情",
				Filename:    "modules.md",
				APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/modules", projectID),
				PreviewText: modulePreviewText,
			},
		}
		h.sendStageUpdate(w, stages)
	} else {
		// ========== 阶段1: 生成计划 ==========
		stages[0].Status = "running"
		h.sendStageUpdate(w, stages)

		requirement := project.Description
		if requirement == "" {
			requirement = project.Name
		}

		if !cliAvailable {
			// 必须走 AI，CLI 不可用时快速失败
			stages[0].Status = "failed"
			stages[0].Message = "Claude Code CLI 不可用，无法生成 AI 计划"
			w.SendOutput("\n✗ Claude Code CLI 不可用，流水线终止（必须走 AI）\n")
			h.sendStageUpdate(w, stages)
			h.sendPipelineDone(w, result, stages, "Claude Code CLI 不可用，流水线终止（必须走 AI）")
			return
		}
		executionPlan, err = h.planGenerator.GeneratePlan(projectID, requirement, func(output string) {
			w.SendOutput(output)
		})
		if err != nil {
			// AI 调用失败，快速失败（不降级到规则引擎）
			stages[0].Status = "failed"
			stages[0].Message = "AI 计划生成失败: " + err.Error()
			w.SendOutput(fmt.Sprintf("\n✗ AI 计划生成失败: %v\n", err))
			h.sendStageUpdate(w, stages)
			h.sendPipelineDone(w, result, stages, "AI 计划生成失败，流水线终止: "+err.Error())
			return
		}

		// 自动确认计划
		now := time.Now()
		executionPlan.Status = "confirmed"
		executionPlan.ConfirmedAt = &now
		h.db.Model(executionPlan).Updates(map[string]interface{}{
			"status":       "confirmed",
			"confirmed_at": &now,
		})

		stages[0].Status = "completed"
		// 生成计划阶段的概要总结和中间产物
		planPreview := executionPlan.PlanContent
		if len([]rune(planPreview)) > 500 {
			planPreview = string([]rune(planPreview)[:500]) + "\n...(完整内容请点击查看)"
		}
		stages[0].Summary = fmt.Sprintf("已基于需求「%s」生成执行计划，包含项目概述、技术栈选型、模块拆分和测试策略。计划已自动确认。", truncate(requirement, 50))
		stages[0].Artifacts = []StageArtifact{
			{
				Type:        "markdown",
				Name:        "执行计划 (Plan.md)",
				Filename:    "plan.md",
				APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/plan", projectID),
				PreviewText: planPreview,
			},
		}
		h.sendStageUpdate(w, stages)

		// ========== 阶段2: 分解任务 ==========
		stages[1].Status = "running"
		h.sendStageUpdate(w, stages)

		if !cliAvailable {
			// 必须走 AI，CLI 不可用时快速失败（plan 阶段已拦截，此处为防御性检查）
			stages[1].Status = "failed"
			stages[1].Message = "Claude Code CLI 不可用，无法进行 AI 任务分解"
			w.SendOutput("\n✗ Claude Code CLI 不可用，流水线终止（必须走 AI）\n")
			h.sendStageUpdate(w, stages)
			h.sendPipelineDone(w, result, stages, "Claude Code CLI 不可用，流水线终止（必须走 AI）")
			return
		}
		modules, err = h.decomposer.DecomposePlan(executionPlan.ID, func(output string) {
			w.SendOutput(output)
		})
		if err != nil {
			// AI 分解失败，快速失败（不降级到规则引擎）
			stages[1].Status = "failed"
			stages[1].Message = "AI 任务分解失败: " + err.Error()
			w.SendOutput(fmt.Sprintf("\n✗ AI 任务分解失败: %v\n", err))
			h.sendStageUpdate(w, stages)
			h.sendPipelineDone(w, result, stages, "AI 任务分解失败，流水线终止: "+err.Error())
			return
		}

		stages[1].Status = "completed"
		if stages[1].Message == "" {
			stages[1].Message = fmt.Sprintf("已分解 %d 个模块", len(modules))
		}
		// 生成分解阶段的概要总结和中间产物
		var modulePreviewLines []string
		for i, m := range modules {
			taskCount := 0
			var tasks []model.Task
			h.db.Where("module_id = ?", m.ID).Find(&tasks)
			taskCount = len(tasks)
			modulePreviewLines = append(modulePreviewLines, fmt.Sprintf("模块%d: %s（%d个任务）", i+1, m.Name, taskCount))
		}
		modulePreviewText := strings.Join(modulePreviewLines, "\n")
		if len([]rune(modulePreviewText)) > 500 {
			modulePreviewText = string([]rune(modulePreviewText)[:500]) + "\n...(完整内容请点击查看)"
		}
		stages[1].Summary = fmt.Sprintf("已将计划分解为 %d 个模块，共包含多个开发任务。各模块按依赖关系顺序执行。", len(modules))
		stages[1].Artifacts = []StageArtifact{
			{
				Type:        "markdown",
				Name:        "模块分解详情",
				Filename:    "modules.md",
				APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/modules", projectID),
				PreviewText: modulePreviewText,
			},
		}
		h.sendStageUpdate(w, stages)
	}

	// ========== 阶段3: 编码实现 ==========
	stages[2].Status = "running"
	h.sendStageUpdate(w, stages)

	if !cliAvailable {
		// CLI 不可用，使用代码脚手架生成可运行的项目代码
		w.SendOutput("Claude Code CLI 不可用，使用代码脚手架生成项目代码...\n")
		scaffolder := service.NewCodeScaffolder(h.db, h.cfg)
		if err := scaffolder.ScaffoldProject(projectID, modules, func(progress string) {
			w.SendOutput("  " + progress + "\n")
		}); err != nil {
			w.SendOutput(fmt.Sprintf("  ✗ 代码生成失败: %v\n", err))
			stages[2].Status = "failed"
			stages[2].Message = "代码生成失败: " + err.Error()
		} else {
			stages[2].Status = "completed"
			stages[2].Message = "代码脚手架已生成基础项目代码"
			w.SendOutput("  ✓ 项目代码生成完成（Go后端+Vue前端）\n")
		}
	} else {
		codeFailed := false
		// 设置编码阶段为执行中
		stages[2].Status = "running"
		stages[2].Message = "开始执行模块任务..."
		h.sendStageUpdate(w, stages)

		// 严格串行：业务模块必须依次通过自动测试门禁（部署→测试→修复，最多3轮）后才启动下一模块。
		// - 完成(4)：跳过
		// - 待测试(2)/测试中(3)：任务已跑完，直接进测试门禁（断点续跑/历史数据兼容）
		// - 其余（0/1/5/6）：（重新）跑任务，然后进测试门禁
		util.SortBySequenceNumber(modules, func(m model.Module) string { return m.SequenceNumber })
		for i, mod := range modules {
			w.SendOutput(fmt.Sprintf("\n▶ 开始执行模块 [%d/%d]: %s\n", i+1, len(modules), mod.Name))

			// 加载模块的最新状态
			h.db.First(&mod, mod.ID)

			switch resolveModuleAction(mod.Status) {
			case moduleActionSkip:
				// 历史回填：已完成但用例为空的业务模块，补跑一次用例生成（不重跑任务/测试）
				h.ensureModuleTestSpecs(&project, &mod, w)
				w.SendOutput(fmt.Sprintf("  模块 %s 已完成（status=%d），跳过\n", mod.Name, mod.Status))
				continue
			case moduleActionTestOnly:
				w.SendOutput(fmt.Sprintf("  ↻ 模块 %s 当前状态=%d（待测试/测试中），任务已完成，直接进入自动测试门禁\n", mod.Name, mod.Status))
				if !h.runBusinessModuleGate(&project, &mod, w) {
					stages[2].Status = "paused"
					stages[2].Message = fmt.Sprintf("模块 [%d/%d] %s 自动测试未通过（已自动修复重试 %d 轮），请人工介入后重启流水线", i+1, len(modules), mod.Name, executor.MaxTestRounds)
					h.sendStageUpdate(w, stages)
					h.sendPipelineDone(w, result, stages, fmt.Sprintf("模块 %s 自动测试失败，流水线已暂停；修复后再次启动可从断点续跑", mod.Name))
					return
				}
				continue
			}

			// moduleActionRunThenTest：先执行任务
			if mod.Status == executor.ModuleStatusTestFailed || mod.Status == executor.ModuleStatusFailed {
				w.SendOutput(fmt.Sprintf("  ↻ 模块 %s 处于失败状态(status=%d)，将重新执行任务\n", mod.Name, mod.Status))
			}

			// 更新编码阶段进度：当前执行的模块
			stages[2].Message = fmt.Sprintf("正在执行模块 [%d/%d]: %s", i+1, len(modules), mod.Name)
			h.sendStageUpdate(w, stages)

			err = h.taskExecutor.ExecuteModuleTasks(mod.ID, func(output string) {
				w.SendOutput(output)
				// 根据输出检测任务进度，更新阶段消息
				if strings.Contains(output, "开始执行任务:") {
					stages[2].Message = fmt.Sprintf("模块 [%d/%d] %s - %s", i+1, len(modules), mod.Name, strings.TrimSpace(strings.TrimPrefix(output, "开始执行任务:")))
					h.sendStageUpdate(w, stages)
				}
			})
			if err != nil {
				w.SendOutput(fmt.Sprintf("  ✗ 模块 %s 执行失败: %v\n", mod.Name, err))
				codeFailed = true
				// 基础架构模块失败必须中断流水线（后续业务模块依赖它）
				if mod.ModuleType == executor.ModuleTypeInfrastructure {
					stages[2].Status = "failed"
					stages[2].Message = fmt.Sprintf("基础架构模块 %s 任务执行失败，流水线终止", mod.Name)
					h.sendStageUpdate(w, stages)
					h.sendPipelineDone(w, result, stages, fmt.Sprintf("基础架构模块 %s 任务执行失败: %v", mod.Name, err))
					return
				}
				// 业务模块失败：标记后继续下一个模块（依赖检查会自动拦截依赖此模块的后续模块）
				continue
			}

			w.SendOutput(fmt.Sprintf("  ✓ 模块 %s 任务执行完成\n", mod.Name))

			// 任务跑完后按模块类型分流处理
			isInfra := mod.ModuleType == executor.ModuleTypeInfrastructure
			if isInfra {
				// 基础架构模块：自动调用 InfraVerify（构建 + 启动校验）
				w.SendOutput(fmt.Sprintf("  ▶ 模块 %s 为基础架构模块，自动执行构建校验 + 启动验证...\n", mod.Name))
				if vErr := h.runInfraVerificationForPipeline(mod.ID, func(output string) {
					w.SendOutput("  " + output)
				}); vErr != nil {
					w.SendOutput(fmt.Sprintf("  ✗ 模块 %s 构建/启动验证失败: %v\n", mod.Name, vErr))
					codeFailed = true
					// 基础架构模块失败必须中断流水线（后续业务模块依赖它）
					stages[2].Status = "failed"
					stages[2].Message = fmt.Sprintf("基础架构模块 %s 验证失败，流水线终止", mod.Name)
					h.sendStageUpdate(w, stages)
					h.sendPipelineDone(w, result, stages, fmt.Sprintf("基础架构模块 %s 验证失败: %v", mod.Name, vErr))
					return
				}
				w.SendOutput(fmt.Sprintf("  ✓ 模块 %s 构建与启动验证通过，状态置为完成\n", mod.Name))
			} else {
				// 业务模块：任务跑完后进入自动测试门禁（部署→测试→修复，最多3轮）
				if !h.runBusinessModuleGate(&project, &mod, w) {
					stages[2].Status = "paused"
					stages[2].Message = fmt.Sprintf("模块 [%d/%d] %s 自动测试未通过（已自动修复重试 %d 轮），请人工介入后重启流水线", i+1, len(modules), mod.Name, executor.MaxTestRounds)
					h.sendStageUpdate(w, stages)
					h.sendPipelineDone(w, result, stages, fmt.Sprintf("模块 %s 自动测试失败，流水线已暂停；修复后再次启动可从断点续跑", mod.Name))
					return
				}
			}

			w.SendOutput(fmt.Sprintf("  ✓ 模块 %s 已彻底完成\n", mod.Name))
			stages[2].Message = fmt.Sprintf("模块 [%d/%d] %s 已完成", i+1, len(modules), mod.Name)
			h.sendStageUpdate(w, stages)
		}

		if codeFailed {
			stages[2].Status = "failed"
			stages[2].Message = "部分模块执行失败，请在修复后重新启动流水线"
			h.sendStageUpdate(w, stages)
			h.sendPipelineDone(w, result, stages, "部分模块执行失败，流水线已终止")
			return
		}
		stages[2].Status = "completed"
		stages[2].Message = fmt.Sprintf("已完成 %d 个模块的编码", len(modules))
	}

	// 生成编码阶段的概要总结和中间产物
	if stages[2].Status == "completed" {
		var codeFileList []string
		if project.GitURL != "" {
			codeFileList = append(codeFileList, fmt.Sprintf("- Git仓库: %s", project.GitURL))
		}
		codeFileList = append(codeFileList, fmt.Sprintf("- 后端: Go + Gin (main.go, handler, model, config)"))
		codeFileList = append(codeFileList, fmt.Sprintf("- 前端: Vue 3 + Element Plus (App.vue, router, api)"))
		codeFileList = append(codeFileList, fmt.Sprintf("- 工作目录: %s", project.WorkDir))
		stages[2].Summary = fmt.Sprintf("已生成 %d 个模块对应的项目代码，包含 Go 后端和 Vue 前端，代码已推送到 Git 仓库。", len(modules))
		stages[2].Artifacts = []StageArtifact{
			{
				Type:        "text",
				Name:        "代码结构概览",
				Filename:    "code_structure.txt",
				APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/code-structure", projectID),
				PreviewText: strings.Join(codeFileList, "\n"),
			},
		}
	}
	h.sendStageUpdate(w, stages)

	// ========== 阶段4: 部署上线 ==========
	stages[3].Status = "running"
	h.sendStageUpdate(w, stages)

	deployment, err := h.deployService.Deploy(projectID, false, func(progress string) {
		w.SendOutput(progress + "\n")
	})
	if err != nil {
		stages[3].Status = "failed"
		stages[3].Message = err.Error()
		h.sendStageUpdate(w, stages)
		h.sendPipelineDone(w, result, stages, "部署失败: "+err.Error())
		return
	}

	stages[3].Status = "completed"
	stages[3].Message = "部署成功"
	result.Deployment = deployment
	if deployment != nil && deployment.AccessURL != "" {
		result.AccessURLs = append(result.AccessURLs, deployment.AccessURL)
	}
	// 生成部署阶段的概要总结和中间产物
	var deployInfo []string
	if deployment != nil {
		deployInfo = append(deployInfo, fmt.Sprintf("- 访问地址: %s", deployment.AccessURL))
		deployInfo = append(deployInfo, fmt.Sprintf("- 前端端口: %d", deployment.FrontendPort))
		deployInfo = append(deployInfo, fmt.Sprintf("- 后端端口: %d", deployment.BackendPort))
		deployInfo = append(deployInfo, fmt.Sprintf("- 部署目录: %s", deployment.BuildDir))
		deployInfo = append(deployInfo, fmt.Sprintf("- Nginx配置: %s", deployment.NginxConfigPath))
		deployInfo = append(deployInfo, fmt.Sprintf("- 后端进程PID: %d", deployment.BackendPID))
	}
	stages[3].Summary = fmt.Sprintf("项目已成功部署，可通过 %s 访问。前端、后端服务均已启动，Nginx 反向代理已配置。", func() string {
		if deployment != nil && deployment.AccessURL != "" {
			return deployment.AccessURL
		}
		return "分配的端口"
	}())
	stages[3].Artifacts = []StageArtifact{
		{
			Type:        "text",
			Name:        "部署信息",
			Filename:    "deployment_info.txt",
			APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/deployment", projectID),
			PreviewText: strings.Join(deployInfo, "\n"),
		},
	}
	if deployment != nil && deployment.AccessURL != "" {
		stages[3].Artifacts = append(stages[3].Artifacts, StageArtifact{
			Type:    "url",
			Name:    "访问地址",
			APIPath: deployment.AccessURL,
		})
	}
	h.sendStageUpdate(w, stages)

	// ========== 阶段5: 测试验证 ==========
	stages[4].Status = "running"
	h.sendStageUpdate(w, stages)

	if deployment != nil && deployment.AccessURL != "" {
		w.SendOutput(fmt.Sprintf("访问地址: %s\n", deployment.AccessURL))
		// 全局冒烟：仅当 tests/ 目录（含子目录）下确实存在 Playwright 用例文件时才运行；
		// 否则退化为可访问性检查（部署记录存在即通过），避免测试 agent 生成的
		// tests/results/module-<id>.json 导致 tests/ 目录存在但无用例而误失败。
		testsDir := filepath.Join(project.WorkDir, "tests")
		if hasPlaywrightSpecs(testsDir) {
			w.SendOutput("运行全局 Playwright 冒烟测试...\n")
			smokeRun, smokeErr := h.playwrightSvc.RunTest(projectID, nil, nil, "smoke", project.WorkDir, func(o string) {
				w.SendOutput(o)
			})
			if smokeErr != nil || smokeRun.Status != "passed" {
				stages[4].Status = "failed"
				stages[4].Message = "全局冒烟测试未通过"
				stages[4].Summary = "部署后全局 Playwright 冒烟测试存在失败用例，详见测试运行记录。"
				h.sendStageUpdate(w, stages)
				h.sendPipelineDone(w, result, stages, "全局冒烟测试未通过")
				return
			}
			stages[4].Status = "completed"
			stages[4].Message = "全局冒烟测试通过"
			stages[4].Summary = fmt.Sprintf("服务可访问，全局 Playwright 冒烟测试通过（%d 个用例）。", smokeRun.PassCount)
		} else {
			stages[4].Status = "completed"
			stages[4].Message = "服务验证通过"
			stages[4].Summary = fmt.Sprintf("已验证服务可正常访问，访问地址 %s 响应正常。", deployment.AccessURL)
		}
		stages[4].Artifacts = []StageArtifact{
			{
				Type:    "url",
				Name:    "在线访问",
				APIPath: deployment.AccessURL,
			},
		}
	} else {
		stages[4].Status = "skipped"
		stages[4].Message = "无访问URL，跳过测试"
		stages[4].Summary = "由于没有可访问的URL，测试阶段已跳过。"
	}
	h.sendStageUpdate(w, stages)

	// ========== 完成 ==========
	h.sendPipelineDone(w, result, stages, "")
}

// hasPlaywrightSpecs 检测 dir 目录（含子目录）下是否存在 Playwright 用例文件
//（*.spec.ts / *.spec.js / *.test.ts / *.test.js）。
func hasPlaywrightSpecs(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 继续遍历其他路径
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".spec.ts") ||
			strings.HasSuffix(name, ".spec.js") ||
			strings.HasSuffix(name, ".test.ts") ||
			strings.HasSuffix(name, ".test.js") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// ensureModuleTestSpecs 门禁第 0 步：业务模块用例（API/Web）为空时自动调测试设计 agent 生成并落库。
// 幂等（两字段均非空直接跳过）；生成失败仅告警不阻断——门禁降级为自由测试模式继续，
// 避免用例生成成为流水线的单点故障。生成成功后重新加载模块，让后续测试提示词拿到用例。
func (h *PipelineHandler) ensureModuleTestSpecs(project *model.Project, mod *model.Module, w ProgressWriter) {
	if mod.ModuleType == executor.ModuleTypeInfrastructure {
		return
	}
	if strings.TrimSpace(mod.APIIntegrationTest) != "" && strings.TrimSpace(mod.WebIntegrationTest) != "" {
		return
	}
	w.SendOutput("  ▶ 模块测试用例为空，启动测试设计 agent 生成 API/Playwright 用例...\n")
	if err := h.testDesigner.RunModuleTestDesign(project, mod, func(o string) { w.SendOutput(o) }); err != nil {
		w.SendOutput(fmt.Sprintf("  ⚠ 用例生成失败: %v（门禁将以自由测试模式继续）\n", err))
		return
	}
	if err := h.db.First(mod, mod.ID).Error; err != nil {
		w.SendOutput(fmt.Sprintf("  ⚠ 用例落库后重新加载模块失败: %v（本轮测试提示词可能不含用例）\n", err))
		return
	}
	w.SendOutput("  ✓ 模块测试用例已生成并落库\n")
}

// runBusinessModuleGate 业务模块质量门禁：部署 → 测试 agent → 失败则修复 agent 重试，
// 最多 MaxTestRounds 轮。全部通过返回 true 并把模块置为完成(4)；
// 轮次耗尽返回 false 并把模块置为测试失败(5)。
func (h *PipelineHandler) runBusinessModuleGate(project *model.Project, mod *model.Module, w ProgressWriter) bool {
	// 第 0 步：用例生成（幂等，仅业务模块且用例为空时真正执行）
	h.ensureModuleTestSpecs(project, mod, w)
	if err := h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", executor.ModuleStatusTesting).Error; err != nil {
		w.SendOutput(fmt.Sprintf("  ✗ 模块状态写入失败(status→3): %v\n", err))
		return false
	}

	var lastResult *executor.ModuleTestResult
	for round := 1; round <= executor.MaxTestRounds; round++ {
		// ① 全量部署（复用部署服务：build + 重启 + Nginx）
		w.SendOutput(fmt.Sprintf("  ▶ [第%d/%d轮] 重新构建并部署项目...\n", round, executor.MaxTestRounds))
		deployment, deployErr := h.deployService.Deploy(project.ID, false, func(p string) {
			w.SendOutput("  " + p + "\n")
		})

		if deployErr != nil {
			w.SendOutput(fmt.Sprintf("  ✗ [第%d/%d轮] 部署失败: %v\n", round, executor.MaxTestRounds, deployErr))
			lastResult = &executor.ModuleTestResult{
				ModuleID: mod.ID,
				Passed:   false,
				Summary:  "部署失败: " + deployErr.Error(),
				Failures: []executor.ModuleTestFailure{{Kind: "build", Name: "deploy", Log: deployErr.Error()}},
			}
		} else {
			accessURL := ""
			if deployment != nil {
				accessURL = deployment.AccessURL
			}
			// ② 测试 agent：集成测试 + Playwright UI 测试
			w.SendOutput(fmt.Sprintf("  ▶ [第%d/%d轮] 启动测试 agent 执行集成测试与 Playwright UI 测试...\n", round, executor.MaxTestRounds))
			testResult, testErr := h.testExecutor.RunModuleTest(project, mod, accessURL, round, func(o string) {
				w.SendOutput(o)
			})
			if testErr != nil {
				w.SendOutput(fmt.Sprintf("  ✗ 测试执行器内部错误: %v\n", testErr))
				testResult = &executor.ModuleTestResult{
					ModuleID: mod.ID,
					Passed:   false,
					Summary:  "测试执行器内部错误: " + testErr.Error(),
					Failures: []executor.ModuleTestFailure{{Kind: "agent", Name: "test-executor", Log: testErr.Error()}},
				}
			}
			lastResult = testResult
		}

		// ③ 判定
		if lastResult.Passed {
			if err := h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", executor.ModuleStatusCompleted).Error; err != nil {
				w.SendOutput(fmt.Sprintf("  ⚠ 模块状态写入失败(status→4): %v\n", err))
				w.SendOutput("  测试已通过，但状态写入失败；下次重启流水线时模块 status 仍为 3（测试中），会重新进门禁并自愈为完成。\n")
			}
			w.SendOutput(fmt.Sprintf("  ✓ 模块 %s 自动测试通过（第%d轮），状态置为完成\n", mod.Name, round))
			return true
		}
		w.SendOutput(fmt.Sprintf("  ✗ [第%d/%d轮] 测试未通过: %s\n", round, executor.MaxTestRounds, lastResult.Summary))

		if round == executor.MaxTestRounds {
			break
		}
		// ④ 修复 agent
		w.SendOutput(fmt.Sprintf("  ▶ [第%d/%d轮] 启动修复 agent 自动修复...\n", round, executor.MaxTestRounds))
		if fixErr := h.testExecutor.RunModuleFix(project, mod, lastResult, round, func(o string) {
			w.SendOutput(o)
		}); fixErr != nil {
			w.SendOutput(fmt.Sprintf("  ✗ 修复 agent 执行异常: %v（将继续下一轮测试）\n", fixErr))
		}
	}

	if err := h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", executor.ModuleStatusTestFailed).Error; err != nil {
		w.SendOutput(fmt.Sprintf("  ✗ 模块状态写入失败(status→5): %v\n", err))
	}
	w.SendOutput(fmt.Sprintf("  ✗ 模块 %s 经过 %d 轮自动测试/修复仍未通过，已置为「测试失败」\n", mod.Name, executor.MaxTestRounds))
	return false
}

// GetPipelineStatus 对应 GET /pipeline/project/:projectId/status。
// 返回当前项目的流水线状态（基于计划、模块、部署记录推断）。
func (h *PipelineHandler) GetPipelineStatus(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	stages := []PipelineStage{
		{Name: "plan", Label: "生成计划", Status: "pending"},
		{Name: "decompose", Label: "分解任务", Status: "pending"},
		{Name: "code", Label: "编码实现", Status: "pending"},
		{Name: "deploy", Label: "部署上线", Status: "pending"},
		{Name: "test", Label: "测试验证", Status: "pending"},
	}

	// 加载模块列表：编码/部署/测试阶段的真实进度以模块状态为准。
	// 注意：不能再用 execution_plans.status 推断编码阶段——流水线运行中该字段
	// 停在 confirmed 后不再推进，会导致编码永远显示 pending；
	// 也不能仅凭部署记录推断部署/测试阶段——业务模块测试门禁每轮都会全量部署，
	// 编码中途暂停时部署记录已存在，会把后两个阶段误染绿。
	var modules []model.Module
	h.db.Where("project_id = ?", projectID).Order("sequence_number ASC, id ASC").Find(&modules)

	// codeFinished: 编码阶段是否彻底完成（所有模块 status=4）。
	// 无模块（历史数据/尚未分解）时回退到旧的计划状态推断。
	codeFinished := false
	if len(modules) > 0 {
		allCompleted := true
		anyFailed := false
		anyActive := false // 执行中/测试中/待测试
		completedCount := 0
		for _, m := range modules {
			switch m.Status {
			case executor.ModuleStatusCompleted:
				completedCount++
			case executor.ModuleStatusFailed, executor.ModuleStatusTestFailed:
				anyFailed = true
				allCompleted = false
			case executor.ModuleStatusInProgress, executor.ModuleStatusPendingTest, executor.ModuleStatusTesting:
				anyActive = true
				allCompleted = false
			default: // pending
				allCompleted = false
			}
		}
		switch {
		case allCompleted:
			codeFinished = true
			stages[2].Status = "completed"
			stages[2].Message = fmt.Sprintf("已完成 %d 个模块的编码", len(modules))
			stages[2].Summary = fmt.Sprintf("全部 %d 个模块均已通过编码与自动测试门禁。", len(modules))
		case anyFailed:
			stages[2].Status = "paused"
			stages[2].Message = "存在执行失败/测试失败的模块，流水线已暂停，修复后可断点续跑"
			stages[2].Summary = fmt.Sprintf("%d/%d 个模块已完成，其余模块未通过自动测试门禁。", completedCount, len(modules))
		case anyActive || completedCount > 0:
			stages[2].Status = "running"
			stages[2].Message = fmt.Sprintf("编码进行中（%d/%d 个模块已完成）", completedCount, len(modules))
		}
	}

	// 查询计划状态（取最新一条，与断点续跑逻辑保持一致）
	var execPlan model.ExecutionPlan
	if err := h.db.Where("project_id = ?", projectID).Order("id DESC").First(&execPlan).Error; err == nil {
		stages[0].Status = "completed"
		// 补充计划阶段的概要总结与中间产物，保证从项目列表恢复时也能查看计划内容
		planPreview := execPlan.PlanContent
		if len([]rune(planPreview)) > 500 {
			planPreview = string([]rune(planPreview)[:500]) + "\n...(完整内容请点击查看)"
		}
		stages[0].Summary = "已生成执行计划，包含项目概述、技术栈选型、模块拆分和测试策略。"
		stages[0].Artifacts = []StageArtifact{
			{
				Type:        "markdown",
				Name:        "执行计划 (Plan.md)",
				Filename:    "plan.md",
				APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/plan", projectID),
				PreviewText: planPreview,
			},
		}

		if execPlan.Status == "confirmed" || execPlan.Status == "decomposed" ||
			execPlan.Status == "executing" || execPlan.Status == "completed" {
			stages[1].Status = "completed"
			// 补充分解阶段的概要总结与中间产物
			if len(modules) > 0 {
				var modulePreviewLines []string
				for i, m := range modules {
					var tasks []model.Task
					h.db.Where("module_id = ?", m.ID).Find(&tasks)
					modulePreviewLines = append(modulePreviewLines, fmt.Sprintf("模块%d: %s（%d个任务, status=%d）", i+1, m.Name, len(tasks), m.Status))
				}
				modulePreviewText := strings.Join(modulePreviewLines, "\n")
				if len([]rune(modulePreviewText)) > 500 {
					modulePreviewText = string([]rune(modulePreviewText)[:500]) + "\n...(完整内容请点击查看)"
				}
				stages[1].Summary = fmt.Sprintf("已将计划分解为 %d 个模块，各模块按依赖关系顺序执行。", len(modules))
				stages[1].Artifacts = []StageArtifact{
					{
						Type:        "markdown",
						Name:        "模块分解详情",
						Filename:    "modules.md",
						APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/modules", projectID),
						PreviewText: modulePreviewText,
					},
				}
			}
			if len(modules) == 0 &&
				(execPlan.Status == "executing" || execPlan.Status == "completed") {
				// 无模块记录的历史数据回退：按计划状态推断编码阶段
				stages[2].Status = "completed"
				codeFinished = true
			}
		}
	}

	// 查询部署状态
	var deployment model.ProjectDeployment
	accessUrls := []string{}
	if err := h.db.Where("project_id = ?", projectID).Order("id DESC").First(&deployment).Error; err == nil {
		if deployment.Status == "running" {
			if deployment.AccessURL != "" {
				accessUrls = append(accessUrls, deployment.AccessURL)
			}
			// 有模块记录时，部署/测试阶段必须以编码完成为前提：
			// 门禁过程中的中途部署不能把后两个阶段染绿。
			// 无模块记录（历史数据）时保持旧的记录存在即完成的推断。
			if len(modules) > 0 && !codeFinished {
				stages[3].Message = "编码未完成，整体部署尚未执行（现有部署记录来自模块测试门禁的中途部署）"
				stages[4].Message = "编码未完成，测试验证未执行"
			} else {
				stages[3].Status = "completed"
				stages[3].Summary = "项目已部署，前端、后端服务均已启动，Nginx 反向代理已配置。"
				deployInfo := []string{
					fmt.Sprintf("- 访问地址: %s", deployment.AccessURL),
					fmt.Sprintf("- 前端端口: %d", deployment.FrontendPort),
					fmt.Sprintf("- 后端端口: %d", deployment.BackendPort),
					fmt.Sprintf("- 部署目录: %s", deployment.BuildDir),
				}
				stages[3].Artifacts = []StageArtifact{
					{
						Type:        "text",
						Name:        "部署信息",
						Filename:    "deployment_info.txt",
						APIPath:     fmt.Sprintf("/pipeline/project/%d/artifact/deployment", projectID),
						PreviewText: strings.Join(deployInfo, "\n"),
					},
				}
				stages[4].Status = "completed"
				if deployment.AccessURL != "" {
					stages[4].Summary = fmt.Sprintf("已验证服务可正常访问，访问地址 %s 响应正常。", deployment.AccessURL)
					stages[4].Artifacts = []StageArtifact{
						{
							Type:    "url",
							Name:    "在线访问",
							APIPath: deployment.AccessURL,
						},
					}
				}
			}
		}
	}

	util.OKWithData(c, gin.H{
		"projectId":  projectID,
		"stages":     stages,
		"accessUrls": accessUrls,
	})
}

// ===== 内部方法 =====

// runInfraVerificationForPipeline 为流水线执行基础架构模块验证（构建校验 + 启动验证）。
// 仅用于 infrastructure 类型模块：在本地项目 work_dir 下执行构建命令（go build / npm build），
// 然后对最近一次部署记录的 AccessURL 发起 HTTP 探活。
// 任一步失败都会更新模块状态为 6(失败) 并返回 error；全部通过则更新为 4(完成)。
func (h *PipelineHandler) runInfraVerificationForPipeline(moduleID int64, onOutput func(string)) error {
	var module model.Module
	if err := h.db.First(&module, moduleID).Error; err != nil {
		return fmt.Errorf("加载模块失败: %w", err)
	}
	if module.ModuleType != executor.ModuleTypeInfrastructure {
		return fmt.Errorf("模块 %s 不是基础架构模块（当前 type=%s），无法执行 InfraVerify", module.Name, module.ModuleType)
	}

	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		return fmt.Errorf("加载项目失败: %w", err)
	}
	if project.WorkDir == "" {
		return fmt.Errorf("项目工作目录未设置，无法执行构建校验")
	}

	workDir := project.WorkDir

	// ===== 阶段1：构建校验（本地执行） =====
	if !runLocalBuildVerify(workDir, onOutput) {
		h.db.Model(&model.Module{}).Where("id = ?", moduleID).Update("status", 6)
		return fmt.Errorf("构建校验未通过")
	}

	// ===== 阶段2：启动验证（HTTP 探活，无部署记录时先自动部署） =====
	if !runStartupVerify(h.db, h.deployService, module.ProjectID, onOutput) {
		h.db.Model(&model.Module{}).Where("id = ?", moduleID).Update("status", 6)
		return fmt.Errorf("启动验证未通过")
	}

	h.db.Model(&model.Module{}).Where("id = ?", moduleID).Update("status", 4)
	return nil
}

// sendStageUpdate 发送阶段状态更新事件

// runLocalCommand 委托 service.RunLocalCommand（已迁移至 service 包以复用）。
func runLocalCommand(command string, timeout time.Duration) (string, error) {
	return service.RunLocalCommand(command, timeout)
}

// sendStageUpdate 发送阶段状态更新事件
func (h *PipelineHandler) sendStageUpdate(w ProgressWriter, stages []PipelineStage) {
	w.SendOutputJSON(map[string]interface{}{
		"type":   "stage_update",
		"stages": stages,
	})
}

// sendPipelineDone 发送流水线完成事件
func (h *PipelineHandler) sendPipelineDone(w ProgressWriter, result *PipelineResult, stages []PipelineStage, errMsg string) {
	result.Stages = stages
	if errMsg != "" {
		w.SendError(errMsg)
	}
	w.SendDone(result)
}

// buildCodeStructureTree 递归读取项目工作目录，生成类 tree 的目录结构字符串。
// 仅展示目录（不展示文件），并过滤掉依赖目录、隐藏目录和构建产物。
// maxDepth 控制递归深度，避免输出过长。
func (h *PipelineHandler) buildCodeStructureTree(root string, prefix string, depth int, maxDepth int) string {
	if root == "" || depth > maxDepth {
		return ""
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return ""
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	// 过滤：只保留非隐藏目录，并排除常见依赖/构建目录
	var dirs []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch name {
		case "node_modules", "vendor", "dist", "build", "target", ".venv", "venv", "__pycache__":
			continue
		}
		dirs = append(dirs, e)
	}

	// 目录排前面，按名称排序
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name() < dirs[j].Name()
	})

	// 限制单层展示数量，避免异常目录导致输出爆炸
	const maxEntries = 30
	if len(dirs) > maxEntries {
		dirs = dirs[:maxEntries]
	}

	var sb strings.Builder
	for i, e := range dirs {
		isLast := i == len(dirs)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}
		sb.WriteString(fmt.Sprintf("%s%s%s/\n", prefix, connector, e.Name()))
		sb.WriteString(h.buildCodeStructureTree(filepath.Join(root, e.Name()), childPrefix, depth+1, maxDepth))
	}
	return sb.String()
}

// GetArtifact 对应 GET /pipeline/project/:projectId/artifact/:type。
// 返回指定阶段的中间产物内容，支持在线查看和下载。
// type 可选值：plan, modules, code-structure, deployment
func (h *PipelineHandler) GetArtifact(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	artifactType := c.Param("type")
	if artifactType == "" {
		util.Fail(c, http.StatusBadRequest, "缺少产物类型参数")
		return
	}

	// 检查是否要求下载模式
	download := c.Query("download") == "1"

	switch artifactType {
	case "plan":
		// 查询执行计划
		var execPlan model.ExecutionPlan
		if err := h.db.Where("project_id = ?", projectID).First(&execPlan).Error; err != nil {
			util.Fail(c, http.StatusNotFound, "未找到执行计划")
			return
		}
		content := execPlan.PlanContent
		if download {
			c.Header("Content-Disposition", `attachment; filename="plan.md"`)
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(content))
		} else {
			util.OKWithData(c, gin.H{
				"type":     "markdown",
				"content":  content,
				"filename": "plan.md",
			})
		}

	case "modules":
		// 查询模块和任务
		var modules []model.Module
		h.db.Where("project_id = ?", projectID).Order("id ASC").Find(&modules)
		util.SortBySequenceNumber(modules, func(m model.Module) string { return m.SequenceNumber })

		var sb strings.Builder
		sb.WriteString("# 模块分解详情\n\n")
		for i, m := range modules {
			sb.WriteString(fmt.Sprintf("## 模块 %d: %s\n", i+1, m.Name))
			sb.WriteString(fmt.Sprintf("- 描述: %s\n", m.Description))
			sb.WriteString(fmt.Sprintf("- 序号: %s\n", m.SequenceNumber))
			if m.BlockedBy != "" {
				sb.WriteString(fmt.Sprintf("- 依赖: 模块 %s\n", m.BlockedBy))
			} else {
				sb.WriteString("- 依赖: 无\n")
			}

			// 查询任务
			var tasks []model.Task
			h.db.Where("module_id = ?", m.ID).Order("id ASC").Find(&tasks)
			util.SortBySequenceNumber(tasks, func(t model.Task) string { return t.SequenceNumber })
			if len(tasks) > 0 {
				sb.WriteString("\n### 任务列表\n")
				for _, t := range tasks {
					sb.WriteString(fmt.Sprintf("- **%s** %s — %s\n", t.SequenceNumber, t.Name, t.Description))
				}
			}
			sb.WriteString("\n")
		}
		content := sb.String()
		if download {
			c.Header("Content-Disposition", `attachment; filename="modules.md"`)
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(content))
		} else {
			util.OKWithData(c, gin.H{
				"type":     "markdown",
				"content":  content,
				"filename": "modules.md",
			})
		}

	case "code-structure":
		// 查询项目信息
		var project model.Project
		if err := h.db.First(&project, projectID).Error; err != nil {
			util.Fail(c, http.StatusNotFound, "项目不存在")
			return
		}
		var sb strings.Builder
		sb.WriteString("# 代码结构概览\n\n")
		sb.WriteString(fmt.Sprintf("## 项目: %s\n\n", project.Name))
		if project.GitURL != "" {
			sb.WriteString(fmt.Sprintf("- **Git仓库**: %s\n", project.GitURL))
		}
		sb.WriteString(fmt.Sprintf("- **工作目录**: %s\n", project.WorkDir))
		sb.WriteString(fmt.Sprintf("- **开发语言**: %s\n\n", project.DevLanguage))
		sb.WriteString("## 目录结构\n```\n")

		// 优先读取实际工作目录结构；如果目录不存在或为空，则展示通用说明。
		if project.WorkDir != "" {
			tree := h.buildCodeStructureTree(project.WorkDir, "", 0, 2)
			if tree != "" {
				sb.WriteString(fmt.Sprintf("%s/\n", project.WorkDir))
				sb.WriteString(tree)
			} else {
				sb.WriteString(fmt.Sprintf("%s/\n", project.WorkDir))
				sb.WriteString("（工作目录尚未生成代码，或当前无法读取目录结构）\n")
			}
		} else {
			sb.WriteString("（工作目录未设置）\n")
		}
		sb.WriteString("```\n")
		content := sb.String()
		if download {
			c.Header("Content-Disposition", `attachment; filename="code_structure.txt"`)
			c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(content))
		} else {
			util.OKWithData(c, gin.H{
				"type":     "text",
				"content":  content,
				"filename": "code_structure.txt",
			})
		}

	case "deployment":
		// 查询部署信息
		var deployment model.ProjectDeployment
		if err := h.db.Where("project_id = ?", projectID).First(&deployment).Error; err != nil {
			util.Fail(c, http.StatusNotFound, "未找到部署记录")
			return
		}
		var sb strings.Builder
		sb.WriteString("# 部署信息\n\n")
		sb.WriteString(fmt.Sprintf("- **访问地址**: %s\n", deployment.AccessURL))
		sb.WriteString(fmt.Sprintf("- **前端端口**: %d\n", deployment.FrontendPort))
		sb.WriteString(fmt.Sprintf("- **后端端口**: %d\n", deployment.BackendPort))
		sb.WriteString(fmt.Sprintf("- **部署目录**: %s\n", deployment.BuildDir))
		sb.WriteString(fmt.Sprintf("- **Nginx配置路径**: %s\n", deployment.NginxConfigPath))
		sb.WriteString(fmt.Sprintf("- **后端二进制路径**: %s\n", deployment.BackendBinary))
		sb.WriteString(fmt.Sprintf("- **后端进程PID**: %d\n", deployment.BackendPID))
		sb.WriteString(fmt.Sprintf("- **部署状态**: %s\n", deployment.Status))
		if deployment.LastDeployedAt != nil {
			sb.WriteString(fmt.Sprintf("- **最后部署时间**: %s\n", deployment.LastDeployedAt.Format("2006-01-02 15:04:05")))
		}
		content := sb.String()
		if download {
			c.Header("Content-Disposition", `attachment; filename="deployment_info.txt"`)
			c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(content))
		} else {
			util.OKWithData(c, gin.H{
				"type":     "text",
				"content":  content,
				"filename": "deployment_info.txt",
			})
		}

	default:
		util.Fail(c, http.StatusBadRequest, "未知的产物类型: "+artifactType)
	}
}

// truncate 截取字符串到指定 rune 长度，超出则添加省略号。
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// extractFeaturesFromRequirement 从需求描述中提取功能点。
// 按标点符号分句，过滤掉非功能描述。
func extractFeaturesFromRequirement(requirement string) []string {
	features := []string{}
	seen := map[string]bool{}

	addFeature := func(f string) {
		f = strings.TrimSpace(f)
		f = strings.TrimPrefix(f, "包含")
		f = strings.TrimPrefix(f, "包括")
		f = strings.TrimPrefix(f, "支持")
		f = strings.TrimPrefix(f, "实现")
		f = strings.TrimPrefix(f, "需要")
		f = strings.TrimSuffix(f, "等功能")
		f = strings.TrimSuffix(f, "功能")
		f = strings.TrimSuffix(f, "等")
		f = strings.TrimSpace(f)
		if f == "" || len([]rune(f)) < 2 || len([]rune(f)) > 20 {
			return
		}
		// 排除明显不是功能的句子
		badPrefixes := []string{"我想要", "我想做", "我要", "帮我", "请", "可以", "是一个", "是一款"}
		for _, p := range badPrefixes {
			if strings.HasPrefix(f, p) {
				return
			}
		}
		if !seen[f] {
			seen[f] = true
			features = append(features, f)
		}
	}

	// 按句号/分号/换行分句
	sentences := strings.FieldsFunc(requirement, func(r rune) bool {
		return r == '。' || r == '.' || r == '；' || r == ';' || r == '\n'
	})
	for _, sent := range sentences {
		sent = strings.TrimSpace(sent)
		if sent == "" {
			continue
		}
		// 检查是否是功能描述句
		isFeatureSent := strings.Contains(sent, "包含") ||
			strings.Contains(sent, "包括") ||
			strings.Contains(sent, "支持") ||
			strings.Contains(sent, "功能") ||
			strings.Contains(sent, "特性") ||
			strings.Contains(sent, "模块")
		if isFeatureSent {
			// 按顿号和逗号分割
			parts := strings.FieldsFunc(sent, func(r rune) bool {
				return r == '、' || r == '，' || r == ','
			})
			for _, p := range parts {
				addFeature(p)
			}
		} else if strings.Contains(sent, "、") {
			// 非功能描述句但有顿号，也尝试分割
			parts := strings.FieldsFunc(sent, func(r rune) bool {
				return r == '、'
			})
			for _, p := range parts {
				addFeature(p)
			}
		}
	}

	// 如果没有提取到功能，尝试按逗号分割全部文本
	if len(features) == 0 {
		parts := strings.FieldsFunc(requirement, func(r rune) bool {
			return r == '，' || r == ',' || r == '。' || r == '.' || r == '；' || r == ';' || r == '\n'
		})
		for _, p := range parts {
			addFeature(p)
		}
	}

	return features
}
