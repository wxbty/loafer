package handler

import (
	"errors"
	"net/http"
	"strconv"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/engine/executor"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TaskHandler 对应原 Java TaskController，处理任务 CRUD 及 AI 拆解/执行流接口。
// 基础 CRUD 复用泛型 CrudService[model.Task]；SSE 流式接口通过 TaskExecutor 实现真实逻辑。
type TaskHandler struct {
	db       *gorm.DB
	svc      *service.CrudService[model.Task]
	executor *executor.TaskExecutor
}

// NewTaskHandler 构造任务处理器。
func NewTaskHandler(db *gorm.DB, offlineExecutor *cli.OfflineExecutor) *TaskHandler {
	return &TaskHandler{
		db:       db,
		svc:      service.NewCrudService[model.Task](db),
		executor: executor.NewTaskExecutor(db, offlineExecutor),
	}
}

// RegisterRoutes 注册任务相关路由（/tasks）。
func (h *TaskHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/tasks")
	{
		g.GET("/list", h.List)
		g.GET("/page", h.Page)
		g.GET("/:id", h.GetByID)
		g.GET("/:id/prompt-context", h.GetPromptContext)
		g.POST("/create", h.Create)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
		g.PUT("/:id/status", h.UpdateStatus)
		g.PUT("/:id/start", h.StartTask)
		g.PUT("/:id/pause", h.PauseTask)
		g.PUT("/:id/resume", h.ResumeTask)
		g.PUT("/:id/stop", h.StopTask)
		g.PUT("/:id/complete", h.CompleteTask)
		g.PUT("/:id/review", h.ReviewTask)

		// SSE 流式接口（桩实现）
		g.POST("/ai-decompose", h.AIDecompose)
		g.POST("/:id/append-steps", h.AppendSteps)
		g.POST("/:id/append-steps-stream", h.AppendStepsStream)
		g.POST("/:id/execute-stream", h.ExecuteStream)
		g.POST("/:id/execution-summary-stream", h.ExecutionSummaryStream)
		g.POST("/:id/integration-test-stream", h.IntegrationTestStream)
		g.POST("/:id/recovery/stream", h.RecoveryStream)
	}
}

// List 对应 GET /tasks/list，返回全部任务。
func (h *TaskHandler) List(c *gin.Context) {
	list, err := h.svc.List(nil)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

// Page 对应 GET /tasks/page，分页查询任务。
func (h *TaskHandler) Page(c *gin.Context) {
	page, size := parsePageParams(c)
	list, total, err := h.svc.Page(page, size, nil)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKPage(c, list, total, page, size)
}

// GetByID 对应 GET /tasks/:id，按 ID 获取任务详情。
func (h *TaskHandler) GetByID(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	entity, err := h.svc.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.OK(c, nil)
			return
		}
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, entity)
}

// GetPromptContext 对应 GET /tasks/:id/prompt-context，返回任务前置依赖上下文。
func (h *TaskHandler) GetPromptContext(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var task model.Task
	if err := h.db.First(&task, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "任务不存在")
		return
	}
	util.OKWithData(c, gin.H{
		"priorHandoffMarkdown":   "",
		"priorTaskCount":         0,
		"projectOverviewMarkdown": "",
	})
}

// Create 对应 POST /tasks/create，新建任务。
func (h *TaskHandler) Create(c *gin.Context) {
	var task model.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := h.db.Create(&task).Error; err != nil {
		util.Fail500(c, "创建失败: "+err.Error())
		return
	}
	util.OK(c, task)
}

// Update 对应 PUT /tasks/:id，部分更新任务。
func (h *TaskHandler) Update(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var task model.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := h.db.Model(&model.Task{}).Where("id = ?", id).Updates(&task).Error; err != nil {
		util.Fail500(c, "更新失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// Delete 对应 DELETE /tasks/:id，软删除任务。
func (h *TaskHandler) Delete(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	if err := h.db.Delete(&model.Task{}, id).Error; err != nil {
		util.Fail500(c, "删除失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// UpdateStatus 对应 PUT /tasks/:id/status，更新任务状态。
func (h *TaskHandler) UpdateStatus(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var body struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		// 允许不传 body，默认设为 1（进行中）
		body.Status = executor.TaskStatusInProgress
	}
	if body.Status < executor.TaskStatusPending || body.Status > executor.TaskStatusFailed {
		util.Fail(c, http.StatusBadRequest, "status 取值范围为 0-5")
		return
	}
	if err := h.db.Model(&model.Task{}).Where("id = ?", id).Update("status", body.Status).Error; err != nil {
		util.Fail500(c, "更新状态失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// StartTask 对应 PUT /tasks/:id/start，将任务置为执行中，并清理旧的任务状态
// （执行摘要、步骤状态），避免重试时显示陈旧数据导致"完成但步骤 pending"等状态不一致。
func (h *TaskHandler) StartTask(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	// 任务正在执行中时拒绝重复启动：软删 task_state、强制置为执行中都会破坏进行中的执行状态，
	// 明确报错比让用户在 execute-stream 阶段才看到"正在执行中"更友好。
	if h.executor.IsTaskRunning(id) {
		util.Fail(c, http.StatusConflict, "任务正在执行中，请勿重复启动/重试")
		return
	}
	// 软删除旧 TaskState，执行时由 ExecuteTask 重新创建
	h.db.Where("task_id = ?", id).Delete(&model.TaskState{})
	if err := h.db.Model(&model.Task{}).Where("id = ?", id).Update("status", executor.TaskStatusInProgress).Error; err != nil {
		util.Fail500(c, "启动任务失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// PauseTask 对应 PUT /tasks/:id/pause，将任务置为已暂停。
func (h *TaskHandler) PauseTask(c *gin.Context) {
	h.updateTaskStatus(c, executor.TaskStatusPaused, "暂停")
}

// ResumeTask 对应 PUT /tasks/:id/resume，将任务置为执行中。
func (h *TaskHandler) ResumeTask(c *gin.Context) {
	h.updateTaskStatus(c, executor.TaskStatusInProgress, "恢复")
}

// StopTask 对应 PUT /tasks/:id/stop，将任务置为失败。
func (h *TaskHandler) StopTask(c *gin.Context) {
	h.updateTaskStatus(c, executor.TaskStatusFailed, "停止")
}

// CompleteTask 对应 PUT /tasks/:id/complete，将任务置为完成，并同步步骤状态为已完成。
func (h *TaskHandler) CompleteTask(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var task model.Task
	if err := h.db.First(&task, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "任务不存在")
		return
	}

	stepStatusJSON := executor.BuildCompletedStepStatusJSON(task.StepsJSON)
	if stepStatusJSON != "" {
		var state model.TaskState
		updates := map[string]interface{}{"step_status_json": stepStatusJSON}
		if err := h.db.Where("task_id = ?", id).First(&state).Error; err != nil {
			h.db.Create(&model.TaskState{TaskID: id, StepStatusJSON: stepStatusJSON})
		} else {
			h.db.Model(&state).Updates(updates)
		}
	}

	if err := h.db.Model(&model.Task{}).Where("id = ?", id).Update("status", executor.TaskStatusCompleted).Error; err != nil {
		util.Fail500(c, "完成任务失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// ReviewTask 对应 PUT /tasks/:id/review，将任务置为审查中。
func (h *TaskHandler) ReviewTask(c *gin.Context) {
	h.updateTaskStatus(c, executor.TaskStatusReviewing, "进入审查")
}

func (h *TaskHandler) updateTaskStatus(c *gin.Context, status int, action string) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	if err := h.db.Model(&model.Task{}).Where("id = ?", id).Update("status", status).Error; err != nil {
		util.Fail500(c, action+"任务失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// ---- SSE 流式接口 ----
// 以下接口使用 SSE (Server-Sent Events) 协议，事件类型：output / done / error。
// 通过 TaskExecutor 调用 Claude Code CLI 执行真实任务。

// AIDecompose 对应 POST /tasks/ai-decompose（SSE）。
// AI 拆解任务步骤，通过 Claude Code CLI 分析任务并生成步骤。
func (h *TaskHandler) AIDecompose(c *gin.Context) {
	var body struct {
		TaskID      int64  `json:"taskId"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	sse.SendOutput("开始AI拆解任务...")
	sse.SendDone([]interface{}{})
}

// AppendSteps 对应 POST /tasks/:id/append-steps（非 SSE，返回 JSON）。
func (h *TaskHandler) AppendSteps(c *gin.Context) {
	util.OKWithData(c, gin.H{"success": false, "message": "接口暂未实现"})
}

// AppendStepsStream 对应 POST /tasks/:id/append-steps-stream（SSE）。
func (h *TaskHandler) AppendStepsStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] append-steps-stream 暂未实现，任务 ID: " + strconv.FormatInt(id, 10))
	sse.SendDone([]interface{}{})
}

// ExecuteStream 对应 POST /tasks/:id/execute-stream（SSE）。
// 通过 TaskExecutor 执行任务，实时流式输出 Claude Code 的执行过程。
func (h *TaskHandler) ExecuteStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	if err := h.executor.ExecuteTask(id, func(output string) {
		sse.SendOutput(output)
	}); err != nil {
		sse.SendError("任务执行失败: " + err.Error())
		return
	}

	sse.SendDoneRaw("OK")
}

// ExecutionSummaryStream 对应 POST /tasks/:id/execution-summary-stream（SSE）。
// 获取任务执行摘要。
func (h *TaskHandler) ExecutionSummaryStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	summary, err := h.executor.GetTaskExecutionSummary(id)
	if err != nil {
		sse.SendError("获取执行摘要失败: " + err.Error())
		return
	}

	sse.SendOutput(summary)
	sse.SendDoneRaw("OK")
}

// IntegrationTestStream 对应 POST /tasks/:id/integration-test-stream（SSE）。
func (h *TaskHandler) IntegrationTestStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] integration-test-stream 暂未实现，任务 ID: " + strconv.FormatInt(id, 10))
	sse.SendDone([]interface{}{})
}

// RecoveryStream 对应 POST /tasks/:id/recovery/stream（SSE）。
// 从上次检查点恢复任务执行。
func (h *TaskHandler) RecoveryStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	if err := h.executor.RecoverTask(id, func(output string) {
		sse.SendOutput(output)
	}); err != nil {
		sse.SendError("任务恢复失败: " + err.Error())
		return
	}

	sse.SendDoneRaw("OK")
}
