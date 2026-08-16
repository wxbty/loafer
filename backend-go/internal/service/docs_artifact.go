package service

import (
	"fmt"
	"strings"
	"time"

	"loafer-agent/internal/config"
)

// DocsArtifactService 负责将计划/分解等中间产物写入项目工作目录的 docs/plans/ 目录，
// 并自动执行 git add + commit + push，使中间产物在仓库中可见可追溯。
// 通过 SSH 在远程服务器（项目工作目录所在主机）上执行所有文件写入和 git 操作。
type DocsArtifactService struct {
	cfg       *config.Config
	sshClient *SSHClient
}

// NewDocsArtifactService 构造文档产物服务，内部建立 SSH 连接。
func NewDocsArtifactService(cfg *config.Config) *DocsArtifactService {
	sshClient, err := NewSSHClient(&cfg.Infra)
	if err != nil {
		fmt.Printf("⚠ DocsArtifactService SSH 客户端创建失败，文档产物将不会写入磁盘: %v\n", err)
	}
	return &DocsArtifactService{cfg: cfg, sshClient: sshClient}
}

// IsAvailable 返回 SSH 客户端是否可用。
func (s *DocsArtifactService) IsAvailable() bool {
	return s.sshClient != nil
}

// WritePlanDoc 将执行计划写入 {workDir}/docs/plans/{date}-{slug}.md 并 git 提交推送。
func (s *DocsArtifactService) WritePlanDoc(workDir, projectName, gitURL, planContent string) error {
	return s.writeDocAndPush(workDir, projectName, gitURL, planContent, "plan")
}

// WriteModulesDoc 将模块分解详情写入 {workDir}/docs/plans/{date}-{slug}-modules.md 并 git 提交推送。
func (s *DocsArtifactService) WriteModulesDoc(workDir, projectName, gitURL, modulesContent string) error {
	return s.writeDocAndPush(workDir, projectName, gitURL, modulesContent, "modules")
}

// writeDocAndPush 写入文档并执行 git add + commit + push。
// docType 取值 "plan" 或 "modules"，决定文件名后缀。
func (s *DocsArtifactService) writeDocAndPush(workDir, projectName, gitURL, content, docType string) error {
	if s.sshClient == nil {
		return fmt.Errorf("SSH 客户端未初始化")
	}
	if workDir == "" {
		return fmt.Errorf("工作目录为空")
	}

	slug := extractSlugFromGitURL(gitURL)
	if slug == "" {
		slug = slugify(projectName)
	}
	if slug == "" {
		slug = "project"
	}

	date := time.Now().Format("2006-01-02")

	var filename string
	if docType == "modules" {
		filename = fmt.Sprintf("%s-%s-modules.md", date, slug)
	} else {
		filename = fmt.Sprintf("%s-%s.md", date, slug)
	}

	docsDir := fmt.Sprintf("%s/docs/plans", workDir)

	// 创建目录
	if err := s.sshClient.MkdirRemote(docsDir); err != nil {
		return fmt.Errorf("创建文档目录失败: %w", err)
	}

	// 写入文件
	remotePath := fmt.Sprintf("%s/%s", docsDir, filename)
	if err := s.sshClient.WriteRemoteFile(remotePath, []byte(content), "0644"); err != nil {
		return fmt.Errorf("写入文档文件失败: %w", err)
	}

	// git add + commit + push -u（-u 设置上游跟踪分支，解决 "no upstream branch" 问题）
	commitMsg := fmt.Sprintf("docs: 自动生成%s文档 (%s)", docTypeLabel(docType), filename)
	gitCmds := fmt.Sprintf(
		"cd %s && git config user.email 'loafer@agent.local' && git config user.name 'Loafer Agent' && git add docs/ && (git commit -m '%s' || true) && (git push -u origin HEAD 2>&1 || true)",
		workDir, commitMsg,
	)
	if _, err := s.sshClient.RunCommandWithTimeout(gitCmds, 2*time.Minute); err != nil {
		return fmt.Errorf("git 提交推送失败: %w", err)
	}

	return nil
}

// docTypeLabel 返回文档类型的中文标签。
func docTypeLabel(docType string) string {
	if docType == "modules" {
		return "模块分解"
	}
	return "执行计划"
}

// extractSlugFromGitURL 从 Git URL 中提取仓库名作为 slug。
// git@gitee.com:owner/repo.git → repo
// https://gitee.com/owner/repo.git → repo
// 下划线替换为连字符（如 simple_todo → simple-todo）。
func extractSlugFromGitURL(gitURL string) string {
	if gitURL == "" {
		return ""
	}
	name := strings.TrimSuffix(gitURL, ".git")
	// 取最后一个路径段
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ToLower(name)
	name = strings.Trim(name, "-")
	return name
}

// slugify 将项目名称转换为 URL 安全的 slug。
// 中文等非 ASCII 字符会被过滤，仅保留小写字母、数字和连字符。
func slugify(name string) string {
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r + 32)
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == ' ' || r == '_':
			sb.WriteRune('-')
		}
	}
	result := strings.Trim(sb.String(), "-")
	return result
}
