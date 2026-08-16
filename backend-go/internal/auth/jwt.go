package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTManager 对应原 Java JwtUtil，负责生成与校验 JWT Token。
type JWTManager struct {
	secret     []byte
	expiration time.Duration
}

// NewJWTManager 根据 config.JWTConfig 构造实例。
func NewJWTManager(secret string, expirationMs int64) *JWTManager {
	return &JWTManager{
		secret:     []byte(secret),
		expiration: time.Duration(expirationMs) * time.Millisecond,
	}
}

type claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken 根据用户 ID 生成 Token（对应 JwtUtil.generateToken）。
func (m *JWTManager) GenerateToken(userID int64) (string, error) {
	now := time.Now()
	c := claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   intToStr(userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiration)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(m.secret)
}

// ValidateToken 校验 Token 是否有效（对应 JwtUtil.validateToken）。
func (m *JWTManager) ValidateToken(tokenStr string) bool {
	_, err := m.parse(tokenStr)
	return err == nil
}

// GetUserIDFromToken 从 Token 提取用户 ID（对应 JwtUtil.getUserIdFromToken）。
func (m *JWTManager) GetUserIDFromToken(tokenStr string) (int64, error) {
	c, err := m.parse(tokenStr)
	if err != nil {
		return 0, err
	}
	return c.UserID, nil
}

func (m *JWTManager) parse(tokenStr string) (*claims, error) {
	c := &claims{}
	token, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}

func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
