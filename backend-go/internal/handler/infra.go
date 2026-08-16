package handler

import (
	"net/http"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InfraHandler 基础设施处理器，统一管理端口分配、数据库供给、短信配置、测试运行。
type InfraHandler struct {
	db             *gorm.DB
	cfg            *config.Config
	portAllocator  *service.PortAllocator
	dbProvisioner  *service.DatabaseProvisioner
	smsService     *service.SmsService
	playwrightSvc  *service.PlaywrightService
}

// NewInfraHandler 构造基础设施处理器。
func NewInfraHandler(db *gorm.DB, cfg *config.Config) *InfraHandler {
	return &InfraHandler{
		db:            db,
		cfg:           cfg,
		portAllocator: service.NewPortAllocator(db, &cfg.Infra),
		dbProvisioner: service.NewDatabaseProvisioner(db, &cfg.Database),
		smsService:    service.NewSmsService(db, cfg),
		playwrightSvc: service.NewPlaywrightService(db, cfg),
	}
}

// RegisterRoutes 注册基础设施相关路由。
func (h *InfraHandler) RegisterRoutes(rg *gin.RouterGroup) {
	// 端口管理
	ports := rg.Group("/infra/ports")
	{
		ports.GET("/allocated", h.GetAllocatedPorts)
		ports.GET("/project/:projectId", h.GetProjectPorts)
		ports.POST("/project/:projectId/allocate", h.AllocatePort)
		ports.DELETE("/:port", h.ReleasePort)
		ports.GET("/range", h.GetPortRange)
	}

	// 数据库供给
	dbs := rg.Group("/infra/database")
	{
		dbs.POST("/project/:projectId/provision", h.ProvisionDatabase)
		dbs.DELETE("/project/:projectId", h.DropDatabase)
		dbs.GET("/project/:projectId", h.GetProjectDatabase)
	}

	// 短信服务
	sms := rg.Group("/infra/sms")
	{
		sms.POST("/send", h.SendSMS)
		sms.POST("/project/:projectId/notify", h.SendProjectNotification)
		sms.GET("/project/:projectId/config", h.GetSmsConfig)
		sms.PUT("/project/:projectId/config", h.SaveSmsConfig)
	}

	// 测试管理
	tests := rg.Group("/infra/tests")
	{
		tests.POST("/project/:projectId/run", h.RunTest)
		tests.POST("/project/:projectId/generate-spec", h.GenerateTestSpec)
		tests.GET("/project/:projectId", h.ListProjectTests)
		tests.GET("/:runId", h.GetTestRun)
	}
}

// ---- 端口管理 ----

// GetPortRange 对应 GET /infra/ports/range。
func (h *InfraHandler) GetPortRange(c *gin.Context) {
	util.OKWithData(c, gin.H{
		"start": h.cfg.Infra.PortRangeStart,
		"end":   h.cfg.Infra.PortRangeEnd,
	})
}

// GetAllocatedPorts 对应 GET /infra/ports/allocated。
func (h *InfraHandler) GetAllocatedPorts(c *gin.Context) {
	ports, err := h.portAllocator.GetAllocatedPorts()
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, ports)
}

// GetProjectPorts 对应 GET /infra/ports/project/:projectId。
func (h *InfraHandler) GetProjectPorts(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	ports, err := h.portAllocator.GetProjectPorts(projectID)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, ports)
}

// AllocatePort 对应 POST /infra/ports/project/:projectId/allocate。
func (h *InfraHandler) AllocatePort(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	var body struct {
		PortType    string `json:"portType"`
		Description string `json:"description"`
	}
	_ = c.ShouldBindJSON(&body)

	port, err := h.portAllocator.AllocatePort(projectID, body.PortType, body.Description)
	if err != nil {
		util.Fail500(c, "分配端口失败: "+err.Error())
		return
	}
	util.OKWithData(c, gin.H{"port": port})
}

// ReleasePort 对应 DELETE /infra/ports/:port。
func (h *InfraHandler) ReleasePort(c *gin.Context) {
	portStr := c.Param("port")
	port := 0
	for _, ch := range portStr {
		if ch < '0' || ch > '9' {
			util.Fail(c, http.StatusBadRequest, "无效的端口号")
			return
		}
		port = port*10 + int(ch-'0')
	}
	if err := h.portAllocator.ReleasePort(port); err != nil {
		util.Fail500(c, "释放端口失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// ---- 数据库供给 ----

// ProvisionDatabase 对应 POST /infra/database/project/:projectId/provision。
func (h *InfraHandler) ProvisionDatabase(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	projectDB, err := h.dbProvisioner.ProvisionDatabase(projectID)
	if err != nil {
		util.Fail500(c, "创建数据库失败: "+err.Error())
		return
	}
	util.OKWithData(c, projectDB)
}

// DropDatabase 对应 DELETE /infra/database/project/:projectId。
func (h *InfraHandler) DropDatabase(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	if err := h.dbProvisioner.DropDatabase(projectID); err != nil {
		util.Fail500(c, "删除数据库失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// GetProjectDatabase 对应 GET /infra/database/project/:projectId。
func (h *InfraHandler) GetProjectDatabase(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	projectDB, err := h.dbProvisioner.GetProjectDatabase(projectID)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, projectDB)
}

// ---- 短信服务 ----

// SendSMS 对应 POST /infra/sms/send。
func (h *InfraHandler) SendSMS(c *gin.Context) {
	var body struct {
		PhoneNumbers  []string          `json:"phoneNumbers"`
		TemplateParams map[string]string `json:"templateParams"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := h.smsService.SendSMS(body.PhoneNumbers, body.TemplateParams); err != nil {
		util.Fail500(c, "发送短信失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// SendProjectNotification 对应 POST /infra/sms/project/:projectId/notify。
func (h *InfraHandler) SendProjectNotification(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	var body struct {
		PhoneNumbers []string `json:"phoneNumbers"`
		ProjectName  string   `json:"projectName"`
		Status       string   `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := h.smsService.SendProjectNotification(projectID, body.PhoneNumbers, body.ProjectName, body.Status); err != nil {
		util.Fail500(c, "发送通知失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// GetSmsConfig 对应 GET /infra/sms/project/:projectId/config。
func (h *InfraHandler) GetSmsConfig(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	cfg, err := h.smsService.GetProjectSmsConfig(projectID)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, cfg)
}

// SaveSmsConfig 对应 PUT /infra/sms/project/:projectId/config。
func (h *InfraHandler) SaveSmsConfig(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	var cfg model.SmsConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	cfg.ProjectID = projectID
	if err := h.smsService.SaveProjectSmsConfig(projectID, cfg); err != nil {
		util.Fail500(c, "保存配置失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// ---- 测试管理 ----

// RunTest 对应 POST /infra/tests/project/:projectId/run（SSE 流式）。
func (h *InfraHandler) RunTest(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	var body struct {
		ModuleID *int64 `json:"moduleId"`
		TaskID   *int64 `json:"taskId"`
		TestType string `json:"testType"`
		WorkDir  string `json:"workDir"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.TestType == "" {
		body.TestType = "playwright"
	}

	// 查询项目工作目录
	if body.WorkDir == "" {
		var project model.Project
		if err := h.db.First(&project, projectID).Error; err == nil {
			body.WorkDir = project.WorkDir
		}
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	testRun, err := h.playwrightSvc.RunTest(projectID, body.ModuleID, body.TaskID, body.TestType, body.WorkDir, func(output string) {
		sse.SendOutput(output)
	})
	if err != nil {
		sse.SendError("测试运行失败: " + err.Error())
		return
	}

	sse.SendDone(testRun)
}

// GenerateTestSpec 对应 POST /infra/tests/project/:projectId/generate-spec。
func (h *InfraHandler) GenerateTestSpec(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	var body struct {
		URL         string `json:"url"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	specPath, err := h.playwrightSvc.GenerateTestSpec(projectID, body.URL, body.Description)
	if err != nil {
		util.Fail500(c, "生成测试配置失败: "+err.Error())
		return
	}
	util.OKWithData(c, gin.H{"path": specPath})
}

// ListProjectTests 对应 GET /infra/tests/project/:projectId。
func (h *InfraHandler) ListProjectTests(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}
	tests, err := h.playwrightSvc.ListProjectTests(projectID)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKWithData(c, tests)
}

// GetTestRun 对应 GET /infra/tests/:runId。
func (h *InfraHandler) GetTestRun(c *gin.Context) {
	runID, ok := parsePathID(c, "runId")
	if !ok {
		return
	}
	testRun, err := h.playwrightSvc.GetTestRun(runID)
	if err != nil {
		util.Fail(c, http.StatusOK, "测试记录不存在")
		return
	}
	util.OKWithData(c, testRun)
}
