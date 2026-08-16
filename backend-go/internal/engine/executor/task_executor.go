package executor

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"loafer-agent/internal/engine/cli"
	"loafer-agent/internal/model"
	"loafer-agent/internal/util"

	"gorm.io/gorm"
)

// 任务状态常量，对应 task.status 字段的取值（与前端 0-5 取值一致）。
// 0=待办 / 1=执行中 / 2=审查中 / 3=完成 / 4=已暂停 / 5=失败
const (
	TaskStatusPending    = 0
	TaskStatusInProgress = 1
	TaskStatusReviewing  = 2
	TaskStatusCompleted  = 3
	TaskStatusPaused     = 4
	TaskStatusFailed     = 5
)

// 模块状态常量，对应 module.status 字段的取值（与前端 0-6 取值一致）。
// 0=待执行 / 1=执行中 / 2=待测试 / 3=测试中 / 4=完成 / 5=测试失败 / 6=失败
const (
	ModuleStatusPending     = 0
	ModuleStatusInProgress  = 1
	ModuleStatusPendingTest = 2
	ModuleStatusTesting     = 3
	ModuleStatusCompleted   = 4
	ModuleStatusTestFailed  = 5
	ModuleStatusFailed      = 6
)

// extractSummary 从 Claude 输出中提取执行摘要。
func extractSummary(output string) string {
	lower := strings.ToLower(output)
	markers := []string{"## 摘要", "## summary", "# 摘要", "# summary", "摘要：", "摘要:", "summary:", "**摘要**", "**summary**"}
	for _, marker := range markers {
		if idx := strings.Index(lower, marker); idx >= 0 {
			summary := strings.TrimSpace(output[idx:])
			if len(summary) > 2000 {
				summary = summary[:2000] + "\n...(已截断)"
			}
			return summary
		}
	}
	if len(output) > 1000 {
		return output[len(output)-1000:]
	}
	return strings.TrimSpace(output)
}

// TaskExecutor 任务执行引擎，通过 Claude Code CLI（--print 模式）执行单个任务并持久化上下文。
type TaskExecutor struct {
	db       *gorm.DB
	executor *cli.OfflineExecutor

	mu          sync.Mutex
	runningTask map[int64]bool
}

// NewTaskExecutor 构造任务执行器。
func NewTaskExecutor(db *gorm.DB, executor *cli.OfflineExecutor) *TaskExecutor {
	return &TaskExecutor{
		db:          db,
		executor:    executor,
		runningTask: make(map[int64]bool),
	}
}

// IsTaskRunning 返回任务是否正在执行中（内存运行表）。
// 供 StartTask 等入口在触发执行前判断,避免对正在运行的任务做破坏性状态写入
// （软删 task_state、强制置为执行中）。
func (e *TaskExecutor) IsTaskRunning(taskID int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runningTask[taskID]
}

// ExecuteTask 执行单个任务的主方法。
// 流程：加载任务 → 检查依赖 → 构建上下文 → 构建提示词 →
// 通过 OfflineExecutor 执行 → 更新任务状态 → 保存摘要到 TaskState → 记录 SliceHistory。
func (e *TaskExecutor) ExecuteTask(taskID int64, onOutput func(string)) error {
	e.mu.Lock()
	if e.runningTask[taskID] {
		e.mu.Unlock()
		return fmt.Errorf("任务 %d 正在执行中", taskID)
	}
	e.runningTask[taskID] = true
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.runningTask, taskID)
		e.mu.Unlock()
	}()

	var task model.Task
	if err := e.db.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("加载任务失败: %w", err)
	}

	if !e.areDependenciesMet(&task) {
		return fmt.Errorf("任务依赖未满足，blockedBy: %s", task.BlockedBy)
	}

	var project model.Project
	if err := e.db.First(&project, task.ProjectID).Error; err != nil {
		return fmt.Errorf("加载项目失败: %w", err)
	}
	if project.WorkDir == "" {
		return fmt.Errorf("项目工作目录未设置")
	}

	context, err := e.BuildTaskContext(taskID)
	if err != nil {
		return fmt.Errorf("构建任务上下文失败: %w", err)
	}

	e.db.Model(&model.Task{}).Where("id = ?", taskID).Update("status", TaskStatusInProgress)

	startTime := time.Now()
	sliceSeq := e.getNextSliceSequence(taskID)
	sliceHistory := &model.SliceHistory{
		TaskID:        taskID,
		SliceSequence: &sliceSeq,
		StartTime:     &startTime,
		Status:        TaskStatusInProgress,
	}
	if err := e.db.Create(sliceHistory).Error; err != nil {
		return fmt.Errorf("创建执行历史记录失败: %w", err)
	}

	prompt := e.buildExecutionPrompt(&task, context)
	if onOutput != nil {
		onOutput(fmt.Sprintf("开始执行任务: %s - %s\n", task.SequenceNumber, task.Name))
	}

	result := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	projectIDPtr := task.ProjectID
	taskIDPtr := taskID
	cli.RecordCall(e.db, "task_execute", &projectIDPtr, &taskIDPtr, prompt, result, project.WorkDir)

	if result.ExitCode != 0 {
		e.markTaskFailed(taskID, sliceHistory.ID, startTime, fmt.Errorf("CLI 退出码 %d: %s", result.ExitCode, result.Error))
		return fmt.Errorf("任务执行失败（退出码 %d）: %s", result.ExitCode, result.Error)
	}

	summary := extractSummary(result.Response)

	endTime := time.Now()
	duration := int(endTime.Sub(startTime).Seconds())
	stepStatusJSON := BuildCompletedStepStatusJSON(task.StepsJSON)
	// 先持久化执行摘要与步骤状态，再将任务标记为完成。
	// 反之若先置完成再写状态，一旦写状态失败会出现"完成但无摘要/步骤 pending"的假完成。
	e.saveTaskState(taskID, summary, result.Response, stepStatusJSON)
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Update("status", TaskStatusCompleted)

	// output_summary 同样需要截断非法字符，避免 MySQL Error 1366。
	outputSummary := truncateUTF8Bytes(summary, 2000)
	e.db.Model(&model.SliceHistory{}).Where("id = ?", sliceHistory.ID).Updates(map[string]interface{}{
		"end_time":         endTime,
		"duration_seconds": duration,
		"status":           TaskStatusCompleted,
		"output_summary":   outputSummary,
	})

	if onOutput != nil {
		onOutput(fmt.Sprintf("\n任务执行完成: %s - %s\n", task.SequenceNumber, task.Name))
	}
	return nil
}

// BuildTaskContext 构建任务执行上下文字符串。
func (e *TaskExecutor) BuildTaskContext(taskID int64) (string, error) {
	var task model.Task
	if err := e.db.First(&task, taskID).Error; err != nil {
		return "", fmt.Errorf("加载任务失败: %w", err)
	}

	var project model.Project
	if err := e.db.First(&project, task.ProjectID).Error; err != nil {
		return "", fmt.Errorf("加载项目失败: %w", err)
	}

	var sb strings.Builder

	sb.WriteString("# 项目上下文\n")
	sb.WriteString(fmt.Sprintf("- 名称: %s\n", project.Name))
	sb.WriteString(fmt.Sprintf("- 描述: %s\n", project.Description))
	sb.WriteString(fmt.Sprintf("- 开发语言: %s\n", project.DevLanguage))
	sb.WriteString(fmt.Sprintf("- Git 仓库: %s\n", project.GitURL))
	sb.WriteString(fmt.Sprintf("- 工作目录: %s\n\n", project.WorkDir))

	if task.ModuleID != nil {
		var module model.Module
		if err := e.db.First(&module, *task.ModuleID).Error; err == nil {
			sb.WriteString("# 模块上下文\n")
			sb.WriteString(fmt.Sprintf("- 名称: %s\n", module.Name))
			sb.WriteString(fmt.Sprintf("- 描述: %s\n", module.Description))
			sb.WriteString(fmt.Sprintf("- 序号: %s\n", module.SequenceNumber))
			sb.WriteString(fmt.Sprintf("- 前置依赖: %s\n\n", module.BlockedBy))
		}
	}

	if task.ModuleID != nil {
		var priorTasks []model.Task
		e.db.Where("module_id = ? AND id != ? AND status >= ?", *task.ModuleID, taskID, TaskStatusCompleted).
			Order("id ASC").Find(&priorTasks)
		util.SortBySequenceNumber(priorTasks, func(t model.Task) string { return t.SequenceNumber })

		if len(priorTasks) > 0 {
			sb.WriteString("# 本模块已完成的任务\n")
			for _, pt := range priorTasks {
				sb.WriteString(fmt.Sprintf("## %s - %s\n", pt.SequenceNumber, pt.Name))
				sb.WriteString(fmt.Sprintf("描述: %s\n", pt.Description))
				var state model.TaskState
				if err := e.db.Where("task_id = ?", pt.ID).First(&state).Error; err == nil {
					if state.ExecutionSummary != "" {
						sb.WriteString(fmt.Sprintf("执行摘要: %s\n", state.ExecutionSummary))
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	if task.BlockedBy != "" {
		depIDs := strings.Split(task.BlockedBy, ",")
		hasDep := false
		for _, depID := range depIDs {
			depID = strings.TrimSpace(depID)
			if depID == "" {
				continue
			}
			var depTask model.Task
			if err := e.db.Where("project_id = ? AND sequence_number = ?", task.ProjectID, depID).
				First(&depTask).Error; err == nil {
				if !hasDep {
					sb.WriteString("# 依赖任务输出\n")
					hasDep = true
				}
				sb.WriteString(fmt.Sprintf("## %s - %s\n", depTask.SequenceNumber, depTask.Name))
				sb.WriteString(fmt.Sprintf("描述: %s\n", depTask.Description))
				var state model.TaskState
				if err := e.db.Where("task_id = ?", depTask.ID).First(&state).Error; err == nil {
					if state.ExecutionSummary != "" {
						sb.WriteString(fmt.Sprintf("输出: %s\n", state.ExecutionSummary))
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	return sb.String(), nil
}

// GetTaskExecutionSummary 获取任务的执行摘要。
func (e *TaskExecutor) GetTaskExecutionSummary(taskID int64) (string, error) {
	var state model.TaskState
	if err := e.db.Where("task_id = ?", taskID).First(&state).Error; err == nil {
		if state.ExecutionSummary != "" {
			return state.ExecutionSummary, nil
		}
	}
	return "", fmt.Errorf("未找到任务 %d 的执行摘要", taskID)
}

// ExecuteModuleTasks 按序执行模块下的所有任务，尊重任务间依赖关系。
func (e *TaskExecutor) ExecuteModuleTasks(moduleID int64, onOutput func(string)) error {
	var module model.Module
	if err := e.db.First(&module, moduleID).Error; err != nil {
		return fmt.Errorf("加载模块失败: %w", err)
	}

	if !e.areModuleDependenciesMet(&module) {
		return fmt.Errorf("模块依赖未满足，blockedBy: %s", module.BlockedBy)
	}

	var tasks []model.Task
	if err := e.db.Where("module_id = ?", moduleID).
		Order("id ASC").
		Find(&tasks).Error; err != nil {
		return fmt.Errorf("加载任务列表失败: %w", err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("模块 %d 下没有任务", moduleID)
	}
	util.SortBySequenceNumber(tasks, func(t model.Task) string { return t.SequenceNumber })

	e.db.Model(&model.Module{}).Where("id = ?", moduleID).Update("status", ModuleStatusInProgress)

	for _, task := range tasks {
		if task.Status >= TaskStatusCompleted {
			if onOutput != nil {
				onOutput(fmt.Sprintf("任务 %s - %s 已完成，跳过\n", task.SequenceNumber, task.Name))
			}
			continue
		}

		if onOutput != nil {
			onOutput(fmt.Sprintf("===== 开始执行任务: %s - %s =====\n", task.SequenceNumber, task.Name))
		}

		if err := e.ExecuteTask(task.ID, onOutput); err != nil {
			e.db.Model(&model.Module{}).Where("id = ?", moduleID).Update("status", ModuleStatusPending)
			return fmt.Errorf("任务 %s 执行失败: %w", task.SequenceNumber, err)
		}

		if onOutput != nil {
			onOutput(fmt.Sprintf("===== 任务 %s - %s 执行完成 =====\n", task.SequenceNumber, task.Name))
		}
	}

	e.db.Model(&model.Module{}).Where("id = ?", moduleID).Update("status", ModuleStatusCompleted)

	if onOutput != nil {
		onOutput(fmt.Sprintf("模块 %s 下所有任务执行完成\n", module.Name))
	}
	return nil
}

// RecoverTask 从最后的检查点恢复任务执行。
func (e *TaskExecutor) RecoverTask(taskID int64, onOutput func(string)) error {
	var task model.Task
	if err := e.db.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("加载任务失败: %w", err)
	}

	var state model.TaskState
	if err := e.db.Where("task_id = ?", taskID).First(&state).Error; err != nil {
		return fmt.Errorf("未找到任务状态记录，无法恢复: %w", err)
	}

	var project model.Project
	if err := e.db.First(&project, task.ProjectID).Error; err != nil {
		return fmt.Errorf("加载项目失败: %w", err)
	}
	if project.WorkDir == "" {
		return fmt.Errorf("项目工作目录未设置")
	}

	prompt := e.buildRecoveryPrompt(&task, &state)
	if onOutput != nil {
		onOutput(fmt.Sprintf("正在恢复任务: %s - %s\n", task.SequenceNumber, task.Name))
	}

	result := e.executor.ExecuteSimple(project.WorkDir, prompt, onOutput)
	projectIDPtr := task.ProjectID
	taskIDPtr := taskID
	cli.RecordCall(e.db, "task_recover", &projectIDPtr, &taskIDPtr, prompt, result, project.WorkDir)
	if result.ExitCode != 0 {
	}

	summary := extractSummary(result.Response)
	e.saveTaskState(taskID, summary, result.Response, "")

	if onOutput != nil {
		onOutput(fmt.Sprintf("\n任务恢复完成: %s - %s\n", task.SequenceNumber, task.Name))
	}
	return nil
}

// ---- 内部辅助方法 ----

func (e *TaskExecutor) areDependenciesMet(task *model.Task) bool {
	if task.BlockedBy == "" {
		return true
	}
	depIDs := strings.Split(task.BlockedBy, ",")
	for _, depID := range depIDs {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		var depTask model.Task
		if err := e.db.Where("project_id = ? AND sequence_number = ?", task.ProjectID, depID).
			First(&depTask).Error; err != nil {
			return false
		}
		if depTask.Status < TaskStatusCompleted {
			return false
		}
	}
	return true
}

func (e *TaskExecutor) areModuleDependenciesMet(module *model.Module) bool {
	if module.BlockedBy == "" {
		return true
	}
	depIDs := strings.Split(module.BlockedBy, ",")
	for _, depID := range depIDs {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		var depModule model.Module
		if err := e.db.Where("project_id = ? AND sequence_number = ?", module.ProjectID, depID).
			First(&depModule).Error; err != nil {
			return false
		}
		if depModule.Status < ModuleStatusCompleted {
			return false
		}
	}
	return true
}

func (e *TaskExecutor) getNextSliceSequence(taskID int64) int {
	var count int64
	e.db.Model(&model.SliceHistory{}).Where("task_id = ?", taskID).Count(&count)
	return int(count) + 1
}

func (e *TaskExecutor) markTaskFailed(taskID, sliceHistoryID int64, startTime time.Time, execErr error) {
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Update("status", TaskStatusFailed)
	endTime := time.Now()
	duration := int(endTime.Sub(startTime).Seconds())
	errMsg := ""
	if execErr != nil {
		errMsg = execErr.Error()
	}
	e.db.Model(&model.SliceHistory{}).Where("id = ?", sliceHistoryID).Updates(map[string]interface{}{
		"end_time":         endTime,
		"duration_seconds": duration,
		"status":           TaskStatusFailed,
		"error_message":    errMsg,
	})
}

// maxCheckpointDataBytes 限制 checkpoint_data 写入长度。
// checkpoint_data 列为 TEXT（上限 65535 字节），而 result.Response 是完整 CLI 输出，
// 长度通常远超该值。若直接写入会触发 MySQL Error 1406: Data too long，导致任务状态
// 保存静默失败，最终表现为"任务完成但步骤 pending、无执行摘要"。
// RecoverTask 只读取该字段前 2000 字符用于断点恢复，因此按字节截断保留开头即可。
const maxCheckpointDataBytes = 60000

// truncateUTF8Bytes 按字节截断字符串，并保证不会切断多字节 UTF-8 字符。
// 若截断点落在字符中间，MySQL 会报 Error 1366: Incorrect string value，因此
// 逐字节回退直到结果为合法 UTF-8（最多回退 3 字节）。
func truncateUTF8Bytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func (e *TaskExecutor) saveTaskState(taskID int64, summary, checkpointData, stepStatusJSON string) {
	// 防止 checkpoint_data 超过 TEXT 列上限导致整条写入失败（见 maxCheckpointDataBytes）。
	checkpointData = truncateUTF8Bytes(checkpointData, maxCheckpointDataBytes)
	summary = truncateUTF8Bytes(summary, maxCheckpointDataBytes)

	var state model.TaskState
	updates := map[string]interface{}{
		"execution_summary": summary,
		"checkpoint_data":   checkpointData,
	}
	if stepStatusJSON != "" {
		updates["step_status_json"] = stepStatusJSON
	}
	if err := e.db.Where("task_id = ?", taskID).First(&state).Error; err != nil {
		state = model.TaskState{
			TaskID:           taskID,
			ExecutionSummary: summary,
			CheckpointData:   checkpointData,
			StepStatusJSON:   stepStatusJSON,
		}
		if err := e.db.Create(&state).Error; err != nil {
			// 写入失败会丢失执行摘要与步骤状态，最终表现为"完成但无摘要、步骤 pending"。
			// 这里显式记录错误，避免再次静默失败后难以定位。
			log.Printf("[task_state] 创建任务状态失败 task_id=%d: %v", taskID, err)
		}
	} else {
		if err := e.db.Model(&state).Updates(updates).Error; err != nil {
			log.Printf("[task_state] 更新任务状态失败 task_id=%d: %v", taskID, err)
		}
	}
}

// BuildCompletedStepStatusJSON 根据任务定义中的 stepsJson 生成所有步骤已完成的 JSON 状态。
// 兼容字符串数组 ["步骤1"] 和对象数组 [{"seq":1,"action":"步骤1"}] 两种格式。
func BuildCompletedStepStatusJSON(stepsJSON string) string {
	if stepsJSON == "" {
		return ""
	}
	var rawSteps []json.RawMessage
	if err := json.Unmarshal([]byte(stepsJSON), &rawSteps); err != nil {
		return ""
	}
	if len(rawSteps) == 0 {
		return ""
	}

	type stepStatus struct {
		Seq    int    `json:"seq"`
		Action string `json:"action"`
		Status string `json:"status"`
	}

	var statuses []stepStatus
	for i, raw := range rawSteps {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			statuses = append(statuses, stepStatus{Seq: i + 1, Action: s, Status: "completed"})
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		action := ""
		if v, ok := obj["action"].(string); ok {
			action = v
		} else if v, ok := obj["name"].(string); ok {
			action = v
		}
		seq := i + 1
		if v, ok := obj["seq"].(float64); ok {
			seq = int(v)
		}
		statuses = append(statuses, stepStatus{Seq: seq, Action: action, Status: "completed"})
	}

	if len(statuses) == 0 {
		return ""
	}
	b, _ := json.Marshal(statuses)
	return string(b)
}

func (e *TaskExecutor) buildExecutionPrompt(task *model.Task, context string) string {
	var sb strings.Builder
	sb.WriteString("你是一位资深开发者，正在执行软件项目中的特定任务。\n\n")
	sb.WriteString(context)
	sb.WriteString("\n# 待执行任务\n")
	sb.WriteString(fmt.Sprintf("- 名称: %s\n", task.Name))
	sb.WriteString(fmt.Sprintf("- 描述: %s\n", task.Description))
	sb.WriteString(fmt.Sprintf("- 类别: %s\n", task.Category))
	sb.WriteString(fmt.Sprintf("- 执行模式: %s\n", task.ExecutionMode))
	if task.StepsJSON != "" {
		sb.WriteString(fmt.Sprintf("- 步骤: %s\n", task.StepsJSON))
	}
	sb.WriteString("\n## 要求\n")
	sb.WriteString("1. 严格按照步骤执行。\n")
	sb.WriteString("2. 在项目工作目录中实现任务。\n")
	sb.WriteString("3. 编写简洁、文档完善的代码，遵循最佳实践。\n")
	sb.WriteString("4. 运行相关测试验证你的工作。\n")
	sb.WriteString("5. 如果遇到问题，调试并修复。\n")
	sb.WriteString("6. 在最后提供执行摘要，以「## 摘要」开头。\n")
	sb.WriteString("7. 所有输出内容必须使用中文。\n")
	sb.WriteString("8. **代码目录结构强制要求**：\n")
	sb.WriteString("   - 若项目为 Go + 前端（React/Vue 等）技术栈，所有后端代码必须放在工作目录下的 `backend-go/` 子目录中，所有前端代码必须放在 `frontend/` 子目录中。\n")
	sb.WriteString("   - 禁止在后端代码根目录（backend-go/）之外直接创建 Go 源文件；禁止在前端代码根目录（frontend/）之外直接创建前端源文件。\n")
	sb.WriteString("   - 如果 `backend-go/` 或 `frontend/` 目录尚不存在，请先创建它们，再把对应代码写入其中。\n")
	sb.WriteString("   - Makefile、README.md、go.mod 等全局配置文件可放在工作目录根目录，但业务代码必须按前后端子目录隔离。\n")
	sb.WriteString("9. **Git 提交推送强制要求**（最高优先级，必须严格遵守）：\n")
	sb.WriteString("   - 完成任务后必须执行 git add → git commit → git push，不能只提交不推送。\n")
	sb.WriteString("   - 推送时必须使用 `git push -u origin HEAD` 设置上游跟踪分支，禁止使用不带 `-u` 的 `git push origin HEAD`。\n")
	sb.WriteString("   - 如果远程仓库已有初始提交（如 Gitee AutoInit 的 README），首次推送前必须先 `git fetch origin` 再 `git pull origin master --allow-unrelated-histories` 合并远程初始提交。\n")
	sb.WriteString("   - 提交前必须执行 `git config user.email` 和 `git config user.name` 配置提交者信息。\n")
	return sb.String()
}

func (e *TaskExecutor) buildRecoveryPrompt(task *model.Task, state *model.TaskState) string {
	var sb strings.Builder
	sb.WriteString("你正在恢复一个被中断的任务。请从上次中断的位置继续执行。\n\n")
	sb.WriteString("# 任务: ")
	sb.WriteString(task.Name)
	sb.WriteString("\n描述: ")
	sb.WriteString(task.Description)
	sb.WriteString("\n\n")

	if state.ChecklistJSON != "" {
		sb.WriteString("## 检查清单进度\n")
		sb.WriteString(state.ChecklistJSON)
		sb.WriteString("\n\n")
	}
	if state.CurrentFocus != "" {
		sb.WriteString("## 当前焦点\n")
		sb.WriteString(state.CurrentFocus)
		sb.WriteString("\n\n")
	}
	if state.CheckpointData != "" {
		checkpoint := state.CheckpointData
		if len(checkpoint) > 2000 {
			checkpoint = checkpoint[:2000] + "\n...(已截断)"
		}
		sb.WriteString("## 上次检查点（之前的输出）\n")
		sb.WriteString(checkpoint)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## 要求\n")
	sb.WriteString("1. 检查上次检查点中的进度。\n")
	sb.WriteString("2. 确定已完成和剩余的工作。\n")
	sb.WriteString("3. 从上次检查点继续实现。\n")
	sb.WriteString("4. 编写简洁、文档完善的代码，遵循最佳实践。\n")
	sb.WriteString("5. 运行相关测试验证你的工作。\n")
	sb.WriteString("6. 在最后提供执行摘要，以「## 摘要」开头。\n")
	sb.WriteString("7. 所有输出内容必须使用中文。\n")
	return sb.String()
}
