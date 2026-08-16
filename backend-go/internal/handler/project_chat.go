package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"loafer-agent/internal/config"
	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ProjectChatHandler 项目对话式创建处理器。
// 提供 AI 需求提炼（SSE 流式）和项目运行上下文预览接口。
type ProjectChatHandler struct {
	db            *gorm.DB
	cfg           *config.Config
	offlineExec   *cli.OfflineExecutor
	giteeService  *service.GiteeService
}

// NewProjectChatHandler 构造对话式创建处理器。
func NewProjectChatHandler(db *gorm.DB, cfg *config.Config, offlineExecutor *cli.OfflineExecutor) *ProjectChatHandler {
	return &ProjectChatHandler{
		db:           db,
		cfg:          cfg,
		offlineExec:  offlineExecutor,
		giteeService: service.NewGiteeService(&cfg.Gitee),
	}
}

// RegisterRoutes 注册对话式创建相关路由。
func (h *ProjectChatHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/projects")
	{
		g.POST("/chat", h.Chat)                           // SSE: AI 需求提炼
		g.POST("/preview-context", h.PreviewContext)      // 项目运行上下文预览
		g.POST("/chat-create", h.ChatCreate)              // 最终创建项目
	}
}

// ChatRequest 对话请求
type ChatRequest struct {
	Message string `json:"message"` // 用户消息
}

// RequirementSummary AI 提炼的需求摘要
type RequirementSummary struct {
	ProjectName      string   `json:"projectName"`      // 默认选中的项目名称
	ProjectNameOptions []string `json:"projectNameOptions"` // 5个候选项目名称（3-8字）
	RepoName         string   `json:"repoName"`         // 默认英文工程名（同时作为git仓库名）
	RepoNameOptions  []string `json:"repoNameOptions"`  // 5个候选英文工程名
	Summary          string   `json:"summary"`          // 需求摘要
	KeyFeatures      []string `json:"keyFeatures"`      // 关键功能列表
	TechRequirements []string `json:"techRequirements"` // 技术要求
	UserType         string   `json:"userType"`         // 目标用户
}

// Chat 对应 POST /projects/chat（SSE 流式）。
// 接收用户的自然语言需求，通过 Claude CLI 提炼关键信息并返回。
func (h *ProjectChatHandler) Chat(c *gin.Context) {
	var body ChatRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		util.Fail(c, http.StatusBadRequest, "消息不能为空")
		return
	}

	sse := util.NewSSEWriter(c.Writer)
	defer sse.Close()

	// 如果 Claude CLI 不可用，直接返回错误（不再降级到规则取名）
	if !cli.IsCLIAvailable() {
		sse.SendError("AI 服务暂不可用，请稍后重试")
		return
	}

	// 使用 Claude CLI 提炼需求（失败自动重试，最多 3 次）
	summary, err := h.aiBasedSummary(body.Message, func(output string) {
		sse.SendOutput(output)
	})
	if err != nil {
		sse.SendError(fmt.Sprintf("需求提炼失败（已重试 3 次）: %v", err))
		return
	}

	sse.SendDone(summary)
}

// aiBasedSummary 使用 Claude CLI（--print 模式）提炼需求。
// AI 调用失败或返回低质量结果时自动重试，最多 3 次。
func (h *ProjectChatHandler) aiBasedSummary(message string, onOutput func(string)) (*RequirementSummary, error) {
	prompt := buildRequirementExtractionPrompt(message)
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if onOutput != nil {
			if attempt == 1 {
				onOutput("正在分析您的需求...\n")
			} else {
				onOutput(fmt.Sprintf("\n（第 %d 次重试...）\n", attempt))
			}
		}

		result := h.offlineExec.ExecuteSimple("/tmp", prompt, onOutput)
		cli.RecordCall(h.db, "chat_extract", nil, nil, prompt, result, "/tmp")
		if result.ExitCode != 0 {
			lastErr = fmt.Errorf("Claude CLI 执行失败（退出码 %d）: %s", result.ExitCode, result.Error)
			continue
		}

		// 优先使用 AIResponse（纯 AI 文本，不含系统事件/思考/完成标记）
		// 为空时回退到 Response（兼容不输出 content_block_delta 的 CLI 版本）
		aiOutput := result.AIResponse
		if aiOutput == "" {
			aiOutput = result.Response
		}
		summary, err := parseRequirementSummary(aiOutput, message)
		if err != nil {
			lastErr = err
			continue
		}
		return summary, nil
	}
	return nil, fmt.Errorf("AI 需求提炼失败（已重试 %d 次）: %w", maxRetries, lastErr)
}

// buildRequirementExtractionPrompt 构建需求提炼提示词
func buildRequirementExtractionPrompt(message string) string {
	var sb strings.Builder
	sb.WriteString("你是一位资深产品经理。请分析以下用户需求，提炼出关键信息。\n\n")
	sb.WriteString("## 用户需求\n")
	sb.WriteString(message)
	sb.WriteString("\n\n")
	sb.WriteString("## 输出要求\n")
	sb.WriteString("请以 JSON 格式输出（不要用 markdown 代码块包裹），包含以下字段：\n")
	sb.WriteString(`{"projectName":"默认选中的中文项目名(3-8字)","projectNameOptions":["中文项目名1(3-8字)","中文项目名2","中文项目名3","中文项目名4","中文项目名5"],"repoName":"默认英文工程名(小写字母+下划线)","repoNameOptions":["english_name_1","english_name_2","english_name_3","english_name_4","english_name_5"],"summary":"一句话需求摘要","keyFeatures":["功能1","功能2"],"techRequirements":["技术要求1"],"userType":"目标用户"}`)
	sb.WriteString("\n\n## 命名规则（非常重要）\n")
	sb.WriteString("- projectNameOptions：5个候选中文项目名，每个3-8个字，必须是简洁、完整、有辨识度的产品名称\n")
	sb.WriteString("  - 提取需求中的核心名词作为项目名主体（如\"记事本\"、\"任务看板\"、\"商城\"）\n")
	sb.WriteString("  - 核心名词必须完整提取，绝不能截断（如\"个人待办\"不能截成\"人待办\"，\"简易商城\"不能截成\"易商城\"）\n")
	sb.WriteString("  - 可添加修饰词（如\"协作记事本\"、\"团队任务看板\"、\"生鲜商城\"）\n")
	sb.WriteString("  - 可使用近义词替换（如\"笔记\"替代\"记事本\"、\"待办\"替代\"任务\"）\n")
	sb.WriteString("  - 5个候选名必须是5个不同的产品概念，不能是同一名称的机械变体\n")
	sb.WriteString("  - 绝对不要使用用户输入的原始句子片段作为项目名（如\"我想要创建一个多\"是错误的）\n")
	sb.WriteString("  - 绝对不要使用无意义的后缀变体（如\"XX助手\"、\"XX系统\"、\"XX工具\"这种机械式生成）\n")
	sb.WriteString("  - 绝对不要使用数字后缀变体（如\"待办1\"、\"待办2\"、\"待办3\"、\"mvp1\"、\"mvp2\"是严重错误）\n")
	sb.WriteString("  - 绝对不要中英文混杂（如\"待办mvp\"、\"商城web\"、\"笔记app\"是错误的，应纯中文）\n")
	sb.WriteString("  - 用户输入中的技术词（如\"mvp\"、\"web\"、\"app\"、\"系统\"）是描述性词汇，不要直接拼进项目名，而要提炼其背后的产品意图（如\"简易mvp\"→\"极简\"、\"轻量\"；\"web系统\"→省略不体现）\n")
	sb.WriteString("- repoNameOptions：5个候选英文工程名，仅含小写字母和下划线，作为git仓库名\n")
	sb.WriteString("  - 将项目名的核心概念翻译为英文（如\"记事本\"→notebook，\"任务\"→task）\n")
	sb.WriteString("  - 可组合修饰词（如collab_notebook、team_task_board）\n")
	sb.WriteString("  - 不要用 project_app、project_system 这种无意义的通用名\n")
	sb.WriteString("- keyFeatures：将需求拆分为独立的功能点，每个功能2-8个字\n")
	sb.WriteString("  - 按顿号/逗号分割后，每个功能点应独立成项\n")
	sb.WriteString("  - 去掉\"包含\"、\"等功能\"等非功能描述\n")
	sb.WriteString("  - 例如\"用户注册登录、创建编辑记事本、用户授权读写\"应拆为[\"用户注册登录\",\"创建编辑记事本\",\"用户授权读写\"]\n")
	sb.WriteString("\n## 示例\n")
	sb.WriteString("输入：我想要创建一个多人协作的记事本应用。包含用户注册登录、创建编辑记事本、用户授权读写等功能\n")
	sb.WriteString(`输出：{"projectName":"协作记事本","projectNameOptions":["协作记事本","多人笔记","团队记事本","协作笔记","共享笔记"],"repoName":"collab_notebook","repoNameOptions":["collab_notebook","team_notes","collab_notes","multi_notebook","shared_notes"],"summary":"多人协作的记事本应用，支持用户注册登录、创建编辑记事本和权限管理","keyFeatures":["用户注册登录","创建编辑记事本","多人实时协作","用户授权读写"],"techRequirements":["Go后端","React前端","MySQL数据库","WebSocket实时同步"],"userType":"团队协作用户"}`)
	sb.WriteString("\n\n输入：我想做一个在线教育平台，需要有课程管理、视频播放、作业提交、学生老师互动等功能\n")
	sb.WriteString(`输出：{"projectName":"在线课堂","projectNameOptions":["在线课堂","教育平台","云课堂","在线学堂","智学平台"],"repoName":"online_classroom","repoNameOptions":["online_classroom","edu_platform","cloud_classroom","online_learning","smart_edu"],"summary":"在线教育平台，支持课程管理、视频播放、作业提交和师生互动","keyFeatures":["课程管理","视频播放","作业提交","师生互动","学习进度跟踪"],"techRequirements":["Go后端","React前端","MySQL数据库","视频流媒体服务"],"userType":"教育用户"}`)
	sb.WriteString("\n\n输入：开发一个简易的个人待办mvp web系统，包含用户名密码注册登陆和最简易的待办功能\n")
	sb.WriteString(`输出：{"projectName":"极简待办","projectNameOptions":["极简待办","个人待办","轻量待办","随身待办","今日待办"],"repoName":"simple_todo","repoNameOptions":["simple_todo","mini_todo","personal_todo","lite_todo","daily_todo"],"summary":"个人待办Web应用，支持账号注册登录和基础的待办事项管理","keyFeatures":["注册登录","创建待办","编辑待办","完成待办"],"techRequirements":["Go后端","React前端","MySQL数据库"],"userType":"个人用户"}`)
	sb.WriteString("\n注意上面这个示例：用户输入了\"mvp\"、\"web系统\"等词，但项目名里没有出现这些英文/技术词，而是提炼为\"极简\"、\"轻量\"等中文修饰词；5个候选名都是不同的产品概念，没有数字后缀变体。\n")
	sb.WriteString("\n请直接输出 JSON，不要有其他内容。")
	return sb.String()
}

// extractFirstJSONObject 从字符串中提取第一个完整的 JSON 对象。
// 采用平衡括号扫描，并跳过字符串字面量内的括号，避免被嵌套或重复的 JSON 片段干扰。
func extractFirstJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// parseRequirementSummary 解析 AI 输出为需求摘要。
// 不再做规则降级：解析失败或质量不达标时返回 error，由调用方决定是否重试。
func parseRequirementSummary(output, originalMessage string) (*RequirementSummary, error) {
	// cleaned already has ANSI stripped by OfflineExecutor's stream-json parsing
	cleaned := output

	// 循环查找包含 projectName 的 JSON 对象。
	// AI 输出中可能夹杂 thinking_tokens 等系统事件 JSON（如 {"type":"system",...}），
	// extractFirstJSONObject 会找到第一个 {，但那可能是系统事件而非实际结果。
	// 因此需要跳过不含 projectName 的 JSON，继续找下一个。
	remaining := cleaned
	var summary RequirementSummary
	jsonFound := false
	for {
		jsonStr := extractFirstJSONObject(remaining)
		if jsonStr == "" {
			break
		}
		// 尝试解析这个 JSON
		var s RequirementSummary
		if err := json.Unmarshal([]byte(jsonStr), &s); err != nil || s.ProjectName == "" {
			// 解析失败或不含 projectName，跳过这个 JSON，继续找下一个
			idx := strings.Index(remaining, jsonStr)
			if idx >= 0 {
				remaining = remaining[idx+len(jsonStr):]
				continue
			}
			break
		}
		summary = s
		jsonFound = true
		break
	}
	if !jsonFound {
		return nil, fmt.Errorf("AI 输出中未找到包含 projectName 的 JSON 对象")
	}
	// 校验 AI 生成的项目名质量
	summary.ProjectName = sanitizeProjectName(summary.ProjectName, originalMessage)
	// 清洗候选项目名列表
	cleanedOptions := []string{}
	seen := map[string]bool{}
	for _, opt := range summary.ProjectNameOptions {
		opt = sanitizeProjectName(opt, originalMessage)
		if opt != "" && !seen[opt] {
			seen[opt] = true
			cleanedOptions = append(cleanedOptions, opt)
		}
	}
	// 无条件用清洗后的列表替换：即使全部被过滤（空列表）也要替换，
	// 否则原始脏数据会残留导致后续逻辑误判
	summary.ProjectNameOptions = cleanedOptions
	// 如果清洗后项目名为空（说明 AI 生成的全是低质量名称），返回 error 让调用方重试
	if summary.ProjectName == "" {
		if len(summary.ProjectNameOptions) > 0 {
			summary.ProjectName = summary.ProjectNameOptions[0]
		} else {
			return nil, fmt.Errorf("AI 返回的项目名全部不符合质量要求（被清洗过滤）")
		}
	}
	// 确保候选列表有值
	if len(summary.ProjectNameOptions) == 0 {
		summary.ProjectNameOptions = []string{summary.ProjectName}
	}
	// 确保 ProjectName 在候选列表中
	found := false
	for _, opt := range summary.ProjectNameOptions {
		if opt == summary.ProjectName {
			found = true
			break
		}
	}
	if !found {
		summary.ProjectNameOptions = append([]string{summary.ProjectName}, summary.ProjectNameOptions...)
	}
	if len(summary.RepoNameOptions) == 0 && summary.RepoName != "" {
		summary.RepoNameOptions = []string{summary.RepoName}
	}
	if summary.RepoName == "" && len(summary.RepoNameOptions) > 0 {
		summary.RepoName = summary.RepoNameOptions[0]
	}
	// 校验 repoName 格式
	validRepo, err := service.ValidateRepoName(summary.RepoName)
	if err == nil && validRepo != "" {
		summary.RepoName = validRepo
	}
	return &summary, nil
}

// sanitizeProjectName 清洗项目名称，过滤掉低质量的结果。
// 返回空字符串表示名称不合格，调用方应重试 AI 调用。
func sanitizeProjectName(name, originalMessage string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	rc := utf8.RuneCountInString(name)
	if rc < 3 || rc > 8 {
		return ""
	}
	// 排除包含口语前缀的名称
	badPrefixes := []string{"我想要", "我想做", "我要做", "帮我", "请帮", "我需要", "我想", "我要"}
	for _, p := range badPrefixes {
		if strings.HasPrefix(name, p) {
			return ""
		}
	}
	// 排除中英文混杂的名称（如"待办mvp"、"商城web"、"笔记app"）
	// 允许纯中文或以中文为主；若同时包含 ASCII 字母视为混杂
	hasChinese := false
	hasASCII := false
	for _, r := range name {
		if r >= 0x4e00 && r <= 0x9fff {
			hasChinese = true
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasASCII = true
		}
	}
	if hasChinese && hasASCII {
		return ""
	}
	// 排除以数字结尾的机械变体（如"待办1"、"待办2"、"mvp3"）
	nameRunes := []rune(name)
	lastRune := nameRunes[len(nameRunes)-1]
	if lastRune >= '0' && lastRune <= '9' {
		return ""
	}
	// 排除与原始输入开头完全相同的名称（说明是截断而非提炼）
	origRunes := []rune(originalMessage)
	if len(nameRunes) <= len(origRunes) {
		sameAsInput := true
		for i, r := range nameRunes {
			if origRunes[i] != r {
				sameAsInput = false
				break
			}
		}
		if sameAsInput {
			return ""
		}
	}
	return name
}

// PreviewContextRequest 上下文预览请求
type PreviewContextRequest struct {
	Name        string `json:"name"`        // 项目名称（中文）
	RepoName    string `json:"repoName"`    // 英文工程名（git仓库名）
	Description string `json:"description"` // 需求描述
}

// ContextPreview 上下文预览响应
type ContextPreview struct {
	WorkDir      string `json:"workDir"`      // 工作目录
	GitRepoName  string `json:"gitRepoName"`  // Git 仓库名
	GitURL       string `json:"gitUrl"`       // Git URL（创建后才有）
	FrontendPort int    `json:"frontendPort"` // 前端端口
	BackendPort  int    `json:"backendPort"`  // 后端端口
	Database     string `json:"database"`     // 数据库名
	NginxDomain  string `json:"nginxDomain"`  // Nginx 域名
	ServerHost   string `json:"serverHost"`   // 服务器地址
	DevLanguage  string `json:"devLanguage"`  // 开发语言
}

// PreviewContext 对应 POST /projects/preview-context。
// 根据项目名称生成项目运行上下文预览（不实际分配资源）。
func (h *ProjectChatHandler) PreviewContext(c *gin.Context) {
	var body PreviewContextRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		util.Fail(c, http.StatusBadRequest, "项目名称不能为空")
		return
	}

	// 英文工程名：优先使用用户传入的 repoName，否则从项目名生成
	repoName := strings.TrimSpace(body.RepoName)
	if repoName == "" {
		repoName, _ = service.ValidateRepoName(body.Name)
	} else {
		repoName, _ = service.ValidateRepoName(repoName)
	}

	// 生成数据库名
	dbName := "loafer_proj_" + repoName

	// 预估端口（从范围内取下一个可用端口）
	// 注意：这里只是预览，不实际写入数据库分配记录
	portAllocator := service.NewPortAllocator(h.db, &h.cfg.Infra)
	frontendPort, backendPort := portAllocator.PeekNextPorts()

	preview := &ContextPreview{
		WorkDir:      "/srv/zfei/projects/" + repoName,
		GitRepoName:  repoName,
		FrontendPort: frontendPort,
		BackendPort:  backendPort,
		Database:     dbName,
		NginxDomain:  fmt.Sprintf("%s:%d", h.cfg.Infra.ServerHost, frontendPort),
		ServerHost:   h.cfg.Infra.ServerHost,
		DevLanguage:  "go+reactjs",
	}

	util.OKWithData(c, preview)
}

// ChatCreateRequest 对话式创建项目请求
type ChatCreateRequest struct {
	Name        string `json:"name"`        // 确认的项目名称（中文）
	RepoName    string `json:"repoName"`    // 英文工程名（git仓库名）
	Description string `json:"description"` // 确认的需求描述
}

// ChatCreate 对应 POST /projects/chat-create。
// 最终创建项目，自动设置开发语言、工作目录、Gitee 仓库。
func (h *ProjectChatHandler) ChatCreate(c *gin.Context) {
	var body ChatCreateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		util.Fail(c, http.StatusBadRequest, "项目名称不能为空")
		return
	}

	// 英文工程名：优先使用用户传入的 repoName，否则从项目名生成
	repoName := strings.TrimSpace(body.RepoName)
	if repoName == "" {
		repoName, _ = service.ValidateRepoName(body.Name)
	} else {
		repoName, _ = service.ValidateRepoName(repoName)
	}
	if repoName == "" {
		repoName = "project"
	}

	// 构建项目
	project := model.Project{
		Name:        body.Name,
		Description: body.Description,
		DevLanguage: "go+reactjs",
	}
	project.WorkDir = "/srv/zfei/projects/" + repoName

	// 在 Gitee 上创建仓库（使用英文工程名）
	if h.giteeService != nil {
		validRepoName, err := service.ValidateRepoName(repoName)
		if err == nil {
			repo, err := h.giteeService.CreateRepo(&service.CreateRepoRequest{
				Name:        validRepoName,
				Description: project.Description,
				Private:     true,
				HasIssues:   true,
				HasWiki:     true,
				AutoInit:    true,
			})
			if err != nil {
				fmt.Printf("⚠ Gitee 仓库创建失败: %v\n", err)
			} else {
				project.GitURL = repo.GetGitURL()
				fmt.Printf("✓ Gitee 仓库创建成功: %s\n", repo.HTMLURL)
			}
		}
	}

	if err := h.db.Create(&project).Error; err != nil {
		util.Fail500(c, "创建失败: "+err.Error())
		return
	}
	util.OK(c, project)
}
