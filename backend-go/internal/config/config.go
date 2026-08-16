package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config 对应 application.yml 的全部配置项，支持环境变量覆盖。
// 环境变量名约定：DB_HOST、DB_PORT、DB_USERNAME、DB_PASSWORD、APP_AUTH_USERNAME 等，
// 与原 Spring Boot 的 ${VAR:default} 占位符保持一致。
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	JWT        JWTConfig
	App        AppConfig
	Infra      InfraConfig
	SMS        SMSConfig
	Playwright PlaywrightConfig
	Gitee      GiteeConfig
}

type ServerConfig struct {
	Port int
}

type DatabaseConfig struct {
	Host     string
	Port     int
	Name     string
	Username string
	Password string
	Params   string
	// 连接池参数（对应 HikariCP）
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime int // 分钟
	ConnMaxIdleTime int // 分钟
}

type JWTConfig struct {
	Secret     string
	Expiration int64 // 毫秒
}

type AppConfig struct {
	LocalIPs    string
	Auth        AuthConfig
	SessionPool SessionPoolConfig
}

type AuthConfig struct {
	Username string
	Password string
}

type SessionPoolConfig struct {
	MaxSize     int
	IdleTimeout int // 分钟
}

// InfraConfig 基础设施配置：服务器、端口范围、Nginx、部署路径等
type InfraConfig struct {
	ServerHost      string // 部署服务器IP
	SSHUser         string // SSH用户名
	SSHKeyPath      string // SSH私钥路径
	SSHPemContent   string // SSH私钥内容（直接使用，不落盘）
	PortRangeStart  int    // 端口范围起始
	PortRangeEnd    int    // 端口范围结束
	NginxConfigDir  string // Nginx站点配置目录
	NginxBinary     string // Nginx可执行文件路径
	DeployBaseDir   string // 部署基础目录（服务器上）
	ProjectBaseDir  string // 项目工作目录基础路径
	PlaywrightPath  string // Playwright可执行文件路径
}

// SMSConfig 火山短信服务配置
type SMSConfig struct {
	AccessKey  string
	SecretKey  string
	AccountID  string
	SignName   string
	TemplateID string
}

// PlaywrightConfig Playwright测试配置
type PlaywrightConfig struct {
	BinaryPath  string // npx playwright 路径
	Headless    bool
	Timeout     int    // 超时秒数
	BaseURL     string // 测试目标基础URL
}

// GiteeConfig Gitee API 配置
type GiteeConfig struct {
	AccessToken string // Gitee 个人访问令牌
	APIBaseURL  string // Gitee API 基础URL
}

// Load 从环境变量加载配置，未设置时使用与 application.yml 一致的默认值。
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port: envInt("SERVER_PORT", 9080),
		},
		Database: DatabaseConfig{
			Host:            envStr("DB_HOST", "127.0.0.1"),
			Port:            envInt("DB_PORT", 3306),
			Name:            envStr("DB_NAME", "loafer"),
			Username:        envStr("DB_USERNAME", "root"),
			Password:        envStr("DB_PASSWORD", ""),
			Params:          "charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai&allowNativePasswords=true",
			MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", 20),
			MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: envInt("DB_CONN_MAX_LIFETIME", 30),
			ConnMaxIdleTime: envInt("DB_CONN_MAX_IDLE_TIME", 5),
		},
		JWT: JWTConfig{
			Secret:     envStr("JWT_SECRET", ""),
			Expiration: envInt64("JWT_EXPIRATION", 315360000000),
		},
		App: AppConfig{
			LocalIPs: envStr("APP_LOCAL_IPS", ""),
			Auth: AuthConfig{
				Username: envStr("APP_AUTH_USERNAME", "admin"),
				Password: envStr("APP_AUTH_PASSWORD", ""),
			},
			SessionPool: SessionPoolConfig{
				MaxSize:     envInt("SESSION_POOL_MAX_SIZE", 5),
				IdleTimeout: envInt("SESSION_POOL_IDLE_TIMEOUT", 40),
			},
		},
		Infra: InfraConfig{
			ServerHost:     envStr("INFRA_SERVER_HOST", ""),
			SSHUser:        envStr("INFRA_SSH_USER", "root"),
			SSHKeyPath:     envStr("INFRA_SSH_KEY_PATH", ""),
			SSHPemContent:  envStr("INFRA_SSH_PEM", ""),
			PortRangeStart: envInt("INFRA_PORT_RANGE_START", 40410),
			PortRangeEnd:   envInt("INFRA_PORT_RANGE_END", 40500),
			NginxConfigDir: envStr("INFRA_NGINX_CONFIG_DIR", "/etc/nginx/conf.d"),
			NginxBinary:    envStr("INFRA_NGINX_BINARY", "nginx"),
			DeployBaseDir:  envStr("INFRA_DEPLOY_BASE_DIR", "/opt/loafer/projects"),
			ProjectBaseDir: envStr("INFRA_PROJECT_BASE_DIR", "/opt/loafer/workspace"),
			PlaywrightPath: envStr("INFRA_PLAYWRIGHT_PATH", "npx"),
		},
		SMS: SMSConfig{
			AccessKey:  envStr("VOLC_SMS_ACCESS_KEY", ""),
			SecretKey:  envStr("VOLC_SMS_SECRET_KEY", ""),
			AccountID:  envStr("VOLC_SMS_ACCOUNT_ID", ""),
			SignName:   envStr("VOLC_SMS_SIGN_NAME", ""),
			TemplateID: envStr("VOLC_SMS_TEMPLATE_ID", ""),
		},
		Playwright: PlaywrightConfig{
			BinaryPath: envStr("PLAYWRIGHT_BINARY", "npx"),
			Headless:   envBool("PLAYWRIGHT_HEADLESS", true),
			Timeout:    envInt("PLAYWRIGHT_TIMEOUT", 120),
			BaseURL:    envStr("PLAYWRIGHT_BASE_URL", ""),
		},
		Gitee: GiteeConfig{
			AccessToken: envStr("GITEE_ACCESS_TOKEN", ""),
			APIBaseURL:  envStr("GITEE_API_BASE_URL", "https://gitee.com/api/v5"),
		},
	}
}

// DSN 返回 GORM/MySQL 使用的数据源名称。
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		c.Username, c.Password, c.Host, c.Port, c.Name, c.Params)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if v == "true" || v == "1" || v == "yes" {
			return true
		}
		if v == "false" || v == "0" || v == "no" {
			return false
		}
	}
	return def
}

// LocalIPList 将逗号分隔的 IP 字符串拆分为切片。
func (c *AppConfig) LocalIPList() []string {
	if c.LocalIPs == "" {
		return nil
	}
	parts := strings.Split(c.LocalIPs, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
