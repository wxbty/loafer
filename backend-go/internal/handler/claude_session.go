package handler

import (
	"net/http"
	"strings"
	"time"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ClaudeSessionHandler 对应原 Java ClaudeSessionController，
// 提供 Claude Code 会话的创建、停止、恢复与状态查询接口。
type ClaudeSessionHandler struct {
	pool *cli.SessionPool
	pm   *cli.ProcessManager
	db   *gorm.DB
}

// NewClaudeSessionHandler 构造 Claude 会话处理器。
func NewClaudeSessionHandler(pool *cli.SessionPool, pm *cli.ProcessManager, db *gorm.DB) *ClaudeSessionHandler {
	return &ClaudeSessionHandler{pool: pool, pm: pm, db: db}
}

// RegisterRoutes 注册 Claude 会话相关路由（/claude/sessions）。
func (h *ClaudeSessionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/claude/sessions")
	{
		g.POST("", h.CreateSession)
		g.DELETE("/:sessionId", h.CloseSession)
		g.GET("", h.ListSessions)
		g.GET("/last-active", h.GetLastActiveSession)
		g.GET("/claude-sessions", h.ListClaudeSessions)
		g.POST("/resume", h.ResumeSession)

		g.GET("/:sessionId", h.GetSession)
		g.GET("/:sessionId/status", h.GetSessionStatus)
		g.GET("/:sessionId/running", h.IsSessionRunning)
		g.GET("/:sessionId/exitCode", h.GetExitCode)
		g.GET("/:sessionId/errorInfo", h.GetErrorInfo)
		g.POST("/:sessionId/write", h.WriteToStdin)
		g.GET("/:sessionId/output", h.GetOutput)
		g.GET("/:sessionId/recent-output", h.GetRecentOutput)

		g.POST("/sessions/pool", h.CreateSessionFromPool)
		g.GET("/sessions/pool/status", h.GetPoolStatus)
		g.GET("/sessions/pool/project/:projectId", h.GetSessionsByProject)
		g.GET("/sessions/pool/task/:taskId", h.GetSessionsByTask)
		g.PUT("/sessions/pool/config", h.UpdatePoolConfig)
		g.POST("/sessions/pool/cleanup", h.CleanupExpiredSessions)

		g.GET("/projects/:projectId/work-dir", h.GetProjectWorkDir)
		g.POST("/projects/:projectId/work-dir", h.SetProjectWorkDir)
	}

	// 前端设置页使用的 session-pool 路由（/system-config/session-pool）
	sp := rg.Group("/system-config/session-pool")
	{
		sp.GET("/status", h.GetSessionPoolStatus)
		sp.GET("/cc-sessions", h.GetCcSessions)
		sp.POST("/cleanup", h.CleanupSessionPool)
		sp.DELETE("/sessions/:sessionId", h.ClosePoolSession)
	}
}

// CreateSession 对应 POST /claude/sessions，创建新会话。
func (h *ClaudeSessionHandler) CreateSession(c *gin.Context) {
	projectID := c.Query("projectId")
	if projectID == "" {
		apiFail(c, "projectId 参数不能为空")
		return
	}

	var taskID *int64
	if tid := c.Query("taskId"); tid != "" {
		if parsed, err := parsePathIDStr(tid); err == nil {
			taskID = &parsed
		}
	}
	profileID := c.Query("profileId")

	// 从数据库获取工作目录
	workDir, err := h.resolveWorkDir(projectID)
	if err != nil {
		apiFail(c, "无法解析项目工作目录: "+err.Error())
		return
	}

	sessionID, err := h.pool.CreateSession(projectID, taskID, workDir, "", profileID)
	if err != nil {
		apiFail(c, "创建会话失败: "+err.Error())
		return
	}

	h.pool.WaitForSessionReady(sessionID, 30)

	apiSuccess(c, gin.H{
		"sessionId":    sessionID,
		"webSocketUrl": "/ws/terminal/" + sessionID,
		"status":       "RUNNING",
	})
}

// CloseSession 对应 DELETE /claude/sessions/:sessionId，关闭会话。
func (h *ClaudeSessionHandler) CloseSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if h.pool.DestroySession(sessionID) {
		apiSuccessMsg(c, "会话已关闭")
	} else {
		apiFail(c, "关闭会话失败")
	}
}

// GetSession 对应 GET /claude/sessions/:sessionId，获取会话信息。
func (h *ClaudeSessionHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	handle := h.pool.GetSession(sessionID)
	if handle == nil {
		apiFail(c, "会话不存在")
		return
	}
	apiSuccess(c, h.handleToMap(handle, sessionID))
}

// GetSessionStatus 对应 GET /claude/sessions/:sessionId/status。
func (h *ClaudeSessionHandler) GetSessionStatus(c *gin.Context) {
	sessionID := c.Param("sessionId")
	status := h.pm.GetStatus(sessionID)
	apiSuccess(c, string(status))
}

// IsSessionRunning 对应 GET /claude/sessions/:sessionId/running。
func (h *ClaudeSessionHandler) IsSessionRunning(c *gin.Context) {
	sessionID := c.Param("sessionId")
	apiSuccess(c, h.pm.IsRunning(sessionID))
}

// GetExitCode 对应 GET /claude/sessions/:sessionId/exitCode。
func (h *ClaudeSessionHandler) GetExitCode(c *gin.Context) {
	sessionID := c.Param("sessionId")
	code := h.pm.GetExitCode(sessionID)
	apiSuccess(c, code)
}

// GetErrorInfo 对应 GET /claude/sessions/:sessionId/errorInfo。
func (h *ClaudeSessionHandler) GetErrorInfo(c *gin.Context) {
	sessionID := c.Param("sessionId")
	apiSuccess(c, h.pm.GetErrorInfo(sessionID))
}

// WriteToStdin 对应 POST /claude/sessions/:sessionId/write。
func (h *ClaudeSessionHandler) WriteToStdin(c *gin.Context) {
	sessionID := c.Param("sessionId")
	command := c.Query("command")
	if err := h.pm.WriteToStdin(sessionID, []byte(command)); err != nil {
		apiFail(c, "写入失败: "+err.Error())
		return
	}
	apiSuccessMsg(c, "命令已写入")
}

// GetOutput 对应 GET /claude/sessions/:sessionId/output。
func (h *ClaudeSessionHandler) GetOutput(c *gin.Context) {
	sessionID := c.Param("sessionId")
	output := h.pm.GetOutput(sessionID)
	apiSuccess(c, []string{string(output)})
}

// GetRecentOutput 对应 GET /claude/sessions/:sessionId/recent-output。
func (h *ClaudeSessionHandler) GetRecentOutput(c *gin.Context) {
	sessionID := c.Param("sessionId")
	output := h.pm.GetOutput(sessionID)
	// 取最后的内容
	lines := strings.Split(string(output), "\n")
	apiSuccess(c, lines)
}

// ListSessions 对应 GET /claude/sessions，列出项目的所有会话。
func (h *ClaudeSessionHandler) ListSessions(c *gin.Context) {
	projectID := c.Query("projectId")
	if projectID == "" {
		apiFail(c, "projectId 参数不能为空")
		return
	}
	handles := h.pool.GetSessionsByProject(projectID)
	list := make([]gin.H, 0, len(handles))
	for _, handle := range handles {
		list = append(list, h.handleToMap(handle, handle.SessionID()))
	}
	apiSuccess(c, list)
}

// GetLastActiveSession 对应 GET /claude/sessions/last-active。
func (h *ClaudeSessionHandler) GetLastActiveSession(c *gin.Context) {
	projectID := c.Query("projectId")
	if projectID == "" {
		apiFail(c, "projectId 参数不能为空")
		return
	}
	handles := h.pool.GetSessionsByProject(projectID)
	if len(handles) == 0 {
		apiSuccessMsg(c, "无活跃会话")
		return
	}
	// 优先返回进程存活的
	var best *cli.SessionHandle
	for _, h2 := range handles {
		if h2.ProcessAlive() {
			if best == nil || h2.LastActiveAt().After(best.LastActiveAt()) {
				best = h2
			}
		}
	}
	if best == nil {
		for _, h2 := range handles {
			if h2.ClaudeSessionUUID() != "" {
				if best == nil || h2.LastActiveAt().After(best.LastActiveAt()) {
					best = h2
				}
			}
		}
	}
	if best == nil {
		apiSuccessMsg(c, "无可恢复会话")
		return
	}
	apiSuccess(c, gin.H{
		"sessionId":         best.SessionID(),
		"claudeSessionUuid": best.ClaudeSessionUUID(),
		"projectId":         best.ProjectID(),
		"taskId":            best.TaskID(),
		"isProcessAlive":    best.ProcessAlive(),
		"resumable":         best.ClaudeSessionUUID() != "",
		"lastActiveAt":      best.LastActiveAt(),
	})
}

// ListClaudeSessions 对应 GET /claude/sessions/claude-sessions（桩实现）。
func (h *ClaudeSessionHandler) ListClaudeSessions(c *gin.Context) {
	apiSuccess(c, []interface{}{})
}

// ResumeSession 对应 POST /claude/sessions/resume，恢复会话。
func (h *ClaudeSessionHandler) ResumeSession(c *gin.Context) {
	projectID := c.Query("projectId")
	claudeSessionUUID := c.Query("claudeSessionUuid")
	if projectID == "" || claudeSessionUUID == "" {
		apiFail(c, "projectId 和 claudeSessionUuid 参数不能为空")
		return
	}

	var taskID *int64
	if tid := c.Query("taskId"); tid != "" {
		if parsed, err := parsePathIDStr(tid); err == nil {
			taskID = &parsed
		}
	}
	profileID := c.Query("profileId")

	workDir, err := h.resolveWorkDir(projectID)
	if err != nil {
		apiFail(c, "无法解析项目工作目录: "+err.Error())
		return
	}

	sessionID, err := h.pool.CreateSession(projectID, taskID, workDir, claudeSessionUUID, profileID)
	if err != nil {
		apiFail(c, "恢复会话失败: "+err.Error())
		return
	}

	h.pool.WaitForSessionReady(sessionID, 30)

	apiSuccess(c, gin.H{
		"sessionId":         sessionID,
		"claudeSessionUuid": claudeSessionUUID,
		"webSocketUrl":      "/ws/terminal/" + sessionID,
		"status":            "RUNNING",
		"resumed":           true,
	})
}

// CreateSessionFromPool 对应 POST /claude/sessions/sessions/pool。
func (h *ClaudeSessionHandler) CreateSessionFromPool(c *gin.Context) {
	projectID := c.Query("projectId")
	workDir := c.Query("workDir")
	if projectID == "" || workDir == "" {
		apiFail(c, "projectId 和 workDir 参数不能为空")
		return
	}
	var taskID *int64
	if tid := c.Query("taskId"); tid != "" {
		if parsed, err := parsePathIDStr(tid); err == nil {
			taskID = &parsed
		}
	}
	sessionID, err := h.pool.CreateSession(projectID, taskID, workDir, "", "")
	if err != nil {
		apiFail(c, "创建会话失败: "+err.Error())
		return
	}
	apiSuccess(c, sessionID)
}

// GetPoolStatus 对应 GET /claude/sessions/sessions/pool/status。
func (h *ClaudeSessionHandler) GetPoolStatus(c *gin.Context) {
	apiSuccess(c, h.pool.GetStatusInfo())
}

// GetSessionsByProject 对应 GET /claude/sessions/sessions/pool/project/:projectId。
func (h *ClaudeSessionHandler) GetSessionsByProject(c *gin.Context) {
	projectID := c.Param("projectId")
	handles := h.pool.GetSessionsByProject(projectID)
	list := make([]gin.H, 0, len(handles))
	for _, handle := range handles {
		list = append(list, h.handleToMap(handle, handle.SessionID()))
	}
	apiSuccess(c, list)
}

// GetSessionsByTask 对应 GET /claude/sessions/sessions/pool/task/:taskId。
func (h *ClaudeSessionHandler) GetSessionsByTask(c *gin.Context) {
	taskID, ok := parsePathID(c, "taskId")
	if !ok {
		return
	}
	handles := h.pool.GetSessionsByTask(taskID)
	list := make([]gin.H, 0, len(handles))
	for _, handle := range handles {
		list = append(list, h.handleToMap(handle, handle.SessionID()))
	}
	apiSuccess(c, list)
}

// UpdatePoolConfig 对应 PUT /claude/sessions/sessions/pool/config。
func (h *ClaudeSessionHandler) UpdatePoolConfig(c *gin.Context) {
	maxSize := parseQueryIntDefault(c, "maxPoolSize", 5)
	idleTimeout := parseQueryIntDefault(c, "idleTimeout", 40)
	h.pool.SetMaxPoolSize(maxSize)
	h.pool.SetIdleTimeout(idleTimeout)
	apiSuccessMsg(c, "会话池配置已更新")
}

// CleanupExpiredSessions 对应 POST /claude/sessions/sessions/pool/cleanup。
func (h *ClaudeSessionHandler) CleanupExpiredSessions(c *gin.Context) {
	h.pool.CleanupExpiredSessions()
	apiSuccessMsg(c, "过期会话已清理")
}

// GetSessionPoolStatus 对应 GET /system-config/session-pool/status。
// 返回前端期望的结构化会话池状态。
func (h *ClaudeSessionHandler) GetSessionPoolStatus(c *gin.Context) {
	info := h.pool.GetDetailedStatusInfo()
	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"activeCount":        info.ActiveCount,
		"pendingCount":       info.PendingCount,
		"maxPoolSize":        info.MaxPoolSize,
		"idleTimeoutMinutes": info.IdleTimeoutMinutes,
		"isFull":             info.IsFull,
	})
}

// GetCcSessions 对应 GET /system-config/session-pool/cc-sessions。
// 返回当前所有 CC 终端会话的详细信息。
func (h *ClaudeSessionHandler) GetCcSessions(c *gin.Context) {
	handles := h.pool.GetAllSessions()
	sessions := make([]gin.H, 0, len(handles))
	for _, handle := range handles {
		sessions = append(sessions, gin.H{
			"sessionId":         handle.SessionID(),
			"projectId":         handle.ProjectID(),
			"projectName":       h.getProjectName(handle.ProjectID()),
			"taskId":            handle.TaskID(),
			"claudeSessionUuid": handle.ClaudeSessionUUID(),
			"status":            string(h.pm.GetStatus(handle.SessionID())),
			"isProcessAlive":    handle.ProcessAlive(),
			"isExecuting":       false,
			"createdAt":         handle.CreatedAt(),
			"lastActiveAt":      handle.LastActiveAt(),
			"idleMinutes":       int(time.Since(handle.LastActiveAt()).Minutes()),
			"resumed":           handle.Resumed(),
			"profileId":         nil,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"sessions": sessions,
	})
}

// CleanupSessionPool 对应 POST /system-config/session-pool/cleanup。
func (h *ClaudeSessionHandler) CleanupSessionPool(c *gin.Context) {
	h.pool.CleanupExpiredSessions()
	apiSuccessMsg(c, "过期会话已清理")
}

// ClosePoolSession 对应 DELETE /system-config/session-pool/sessions/:sessionId。
func (h *ClaudeSessionHandler) ClosePoolSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if h.pool.DestroySession(sessionID) {
		apiSuccessMsg(c, "会话已关闭")
	} else {
		apiFail(c, "关闭会话失败")
	}
}

// getProjectName 根据项目 ID 获取项目名称。
func (h *ClaudeSessionHandler) getProjectName(projectID string) string {
	var project model.Project
	if err := h.db.Select("name").First(&project, projectID).Error; err != nil {
		return ""
	}
	return project.Name
}

// GetProjectWorkDir 对应 GET /claude/sessions/projects/:projectId/work-dir。
func (h *ClaudeSessionHandler) GetProjectWorkDir(c *gin.Context) {
	projectID := c.Param("projectId")
	workDir, err := h.resolveWorkDir(projectID)
	if err != nil {
		apiFail(c, "无法解析工作目录: "+err.Error())
		return
	}
	apiSuccess(c, workDir)
}

// SetProjectWorkDir 对应 POST /claude/sessions/projects/:projectId/work-dir。
func (h *ClaudeSessionHandler) SetProjectWorkDir(c *gin.Context) {
	projectID := c.Param("projectId")
	workDir := c.Query("workDir")
	if workDir == "" {
		apiFail(c, "workDir 参数不能为空")
		return
	}
	// 更新数据库中的工作目录
	h.db.Model(&model.Project{}).Where("id = ?", projectID).Update("work_dir", workDir)
	apiSuccessMsg(c, "工作目录已设置")
}

// ---- 内部辅助方法 ----

func (h *ClaudeSessionHandler) resolveWorkDir(projectID string) (string, error) {
	var project model.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		return "", err
	}
	if project.WorkDir != "" {
		return project.WorkDir, nil
	}
	return "", nil
}

func (h *ClaudeSessionHandler) handleToMap(handle *cli.SessionHandle, sessionID string) gin.H {
	return gin.H{
		"sessionId":         handle.SessionID(),
		"claudeSessionUuid": handle.ClaudeSessionUUID(),
		"projectId":         handle.ProjectID(),
		"taskId":            handle.TaskID(),
		"webSocketUrl":      "/ws/terminal/" + sessionID,
		"status":            string(h.pm.GetStatus(sessionID)),
		"createdAt":         handle.CreatedAt(),
		"lastActiveAt":      handle.LastActiveAt(),
		"isProcessAlive":    handle.ProcessAlive(),
		"resumed":           handle.Resumed(),
	}
}

func parsePathIDStr(s string) (int64, error) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, errInvalidID
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}

func parseQueryIntDefault(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// apiSuccess 返回 {success:true, data:...} 格式的响应（对应 Java ApiResponse）。
func apiSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

// apiSuccessMsg 返回 {success:true, message:...} 格式的响应。
func apiSuccessMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": msg, "data": nil})
}

// apiFail 返回 {success:false, message:...} 格式的响应。
func apiFail(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": false, "message": msg, "data": nil})
}

var errInvalidID = &invalidIDError{}

type invalidIDError struct{}

func (e *invalidIDError) Error() string { return "invalid ID" }

// 防止 util 未引用告警（handler 中部分接口使用 util.Fail）
var _ = util.OK
