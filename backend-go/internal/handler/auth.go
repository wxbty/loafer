package handler

import (
	"net/http"

	"loafer-agent/internal/auth"
	"loafer-agent/internal/config"
	"loafer-agent/internal/dto"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
)

// AuthHandler 对应原 AuthController，处理单用户登录。
type AuthHandler struct {
	jm            *auth.JWTManager
	configUser    string
	configPass    string
}

// NewAuthHandler 构造认证处理器。
func NewAuthHandler(jm *auth.JWTManager, cfg *config.AppConfig) *AuthHandler {
	return &AuthHandler{
		jm:         jm,
		configUser: cfg.Auth.Username,
		configPass: cfg.Auth.Password,
	}
}

// Login 对应 POST /api/auth/login。
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	if req.Username != h.configUser || req.Password != h.configPass {
		util.Fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	// 固定用户 ID 1（对应原逻辑 jwtUtil.generateToken(1L)）
	token, err := h.jm.GenerateToken(1)
	if err != nil {
		util.Fail500(c, "生成令牌失败")
		return
	}

	resp := dto.LoginResponse{
		Token:    token,
		UserID:   1,
		Username: h.configUser,
		Nickname: h.configUser,
		Role:     "admin",
	}
	util.OKWithData(c, resp)
}

// RegisterRoutes 注册认证路由。
func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.Login)
}
