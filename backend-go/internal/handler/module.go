package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"loafer-agent/internal/config"
	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/engine/executor"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ModuleHandler 对应原 Java ModuleController，处理模块 CRUD、批量保存、
// 修复历史查询等接口；TDD / SSE 流式接口暂为桩实现。
// 基础查询复用泛型 CrudService[model.Module]；create/update/delete 按原
// MyBatis-Plus 语义直接使用 GORM 会话，与 ProjectHandler 保持一致。
type ModuleHandler struct {
	db            *gorm.DB
	cfg           *config.Config
	svc           *service.CrudService[model.Module]
	executor      *executor.TaskExecutor
	deployService *service.DeployService
	testDesigner  *executor.TestDesignExecutor
	testExecutor  *executor.TestExecutor
}

// NewModuleHandler 构造模块处理器。
func NewModuleHandler(db *gorm.DB, offlineExecutor *cli.OfflineExecutor, cfg *config.Config) *ModuleHandler {
	return &ModuleHandler{
		db:            db,
		cfg:           cfg,
		svc:           service.NewCrudService[model.Module](db),
		executor:      executor.NewTaskExecutor(db, offlineExecutor),
		deployService: service.NewDeployService(db, cfg),
		testDesigner:  executor.NewTestDesignExecutor(db, offlineExecutor),
		testExecutor:  executor.NewTestExecutor(db, offlineExecutor),
	}
}

// RegisterRoutes 注册模块相关路由（/modules）。
func (h *ModuleHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/modules")
	{
		// 基础 CRUD
		g.POST("", h.Create)
		g.GET("/:id", h.GetByID)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)

		// 按项目维度查询 / 清理
		g.DELETE("/project/:projectId/all", h.DeleteAllByProject)
		g.GET("/project/:projectId", h.GetByProject)
		g.GET("/project/:projectId/executable", h.GetExecutable)

		// 批量保存 / 状态更新（桩）
		g.POST("/batch-save", h.BatchSave)
		g.PUT("/:id/status", h.UpdateStatus)

		// 修复历史
		g.GET("/:id/fix-history", h.ListFixHistory)
		g.GET("/fix-history/:historyId", h.GetFixHistoryDetail)

		// TDD / SSE 流式接口
		g.POST("/:id/tdd/run-all-assertions", h.RunAllAssertions)
		g.POST("/:id/tdd/run-assertion", h.RunAssertion)
		g.POST("/:id/tdd/fix-all-stream", h.FixAllStream)
		g.POST("/:id/tdd/fix-single-stream", h.FixSingleStream)
		g.POST("/:id/tdd/regenerate-criterion-stream", h.RegenerateCriterionStream)
		g.POST("/:id/generate-test-stream", h.GenerateTestStream)
		g.POST("/:id/scenarios/:type/generate-stream", h.GenerateScenarioStream)
		g.POST("/:id/scenarios/:type/:index/run-stream", h.RunScenarioStream)
		// 「全量测试」按钮：顺序执行该类型全部场景，失败时开发 agent 修复→强制部署→重测（≤3轮）
		g.POST("/:id/scenarios/:type/run-all-stream", h.RunAllScenariosStream)
		g.POST("/:id/append-tasks-stream", h.AppendTasksStream)
		g.POST("/decompose-stream", h.DecomposeStream)
		g.POST("/:id/execute-tasks-stream", h.ExecuteModuleTasksStream)
		// 基础架构模块验证：构建校验 + 启动验证（仅 infrastructure 模块）
		g.POST("/:id/infra-verify-stream", h.InfraVerifyStream)
	}

	// 模块测试截图（tests/results/screenshots/module-<id>/ 下的图片）。
	// 由于 <img> 无法携带 Bearer token，该路由需在 DefaultSkipPrefixes 中豁免认证。
	rg.GET("/module-screenshots/:id/:file", h.GetModuleScreenshot)
}

// Create 对应 POST /modules，新建模块并返回创建后的实体。
func (h *ModuleHandler) Create(c *gin.Context) {
	var module model.Module
	if err := c.ShouldBindJSON(&module); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := h.db.Create(&module).Error; err != nil {
		util.Fail500(c, "创建失败: "+err.Error())
		return
	}
	util.OKWithData(c, module)
}

// GetByID 对应 GET /modules/:id，按 ID 获取模块详情。
// 未找到时返回 {success:false, message:"模块不存在"}（对应 Java 的 200 + success:false）。
func (h *ModuleHandler) GetByID(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	entity, err := h.svc.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Fail(c, http.StatusOK, "模块不存在")
			return
		}
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, entity)
}

// Update 对应 PUT /modules/:id，按 ID 部分更新（仅更新非零字段，对应 updateById），
// 返回更新后的模块。
func (h *ModuleHandler) Update(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var module model.Module
	if err := c.ShouldBindJSON(&module); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	// GORM 的 Updates(struct) 会跳过零值字段：把 api_integration_test / web_integration_test
	// 清空（删除全部场景后变空串）时不写库，刷新后旧值又回来。用 Select 显式列出可更新列，
	// 强制零值也写入。status / created_at / updated_at / deleted / id 由专有端点或 GORM 管理，不在此覆盖。
	cols := []string{
		"name", "description", "sequence_number", "blocked_by", "module_type",
		"integration_test_spec", "api_integration_test", "web_integration_test",
		"pipeline_mode", "simple_mode",
		"tdd_step_status_json", "tdd_assertions_json", "tdd_test_spec_json",
	}
	if err := h.db.Model(&model.Module{}).Where("id = ?", id).Select(cols).Updates(&module).Error; err != nil {
		util.Fail500(c, "更新失败: "+err.Error())
		return
	}
	var updated model.Module
	if err := h.db.First(&updated, id).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, updated)
}

// Delete 对应 DELETE /modules/:id，软删除模块。
func (h *ModuleHandler) Delete(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	if err := h.db.Delete(&model.Module{}, id).Error; err != nil {
		util.Fail500(c, "删除失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// DeleteAllByProject 对应 DELETE /modules/project/:projectId/all，
// 在事务中删除项目下所有任务与模块。
func (h *ModuleHandler) DeleteAllByProject(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	tx := h.db.Begin()
	if err := tx.Where("project_id = ?", projectID).Delete(&model.Task{}).Error; err != nil {
		tx.Rollback()
		util.Fail500(c, "删除任务失败: "+err.Error())
		return
	}
	if err := tx.Where("project_id = ?", projectID).Delete(&model.Module{}).Error; err != nil {
		tx.Rollback()
		util.Fail500(c, "删除模块失败: "+err.Error())
		return
	}
	if err := tx.Commit().Error; err != nil {
		util.Fail500(c, "提交事务失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// GetByProject 对应 GET /modules/project/:projectId，返回项目下全部模块。
func (h *ModuleHandler) GetByProject(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	var modules []model.Module
	if err := h.db.Where("project_id = ?", projectID).
		Order("id ASC").
		Find(&modules).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.SortBySequenceNumber(modules, func(m model.Module) string { return m.SequenceNumber })
	util.OKWithData(c, modules)
}

// BatchSave 对应 POST /modules/batch-save，批量保存模块与任务。
// 桩实现：仅解析请求体中的 projectId 与 modulesWithTasks，实际批量保存逻辑暂未实现。
func (h *ModuleHandler) BatchSave(c *gin.Context) {
	var payload struct {
		ProjectID        interface{} `json:"projectId"`
		ModulesWithTasks interface{} `json:"modulesWithTasks"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	util.OKWithData(c, true)
}

// GetExecutable 对应 GET /modules/project/:projectId/executable，
// 返回项目下可执行模块（status >= 0 且未删除，本桩中即全部未删除模块）。
func (h *ModuleHandler) GetExecutable(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	var modules []model.Module
	if err := h.db.Where("project_id = ? AND status >= 0", projectID).
		Order("id ASC").
		Find(&modules).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.SortBySequenceNumber(modules, func(m model.Module) string { return m.SequenceNumber })
	util.OKWithData(c, modules)
}

// UpdateStatus 对应 PUT /modules/:id/status，更新模块状态。
// 支持任意合法状态值：0=待执行 / 1=执行中 / 2=待测试 / 3=测试中 / 4=完成 / 5=测试失败 / 6=失败。
func (h *ModuleHandler) UpdateStatus(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var body struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		// 允许不传 body，默认设为 1（进行中）
		body.Status = 1
	}
	if body.Status < 0 || body.Status > 6 {
		util.Fail(c, http.StatusBadRequest, "status 取值范围为 0-6")
		return
	}
	if err := h.db.Model(&model.Module{}).Where("id = ?", id).Update("status", body.Status).Error; err != nil {
		util.Fail500(c, "更新状态失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// ListFixHistory 对应 GET /modules/:id/fix-history，列出模块修复历史（按创建时间倒序）。
func (h *ModuleHandler) ListFixHistory(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var list []model.ModuleFixHistory
	if err := h.db.Where("module_id = ?", id).Order("created_at DESC").Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, list)
}

// GetFixHistoryDetail 对应 GET /modules/fix-history/:historyId，查询单条修复记录详情。
// 未找到时返回 {success:false, message:"记录不存在"}（对应 Java 的 200 + success:false）。
func (h *ModuleHandler) GetFixHistoryDetail(c *gin.Context) {
	historyID, ok := parsePathID(c, "historyId")
	if !ok {
		return
	}
	var history model.ModuleFixHistory
	if err := h.db.First(&history, historyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Fail(c, http.StatusOK, "记录不存在")
			return
		}
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, history)
}

// GetModuleScreenshot 对应 GET /module-screenshots/:id/:file。
// 提供模块最新一轮测试的 Playwright 截图；文件名经 ResolveScreenshotPath 严格校验防路径穿越。
func (h *ModuleHandler) GetModuleScreenshot(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "截图不存在")
		return
	}
	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "截图不存在")
		return
	}
	full, err := executor.ResolveScreenshotPath(project.WorkDir, module.ID, c.Param("file"))
	if err != nil {
		util.Fail(c, http.StatusNotFound, "截图不存在")
		return
	}
	if info, statErr := os.Stat(full); statErr != nil || info.IsDir() {
		util.Fail(c, http.StatusNotFound, "截图不存在")
		return
	}
	// 截图同名覆盖（每轮只留最新），禁止缓存
	c.Header("Cache-Control", "no-cache")
	c.File(full)
}

// ---- SSE 流式接口 ----
// 以下接口对应原 Java 返回 SseEmitter 的接口，使用 SSE (Server-Sent Events) 协议。
// 事件类型：output（进度）、done（完成）、error（错误）。
// 当前实现为桩：发送未实现提示后返回 done 事件，保持 SSE 协议一致性。

// RunAllAssertions 对应 POST /modules/:id/tdd/run-all-assertions。
func (h *ModuleHandler) RunAllAssertions(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] run-all-assertions 暂未实现，模块 ID: " + strconv.FormatInt(id, 10))

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}
	sse.SendDone(module)
}

// RunAssertion 对应 POST /modules/:id/tdd/run-assertion。
func (h *ModuleHandler) RunAssertion(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] run-assertion 暂未实现，模块 ID: " + strconv.FormatInt(id, 10))

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}
	sse.SendDone(module)
}

// FixAllStream 对应 POST /modules/:id/tdd/fix-all-stream。
func (h *ModuleHandler) FixAllStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] fix-all-stream 暂未实现，模块 ID: " + strconv.FormatInt(id, 10))

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}
	sse.SendDone(gin.H{"module": module, "fixHistory": nil})
}

// FixSingleStream 对应 POST /modules/:id/tdd/fix-single-stream?criteriaId=...
func (h *ModuleHandler) FixSingleStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] fix-single-stream 暂未实现，模块 ID: " + strconv.FormatInt(id, 10))

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}
	sse.SendDone(gin.H{"module": module, "fixHistory": nil})
}

// RegenerateCriterionStream 对应 POST /modules/:id/tdd/regenerate-criterion-stream?criterionId=...
func (h *ModuleHandler) RegenerateCriterionStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] regenerate-criterion-stream 暂未实现，模块 ID: " + strconv.FormatInt(id, 10))

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}
	sse.SendDone(module)
}

// GenerateTestStream 对应 POST /modules/:id/generate-test-stream?mode=LEGACY|TDD。
// LEGACY：调测试设计 agent 生成 API+Playwright 用例并落库（与门禁第 0 步同源）；
// TDD：验收标准生成暂未实现，保持桩提示。
func (h *ModuleHandler) GenerateTestStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	mode := c.DefaultQuery("mode", "TDD")
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}

	if mode != "LEGACY" {
		sse.SendOutput("[SSE] TDD 验收标准生成暂未实现，模块 ID: " + strconv.FormatInt(id, 10))
		sse.SendDone(module)
		return
	}

	// force=1：手动「AI 生成」按钮强制重新生成，覆盖已有用例。
	// 默认幂等：已有用例时不重新生成（门禁第 0 步复用同一方法）。
	force := c.DefaultQuery("force", "") == "1"
	if force {
		module.APIIntegrationTest = ""
		module.WebIntegrationTest = ""
		sse.SendOutput("[SSE] 强制重新生成，将覆盖现有 API/Web 用例\n")
	}

	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		sse.SendError("项目不存在: " + err.Error())
		return
	}
	sse.SendOutput("[SSE] 启动测试设计 agent 生成 API/Playwright 用例...\n")
	if err := h.testDesigner.RunModuleTestDesign(&project, &module, func(o string) { sse.SendOutput(o) }); err != nil {
		sse.SendError("用例生成失败: " + err.Error())
		return
	}
	// 返回落库后的最新模块，前端据此刷新用例面板
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("重新加载模块失败: " + err.Error())
		return
	}
	sse.SendDone(module)
}

// GenerateScenarioStream 对应 POST /modules/:id/scenarios/:type/generate-stream。
func (h *ModuleHandler) GenerateScenarioStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	scenarioType := c.Param("type")
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] generate-stream 暂未实现，模块 ID: " + strconv.FormatInt(id, 10) + ", type: " + scenarioType)

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}
	sse.SendDone(module)
}

// RunScenarioStream 对应 POST /modules/:id/scenarios/:type/:index/run-stream。
func (h *ModuleHandler) RunScenarioStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	scenarioType := c.Param("type") // api | web
	sceIndex, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		util.NewSSEWriter(c.Writer).SendError("场景索引无效: " + c.Param("index"))
		return
	}

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		util.NewSSEWriter(c.Writer).SendError("模块不存在: " + err.Error())
		return
	}

	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		util.NewSSEWriter(c.Writer).SendError("项目不存在: " + err.Error())
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	updatedMod, runErr := h.testExecutor.RunSingleScenario(&project, &module, scenarioType, sceIndex, func(o string) {
		sse.SendOutput(o)
	})
	if runErr != nil {
		sse.SendError(runErr.Error())
		return
	}
	sse.SendDone(updatedMod)
}

// RunAllScenariosStream 对应 POST /modules/:id/scenarios/:type/run-all-stream（SSE）。
// 「全量测试」按钮：顺序执行该类型全部场景；测试失败时开发 agent 修复 →
// 强制重新部署 → 重测，最多 executor.MaxTestRounds 轮。整个修复循环写入各场景
// 步骤明细（lastSteps）。done 帧返回回写里程碑后的最新模块（与 RunScenarioStream 契约一致）。
func (h *ModuleHandler) RunAllScenariosStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	scenarioType := c.Param("type") // api | web
	if scenarioType != "api" && scenarioType != "web" {
		util.NewSSEWriter(c.Writer).SendError("不支持的场景类型: " + scenarioType + "（仅支持 api/web）")
		return
	}

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		util.NewSSEWriter(c.Writer).SendError("模块不存在: " + err.Error())
		return
	}
	if module.ModuleType == executor.ModuleTypeInfrastructure {
		util.NewSSEWriter(c.Writer).SendError("基础架构模块不支持全量集成测试")
		return
	}
	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		util.NewSSEWriter(c.Writer).SendError("项目不存在: " + err.Error())
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	updatedMod, runErr := h.runModuleScenarioGate(&project, &module, scenarioType, func(o string) {
		sse.SendOutput(o)
	})
	if runErr != nil {
		sse.SendError(runErr.Error())
		return
	}
	sse.SendDone(updatedMod)
}

// runModuleScenarioGate 单类型「全量测试」门禁循环：顺序执行该类型全部场景；
// 失败时开发 agent 修复 → 强制重新部署 → 重测，最多 executor.MaxTestRounds 轮。
// 每轮为每个场景记录一条「第N轮测试」里程碑，修复/部署再补里程碑；循环结束后
// 把里程碑前置注入失败场景的 lastSteps（步骤明细可回放整个修复过程）。
// 全部通过模块状态置为 4(完成)，轮次耗尽置为 5(测试失败)。
func (h *ModuleHandler) runModuleScenarioGate(project *model.Project, mod *model.Module, scenarioType string, onOutput func(string)) (*model.Module, error) {
	var column, label string
	switch scenarioType {
	case "api":
		column, label = "api_integration_test", "API"
	default:
		column, label = "web_integration_test", "Web"
	}
	specJSON := strings.TrimSpace(mod.APIIntegrationTest)
	if scenarioType == "web" {
		specJSON = strings.TrimSpace(mod.WebIntegrationTest)
	}
	if specJSON == "" {
		return nil, fmt.Errorf("模块「%s」%s 用例为空，请先通过「AI 生成用例」生成 %s 用例", mod.Name, label, label)
	}

	if err := h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", executor.ModuleStatusTesting).Error; err != nil {
		return nil, fmt.Errorf("模块状态写入失败(status→3): %w", err)
	}

	// 里程碑：场景名 → 按时间顺序的循环记录；failedScenarios 标记至少失败过一次的场景
	cycleSteps := map[string][]executor.ScenarioStepResult{}
	failedScenarios := map[string]bool{}
	var lastResult *executor.ModuleTestResult

	for round := 1; round <= executor.MaxTestRounds; round++ {
		onOutput(fmt.Sprintf("▶ [全量%s测试·第%d/%d轮] 顺序执行全部场景...\n", label, round, executor.MaxTestRounds))

		// 修复后的轮次：强制重新部署，保证测试的是最新代码
		if round > 1 {
			onOutput("  ▶ 重新构建并部署项目...\n")
			var deployLog strings.Builder
			_, depErr := h.deployService.Deploy(project.ID, true, func(p string) {
				onOutput("  " + p + "\n")
				deployLog.WriteString(p)
				deployLog.WriteByte('\n')
			})
			if depErr != nil {
				onOutput(fmt.Sprintf("  ✗ 重新部署失败: %v\n", depErr))
				lastResult = &executor.ModuleTestResult{
					ModuleID: mod.ID,
					Passed:   false,
					Summary:  "重新部署失败: " + depErr.Error(),
					Failures: []executor.ModuleTestFailure{{Kind: "build", Name: "deploy", Log: depErr.Error()}},
				}
				break
			}
			for name := range failedScenarios {
				cycleSteps[name] = append(cycleSteps[name], executor.ScenarioStepResult{
					Action: "重新部署",
					OK:     true,
					Output: summarizeDeployLog(deployLog.String()),
				})
			}
		}

		// 本轮测试：顺序执行该类型全部场景
		onOutput(fmt.Sprintf("  ▶ 执行 %s 集成测试...\n", label))
		testResult, testErr := h.testExecutor.RunModuleScenariosTest(project, mod, scenarioType, round, func(o string) {
			onOutput(o)
		})
		if testErr != nil {
			onOutput(fmt.Sprintf("  ✗ %s 测试执行器内部错误: %v\n", label, testErr))
			testResult = &executor.ModuleTestResult{
				ModuleID: mod.ID,
				Passed:   false,
				Summary:  fmt.Sprintf("%s 测试执行器内部错误: %v", label, testErr),
				Failures: []executor.ModuleTestFailure{{Kind: "agent", Name: "test-executor", Log: testErr.Error()}},
			}
		}
		lastResult = testResult

		// 本轮逐场景里程碑
		currentFailed := map[string]bool{}
		for _, sc := range testResult.Scenarios {
			ms := executor.ScenarioStepResult{
				Action: fmt.Sprintf("第%d轮%s全量测试", round, label),
				OK:     sc.Passed,
				Output: tailSummary(sc.Log),
			}
			if !sc.Passed {
				ms.Error = tailSummary(sc.Log)
				currentFailed[sc.Name] = true
				failedScenarios[sc.Name] = true
			}
			cycleSteps[sc.Name] = append(cycleSteps[sc.Name], ms)
		}

		if testResult.Passed {
			onOutput(fmt.Sprintf("  ✓ [第%d/%d轮] %s 全量测试通过\n", round, executor.MaxTestRounds, label))
			break
		}
		onOutput(fmt.Sprintf("  ✗ [第%d/%d轮] %s 全量测试未通过: %s\n", round, executor.MaxTestRounds, label, testResult.Summary))
		if round == executor.MaxTestRounds {
			break
		}

		// 开发 agent 修复
		onOutput(fmt.Sprintf("  ▶ [第%d/%d轮] 启动开发 agent 修复...\n", round, executor.MaxTestRounds))
		var fixLog strings.Builder
		if fixErr := h.testExecutor.RunModuleFix(project, mod, testResult, round, func(o string) {
			onOutput(o)
			fixLog.WriteString(o)
			fixLog.WriteByte('\n')
		}); fixErr != nil {
			onOutput(fmt.Sprintf("  ✗ 修复 agent 执行异常: %v（将继续下一轮测试）\n", fixErr))
		}
		for name := range currentFailed {
			cycleSteps[name] = append(cycleSteps[name], executor.ScenarioStepResult{
				Action: fmt.Sprintf("开发agent修复第%d轮失败", round),
				OK:     true,
				Output: tailSummary(fixLog.String()),
			})
		}
	}

	// 循环结束：重新加载最新用例（含最后一轮回写的 lastRunAt/lastSuccess/lastSteps），
	// 把失败场景的里程碑前置注入，避免覆盖最后一轮的运行结果回写。
	if len(failedScenarios) > 0 {
		var fresh model.Module
		if err := h.db.First(&fresh, mod.ID).Error; err != nil {
			onOutput(fmt.Sprintf("  ⚠ 重新加载模块失败: %v\n", err))
		} else {
			cur := fresh.APIIntegrationTest
			if scenarioType == "web" {
				cur = fresh.WebIntegrationTest
			}
			injectMap := make(map[string][]executor.ScenarioStepResult, len(failedScenarios))
			for name := range failedScenarios {
				injectMap[name] = cycleSteps[name]
			}
			if updatedSpec := executor.InjectCycleStepsToSpec(cur, injectMap); updatedSpec != cur {
				if err := h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update(column, updatedSpec).Error; err != nil {
					onOutput(fmt.Sprintf("  ⚠ 步骤明细回写失败: %v\n", err))
				}
			}
		}
	}

	// 最终状态：全部通过 → 完成(4)；否则测试失败(5)
	finalStatus := executor.ModuleStatusTestFailed
	if lastResult != nil && lastResult.Passed {
		finalStatus = executor.ModuleStatusCompleted
	}
	if err := h.db.Model(&model.Module{}).Where("id = ?", mod.ID).Update("status", finalStatus).Error; err != nil {
		onOutput(fmt.Sprintf("  ⚠ 模块状态写入失败(status→%d): %v\n", finalStatus, err))
	}

	var updated model.Module
	if err := h.db.First(&updated, mod.ID).Error; err != nil {
		return mod, nil
	}
	return &updated, nil
}

// tailSummary 取字符串尾部 maxLen 个 rune（避免截断多字节字符），用于里程碑摘要。
func tailSummary(s string) string {
	runes := []rune(s)
	const maxLen = 200
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[len(runes)-maxLen:])
}

// summarizeDeployLog 从部署输出中提取简要摘要（取末尾几行并截断，避免整段刷进步骤明细）。
func summarizeDeployLog(log string) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	s := strings.TrimSpace(strings.Join(lines, "\n"))
	runes := []rune(s)
	if len(runes) > 300 {
		return string(runes[len(runes)-300:])
	}
	return s
}

// AppendTasksStream 对应 POST /modules/:id/append-tasks-stream。
func (h *ModuleHandler) AppendTasksStream(c *gin.Context) {
	moduleID, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] append-tasks-stream 暂未实现，模块 ID: " + strconv.FormatInt(moduleID, 10))
	sse.SendDone([]interface{}{})
}

// DecomposeStream 对应 POST /modules/decompose-stream。
func (h *ModuleHandler) DecomposeStream(c *gin.Context) {
	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()
	sse.SendOutput("[SSE] decompose-stream 暂未实现，请使用 /plans/:id/decompose 接口")
	sse.SendDone([]interface{}{})
}

// ExecuteModuleTasksStream 对应 POST /modules/:id/execute-tasks-stream（SSE）。
// 顺序执行模块下所有任务，实时流式输出执行过程。
func (h *ModuleHandler) ExecuteModuleTasksStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	if err := h.executor.ExecuteModuleTasks(id, func(output string) {
		sse.SendOutput(output)
	}); err != nil {
		sse.SendError("模块任务执行失败: " + err.Error())
		return
	}

	sse.SendDoneRaw("OK")
}

// InfraVerifyStream 对应 POST /modules/:id/infra-verify-stream（SSE）。
// 仅用于 infrastructure 类型模块：执行「构建校验 + 启动验证」两步。
// 1. 构建校验：在本地项目 work_dir 下执行 go build ./...（或前端构建）；
// 2. 启动验证：查询项目最近部署记录，对 AccessURL 发起 HTTP 探活。
// 任一步通过/失败都会通过 SSE 事件实时推送，最后根据结果将模块状态置为 4(完成) 或 6(失败)。
func (h *ModuleHandler) InfraVerifyStream(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	sendJSON := func(v interface{}) {
		sse.SendOutputJSON(v)
	}

	var module model.Module
	if err := h.db.First(&module, id).Error; err != nil {
		sse.SendError("模块不存在: " + err.Error())
		return
	}

	if module.ModuleType != executor.ModuleTypeInfrastructure {
		sse.SendError("仅基础架构模块（module_type=infrastructure）支持构建/启动验证；当前模块类型: " + module.ModuleType)
		return
	}

	var project model.Project
	if err := h.db.First(&project, module.ProjectID).Error; err != nil {
		sse.SendError("加载项目失败: " + err.Error())
		return
	}
	if project.WorkDir == "" {
		sse.SendError("项目工作目录未设置，无法执行构建校验")
		return
	}

	workDir := project.WorkDir

	// ===== 阶段1：构建校验（本地执行） =====
	sendJSON(map[string]interface{}{"type": "build_status", "status": "running"})

	buildPassed := runLocalBuildVerify(workDir, func(msg string) {
		sendJSON(map[string]interface{}{"type": "output", "data": msg + "\n"})
	})
	if buildPassed {
		sendJSON(map[string]interface{}{"type": "build_status", "status": "passed"})
	} else {
		sendJSON(map[string]interface{}{"type": "build_status", "status": "failed"})
	}

	if !buildPassed {
		// 构建失败，整体失败，更新模块状态为 6
		h.db.Model(&model.Module{}).Where("id = ?", id).Update("status", executor.ModuleStatusFailed)
		sendJSON(map[string]interface{}{
			"type":          "done",
			"failed":        true,
			"buildStatus":   "failed",
			"startupStatus": "pending",
			"message":       "构建校验失败，已停止后续验证",
		})
		return
	}

	// ===== 阶段2：启动验证 =====
	sendJSON(map[string]interface{}{"type": "startup_status", "status": "running"})

	startupPassed := runStartupVerify(h.db, h.deployService, module.ProjectID, func(msg string) {
		sendJSON(map[string]interface{}{"type": "output", "data": msg + "\n"})
	})
	if startupPassed {
		sendJSON(map[string]interface{}{"type": "startup_status", "status": "passed"})
	} else {
		sendJSON(map[string]interface{}{"type": "startup_status", "status": "failed"})
	}

	// ===== 更新模块状态 =====
	finalStatus := executor.ModuleStatusCompleted
	failed := false
	if !startupPassed {
		finalStatus = executor.ModuleStatusFailed
		failed = true
	}
	h.db.Model(&model.Module{}).Where("id = ?", id).Update("status", finalStatus)

	sendJSON(map[string]interface{}{
		"type":        "done",
		"failed":      failed,
		"buildStatus": "passed",
		"startupStatus": func() string {
			if startupPassed {
				return "passed"
			}
			return "failed"
		}(),
		"message": func() string {
			if startupPassed {
				return "构建校验 + 启动验证 均通过，模块已标记为完成"
			}
			return "启动验证失败"
		}(),
	})
}
