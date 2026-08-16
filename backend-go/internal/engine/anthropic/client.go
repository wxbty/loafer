package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// 默认配置
const (
	defaultMaxTokens = 4096
	defaultTimeout   = 120 * time.Second
	apiVersionHeader = "2023-06-01"
)

// settingsConfig 对应 ~/.claude/settings.json 的结构。
type settingsConfig struct {
	Env   map[string]string `json:"env"`
	Model string            `json:"model"`
}

// Client 是 Anthropic 兼容 API 的轻量 HTTP 客户端。
// 自动读取 ~/.claude/settings.json 中的三方模型配置，
// 支持 ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN 或 ANTHROPIC_API_KEY。
type Client struct {
	baseURL    string
	authToken  string
	apiKey     string
	model      string
	maxTokens  int
	http       *http.Client
}

// NewClient 从 ~/.claude/settings.json 读取配置并创建客户端。
// 如果配置文件不存在或缺少必要的配置，返回 nil。
func NewClient() *Client {
	cfg := loadSettings()

	baseURL := cfg.Env["ANTHROPIC_BASE_URL"]
	authToken := cfg.Env["ANTHROPIC_AUTH_TOKEN"]
	apiKey := cfg.Env["ANTHROPIC_API_KEY"]
	model := cfg.Model

	// 也允许通过环境变量覆盖
	if envBase := os.Getenv("ANTHROPIC_BASE_URL"); envBase != "" {
		baseURL = envBase
	}
	if envToken := os.Getenv("ANTHROPIC_AUTH_TOKEN"); envToken != "" {
		authToken = envToken
	}
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		apiKey = envKey
	}
	if envModel := os.Getenv("ANTHROPIC_MODEL"); envModel != "" {
		model = envModel
	}

	// 必须有 baseURL 和认证凭据
	if baseURL == "" {
		return nil
	}
	if authToken == "" && apiKey == "" {
		return nil
	}

	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		authToken: authToken,
		apiKey:    apiKey,
		model:     model,
		maxTokens: defaultMaxTokens,
		http:      &http.Client{Timeout: defaultTimeout},
	}
}

// IsAvailable 检查三方 API 是否可用（即 settings.json 中是否配置了 base URL 和认证）。
func IsAvailable() bool {
	return NewClient() != nil
}

// WithMaxTokens 设置最大输出 token 数。
func (c *Client) WithMaxTokens(n int) *Client {
	c.maxTokens = n
	return c
}

// GetModel 返回当前配置的模型名。
func (c *Client) GetModel() string {
	return c.model
}

// messagesRequest 对应 Anthropic Messages API 的请求体。
type messagesRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []messagePart `json:"messages"`
}

type messagePart struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse 对应 Anthropic Messages API 的响应体。
type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SendMessage 发送一条用户消息并返回模型文本回复。
func (c *Client) SendMessage(prompt string) (string, error) {
	reqBody := messagesRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Messages: []messagePart{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	// 构建完整 API URL：{baseURL}/v1/messages
	apiURL := c.baseURL + "/v1/messages"

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", apiVersionHeader)

	// 认证：优先使用 auth token（Bearer），其次使用 api key（x-api-key）
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(respBytes))
	}

	var msgResp messagesResponse
	if err := json.Unmarshal(respBytes, &msgResp); err != nil {
		return "", fmt.Errorf("解析响应 JSON 失败: %w", err)
	}

	if msgResp.Error != nil {
		return "", fmt.Errorf("API 返回错误: %s - %s", msgResp.Error.Type, msgResp.Error.Message)
	}

	// 拼接所有 text 类型的 content
	var result string
	for _, block := range msgResp.Content {
		if block.Type == "text" {
			result += block.Text
		}
	}

	if result == "" {
		return "", fmt.Errorf("API 返回空响应")
	}

	return result, nil
}

// loadSettings 读取 ~/.claude/settings.json 配置文件。
func loadSettings() settingsConfig {
	var cfg settingsConfig

	path := getSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	// 宽松解析：忽略未知字段
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// getSettingsPath 返回 ~/.claude/settings.json 的路径。
func getSettingsPath() string {
	home := getUserHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// getUserHome 返回当前用户家目录。
func getUserHome() string {
	if runtime.GOOS == "windows" {
		// Windows: %USERPROFILE%
		if p := os.Getenv("USERPROFILE"); p != "" {
			return p
		}
	}
	// Unix-like: $HOME
	if p := os.Getenv("HOME"); p != "" {
		return p
	}
	return ""
}
