package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"loafer-agent/internal/config"
	"loafer-agent/internal/engine/skill"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ProjectHandler 对应原 Java ProjectController，处理项目 CRUD 及工作目录相关接口。
// 基础 CRUD 复用泛型 CrudService[model.Project]；create/update/delete 按原
// MyBatis-Plus 语义直接使用 GORM 会话；工作目录接口暂为桩实现。
type ProjectHandler struct {
	db            *gorm.DB
	svc           *service.CrudService[model.Project]
	giteeService  *service.GiteeService
	deployService *service.DeployService
}

// NewProjectHandler 构造项目处理器。
func NewProjectHandler(db *gorm.DB, cfg *config.Config) *ProjectHandler {
	return &ProjectHandler{
		db:            db,
		svc:           service.NewCrudService[model.Project](db),
		giteeService:  service.NewGiteeService(&cfg.Gitee),
		deployService: service.NewDeployService(db, cfg),
	}
}

// RegisterRoutes 注册项目相关路由（/projects）。
func (h *ProjectHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/projects")
	{
		g.GET("/list", h.List)
		g.GET("/page", h.Page)
		g.GET("/:id", h.GetByID)
		g.POST("/create", h.Create)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)

		g.GET("/:id/claude-config", h.GetClaudeConfig)
		g.PUT("/:id/claude-config", h.UpdateClaudeConfig)

		g.GET("/:id/workdir/git-changed-paths", h.GitChangedPaths)
		g.GET("/:id/workdir/git-log", h.GitLog)
		g.POST("/:id/workdir/env-check", h.EnvCheck)
		g.POST("/:id/workdir/install-deps", h.InstallDeps)

		// 桩实现：返回空列表，避免前端 404
		g.GET("/:id/available-skills", h.GetAvailableSkills)
		g.GET("/:id/agents", h.ListAgents)
		g.GET("/:id/database-config", h.GetDatabaseConfig)
		g.GET("/:id/database-tables", h.ListDatabaseTables)
		g.GET("/:id/test-cases", h.ListTestCases)
		g.GET("/:id/env-vars", h.ListEnvVars)
		g.GET("/:id/prompt-vars", h.ListPromptVars)
	}
}

// List 对应 GET /projects/list，返回全部项目。
func (h *ProjectHandler) List(c *gin.Context) {
	list, err := h.svc.List(nil)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

// Page 对应 GET /projects/page，分页查询项目。
func (h *ProjectHandler) Page(c *gin.Context) {
	page, size := parsePageParams(c)
	list, total, err := h.svc.Page(page, size, nil)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKPage(c, list, total, page, size)
}

// GetByID 对应 GET /projects/:id，按 ID 获取项目详情。
func (h *ProjectHandler) GetByID(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	entity, err := h.svc.GetByID(id)
	if err != nil {
		util.Fail500(c, "记录不存在")
		return
	}
	util.OK(c, entity)
}

// Create 对应 POST /projects/create，新建项目并返回创建后的实体。
// 自动设置开发语言为 go+reactjs，工作目录为 /srv/zfei/projects+应用名。
// 同时在 Gitee 上创建仓库并关联。
func (h *ProjectHandler) Create(c *gin.Context) {
	var project model.Project
	if err := c.ShouldBindJSON(&project); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	// 统一开发语言：Go + React/TypeScript
	project.DevLanguage = "go+reactjs"

	// 统一工作目录：/srv/zfei/projects+应用名
	// 将项目名转为路径安全的格式（小写、下划线替换特殊字符）
	safeName := util.Slugify(project.Name)
	project.WorkDir = "/srv/zfei/projects/" + safeName

	// 在 Gitee 上创建仓库
	if h.giteeService != nil {
		repoName, err := service.ValidateRepoName(project.Name)
		if err != nil {
			util.Fail(c, http.StatusBadRequest, "项目名称无效: "+err.Error())
			return
		}

		repo, err := h.giteeService.CreateRepo(&service.CreateRepoRequest{
			Name:        repoName,
			Description: project.Description,
			Private:     true, // 默认私有仓库
			HasIssues:   true,
			HasWiki:     true,
			AutoInit:    true, // 自动初始化，生成 README
		})

		if err != nil {
			// Gitee 创建失败不影响项目创建，仅记录日志
			fmt.Printf("⚠ Gitee 仓库创建失败: %v\n", err)
		} else {
			// 关联 Git URL
			project.GitURL = repo.GetGitURL()
			fmt.Printf("✓ Gitee 仓库创建成功: %s\n", repo.HTMLURL)
		}
	}

	if err := h.db.Create(&project).Error; err != nil {
		util.Fail500(c, "创建失败: "+err.Error())
		return
	}
	util.OK(c, project)
}

// Update 对应 PUT /projects/:id，按 ID 部分更新（仅更新非零字段，对应 updateById）。
func (h *ProjectHandler) Update(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var project model.Project
	if err := c.ShouldBindJSON(&project); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := h.db.Model(&model.Project{}).Where("id = ?", id).Updates(&project).Error; err != nil {
		util.Fail500(c, "更新失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// Delete 对应 DELETE /projects/:id，软删除项目并清理所有关联资源。
// 清理顺序：卸载部署（停止后端、移除Nginx、释放端口、删除数据库） → 删除Gitee仓库 → 软删除项目记录。
func (h *ProjectHandler) Delete(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	// 先查询项目信息，获取 GitURL 用于删除 Gitee 仓库
	var project model.Project
	if err := h.db.First(&project, id).Error; err != nil {
		util.Fail500(c, "项目不存在: "+err.Error())
		return
	}

	// 1. 卸载部署资源（停止后端进程、移除Nginx配置、释放端口、删除项目数据库）
	if h.deployService != nil {
		if err := h.deployService.Undeploy(id); err != nil {
			// 部署卸载失败不阻断项目删除，仅记录日志
			fmt.Printf("⚠ 卸载部署资源失败 (项目 %d): %v\n", id, err)
		} else {
			fmt.Printf("✓ 部署资源已卸载: 项目 %d（后端进程停止、Nginx配置移除、端口释放、数据库删除）\n", id)
		}
	}

	// 2. 删除 Gitee 仓库（如果有 GitURL）
	if project.GitURL != "" && h.giteeService != nil {
		owner, repo := service.ExtractOwnerRepoFromGitURL(project.GitURL)
		if owner != "" && repo != "" {
			if err := h.giteeService.DeleteRepo(owner, repo); err != nil {
				// 仓库删除失败不阻断项目删除，仅记录日志
				fmt.Printf("⚠ Gitee 仓库删除失败 (%s/%s): %v\n", owner, repo, err)
			} else {
				fmt.Printf("✓ Gitee 仓库已删除: %s/%s\n", owner, repo)
			}
		}
	}

	// 3. 软删除项目
	if err := h.db.Delete(&model.Project{}, id).Error; err != nil {
		util.Fail500(c, "删除失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// GetClaudeConfig 对应 GET /projects/:id/claude-config。
// 从 system_config 表中读取项目级 Claude 配置（若不存在则返回空配置）。
func (h *ProjectHandler) GetClaudeConfig(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}

	key := fmt.Sprintf("project:%d:claude_config", id)
	var cfg model.SystemConfig
	err := h.db.Where("config_key = ?", key).First(&cfg).Error

	result := gin.H{
		"claudeMd":          "",
		"claudeSettings":    "",
		"skillRelativePath": "",
		"skillContent":      "",
	}

	if err == nil && cfg.ConfigValue != "" {
		var parsed map[string]interface{}
		if json.Unmarshal([]byte(cfg.ConfigValue), &parsed) == nil {
			if v, ok := parsed["claudeMd"].(string); ok {
				result["claudeMd"] = v
			}
			if v, ok := parsed["claudeSettings"].(string); ok {
				result["claudeSettings"] = v
			}
			if v, ok := parsed["skillRelativePath"].(string); ok {
				result["skillRelativePath"] = v
			}
			if v, ok := parsed["skillContent"].(string); ok {
				result["skillContent"] = v
			}
		}
	}

	util.OKWithData(c, result)
}

// UpdateClaudeConfig 对应 PUT /projects/:id/claude-config，
// 桩实现：将请求体以 JSON 字符串形式写入 system_config 表。
func (h *ProjectHandler) UpdateClaudeConfig(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	jsonBytes, _ := json.Marshal(body)
	value := string(jsonBytes)
	key := fmt.Sprintf("project:%d:claude_config", id)

	var cfg model.SystemConfig
	err := h.db.Where("config_key = ?", key).First(&cfg).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		cfg = model.SystemConfig{
			ConfigKey:   key,
			ConfigValue: value,
			Description: fmt.Sprintf("Claude config for project %d", id),
		}
		if err := h.db.Create(&cfg).Error; err != nil {
			util.Fail500(c, "保存失败: "+err.Error())
			return
		}
	case err != nil:
		util.Fail500(c, "查询失败: "+err.Error())
		return
	default:
		if err := h.db.Model(&model.SystemConfig{}).Where("id = ?", cfg.ID).
			Update("config_value", value).Error; err != nil {
			util.Fail500(c, "保存失败: "+err.Error())
			return
		}
	}
	util.OKWithData(c, gin.H{"saved": true})
}

// GitChangedPaths 对应 GET /projects/:id/workdir/git-changed-paths，桩实现返回空列表。
func (h *ProjectHandler) GitChangedPaths(c *gin.Context) {
	if _, ok := parsePathID(c, "id"); !ok {
		return
	}
	util.OK(c, []interface{}{})
}

// GitLog 对应 GET /projects/:id/workdir/git-log，桩实现返回空列表。
func (h *ProjectHandler) GitLog(c *gin.Context) {
	if _, ok := parsePathID(c, "id"); !ok {
		return
	}
	util.OK(c, []interface{}{})
}

// EnvCheck 对应 POST /projects/:id/workdir/env-check，桩实现返回成功。
func (h *ProjectHandler) EnvCheck(c *gin.Context) {
	if _, ok := parsePathID(c, "id"); !ok {
		return
	}
	util.OKWithData(c, gin.H{})
}

// InstallDeps 对应 POST /projects/:id/workdir/install-deps，桩实现返回成功。
func (h *ProjectHandler) InstallDeps(c *gin.Context) {
	if _, ok := parsePathID(c, "id"); !ok {
		return
	}
	util.OKWithData(c, gin.H{})
}

// GetAvailableSkills 对应 GET /projects/:id/available-skills。
// 返回 Loafer 内置的 8 个职能 Skill（能力契约 + 注册表），供前端展示与审计。
func (h *ProjectHandler) GetAvailableSkills(c *gin.Context) {
	util.OKWithData(c, gin.H{
		"skills":           skill.Global().List(),
		"globalSkillsPath": "",
	})
}

// ListAgents 对应 GET /projects/:id/agents
func (h *ProjectHandler) ListAgents(c *gin.Context) {
	util.OK(c, []interface{}{})
}

// GetDatabaseConfig 对应 GET /projects/:id/database-config
func (h *ProjectHandler) GetDatabaseConfig(c *gin.Context) {
	util.OKWithData(c, gin.H{"tables": []interface{}{}})
}

// ListDatabaseTables 对应 GET /projects/:id/database-tables
func (h *ProjectHandler) ListDatabaseTables(c *gin.Context) {
	util.OK(c, []interface{}{})
}

// ListTestCases 对应 GET /projects/:id/test-cases
func (h *ProjectHandler) ListTestCases(c *gin.Context) {
	util.OK(c, []interface{}{})
}

// ListEnvVars 对应 GET /projects/:id/env-vars
func (h *ProjectHandler) ListEnvVars(c *gin.Context) {
	util.OK(c, []interface{}{})
}

// ListPromptVars 对应 GET /projects/:id/prompt-vars
func (h *ProjectHandler) ListPromptVars(c *gin.Context) {
	util.OK(c, []interface{}{})
}
