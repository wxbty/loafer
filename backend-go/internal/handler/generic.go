package handler

import (
	"net/http"
	"strconv"

	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GenericCrud 是泛型 CRUD 处理器，覆盖原 MyBatis-Plus ServiceImpl 的通用接口：
// GET /{id}、POST /create、PUT /{id}、DELETE /{id}、GET /list、GET /page。
// 各实体处理器嵌入本结构体后追加自定义查询路由（如 by-task）。
type GenericCrud[T any] struct {
	Svc *service.CrudService[T]
}

// parsePathID 从 URL 路径参数解析 int64 ID。
func parsePathID(c *gin.Context, key string) (int64, bool) {
	raw := c.Param(key)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, "无效的 ID: "+raw)
		return 0, false
	}
	return id, true
}

// parsePageParams 解析分页查询参数 page / size。
func parsePageParams(c *gin.Context) (int64, int64) {
	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
	size, _ := strconv.ParseInt(c.DefaultQuery("size", "10"), 10, 64)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	return page, size
}

// GetByID 对应 GET /{id}。
func (g *GenericCrud[T]) GetByID(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	entity, err := g.Svc.GetByID(id)
	if err != nil {
		util.Fail500(c, "记录不存在")
		return
	}
	util.OK(c, entity)
}

// Create 对应 POST /create。
func (g *GenericCrud[T]) Create(c *gin.Context) {
	var entity T
	if err := c.ShouldBindJSON(&entity); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := g.Svc.Save(&entity); err != nil {
		util.Fail500(c, "创建失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// Update 对应 PUT /{id}（按 ID 更新非零字段，对应 MyBatis-Plus updateById）。
func (g *GenericCrud[T]) Update(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var entity T
	if err := c.ShouldBindJSON(&entity); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if err := g.Svc.UpdateByID(id, &entity); err != nil {
		util.Fail500(c, "更新失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// Delete 对应 DELETE /{id}（软删除）。
func (g *GenericCrud[T]) Delete(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	if err := g.Svc.Delete(id); err != nil {
		util.Fail500(c, "删除失败: "+err.Error())
		return
	}
	util.OK(c, true)
}

// List 对应 GET /list。
func (g *GenericCrud[T]) List(c *gin.Context) {
	list, err := g.Svc.List(nil)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OK(c, list)
}

// Page 对应 GET /page。
func (g *GenericCrud[T]) Page(c *gin.Context) {
	page, size := parsePageParams(c)
	list, total, err := g.Svc.Page(page, size, nil)
	if err != nil {
		util.Fail500(c, "查询失败: "+err.Error())
		return
	}
	util.OKPage(c, list, total, page, size)
}

// noopDB 防止 gorm 在泛型零值时被误判未引用（编译期占位）。
var _ = gorm.ErrRecordNotFound
