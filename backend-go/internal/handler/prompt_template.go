package handler

import (
	"errors"
	"net/http"

	"loafer-agent/internal/model"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PromptTemplateHandler 对应原 Java PromptTemplateController，
// 处理提示词模板的 CRUD、启用/禁用与变量查询。
type PromptTemplateHandler struct {
	db *gorm.DB
}

// NewPromptTemplateHandler 构造提示词模板处理器。
func NewPromptTemplateHandler(db *gorm.DB) *PromptTemplateHandler {
	return &PromptTemplateHandler{db: db}
}

// RegisterRoutes 注册提示词模板相关路由（/prompt-templates）。
func (h *PromptTemplateHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/prompt-templates")
	{
		g.GET("/list", h.List)
		g.GET("/enabled", h.ListEnabled)
		g.GET("/:id", h.GetByID)
		g.GET("/key/:templateKey", h.GetByKey)
		g.GET("/:id/variables", h.GetVariables)
		g.POST("/", h.Create)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
		g.PUT("/:id/toggle", h.ToggleEnabled)
	}
}

// List 对应 GET /prompt-templates/list，返回全部模板。
func (h *PromptTemplateHandler) List(c *gin.Context) {
	var list []model.PromptTemplate
	if err := h.db.Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

// ListEnabled 对应 GET /prompt-templates/enabled，仅返回已启用模板。
func (h *PromptTemplateHandler) ListEnabled(c *gin.Context) {
	var list []model.PromptTemplate
	if err := h.db.Where("is_enabled = ?", 1).Find(&list).Error; err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

// GetByID 对应 GET /prompt-templates/:id，按 ID 获取模板。
func (h *PromptTemplateHandler) GetByID(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var tpl model.PromptTemplate
	if err := h.db.First(&tpl, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, nil)
			return
		}
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, tpl)
}

// GetByKey 对应 GET /prompt-templates/key/:templateKey，按 key 获取模板。
func (h *PromptTemplateHandler) GetByKey(c *gin.Context) {
	key := c.Param("templateKey")
	var tpl model.PromptTemplate
	if err := h.db.Where("template_key = ?", key).First(&tpl).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, nil)
			return
		}
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, tpl)
}

// GetVariables 对应 GET /prompt-templates/:id/variables，解析模板变量。
func (h *PromptTemplateHandler) GetVariables(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var tpl model.PromptTemplate
	if err := h.db.First(&tpl, id).Error; err != nil {
		util.OKWithData(c, nil)
		return
	}
	// 如果 variables_json 已有值，直接返回；否则返回空 map
	if tpl.VariablesJSON != "" {
		util.OKWithData(c, gin.H{"raw": tpl.VariablesJSON})
		return
	}
	util.OKWithData(c, gin.H{})
}

// Create 对应 POST /prompt-templates/，创建模板。
func (h *PromptTemplateHandler) Create(c *gin.Context) {
	var tpl model.PromptTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if tpl.TemplateKey == "" {
		util.Fail(c, http.StatusBadRequest, "模板标识键不能为空")
		return
	}
	if tpl.TemplateName == "" {
		util.Fail(c, http.StatusBadRequest, "模板名称不能为空")
		return
	}
	if tpl.TemplateContent == "" {
		util.Fail(c, http.StatusBadRequest, "模板内容不能为空")
		return
	}
	if tpl.IsEnabled == 0 {
		tpl.IsEnabled = 1
	}
	tpl.IsSystem = 0
	tpl.UseCount = 0
	if err := h.db.Create(&tpl).Error; err != nil {
		util.Fail500(c, "创建失败: "+err.Error())
		return
	}
	util.OKWithData(c, tpl)
}

// Update 对应 PUT /prompt-templates/:id，更新模板。
func (h *PromptTemplateHandler) Update(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var tpl model.PromptTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	tpl.ID = id
	if err := h.db.Model(&model.PromptTemplate{}).Where("id = ?", id).Updates(&tpl).Error; err != nil {
		util.Fail500(c, "更新失败: "+err.Error())
		return
	}
	// 重新查询返回更新后的实体
	h.db.First(&tpl, id)
	util.OKWithData(c, tpl)
}

// Delete 对应 DELETE /prompt-templates/:id，删除模板。
func (h *PromptTemplateHandler) Delete(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	result := h.db.Delete(&model.PromptTemplate{}, id)
	if result.Error != nil {
		util.Fail500(c, "删除失败: "+result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		util.Fail(c, http.StatusNotFound, "模板不存在")
		return
	}
	util.OK(c, true)
}

// ToggleEnabled 对应 PUT /prompt-templates/:id/toggle，启用/禁用模板。
func (h *PromptTemplateHandler) ToggleEnabled(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.Enabled = true
	}
	enabled := 0
	if body.Enabled {
		enabled = 1
	}
	result := h.db.Model(&model.PromptTemplate{}).Where("id = ?", id).Update("is_enabled", enabled)
	if result.Error != nil {
		util.Fail500(c, "操作失败: "+result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		util.Fail(c, http.StatusNotFound, "模板不存在")
		return
	}
	util.OK(c, true)
}
