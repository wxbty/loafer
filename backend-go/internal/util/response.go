package util

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// Slugify 将字符串转换为路径安全的格式：小写、下划线替换特殊字符。
// 例如："My Project Name" -> "my_project_name"
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// 将非字母数字字符替换为下划线
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	// 去除首尾下划线
	s = strings.Trim(s, "_")
	// 防止空字符串
	if s == "" {
		s = "project"
	}
	return s
}

// 统一响应结构，对应前端 request.ts 的约定。
// 简单 CRUD 接口直接返回数据（用 OK）；需要包裹的接口用 OKWithData / Fail。

// R 是带 success 标记的统一响应体。
type R struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// PageResult 分页结果，对应 MyBatis-Plus 的 Page 序列化结构 {records, total, current, size}。
type PageResult struct {
	Records interface{} `json:"records"`
	Total   int64       `json:"total"`
	Current int64       `json:"current"`
	Size    int64       `json:"size"`
}

// OK 直接返回数据（对应 Spring 直接 return entity / List 的接口）。
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}

// OKWithData 返回 {success:true, data:...}（对应 Spring 返回 successResponse 的接口）。
func OKWithData(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, R{Success: true, Data: data})
}

// OKMsg 返回 {success:true, message:...}。
func OKMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, R{Success: true, Message: msg})
}

// OKPage 返回 MyBatis-Plus 风格的分页结构。
func OKPage(c *gin.Context, records interface{}, total, current, size int64) {
	c.JSON(http.StatusOK, PageResult{
		Records: records,
		Total:   total,
		Current: current,
		Size:    size,
	})
}

// Fail 返回错误响应（对应 Spring 抛异常 / 手动返回 errorResponse）。
func Fail(c *gin.Context, code int, msg string) {
	c.JSON(code, R{Success: false, Message: msg})
}

// Fail500 便捷返回 500 错误。
func Fail500(c *gin.Context, msg string) {
	Fail(c, http.StatusInternalServerError, msg)
}
