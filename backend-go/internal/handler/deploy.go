package handler

import (
	"net/http"

	"loafer-agent/internal/config"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DeployHandler 部署管理处理器，提供项目部署、卸载、状态查询等接口。
type DeployHandler struct {
	db      *gorm.DB
	cfg     *config.Config
	svc     *service.DeployService
}

// NewDeployHandler 构造部署处理器。
func NewDeployHandler(db *gorm.DB, cfg *config.Config) *DeployHandler {
	return &DeployHandler{
		db:  db,
		cfg: cfg,
		svc: service.NewDeployService(db, cfg),
	}
}

// RegisterRoutes 注册部署相关路由（/deploy）。
func (h *DeployHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/deploy")
	{
		g.POST("/project/:projectId", h.Deploy)
		g.DELETE("/project/:projectId", h.Undeploy)
		g.GET("/project/:projectId", h.GetDeployment)
		g.GET("/project/:projectId/status", h.GetStatus)
		g.GET("/project/:projectId/logs", h.GetLogs)
	}
}

// Deploy 对应 POST /deploy/project/:projectId（SSE 流式）。
// 执行完整部署流程：分配端口 → 供给数据库 → 构建前端 → 配置Nginx → 启动后端。
// 支持 force 参数（query 或 JSON body 均可）：为 true 时即使项目已运行也强制重新部署
//（复用既有端口，访问地址不变），用于「一键部署」等需要推送最新代码的场景。
func (h *DeployHandler) Deploy(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	// 读取 JSON body（空 body 或非法 JSON 时 Force 保持默认 false，不阻断部署）
	_ = c.ShouldBindJSON(&req)
	force := req.Force || c.Query("force") == "true" || c.Query("force") == "1"

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	deployment, err := h.svc.Deploy(projectID, force, func(progress string) {
		sse.SendOutput(progress)
	})
	if err != nil {
		sse.SendError("部署失败: " + err.Error())
		return
	}

	sse.SendDone(deployment)
}

// Undeploy 对应 DELETE /deploy/project/:projectId。
func (h *DeployHandler) Undeploy(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	if err := h.svc.Undeploy(projectID); err != nil {
		util.Fail500(c, "卸载失败: "+err.Error())
		return
	}
	util.OKWithData(c, true)
}

// GetDeployment 对应 GET /deploy/project/:projectId。
func (h *DeployHandler) GetDeployment(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	deployment, err := h.svc.GetDeployment(projectID)
	if err != nil {
		util.Fail(c, http.StatusOK, "部署记录不存在")
		return
	}
	util.OKWithData(c, deployment)
}

// GetStatus 对应 GET /deploy/project/:projectId/status。
func (h *DeployHandler) GetStatus(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	status, err := h.svc.GetDeployStatus(projectID)
	if err != nil {
		util.Fail500(c, "查询状态失败: "+err.Error())
		return
	}
	util.OKWithData(c, gin.H{"status": status})
}

// GetLogs 对应 GET /deploy/project/:projectId/logs。
func (h *DeployHandler) GetLogs(c *gin.Context) {
	projectID, ok := parsePathID(c, "projectId")
	if !ok {
		return
	}

	deployment, err := h.svc.GetDeployment(projectID)
	if err != nil {
		util.Fail(c, http.StatusOK, "部署记录不存在")
		return
	}
	util.OKWithData(c, deployment.DeployLog)
}
