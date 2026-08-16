package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"loafer-agent/internal/config"
)

// GiteeService 封装 Gitee API 操作，用于自动创建项目仓库。
type GiteeService struct {
	cfg *config.GiteeConfig
}

// NewGiteeService 构造 Gitee 服务实例。
func NewGiteeService(cfg *config.GiteeConfig) *GiteeService {
	return &GiteeService{cfg: cfg}
}

// CreateRepoRequest 创建仓库请求参数
type CreateRepoRequest struct {
	Name        string `json:"name"`        // 仓库名称
	Description string `json:"description"` // 仓库描述
	Private     bool   `json:"private"`     // 是否私有
	HasIssues   bool   `json:"has_issues"`  // 开启Issue
	HasWiki     bool   `json:"has_wiki"`    // 开启Wiki
	AutoInit    bool   `json:"auto_init"`   // 自动初始化仓库
}

// CreateRepoResponse 创建仓库响应
type CreateRepoResponse struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Path          string `json:"path"`
	HTMLURL       string `json:"html_url"`
	SSHURL        string `json:"ssh_url"`
	CloneURL      string `json:"clone_url"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	Public        bool   `json:"public"`
	DefaultBranch string `json:"default_branch"`
}

// CreateRepo 创建一个新仓库。
// 返回仓库信息和可能的错误。
func (s *GiteeService) CreateRepo(req *CreateRepoRequest) (*CreateRepoResponse, error) {
	if s.cfg.AccessToken == "" {
		return nil, fmt.Errorf("Gitee access token 未配置，请设置 GITEE_ACCESS_TOKEN 环境变量")
	}

	// 构建请求 URL
	apiURL := fmt.Sprintf("%s/user/repos", s.cfg.APIBaseURL)

	// 构建请求体
	body := map[string]interface{}{
		"access_token": s.cfg.AccessToken,
		"name":         req.Name,
		"description":  req.Description,
		"private":      req.Private,
		"has_issues":   req.HasIssues,
		"has_wiki":     req.HasWiki,
		"auto_init":    req.AutoInit,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 发送 POST 请求
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * 1000 * 1000 * 1000} // 30秒超时
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求 Gitee API 失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusCreated {
		var errResp map[string]interface{}
		if json.Unmarshal(respBody, &errResp) == nil {
			if msg, ok := errResp["message"].(string); ok {
				return nil, fmt.Errorf("Gitee API 错误 (%d): %s", resp.StatusCode, msg)
			}
		}
		return nil, fmt.Errorf("Gitee API 错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var repo CreateRepoResponse
	if err := json.Unmarshal(respBody, &repo); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &repo, nil
}

// GetGitURL 获取仓库的 Git URL（优先返回 SSH URL）。
func (r *CreateRepoResponse) GetGitURL() string {
	if r.SSHURL != "" {
		return r.SSHURL
	}
	return r.CloneURL
}

// DeleteRepo 删除指定仓库。
// owner 为 Gitee 用户名/组织名，repo 为仓库名。
func (s *GiteeService) DeleteRepo(owner, repo string) error {
	if s.cfg.AccessToken == "" {
		return fmt.Errorf("Gitee access token 未配置")
	}
	if owner == "" || repo == "" {
		return fmt.Errorf("owner 和 repo 不能为空")
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s?access_token=%s",
		s.cfg.APIBaseURL, owner, repo, s.cfg.AccessToken)

	httpReq, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	client := &http.Client{Timeout: 30 * 1000 * 1000 * 1000}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("请求 Gitee API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitee API 错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ExtractOwnerRepoFromGitURL 从 Git URL 中提取 owner 和 repo 名称。
// 支持 SSH 格式: git@gitee.com:owner/repo.git
// 支持 HTTPS 格式: https://gitee.com/owner/repo.git
func ExtractOwnerRepoFromGitURL(gitURL string) (owner, repo string) {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return "", ""
	}

	// SSH 格式: git@gitee.com:owner/repo.git
	if strings.HasPrefix(gitURL, "git@") {
		// 取冒号后的部分
		idx := strings.Index(gitURL, ":")
		if idx < 0 {
			return "", ""
		}
		path := gitURL[idx+1:]
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			owner = parts[0]
			repo = parts[1]
		}
	} else {
		// HTTPS 格式: https://gitee.com/owner/repo.git
		// 去掉协议头
		u := gitURL
		if idx := strings.Index(u, "://"); idx >= 0 {
			u = u[idx+3:]
		}
		// 去掉域名
		if idx := strings.Index(u, "/"); idx >= 0 {
			u = u[idx+1:]
		}
		parts := strings.Split(u, "/")
		if len(parts) >= 2 {
			owner = parts[0]
			repo = parts[1]
		}
	}

	// 去掉 .git 后缀
	repo = strings.TrimSuffix(repo, ".git")
	return owner, repo
}

// ParseGiteeTokenFromURL 从 Gitee 的 OAuth 回调 URL 中解析 access_token。
// 用于支持用户通过浏览器授权后获取 token。
func ParseGiteeTokenFromURL(callbackURL string) (string, error) {
	u, err := url.Parse(callbackURL)
	if err != nil {
		return "", fmt.Errorf("解析 URL 失败: %w", err)
	}

	// 尝试从查询参数获取
	if token := u.Query().Get("access_token"); token != "" {
		return token, nil
	}

	// 尝试从 fragment 获取（OAuth2 implicit grant）
	fragment := u.Fragment
	if fragment != "" {
		values, err := url.ParseQuery(fragment)
		if err != nil {
			return "", nil
		}
		if token := values.Get("access_token"); token != "" {
			return token, nil
		}
	}

	return "", nil
}

// ValidateRepoName 验证仓库名称是否符合 Gitee 规则。
// Gitee 仓库名规则：字母、数字、下划线、中划线，不能以 .git 结尾
func ValidateRepoName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")

	// 替换非法字符
	var result []rune
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result = append(result, r)
		} else {
			result = append(result, '-')
		}
	}
	name = string(result)

	// 去除首尾的特殊字符
	name = strings.Trim(name, "-_")

	// 不能以 .git 结尾
	if strings.HasSuffix(name, ".git") {
		name = name[:len(name)-4]
	}

	if name == "" {
		return "", fmt.Errorf("仓库名称不能为空")
	}

	return name, nil
}