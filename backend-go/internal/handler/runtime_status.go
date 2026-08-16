package handler

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RuntimeStatusHandler 对应原 Java RuntimeStatusController，
// 提供前后端进程状态、访问链接、系统信息等接口。
type RuntimeStatusHandler struct {
	db    *gorm.DB
	cfg   *config.Config
}

// NewRuntimeStatusHandler 构造运行状态处理器。
func NewRuntimeStatusHandler(db *gorm.DB, cfg *config.Config) *RuntimeStatusHandler {
	return &RuntimeStatusHandler{db: db, cfg: cfg}
}

// RegisterRoutes 注册运行状态相关路由（/runtime-status）。
func (h *RuntimeStatusHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/runtime-status")
	{
		g.GET("", h.GetRuntimeStatus)
		g.GET("/backend", h.GetBackendStatus)
		g.GET("/frontend", h.GetFrontendStatus)
		g.GET("/access-urls", h.GetAccessUrls)
		g.GET("/system", h.GetSystemInfo)
	}
}

// GetRuntimeStatus 对应 GET /runtime-status，获取指定项目的运行状态总览。
func (h *RuntimeStatusHandler) GetRuntimeStatus(c *gin.Context) {
	projectIDStr := c.Query("projectId")
	var project model.Project
	projectWorkDir, _ := os.Getwd()
	projectName := "当前项目"

	if projectIDStr != "" {
		if pid, err := strconv.ParseInt(projectIDStr, 10, 64); err == nil {
			if err := h.db.First(&project, pid).Error; err == nil {
				if project.WorkDir != "" {
					projectWorkDir = project.WorkDir
				}
				if project.Name != "" {
					projectName = project.Name
				}
			}
		}
	}

	backendStatus := h.buildBackendStatus(projectWorkDir)
	frontendStatus := h.buildFrontendStatus(projectWorkDir, projectName)

	util.OKWithData(c, gin.H{
		"success":         true,
		"projectId":       projectIDStr,
		"projectName":     projectName,
		"projectWorkDir":  projectWorkDir,
		"backend":         backendStatus,
		"frontend":        frontendStatus,
		"accessUrls":      h.buildAccessUrls(backendStatus["port"]),
		"system":          h.buildSystemInfo(),
	})
}

// GetBackendStatus 对应 GET /runtime-status/backend，获取后端状态。
func (h *RuntimeStatusHandler) GetBackendStatus(c *gin.Context) {
	projectWorkDir := c.Query("projectWorkDir")
	if projectWorkDir == "" {
		projectWorkDir, _ = os.Getwd()
	}
	util.OKWithData(c, h.buildBackendStatus(projectWorkDir))
}

// GetFrontendStatus 对应 GET /runtime-status/frontend，获取前端状态。
func (h *RuntimeStatusHandler) GetFrontendStatus(c *gin.Context) {
	projectWorkDir := c.Query("projectWorkDir")
	if projectWorkDir == "" {
		projectWorkDir, _ = os.Getwd()
	}
	projectName := c.DefaultQuery("projectName", "claude_sprint")
	util.OKWithData(c, h.buildFrontendStatus(projectWorkDir, projectName))
}

// GetAccessUrls 对应 GET /runtime-status/access-urls，获取访问链接。
func (h *RuntimeStatusHandler) GetAccessUrls(c *gin.Context) {
	util.OKWithData(c, h.buildAccessUrls(nil))
}

// GetSystemInfo 对应 GET /runtime-status/system，获取系统信息。
func (h *RuntimeStatusHandler) GetSystemInfo(c *gin.Context) {
	util.OKWithData(c, h.buildSystemInfo())
}

// ---- 内部方法 ----

func (h *RuntimeStatusHandler) buildBackendStatus(projectWorkDir string) gin.H {
	port := h.cfg.Server.Port
	status := "running"

	// 检查端口是否可连接
	conn, err := net.Dial("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		status = "stopped"
	} else {
		conn.Close()
	}

	return gin.H{
		"status":         status,
		"port":           port,
		"portSource":     "config",
		"projectWorkDir": projectWorkDir,
	}
}

func (h *RuntimeStatusHandler) buildFrontendStatus(projectWorkDir, projectName string) gin.H {
	frontendDir := filepath.Join(projectWorkDir, "frontend")
	packageJSON := filepath.Join(frontendDir, "package.json")
	nodeModules := filepath.Join(frontendDir, "node_modules")
	frontendDistDir := filepath.Join("/opt", projectName, "frontend-dist")

	return gin.H{
		"status":                "stopped",
		"processCount":          0,
		"frontendDir":           frontendDir,
		"frontendDirExists":     dirExists(frontendDir),
		"packageJsonExists":     fileExists(packageJSON),
		"nodeModulesExists":     dirExists(nodeModules),
		"frontendDistDir":       frontendDistDir,
		"frontendDistDirExists": dirExists(frontendDistDir),
	}
}

func (h *RuntimeStatusHandler) buildAccessUrls(detectedPort interface{}) gin.H {
	hostname := h.getLocalIP()
	if hostname == "" {
		hostname = "localhost"
	}

	backendPort := strconv.Itoa(h.cfg.Server.Port)
	if detectedPort != nil {
		if p, ok := detectedPort.(int); ok && p > 0 {
			backendPort = strconv.Itoa(p)
		}
	}
	frontendPort := "5173"

	return gin.H{
		"backendApi":           fmt.Sprintf("http://%s:%s", hostname, backendPort),
		"websocket":            fmt.Sprintf("ws://%s:%s/ws", hostname, backendPort),
		"frontendDev":          fmt.Sprintf("http://%s:%s", hostname, frontendPort),
		"production":           fmt.Sprintf("http://%s:%s", hostname, backendPort),
		"localhost":            fmt.Sprintf("http://localhost:%s", backendPort),
		"localhostApi":         fmt.Sprintf("http://localhost:%s", backendPort),
		"localIps":             h.getLocalIPs(),
		"actualBackendPort":    backendPort,
		"actualFrontendPort":   frontendPort,
		"portSource":           "default",
	}
}

func (h *RuntimeStatusHandler) buildSystemInfo() gin.H {
	host, _ := os.Hostname()
	return gin.H{
		"osName":      runtime.GOOS,
		"osArch":      runtime.GOARCH,
		"cpus":        runtime.NumCPU(),
		"goVersion":   runtime.Version(),
		"hostname":    host,
	}
}

func (h *RuntimeStatusHandler) getLocalIP() string {
	// 优先使用配置的 IP
	if h.cfg.App.LocalIPs != "" {
		parts := strings.Split(h.cfg.App.LocalIPs, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	// 自动探测
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}

func (h *RuntimeStatusHandler) getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ips = append(ips, ipNet.IP.String())
			}
		}
	}
	return ips
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// 防止 http 未引用告警（保留给未来扩展用）
var _ = http.StatusOK
