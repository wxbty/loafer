package middleware

import (
	"net/http"
	"strings"

	"loafer-agent/internal/auth"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
)

// CtxUserID 是存入 gin.Context 的用户 ID 键（对应原 AuthInterceptor.USER_ID_ATTR）。
const CtxUserID = "userId"

// Auth 对应原 AuthInterceptor，校验 Authorization: Bearer <token> 头。
// skipPrefixes 中的路径前缀不校验（对应 WebMvcConfig 的 excludePathPatterns）。
func Auth(jm *auth.JWTManager, skipPrefixes []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, p := range skipPrefixes {
			if strings.HasPrefix(path, p) {
				c.Next()
				return
			}
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			util.Fail(c, http.StatusUnauthorized, "未授权：缺少有效的认证令牌")
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if !jm.ValidateToken(token) {
			util.Fail(c, http.StatusUnauthorized, "未授权：认证令牌无效或已过期")
			c.Abort()
			return
		}

		uid, err := jm.GetUserIDFromToken(token)
		if err != nil {
			util.Fail(c, http.StatusUnauthorized, "未授权：认证令牌无效或已过期")
			c.Abort()
			return
		}
		c.Set(CtxUserID, uid)
		c.Next()
	}
}

// DefaultSkipPrefixes 返回不需要认证的路径前缀（对应 WebMvcConfig.excludePathPatterns）。
func DefaultSkipPrefixes() []string {
	return []string{
		"/api/auth/",
		"/ws/",
		"/api/files/image-preview",
		"/api/files/audio",
		"/api/files/download",
		"/api/module-screenshots/", // <img> 无法携带 Bearer token，与 image-preview 同理豁免
		"/static/",
		"/css/",
		"/js/",
		"/images/",
		"/fonts/",
		"/favicon.ico",
		"/index.html",
	}
}
