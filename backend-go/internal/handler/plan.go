package handler

import (
	"net/http"

	"loafer-agent/internal/engine/cli"
	executor2 "loafer-agent/internal/engine/executor"
	"loafer-agent/internal/engine/plan"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PlanHandler 执行计划处理器，提供需求分析、计划生成、确认、拆解等接口。
// 对应前端对话窗口的核心后端逻辑：自然语言需求 → 生成plan.md → 确认 → 拆解为模块任务。
type PlanHandler struct {
	db         *gorm.DB
	generator  *plan.PlanGenerator
	decomposer *executor2.Decomposer
}

// NewPlanHandler 构造计划处理器。
func NewPlanHandler(db *gorm.DB, executor *cli.OfflineExecutor, docsService *service.DocsArtifactService) *PlanHandler {
	return &PlanHandler{
		db:         db,
		generator:  plan.NewPlanGenerator(db, executor, docsService),
		decomposer: executor2.NewDecomposer(db, executor, docsService),
	}
}

// RegisterRoutes 注册计划相关路由（/plans）。
func (h *PlanHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/plans")
	{
		// 计划生成与管理
		g.POST("/generate", h.GeneratePlan)       // SSE: 自然语言 → 执行计划
		g.POST("/:id/refine", h.RefinePlan)       // SSE: 优化计划
		g.PUT("/:id/confirm", h.ConfirmPlan)      // 确认计划
		g.POST("/:id/decompose", h.DecomposePlan) // SSE: 拆解为模块任务
		g.GET("/project/:projectId", h.GetPlan)   // 获取项目的计划
		g.GET("/:id", h.GetPlanByID)              // 按ID获取计划
	}
}

// GeneratePlan 对应 POST /plans/generate（SSE 流式）。
// 接收自然语言需求，通过 Claude Code CLI 生成执行计划。
func (h *PlanHandler) GeneratePlan(c *gin.Context) {
	var body struct {
		ProjectID   int64  `json:"projectId"`
		Requirement string `json:"requirement"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if body.ProjectID == 0 || body.Requirement == "" {
		util.Fail(c, http.StatusBadRequest, "projectId 和 requirement 不能为空")
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	executionPlan, err := h.generator.GeneratePlan(body.ProjectID, body.Requirement, func(output string) {
		sse.SendOutput(output)
	})
	if err != nil {
		sse.SendError("生成计划失败: " + err.Error())
		return
	}

	sse.SendDone(executionPlan)
}

// RefinePlan 对应 POST /plans/:id/refine（SSE 流式）。
// 根据用户反馈优化已有计划。
func (h *PlanHandler) RefinePlan(c *gin.Context) {
	planID, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	executionPlan, err := h.generator.RefinePlan(planID, body.Feedback, func(output string) {
		sse.SendOutput(output)
	})
	if err != nil {
		sse.SendError("优化计划失败: " + err.Error())
		return
	}

	sse.SendDone(executionPlan)
}

// ConfirmPlan 对应 PUT /plans/:id/confirm。
// 确认执行计划，将其状态从 draft 变为 confirmed。
func (h *PlanHandler) ConfirmPlan(c *gin.Context) {
	planID, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	if err := h.generator.ConfirmPlan(planID); err != nil {
		util.Fail500(c, "确认计划失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// DecomposePlan 对应 POST /plans/:id/decompose（SSE 流式）。
// 将已确认的计划拆解为具体的模块和任务。
func (h *PlanHandler) DecomposePlan(c *gin.Context) {
	planID, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	modules, err := h.decomposer.DecomposePlan(planID, func(output string) {
		sse.SendOutput(output)
	})
	if err != nil {
		sse.SendError("拆解计划失败: " + err.Error())
		return
	}

	sse.SendDone(modules)
}

// GetPlan 对应 GET /plans/project/:projectId。
// 获取项目的最新执行计划。
func (h *PlanHandler) GetPlan(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	executionPlan, err := h.generator.GetPlan(projectID)
	if err != nil {
		util.Fail(c, http.StatusOK, "计划不存在")
		return
	}
	util.OKWithData(c, executionPlan)
}

// GetPlanByID 对应 GET /plans/:id。
func (h *PlanHandler) GetPlanByID(c *gin.Context) {
	planID, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	executionPlan, err := h.generator.GetPlanByID(planID)
	if err != nil {
		util.Fail(c, http.StatusOK, "计划不存在")
		return
	}
	util.OKWithData(c, executionPlan)
}
