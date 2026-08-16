package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"loafer-agent/internal/auth"
	"loafer-agent/internal/config"
	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/handler"
	"loafer-agent/internal/middleware"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Setup 构建并返回 Gin 引擎，注册全部中间件、路由组与 SPA 静态文件服务。
// 对应原 Spring Boot 的 WebMvcConfig + 各 Controller 注册逻辑。
func Setup(cfg *config.Config, db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// CORS（对应 WebMvcConfig 的跨域配置，允许所有来源）
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	jm := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.Expiration)
	skipPrefixes := middleware.DefaultSkipPrefixes()

	// CLI 进程管理器与会话池（Claude session handler 与 WebSocket handler 共享同一实例）
	pm := cli.NewProcessManager()
	pool := cli.NewSessionPool(pm, cfg.App.SessionPool.MaxSize, cfg.App.SessionPool.IdleTimeout)

	// 离线执行器（基于 claude --print --output-format stream-json 的非交互式执行）
	offlineExecutor := cli.NewOfflineExecutor()

	// API group with auth
	api := r.Group("/api")
	api.Use(middleware.Auth(jm, skipPrefixes))
	{
		// Auth (login doesn't need auth, but it's in skipPrefixes)
		authHandler := handler.NewAuthHandler(jm, &cfg.App)
		authHandler.RegisterRoutes(api)

		// Simple CRUD handlers
		simpleCrud := handler.NewSimpleCrudHandlers(db)
		simpleCrud.RegisterRoutes(api)

		// Project handler
		projectHandler := handler.NewProjectHandler(db, cfg)
		projectHandler.RegisterRoutes(api)

		// Module handler
		moduleHandler := handler.NewModuleHandler(db, offlineExecutor, cfg)
		moduleHandler.RegisterRoutes(api)

		// Task handler
		taskHandler := handler.NewTaskHandler(db, offlineExecutor)
		taskHandler.RegisterRoutes(api)

		// Prompt template handler
		promptHandler := handler.NewPromptTemplateHandler(db)
		promptHandler.RegisterRoutes(api)

		// System config handler
		sysConfigHandler := handler.NewSystemConfigHandler(db)
		sysConfigHandler.RegisterRoutes(api)

		// File handler
		fileHandler := handler.NewFileHandler(&cfg.App)
		fileHandler.RegisterRoutes(api)

		// Runtime status handler
		runtimeHandler := handler.NewRuntimeStatusHandler(db, cfg)
		runtimeHandler.RegisterRoutes(api)

		// Claude session handler
		claudeHandler := handler.NewClaudeSessionHandler(pool, pm, db)
		claudeHandler.RegisterRoutes(api)

		// Plan handler (需求分析 → 计划生成 → 确认 → 拆解)
		docsService := service.NewDocsArtifactService(cfg)
		planHandler := handler.NewPlanHandler(db, offlineExecutor, docsService)
		planHandler.RegisterRoutes(api)

		// Deploy handler (部署管理)
		deployHandler := handler.NewDeployHandler(db, cfg)
		deployHandler.RegisterRoutes(api)

		// Infrastructure handler (端口、数据库、短信、测试)
		infraHandler := handler.NewInfraHandler(db, cfg)
		infraHandler.RegisterRoutes(api)

		// Project chat handler (对话式创建项目：AI 需求提炼 + 上下文预览 + 一键创建)
		projectChatHandler := handler.NewProjectChatHandler(db, cfg, offlineExecutor)
		projectChatHandler.RegisterRoutes(api)

		// Pipeline handler (端到端全链路：计划→分解→编码→部署→测试)
		pipelineHandler := handler.NewPipelineHandler(db, cfg, offlineExecutor)
		pipelineHandler.RegisterRoutes(api)
	}

	// WebSocket terminal handler (registered on root engine, not under /api)
	wsHandler := handler.NewTerminalWebSocketHandler(pool, pm)
	wsHandler.RegisterRoutes(r)

	// SPA static file serving
	// 前端构建产物位于 ../frontend/dist，未匹配的路由回退到 index.html 以支持前端路由。
	frontendDir := filepath.Join("..", "frontend", "dist")
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// 对于未匹配的 API / WebSocket 路由，返回 JSON 404
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws/") {
			util.Fail(c, http.StatusNotFound, "资源不存在")
			return
		}

		// 尝试提供请求路径对应的静态文件
		filePath := filepath.Join(frontendDir, path)
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			c.File(filePath)
			return
		}

		// SPA 回退：对于前端路由（如 /projects/1）返回 index.html
		c.File(filepath.Join(frontendDir, "index.html"))
	})

	return r
}
