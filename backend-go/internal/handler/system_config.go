package handler

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"loafer-agent/internal/model"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SystemConfigHandler 对应原 Java SystemConfigController，
// 处理系统配置的增删改查及工作目录/数据目录管理。
type SystemConfigHandler struct {
	db *gorm.DB
}

// 系统配置键常量（对应原 SystemConfigService.KEY_*）。
const (
	cfgKeyWorkspaceRoot  = "workspace_root"
	cfgKeyDataRoot       = "data_root"
	cfgKeyShowMoreActions = "show_more_actions"
	cfgKeyLocalIP        = "local_ip"
)

// NewSystemConfigHandler 构造系统配置处理器。
func NewSystemConfigHandler(db *gorm.DB) *SystemConfigHandler {
	return &SystemConfigHandler{db: db}
}

// RegisterRoutes 注册系统配置相关路由（/system-config）。
func (h *SystemConfigHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/system-config")
	{
		g.GET("/list", h.List)
		g.GET("/:key", h.GetByKey)
		g.PUT("/:key", h.SetByKey)

		g.GET("/workspace-root", h.GetWorkspaceRoot)
		g.PUT("/workspace-root", h.SetWorkspaceRoot)
		g.GET("/data-root", h.GetDataRoot)
		g.PUT("/data-root", h.SetDataRoot)
		g.GET("/show-more-actions", h.GetShowMoreActions)
		g.PUT("/show-more-actions", h.SetShowMoreActions)
		g.GET("/claude-info", h.GetClaudeInfo)
		g.GET("/claude-profiles", h.GetClaudeProfiles)
	}
}

// List 对应 GET /system-config/list，返回全部配置项。
func (h *SystemConfigHandler) List(c *gin.Context) {
	var list []model.SystemConfig
	if err := h.db.Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

// GetByKey 对应 GET /system-config/:key，按键查询配置值。
func (h *SystemConfigHandler) GetByKey(c *gin.Context) {
	key := c.Param("key")
	var cfg model.SystemConfig
	err := h.db.Where("config_key = ?", key).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.OK(c, gin.H{"key": key, "value": nil})
			return
		}
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"key": key, "value": cfg.ConfigValue})
}

// SetByKey 对应 PUT /system-config/:key，设置配置值。
func (h *SystemConfigHandler) SetByKey(c *gin.Context) {
	key := c.Param("key")
	var body struct {
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, 400, "请求参数格式错误")
		return
	}

	// 对路径类配置做校验
	if (key == cfgKeyWorkspaceRoot || key == cfgKeyDataRoot) && body.Value != "" {
		if !filepath.IsAbs(body.Value) {
			util.OKWithData(c, gin.H{"success": false, "message": "路径必须是绝对路径"})
			return
		}
		info, err := os.Stat(body.Value)
		if err != nil || !info.IsDir() {
			util.OKWithData(c, gin.H{"success": false, "message": "目录不存在或不是有效目录"})
			return
		}
	}

	h.upsertConfig(c, key, body.Value, body.Description)
}

// GetWorkspaceRoot 对应 GET /system-config/workspace-root。
func (h *SystemConfigHandler) GetWorkspaceRoot(c *gin.Context) {
	h.getConfigPath(c, cfgKeyWorkspaceRoot)
}

// SetWorkspaceRoot 对应 PUT /system-config/workspace-root。
func (h *SystemConfigHandler) SetWorkspaceRoot(c *gin.Context) {
	h.setConfigPath(c, cfgKeyWorkspaceRoot)
}

// GetDataRoot 对应 GET /system-config/data-root。
func (h *SystemConfigHandler) GetDataRoot(c *gin.Context) {
	h.getConfigPath(c, cfgKeyDataRoot)
}

// SetDataRoot 对应 PUT /system-config/data-root。
func (h *SystemConfigHandler) SetDataRoot(c *gin.Context) {
	h.setConfigPath(c, cfgKeyDataRoot)
}

// GetShowMoreActions 对应 GET /system-config/show-more-actions。
func (h *SystemConfigHandler) GetShowMoreActions(c *gin.Context) {
	var cfg model.SystemConfig
	err := h.db.Where("config_key = ?", cfgKeyShowMoreActions).First(&cfg).Error
	if err != nil {
		util.OK(c, gin.H{"value": false})
		return
	}
	util.OK(c, gin.H{"value": cfg.ConfigValue == "true"})
}

// SetShowMoreActions 对应 PUT /system-config/show-more-actions。
func (h *SystemConfigHandler) SetShowMoreActions(c *gin.Context) {
	var body struct {
		Value bool `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, 400, "请求参数格式错误")
		return
	}
	val := "false"
	if body.Value {
		val = "true"
	}
	h.upsertConfig(c, cfgKeyShowMoreActions, val, "是否显示更多操作")
}

// GetClaudeInfo 对应 GET /system-config/claude-info，返回 Claude Code CLI 信息。
func (h *SystemConfigHandler) GetClaudeInfo(c *gin.Context) {
	// 桩实现：尝试查找 claude 命令路径
	claudePath := ""
	if p, err := exec.LookPath("claude"); err == nil {
		claudePath = p
	}
	util.OKWithData(c, gin.H{
		"success":  true,
		"path":     claudePath,
		"model":    "",
		"version":  "",
	})
}

// GetClaudeProfiles 对应 GET /system-config/claude-profiles，返回 Claude 模型方案列表。
func (h *SystemConfigHandler) GetClaudeProfiles(c *gin.Context) {
	util.OKWithData(c, gin.H{"profiles": []interface{}{}, "activeProfileId": ""})
}

// ---- 内部辅助方法 ----

func (h *SystemConfigHandler) getConfigPath(c *gin.Context, key string) {
	var cfg model.SystemConfig
	err := h.db.Where("config_key = ?", key).First(&cfg).Error
	if err != nil {
		util.OK(c, gin.H{"value": nil, "exists": false})
		return
	}
	exists := false
	if cfg.ConfigValue != "" {
		if _, err := os.Stat(cfg.ConfigValue); err == nil {
			exists = true
		}
	}
	util.OK(c, gin.H{"value": cfg.ConfigValue, "exists": exists})
}

func (h *SystemConfigHandler) setConfigPath(c *gin.Context, key string) {
	var body struct {
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, 400, "请求参数格式错误")
		return
	}
	if body.Value != "" {
		if !filepath.IsAbs(body.Value) {
			util.OKWithData(c, gin.H{"success": false, "message": "路径必须是绝对路径"})
			return
		}
		info, err := os.Stat(body.Value)
		if err != nil || !info.IsDir() {
			util.OKWithData(c, gin.H{"success": false, "message": "目录不存在或不是有效目录"})
			return
		}
	}
	h.upsertConfig(c, key, body.Value, body.Description)
}

func (h *SystemConfigHandler) upsertConfig(c *gin.Context, key, value, description string) {
	var cfg model.SystemConfig
	err := h.db.Where("config_key = ?", key).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cfg = model.SystemConfig{
			ConfigKey:   key,
			ConfigValue: value,
			Description: description,
		}
		if err := h.db.Create(&cfg).Error; err != nil {
			util.Fail500(c, "保存失败: "+err.Error())
			return
		}
	} else if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	} else {
		updates := map[string]interface{}{"config_value": value}
		if description != "" {
			updates["description"] = description
		}
		if err := h.db.Model(&model.SystemConfig{}).Where("id = ?", cfg.ID).Updates(updates).Error; err != nil {
			util.Fail500(c, "保存失败: "+err.Error())
			return
		}
	}
	util.OKWithData(c, gin.H{"success": true, "message": "配置已保存"})
}


