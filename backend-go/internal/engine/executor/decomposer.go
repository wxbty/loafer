package executor

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"
	"loafer-agent/internal/service"
	"loafer-agent/internal/util"

	"gorm.io/gorm"
)

// 模块类型常量。
const (
	ModuleTypeInfrastructure = "infrastructure"
	ModuleTypeBusiness       = "business"
)

// ModuleDef 模块定义，用于解析 Claude 分解结果中的模块结构。
type ModuleDef struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	SequenceNumber string `json:"sequenceNumber"`
	BlockedBy      string `json:"blockedBy"`
	// Type 模块类型：infrastructure（基础架构，仅做构建+启动校验）/ business（业务，含 API/Web/TDD 测试）
	Type  string    `json:"type"`
	Tasks []TaskDef `json:"tasks"`
}

// TaskDef 任务定义，用于解析 Claude 分解结果中的任务结构。
type TaskDef struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	SequenceNumber string          `json:"sequenceNumber"`
	StepsJSON      json.RawMessage `json:"stepsJSON"`
	Category       string          `json:"category"`
	BlockedBy      string          `json:"blockedBy"`
}

// stepsJSONToString 将 json.RawMessage 转换为字符串，兼容字符串和数组两种格式。
// 如果 Claude 返回 ["步骤1","步骤2"]（数组），转换为 JSON 字符串 '["步骤1","步骤2"]'。
// 如果 Claude 返回 "[\"步骤1\"]"（字符串），直接使用。
func stepsJSONToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试作为字符串解析
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// 已经是 JSON 数组，直接转为字符串
	return string(raw)
}

// codeBlockRegex 匹配 Markdown 代码块中的 JSON 内容。
var codeBlockRegex = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")

// Decomposer 计划分解引擎，将已确认的执行计划分解为模块和任务。
// 通过 Claude Code CLI（--print 模式）分析计划内容，输出结构化 JSON，
// 再解析并批量保存 Module 和 Task 记录到数据库。
type Decomposer struct {
	db          *gorm.DB
	executor    *cli.OfflineExecutor
	docsService *service.DocsArtifactService
}

// NewDecomposer 构造计划分解器。
// docsService 可为 nil（此时跳过 docs 目录产物写入）。
func NewDecomposer(db *gorm.DB, executor *cli.OfflineExecutor, docsService *service.DocsArtifactService) *Decomposer {
	return &Decomposer{db: db, executor: executor, docsService: docsService}
}

// DecomposePlan 将已确认的执行计划分解为模块和任务。
// 流程：加载计划 → 校验状态 → 构建提示词 → 通过 OfflineExecutor 执行 →
// 解析 JSON → 批量保存 Module/Task → 更新计划状态为 decomposed。
func (d *Decomposer) DecomposePlan(planID int64, onOutput func(string)) ([]model.Module, error) {
	var plan model.ExecutionPlan
	if err := d.db.First(&plan, planID).Error; err != nil {
		return nil, fmt.Errorf("加载执行计划失败: %w", err)
	}

	if plan.Status != "confirmed" {
		return nil, fmt.Errorf("执行计划状态必须为 confirmed 才能分解，当前状态: %s", plan.Status)
	}

	var project model.Project
	if err := d.db.First(&project, plan.ProjectID).Error; err != nil {
		return nil, fmt.Errorf("加载项目失败: %w", err)
	}
	if project.WorkDir == "" {
		return nil, fmt.Errorf("项目工作目录未设置")
	}

	prompt := d.buildDecomposePrompt(&plan)
	if onOutput != nil {
		onOutput("正在向 Claude 发送计划分解请求...\n")
	}

	result := d.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	projectIDPtr := plan.ProjectID
	cli.RecordCall(d.db, "decompose", &projectIDPtr, nil, prompt, result, project.WorkDir)
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("Claude CLI 执行失败（退出码 %d）: %s", result.ExitCode, result.Error)
	}

	moduleDefs, err := d.ParseDecomposeResult(result.Response)
	if err != nil {
		return nil, fmt.Errorf("解析分解结果失败: %w", err)
	}

	if onOutput != nil {
		onOutput(fmt.Sprintf("\n解析到 %d 个模块，正在保存到数据库...\n", len(moduleDefs)))
	}

	modules, err := d.BatchSaveModules(plan.ProjectID, moduleDefs)
	if err != nil {
		return nil, fmt.Errorf("保存模块和任务失败: %w", err)
	}

	d.db.Model(&model.ExecutionPlan{}).Where("id = ?", planID).Update("status", "decomposed")

	// 将模块分解详情写入 docs/plans/ 目录并 git 提交推送（best-effort，失败不阻断流程）
	d.writeModulesDocBestEffort(project.WorkDir, project.Name, project.GitURL, modules, onOutput)

	if onOutput != nil {
		onOutput(fmt.Sprintf("计划分解完成，共创建 %d 个模块\n", len(modules)))
	}
	return modules, nil
}

// ParseDecomposeResult 解析 Claude 输出，提取模块定义的 JSON 数组。
func (d *Decomposer) ParseDecomposeResult(output string) ([]ModuleDef, error) {
	// 策略 1：从 Markdown 代码块中提取
	jsonStr := extractJSONFromCodeBlock(output)

	// 策略 2：回退到查找方括号
	if jsonStr == "" {
		jsonStr = extractJSONByBrackets(output)
	}

	if jsonStr == "" {
		return nil, fmt.Errorf("未在输出中找到 JSON 内容")
	}

	var modules []ModuleDef
	if err := json.Unmarshal([]byte(jsonStr), &modules); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w (json: %s)", err, truncateForError(jsonStr, 200))
	}

	if len(modules) == 0 {
		return nil, fmt.Errorf("解析到的模块列表为空")
	}

	return modules, nil
}

// sanitizeBlockedBy 清洗 blockedBy 依赖列表：剔除不存在于 validSeqs 中的序号及自依赖。
// LLM 分解结果可能把模块序号（如 "2"）误填进任务级 blockedBy，这类序号在 task 表中
// 永远查不到，会让执行期依赖检查永远不满足，造成流水线死锁，因此入库前必须过滤。
// 返回保留的依赖串与被剔除的序号列表（用于日志告警）。
func sanitizeBlockedBy(blockedBy string, validSeqs map[string]bool, self string) (string, []string) {
	if blockedBy == "" {
		return "", nil
	}
	var kept, dropped []string
	for _, dep := range strings.Split(blockedBy, ",") {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		if dep == self || !validSeqs[dep] {
			dropped = append(dropped, dep)
			continue
		}
		kept = append(kept, dep)
	}
	return strings.Join(kept, ","), dropped
}

// sanitizeModulesDependencies 在校验集合内清洗模块与任务的 blockedBy，
// 剔除引用不存在的模块/任务序号的依赖项。
func sanitizeModulesDependencies(modules []ModuleDef) {
	moduleSeqs := make(map[string]bool, len(modules))
	taskSeqs := make(map[string]bool)
	for _, m := range modules {
		moduleSeqs[m.SequenceNumber] = true
		for _, t := range m.Tasks {
			taskSeqs[t.SequenceNumber] = true
		}
	}
	for i := range modules {
		kept, dropped := sanitizeBlockedBy(modules[i].BlockedBy, moduleSeqs, modules[i].SequenceNumber)
		if len(dropped) > 0 {
			log.Printf("[decomposer] 模块 %s 的 blockedBy 含无法解析的序号 %v，已剔除", modules[i].SequenceNumber, dropped)
			modules[i].BlockedBy = kept
		}
		for j := range modules[i].Tasks {
			kept, dropped := sanitizeBlockedBy(modules[i].Tasks[j].BlockedBy, taskSeqs, modules[i].Tasks[j].SequenceNumber)
			if len(dropped) > 0 {
				log.Printf("[decomposer] 任务 %s 的 blockedBy 含无法解析的序号 %v，已剔除", modules[i].Tasks[j].SequenceNumber, dropped)
				modules[i].Tasks[j].BlockedBy = kept
			}
		}
	}
}

// BatchSaveModules 在单个事务中批量保存模块及其下属任务到数据库。
func (d *Decomposer) BatchSaveModules(projectID int64, modules []ModuleDef) ([]model.Module, error) {
	var result []model.Module

	sanitizeModulesDependencies(modules)

	err := d.db.Transaction(func(tx *gorm.DB) error {
		for _, mdef := range modules {
			// 兼容性：未指定 type 时按业务模块处理
			moduleType := mdef.Type
			if moduleType != ModuleTypeInfrastructure && moduleType != ModuleTypeBusiness {
				moduleType = ModuleTypeBusiness
			}
			module := model.Module{
				ProjectID:      projectID,
				Name:           mdef.Name,
				Description:    mdef.Description,
				SequenceNumber: mdef.SequenceNumber,
				BlockedBy:      mdef.BlockedBy,
				ModuleType:     moduleType,
				Status:         ModuleStatusPending,
			}
			if err := tx.Create(&module).Error; err != nil {
				return fmt.Errorf("创建模块 [%s] 失败: %w", mdef.Name, err)
			}

			for _, tdef := range mdef.Tasks {
				task := model.Task{
					ProjectID:      projectID,
					ModuleID:       &module.ID,
					Name:           tdef.Name,
					Description:    tdef.Description,
					SequenceNumber: tdef.SequenceNumber,
					StepsJSON:      stepsJSONToString(tdef.StepsJSON),
					Category:       tdef.Category,
					BlockedBy:      tdef.BlockedBy,
					Status:         TaskStatusPending,
				}
				if err := tx.Create(&task).Error; err != nil {
					return fmt.Errorf("创建任务 [%s] 失败: %w", tdef.Name, err)
				}
			}

			result = append(result, module)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetModuleTasks 查询模块下的所有任务，按 sequence_number 排序。
func (d *Decomposer) GetModuleTasks(moduleID int64) ([]model.Task, error) {
	var tasks []model.Task
	if err := d.db.Where("module_id = ?", moduleID).
		Order("id ASC").
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("查询任务失败: %w", err)
	}
	util.SortBySequenceNumber(tasks, func(t model.Task) string { return t.SequenceNumber })
	return tasks, nil
}

// ---- 内部辅助方法 ----

// buildDecomposePrompt 构建计划分解提示词，指导 Claude 输出结构化 JSON。
func (d *Decomposer) buildDecomposePrompt(plan *model.ExecutionPlan) string {
	var sb strings.Builder
	sb.WriteString("【语言要求 - 最高优先级】你必须使用中文输出所有内容，包括模块名称、任务名称、描述、步骤。不要使用英文输出。\n\n")

	sb.WriteString("你是一位项目分解专家。")
	sb.WriteString("请将以下执行计划分解为结构化的模块和任务。\n\n")

	sb.WriteString("## 执行计划\n")
	sb.WriteString(plan.PlanContent)
	sb.WriteString("\n\n")

	sb.WriteString("## 要求\n")
	sb.WriteString("分析计划并分解为模块和任务。")
	sb.WriteString("输出一个 JSON 数组，每个元素是一个模块，结构如下：\n\n")

	sb.WriteString("```json\n")
	sb.WriteString(`[
  {
    "name": "模块名称",
    "description": "模块描述",
    "sequenceNumber": "1",
    "blockedBy": "",
    "type": "infrastructure",
    "tasks": [
      {
        "name": "任务名称",
        "description": "任务描述",
        "sequenceNumber": "1.1",
        "stepsJSON": "[\"步骤1\", \"步骤2\", \"步骤3\"]",
        "category": "backend",
        "blockedBy": ""
      }
    ]
  }
]`)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## 模块划分规则（最高优先级，必须严格遵守）\n\n")
	sb.WriteString("### 1. 模块分类\n")
	sb.WriteString("项目必须且只能划分为两类模块：\n")
	sb.WriteString("- **infrastructure（基础架构模块）**：项目初始化、目录结构、依赖管理、配置加载、数据库连接、基础 HTTP 框架/路由骨架、日志/中间件、健康检查等「非业务基础工作」。整个项目只能有 1 个此类模块。\n")
	sb.WriteString("- **business（业务模块）**：具体业务功能（用户管理、订单、统计、报表、业务流程等）。可以有多个，每个业务领域一个模块。\n\n")

	sb.WriteString("### 2. 基础架构模块必须合并（最高优先级）\n")
	sb.WriteString("以下内容全部属于「非业务基础工作」，必须合并到唯一一个 type=infrastructure 的模块中，绝对禁止拆成独立模块：\n")
	sb.WriteString("- 项目初始化、目录结构、依赖管理（go.mod / package.json）\n")
	sb.WriteString("- Go API 服务 / 后端框架 / HTTP 路由骨架 / 中间件\n")
	sb.WriteString("- 前端基础框架（React/Vue 初始化、路由、基础布局）\n")
	sb.WriteString("- 数据库连接、模型基类、迁移脚本\n")
	sb.WriteString("- 日志、认证、健康检查、配置加载、环境变量\n")
	sb.WriteString("- Git 仓库初始化、开发环境配置\n")
	sb.WriteString("基础设施模块名称固定为「基础架构模块」，sequenceNumber 固定为 \"1\"，blockedBy 留空，type 必须为 \"infrastructure\"。")
	sb.WriteString("其下的任务应覆盖：目录脚手架、前后端依赖初始化、配置加载、DB 连接、后端 HTTP 服务骨架、前端基础框架、健康检查接口、Git 仓库初始化等。\n")
	sb.WriteString("**错误示例（严禁出现）**：单独划分「后端核心服务」「前端基础框架」「项目初始化与基础配置」「Go API 服务」等模块。\n")
	sb.WriteString("**正确示例**：只有一个模块 1「基础架构模块」（type=infrastructure），里面包含初始化 Go 项目、初始化 React 项目、配置开发环境、建立 Git 仓库、定义数据模型、实现 HTTP 处理器等任务。\n\n")

	sb.WriteString("### 3. 业务模块\n")
	sb.WriteString("所有业务功能（如用户管理、订单、待办事项、统计报表、业务流程等）都必须归类为 type=business 的业务模块，sequenceNumber 从 \"2\" 开始递增。")
	sb.WriteString("业务模块间允许通过 blockedBy 声明依赖（如 \"3\" 依赖 \"1,2\"，但通常都依赖基础架构模块 \"1\"）。\n\n")

	sb.WriteString("### 4. 不同类型模块的验证范围（影响后续测试面板，必须正确填写 type）\n")
	sb.WriteString("- infrastructure 模块：仅做「构建校验」（go build / npm build）和「启动验证」（端口可达）。不要为其编写 API 集成测试、Web 集成测试、TDD 验收标准。\n")
	sb.WriteString("- business 模块：才需要且必须支持 API 集成测试、Web 集成测试、TDD 验收标准。\n\n")

	sb.WriteString("## 其他规则\n")
	sb.WriteString("- sequenceNumber 应反映执行顺序（基础设施模块固定为 \"1\"；业务模块从 \"2\" 起递增；模块 N 内的任务用 \"N.1\", \"N.2\"）。\n")
	sb.WriteString("- blockedBy 为必须先完成的模块/任务序号（逗号分隔），无则留空字符串。业务模块通常 blockedBy=\"1\"（依赖基础架构模块）。\n")
	sb.WriteString("- 注意：模块的 blockedBy 只能引用模块序号（如 \"1\"），任务的 blockedBy 只能引用任务序号（如 \"1.5\"），两者不可混用；任务不得把模块序号（如 \"2\"）填入自己的 blockedBy。\n")
	sb.WriteString("- stepsJSON 必须是有效的 JSON 字符串数组，描述具体的实现步骤，至少包含 3 个步骤，不能为空数组。\n")
	sb.WriteString("- category 为以下之一: backend, frontend, database, testing, devops, documentation。\n")
	sb.WriteString("- 基础架构模块允许 4-10 个任务（覆盖范围广）；业务模块应有 3-8 个任务。\n")
	sb.WriteString("- 描述要简洁清晰。\n")
	sb.WriteString("- 所有内容必须使用中文。\n\n")

	sb.WriteString("## 正确示例（必须严格遵循此结构）\n")
	sb.WriteString("```json\n")
	sb.WriteString(`[
  {
    "name": "基础架构模块",
    "description": "项目初始化、目录结构、前后端框架、数据库连接、Git仓库、配置加载等非业务基础工作",
    "sequenceNumber": "1",
    "blockedBy": "",
    "type": "infrastructure",
    "tasks": [
      { "name": "初始化Go项目", "description": "创建backend-go目录、go.mod、目录结构", "sequenceNumber": "1.1", "stepsJSON": "[\"创建 backend-go/ 目录\", \"初始化 go.mod\", \"创建 internal/handler 等目录\"]", "category": "backend", "blockedBy": "" },
      { "name": "初始化React项目", "description": "创建frontend目录、package.json、基础路由和布局", "sequenceNumber": "1.2", "stepsJSON": "[\"创建 frontend/ 目录\", \"初始化 package.json\", \"创建 src/router 和基础布局\"]", "category": "frontend", "blockedBy": "" },
      { "name": "配置开发环境", "description": "Makefile、README.md、环境变量、健康检查接口", "sequenceNumber": "1.3", "stepsJSON": "[\"创建 Makefile\", \"编写 README.md\", \"实现 /api/health 健康检查\"]", "category": "devops", "blockedBy": "" },
      { "name": "建立Git仓库", "description": "初始化git仓库并推送初始提交", "sequenceNumber": "1.4", "stepsJSON": "[\"git init\", \"git remote add origin <仓库地址>\", \"git fetch origin\", \"git pull origin master --allow-unrelated-histories\", \"创建 .gitignore\", \"git add -A && git commit\", \"git push -u origin HEAD\"]", "category": "devops", "blockedBy": "" },
      { "name": "定义数据模型", "description": "定义基础数据模型和数据库连接", "sequenceNumber": "1.5", "stepsJSON": "[\"创建 model 包\", \"定义业务实体\", \"实现数据库连接\"]", "category": "database", "blockedBy": "" },
      { "name": "实现HTTP处理器", "description": "实现基础CRUD HTTP接口和路由注册", "sequenceNumber": "1.6", "stepsJSON": "[\"创建 handler 包\", \"实现 CRUD 接口\", \"注册 /api 路由\"]", "category": "backend", "blockedBy": "1.5" }
    ]
  },
  {
    "name": "业务功能模块示例",
    "description": "具体的业务功能（如用户管理、待办事项、订单等）",
    "sequenceNumber": "2",
    "blockedBy": "1",
    "type": "business",
    "tasks": [
      { "name": "实现业务功能", "description": "根据需求实现具体业务逻辑", "sequenceNumber": "2.1", "stepsJSON": "[\"分析业务需求\", \"实现业务逻辑\", \"编写单元测试\"]", "category": "backend", "blockedBy": "" },
      { "name": "前端页面", "description": "实现对应的前端页面和交互", "sequenceNumber": "2.2", "stepsJSON": "[\"设计页面结构\", \"实现 API 调用\", \"联调前后端\"]", "category": "frontend", "blockedBy": "2.1" }
    ]
  }
]`)
	sb.WriteString("\n```\n\n")
	sb.WriteString("只输出包裹在 ```json 代码块中的 JSON 数组，不要输出任何其他文字。")
	return sb.String()
}

// extractJSONFromCodeBlock 从 Markdown 代码块中提取 JSON 内容。
func extractJSONFromCodeBlock(s string) string {
	matches := codeBlockRegex.FindStringSubmatch(s)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractJSONByBrackets 从文本中查找第一个 [ 和最后一个 ] 之间的内容作为 JSON。
func extractJSONByBrackets(s string) string {
	start := strings.Index(s, "[")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(s, "]")
	if end < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

// truncateForError 截断字符串用于错误信息展示。
func truncateForError(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// writeModulesDocBestEffort 将模块分解详情写入 docs/plans/ 目录并 git 提交推送。
// best-effort 模式：失败时仅输出提示信息，不阻断分解主流程。
func (d *Decomposer) writeModulesDocBestEffort(workDir, projectName, gitURL string, modules []model.Module, onOutput func(string)) {
	if d.docsService == nil || !d.docsService.IsAvailable() {
		return
	}
	if onOutput != nil {
		onOutput("\n[文档] 正在将模块分解详情写入 docs/plans/ 并提交到 Git...\n")
	}
	content := buildModulesMarkdown(modules, d.db)
	if err := d.docsService.WriteModulesDoc(workDir, projectName, gitURL, content); err != nil {
		if onOutput != nil {
			onOutput(fmt.Sprintf("[文档] 写入模块分解文档失败（不影响流程）: %v\n", err))
		}
		return
	}
	if onOutput != nil {
		onOutput("[文档] 模块分解详情已写入 docs/plans/ 并推送到 Git 仓库\n")
	}
}

// buildModulesMarkdown 根据已保存的模块和任务生成 Markdown 文档内容。
func buildModulesMarkdown(modules []model.Module, db *gorm.DB) string {
	var sb strings.Builder
	sb.WriteString("# 模块分解详情\n\n")
	for i, m := range modules {
		sb.WriteString(fmt.Sprintf("## 模块 %d: %s\n", i+1, m.Name))
		sb.WriteString(fmt.Sprintf("- 描述: %s\n", m.Description))
		sb.WriteString(fmt.Sprintf("- 序号: %s\n", m.SequenceNumber))
		sb.WriteString(fmt.Sprintf("- 类型: %s\n", m.ModuleType))
		if m.BlockedBy != "" {
			sb.WriteString(fmt.Sprintf("- 依赖: 模块 %s\n", m.BlockedBy))
		} else {
			sb.WriteString("- 依赖: 无\n")
		}

		var tasks []model.Task
		db.Where("module_id = ?", m.ID).Order("id ASC").Find(&tasks)
		util.SortBySequenceNumber(tasks, func(t model.Task) string { return t.SequenceNumber })
		if len(tasks) > 0 {
			sb.WriteString("\n### 任务列表\n")
			for _, t := range tasks {
				sb.WriteString(fmt.Sprintf("- **%s** %s — %s\n", t.SequenceNumber, t.Name, t.Description))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
