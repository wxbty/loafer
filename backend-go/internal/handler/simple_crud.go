package handler

import (
	"net/http"

	"loafer-agent/internal/engine/executor"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SimpleCrudHandlers 持有所有简单 CRUD 实体的服务和泛型处理器。
// 对应原 Java 中 ChecklistItem/DecisionLog/SliceHistory/TaskState/
// DependencyGraph/EnvironmentState/Checkpoint 等控制器。
type SimpleCrudHandlers struct {
	ChecklistItem    *GenericCrud[model.ChecklistItem]
	DecisionLog      *GenericCrud[model.DecisionLog]
	SliceHistory     *GenericCrud[model.SliceHistory]
	TaskState        *GenericCrud[model.TaskState]
	DependencyGraph  *GenericCrud[model.DependencyGraph]
	EnvironmentState *GenericCrud[model.EnvironmentState]
	Checkpoint       *GenericCrud[model.Checkpoint]

	// 各实体的 GORM 会话，用于自定义查询（by-task 等）
	db *gorm.DB
}

// NewSimpleCrudHandlers 构造全部简单 CRUD 处理器。
func NewSimpleCrudHandlers(db *gorm.DB) *SimpleCrudHandlers {
	return &SimpleCrudHandlers{
		ChecklistItem:    &GenericCrud[model.ChecklistItem]{Svc: service.NewCrudService[model.ChecklistItem](db)},
		DecisionLog:      &GenericCrud[model.DecisionLog]{Svc: service.NewCrudService[model.DecisionLog](db)},
		SliceHistory:     &GenericCrud[model.SliceHistory]{Svc: service.NewCrudService[model.SliceHistory](db)},
		TaskState:        &GenericCrud[model.TaskState]{Svc: service.NewCrudService[model.TaskState](db)},
		DependencyGraph:  &GenericCrud[model.DependencyGraph]{Svc: service.NewCrudService[model.DependencyGraph](db)},
		EnvironmentState: &GenericCrud[model.EnvironmentState]{Svc: service.NewCrudService[model.EnvironmentState](db)},
		Checkpoint:       &GenericCrud[model.Checkpoint]{Svc: service.NewCrudService[model.Checkpoint](db)},
		db:               db,
	}
}

// RegisterRoutes 注册所有简单 CRUD 路由。
func (h *SimpleCrudHandlers) RegisterRoutes(api *gin.RouterGroup) {
	// ===== ChecklistItem /api/checklist-items =====
	ci := api.Group("/checklist-items")
	{
		ci.GET("/by-task/:taskId", h.checklistByTask)
		ci.GET("/:id", h.ChecklistItem.GetByID)
		ci.POST("/create", h.ChecklistItem.Create)
		ci.PUT("/:id", h.ChecklistItem.Update)
		ci.DELETE("/:id", h.ChecklistItem.Delete)
	}

	// ===== DecisionLog /api/decision-logs =====
	dl := api.Group("/decision-logs")
	{
		dl.GET("/by-task/:taskId", h.decisionLogByTask)
		dl.GET("/:id", h.DecisionLog.GetByID)
		dl.POST("/create", h.DecisionLog.Create)
		dl.PUT("/:id", h.DecisionLog.Update)
		dl.DELETE("/:id", h.DecisionLog.Delete)
	}

	// ===== SliceHistory /api/slice-histories =====
	sh := api.Group("/slice-histories")
	{
		sh.GET("/by-task/:taskId", h.sliceHistoryByTask)
		sh.GET("/:id", h.SliceHistory.GetByID)
		sh.POST("/create", h.SliceHistory.Create)
		sh.PUT("/:id", h.SliceHistory.Update)
		sh.DELETE("/:id", h.SliceHistory.Delete)
	}

	// ===== TaskState /api/task-states =====
	ts := api.Group("/task-states")
	{
		ts.GET("/by-task/:taskId", h.taskStateByTask)
		ts.GET("/by-project/:projectId", h.taskStateByProject)
		ts.GET("/list", h.TaskState.List)
		ts.GET("/:id", h.TaskState.GetByID)
		ts.POST("/create", h.TaskState.Create)
		ts.PUT("/:id", h.TaskState.Update)
		ts.DELETE("/:id", h.TaskState.Delete)
	}

	// ===== DependencyGraph /api/dependency-graph =====
	dg := api.Group("/dependency-graph")
	{
		dg.GET("/list", h.DependencyGraph.List)
		dg.GET("/page", h.DependencyGraph.Page)
		dg.GET("/by-task/:taskId", h.dependencyGraphByTask)
		dg.GET("/:id", h.DependencyGraph.GetByID)
		dg.POST("/create", h.DependencyGraph.Create)
		dg.PUT("/:id", h.DependencyGraph.Update)
		dg.DELETE("/:id", h.DependencyGraph.Delete)
	}

	// ===== EnvironmentState /api/environment-state =====
	es := api.Group("/environment-state")
	{
		es.GET("/list", h.EnvironmentState.List)
		es.GET("/page", h.EnvironmentState.Page)
		es.GET("/by-task/:taskId", h.environmentStateByTask)
		es.GET("/:id", h.EnvironmentState.GetByID)
		es.POST("/create", h.EnvironmentState.Create)
		es.PUT("/:id", h.EnvironmentState.Update)
		es.DELETE("/:id", h.EnvironmentState.Delete)
	}

	// ===== Checkpoint /api/checkpoint =====
	cp := api.Group("/checkpoint")
	{
		cp.GET("/list", h.Checkpoint.List)
		cp.GET("/page", h.Checkpoint.Page)
		cp.GET("/by-task/:taskId", h.checkpointByTask)
		cp.GET("/:id", h.Checkpoint.GetByID)
		cp.POST("/create", h.Checkpoint.Create)
		cp.PUT("/:id", h.Checkpoint.Update)
		cp.DELETE("/:id", h.Checkpoint.Delete)
	}
}

// ---- 自定义查询：by-task / by-project ----

func (h *SimpleCrudHandlers) checklistByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	var list []model.ChecklistItem
	if err := h.db.Where("task_id = ?", taskID).Order("sequence ASC").Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

func (h *SimpleCrudHandlers) decisionLogByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	var list []model.DecisionLog
	if err := h.db.Where("task_id = ?", taskID).Order("created_at DESC").Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

func (h *SimpleCrudHandlers) sliceHistoryByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	var list []model.SliceHistory
	if err := h.db.Where("task_id = ?", taskID).Order("slice_sequence ASC").Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

func (h *SimpleCrudHandlers) taskStateByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	var ts model.TaskState
	if err := h.db.Where("task_id = ?", taskID).First(&ts).Error; err != nil {
		// task_state 缺失（如历史上 checkpoint_data 写入失败导致整条未入库）时，
		// 从最近一次完成的 slice_history 兜底构造展示状态，避免"完成但无摘要、步骤 pending"。
		fallback := h.synthesizeTaskStateFromSlices(taskID)
		util.OK(c, fallback)
		return
	}
	util.OK(c, ts)
}

// synthesizeTaskStateFromSlices 在 task_state 缺失时，根据最近的执行切片兜底构造展示状态。
// 返回 nil 表示无可用数据（前端按无状态处理）。
func (h *SimpleCrudHandlers) synthesizeTaskStateFromSlices(taskID int64) *model.TaskState {
	var slices []model.SliceHistory
	if err := h.db.Where("task_id = ?", taskID).Order("slice_sequence ASC").Find(&slices).Error; err != nil {
		return nil
	}
	// 取最近一个非空的 output_summary（ExecuteTask 中即 extractSummary 的结果）。
	var output string
	for i := len(slices) - 1; i >= 0; i-- {
		if slices[i].OutputSummary != "" {
			output = slices[i].OutputSummary
			break
		}
	}
	if output == "" {
		return nil
	}
	synth := &model.TaskState{
		TaskID:           taskID,
		ExecutionSummary: output,
	}
	// 任务本身已完成为 3 时，同步补全步骤状态为全部完成，前端据此展示步骤状态而非 pending。
	// 失败(5)/审查(2)等状态不补全，避免误标步骤为已完成。
	var task model.Task
	if err := h.db.First(&task, taskID).Error; err == nil && task.Status == executor.TaskStatusCompleted {
		if sj := executor.BuildCompletedStepStatusJSON(task.StepsJSON); sj != "" {
			synth.StepStatusJSON = sj
		}
	}
	return synth
}

func (h *SimpleCrudHandlers) taskStateByProject(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	var taskIDs []int64
	if err := h.db.Model(&model.Task{}).Where("project_id = ?", projectID).Pluck("id", &taskIDs).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	if len(taskIDs) == 0 {
		util.OK(c, []model.TaskState{})
		return
	}
	var list []model.TaskState
	if err := h.db.Where("task_id IN ?", taskIDs).Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

func (h *SimpleCrudHandlers) dependencyGraphByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	var list []model.DependencyGraph
	if err := h.db.Where("task_id = ?", taskID).Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

func (h *SimpleCrudHandlers) environmentStateByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	var list []model.EnvironmentState
	if err := h.db.Where("task_id = ?", taskID).Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

func (h *SimpleCrudHandlers) checkpointByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	var list []model.Checkpoint
	if err := h.db.Where("task_id = ?", taskID).Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

// 防止 http 未引用告警（部分 handler 未来扩展会用）
var _ = http.StatusOK
