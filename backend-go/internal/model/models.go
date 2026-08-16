package model

import (
	"time"

	"gorm.io/gorm"
)

// 以下模型对应原 Java entity 包的 14 个实体，字段与 001-init-schema.sql 表结构一一对应。
// 逻辑删除：原 MyBatis-Plus @TableLogic 使用 deleted INT(0/1)，这里用 GORM softDelete:flag，
// 查询时自动过滤 deleted=0，删除时写入非零值，与现有数据语义兼容。
// created_at / updated_at 由 GORM 自动填充（字段名约定）。

// Project 项目表
type Project struct {
	ID          int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name        string         `gorm:"column:name;not null" json:"name"`
	Description string         `gorm:"column:description;type:text" json:"description"`
	Status      int            `gorm:"column:status;default:0" json:"status"`
	WorkDir     string         `gorm:"column:work_dir;type:varchar(500)" json:"workDir"`
	GitURL      string         `gorm:"column:git_url;type:varchar(255)" json:"gitUrl"`
	DevLanguage string         `gorm:"column:dev_language;type:varchar(100)" json:"devLanguage"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted     gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (Project) TableName() string { return "project" }

// Module 模块表
type Module struct {
	ID                  int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID           int64          `gorm:"column:project_id;not null;index" json:"projectId"`
	Name                string         `gorm:"column:name;not null" json:"name"`
	Description         string         `gorm:"column:description;type:text" json:"description"`
	SequenceNumber      string         `gorm:"column:sequence_number;type:varchar(50)" json:"sequenceNumber"`
	BlockedBy           string         `gorm:"column:blocked_by;type:varchar(500)" json:"blockedBy"`
	// ModuleType 模块类型：infrastructure（基础架构）/ business（业务）。
	// 基础架构模块仅做构建校验与启动验证；业务模块才有 API/Web/TDD 测试面板。
	ModuleType          string         `gorm:"column:module_type;type:varchar(20);default:business" json:"moduleType"`
	IntegrationTestSpec string         `gorm:"column:integration_test_spec;type:text" json:"integrationTestSpec"`
	APIIntegrationTest  string         `gorm:"column:api_integration_test;type:text" json:"apiIntegrationTest"`
	WebIntegrationTest  string         `gorm:"column:web_integration_test;type:text" json:"webIntegrationTest"`
	Status              int            `gorm:"column:status;default:0" json:"status"`
	PipelineMode        string         `gorm:"column:pipeline_mode;type:varchar(50)" json:"pipelineMode"`
	SimpleMode          int            `gorm:"column:simple_mode;default:0" json:"simpleMode"`
	TddStepStatusJSON   string         `gorm:"column:tdd_step_status_json;type:text" json:"tddStepStatusJson"`
	TddAssertionsJSON   string         `gorm:"column:tdd_assertions_json;type:text" json:"tddAssertionsJson"`
	TddTestSpecJSON     string         `gorm:"column:tdd_test_spec_json;type:text" json:"tddTestSpecJson"`
	CreatedAt           time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt           time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted             gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (Module) TableName() string { return "module" }

// Task 任务主表
type Task struct {
	ID            int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID     int64          `gorm:"column:project_id;not null;index" json:"projectId"`
	ModuleID      *int64         `gorm:"column:module_id;index" json:"moduleId"`
	Name          string         `gorm:"column:name;not null" json:"name"`
	Description   string         `gorm:"column:description;type:text" json:"description"`
	Status        int            `gorm:"column:status;default:0" json:"status"`
	SequenceNumber string        `gorm:"column:sequence_number;type:varchar(50)" json:"sequenceNumber"`
	StepsJSON     string         `gorm:"column:steps_json;type:text" json:"stepsJson"`
	Category      string         `gorm:"column:category;type:varchar(100)" json:"category"`
	CreatedAt     time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	BlockedBy     string         `gorm:"column:blocked_by;type:varchar(500)" json:"blockedBy"`
	Notes         string         `gorm:"column:notes;type:text" json:"notes"`
	ExecutionMode string         `gorm:"column:execution_mode;type:varchar(50)" json:"executionMode"`
	Deleted       gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (Task) TableName() string { return "task" }

// TaskState 任务实时状态表
type TaskState struct {
	ID                      int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID                  int64          `gorm:"column:task_id;not null;index" json:"taskId"`
	ChecklistJSON           string         `gorm:"column:checklist_json;type:text" json:"checklistJson"`
	CurrentFocus            string         `gorm:"column:current_focus;type:varchar(500)" json:"currentFocus"`
	CurrentStep             *int           `gorm:"column:current_step" json:"currentStep"`
	StepStatusJSON          string         `gorm:"column:step_status_json;type:text" json:"stepStatusJson"`
	RetryHistoryJSON        string         `gorm:"column:retry_history_json;type:text" json:"retryHistoryJson"`
	AssistanceRequestsJSON  string         `gorm:"column:assistance_requests_json;type:text" json:"assistanceRequestsJson"`
	CheckpointData          string         `gorm:"column:checkpoint_data;type:text" json:"checkpointData"`
	ExecutionSummary        string         `gorm:"column:execution_summary;type:text" json:"executionSummary"`
	PendingFixContext       string         `gorm:"column:pending_fix_context;type:text" json:"pendingFixContext"`
	Version                 int            `gorm:"column:version;default:0" json:"version"`
	UpdatedAt               time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted                 gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (TaskState) TableName() string { return "task_state" }

// ChecklistItem 子任务清单项表
type ChecklistItem struct {
	ID               int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID           int64          `gorm:"column:task_id;not null;index" json:"taskId"`
	ItemName         string         `gorm:"column:item_name;not null;type:varchar(500)" json:"itemName"`
	Status           int            `gorm:"column:status;default:0" json:"status"`
	Sequence         int            `gorm:"column:sequence;default:0" json:"sequence"`
	EstimatedDuration *int          `gorm:"column:estimated_duration" json:"estimatedDuration"`
	CompletedAt      *time.Time     `gorm:"column:completed_at" json:"completedAt"`
	CompletedBySlice *int           `gorm:"column:completed_by_slice" json:"completedBySlice"`
	CreatedAt        time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted          gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (ChecklistItem) TableName() string { return "checklist_item" }

// SliceHistory 分片执行历史表
type SliceHistory struct {
	ID              int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID          int64          `gorm:"column:task_id;not null;index" json:"taskId"`
	SliceSequence   *int           `gorm:"column:slice_sequence" json:"sliceSequence"`
	StartTime       *time.Time     `gorm:"column:start_time" json:"startTime"`
	EndTime         *time.Time     `gorm:"column:end_time" json:"endTime"`
	DurationSeconds *int           `gorm:"column:duration_seconds" json:"durationSeconds"`
	GitCommitID     string         `gorm:"column:git_commit_id;type:varchar(100)" json:"gitCommitId"`
	OutputSummary   string         `gorm:"column:output_summary;type:text" json:"outputSummary"`
	Status          int            `gorm:"column:status;default:0" json:"status"`
	ErrorMessage    string         `gorm:"column:error_message;type:text" json:"errorMessage"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted         gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (SliceHistory) TableName() string { return "slice_history" }

// DecisionLog 决策日志表
type DecisionLog struct {
	ID              int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID          int64          `gorm:"column:task_id;not null;index" json:"taskId"`
	ChecklistItemID *int64         `gorm:"column:checklist_item_id" json:"checklistItemId"`
	DecisionContent string         `gorm:"column:decision_content;type:text" json:"decisionContent"`
	DecisionReason  string         `gorm:"column:decision_reason;type:text" json:"decisionReason"`
	Status          int            `gorm:"column:status;default:0" json:"status"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted         gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (DecisionLog) TableName() string { return "decision_log" }

// DependencyGraph 依赖关系表
type DependencyGraph struct {
	ID               int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID           int64          `gorm:"column:task_id;not null;index" json:"taskId"`
	NodeType         string         `gorm:"column:node_type;type:varchar(50)" json:"nodeType"`
	NodeName         string         `gorm:"column:node_name;type:varchar(255)" json:"nodeName"`
	DependenciesJSON string         `gorm:"column:dependencies_json;type:text" json:"dependenciesJson"`
	CreatedAt        time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted          gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (DependencyGraph) TableName() string { return "dependency_graph" }

// EnvironmentState 环境状态表
type EnvironmentState struct {
	ID                 int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID             int64          `gorm:"column:task_id;not null;index" json:"taskId"`
	ConfigSnapshot     string         `gorm:"column:config_snapshot;type:text" json:"configSnapshot"`
	DependencyVersions string         `gorm:"column:dependency_versions;type:text" json:"dependencyVersions"`
	PathInfo           string         `gorm:"column:path_info;type:text" json:"pathInfo"`
	CreatedAt          time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted            gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (EnvironmentState) TableName() string { return "environment_state" }

// Checkpoint 检查点表
type Checkpoint struct {
	ID               int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID           int64          `gorm:"column:task_id;not null;index" json:"taskId"`
	CheckpointType   string         `gorm:"column:checkpoint_type;type:varchar(50)" json:"checkpointType"`
	GitCommitID      string         `gorm:"column:git_commit_id;type:varchar(100)" json:"gitCommitId"`
	ChecklistSnapshot string        `gorm:"column:checklist_snapshot;type:text" json:"checklistSnapshot"`
	CreatedAt        time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	IsRollbackTarget int            `gorm:"column:is_rollback_target;default:0" json:"isRollbackTarget"`
	Deleted          gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (Checkpoint) TableName() string { return "checkpoint" }

// PromptTemplate 提示词模板表
type PromptTemplate struct {
	ID              int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateKey     string         `gorm:"column:template_key;type:varchar(255);not null;uniqueIndex:uk_template_key" json:"templateKey"`
	TemplateName    string         `gorm:"column:template_name;not null" json:"templateName"`
	TemplateContent string         `gorm:"column:template_content;type:text;not null" json:"templateContent"`
	Description     string         `gorm:"column:description;type:text" json:"description"`
	VariablesJSON   string         `gorm:"column:variables_json;type:text" json:"variablesJson"`
	IsEnabled       int            `gorm:"column:is_enabled;default:1" json:"isEnabled"`
	IsSystem        int            `gorm:"column:is_system;default:0" json:"isSystem"`
	UseCount        int            `gorm:"column:use_count;default:0" json:"useCount"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted         gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (PromptTemplate) TableName() string { return "prompt_template" }

// SystemConfig 系统配置表
type SystemConfig struct {
	ID          int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ConfigKey   string         `gorm:"column:config_key;type:varchar(255);not null;uniqueIndex:uk_config_key" json:"configKey"`
	ConfigValue string         `gorm:"column:config_value;type:text" json:"configValue"`
	Description string         `gorm:"column:description;type:varchar(500)" json:"description"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted     gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (SystemConfig) TableName() string { return "system_config" }

// ModuleFixHistory 模块修复历史表（无逻辑删除字段）
type ModuleFixHistory struct {
	ID           int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ModuleID     int64      `gorm:"column:module_id;not null;index" json:"moduleId"`
	ProjectID    int64      `gorm:"column:project_id;not null;index" json:"projectId"`
	Scope        string     `gorm:"column:scope;type:varchar(100)" json:"scope"`
	CriteriaIDs  string     `gorm:"column:criteria_ids;type:text" json:"criteriaIds"`
	Status       string     `gorm:"column:status;type:varchar(50)" json:"status"`
	ErrorMessage string     `gorm:"column:error_message;type:text" json:"errorMessage"`
	StartedAt    *time.Time `gorm:"column:started_at" json:"startedAt"`
	CompletedAt  *time.Time `gorm:"column:completed_at" json:"completedAt"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	FixSummary   string     `gorm:"column:fix_summary;type:text" json:"fixSummary"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (ModuleFixHistory) TableName() string { return "module_fix_history" }

// TddRetryChain TDD 重试链表（无逻辑删除字段）
type TddRetryChain struct {
	ID                int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ModuleID          int64     `gorm:"column:module_id;not null;index" json:"moduleId"`
	TaskID            *int64    `gorm:"column:task_id;index" json:"taskId"`
	DependsOnTaskIDs  string    `gorm:"column:depends_on_task_ids;type:text" json:"dependsOnTaskIds"`
	FixContext        string    `gorm:"column:fix_context;type:text" json:"fixContext"`
	Status            string    `gorm:"column:status;type:varchar(50)" json:"status"`
	TriggerRound      string    `gorm:"column:trigger_round;type:varchar(50)" json:"triggerRound"`
	Note              string    `gorm:"column:note;type:text" json:"note"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (TddRetryChain) TableName() string { return "tdd_retry_chain" }

// LlmCallLog LLM 调用日志表，记录每次 Claude CLI 调用的原始输入输出。
// 参考 claude_sprint 的 LlmCallLogService.record()，用于问题排查和调用审计。
type LlmCallLog struct {
	ID         int64       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID  *int64      `gorm:"column:project_id;index" json:"projectId"`
	TaskID     *int64      `gorm:"column:task_id;index" json:"taskId"`
	CallType   string      `gorm:"column:call_type;type:varchar(100)" json:"callType"` // plan_generate, plan_refine, decompose, task_execute, task_recover, chat_extract
	Prompt     string      `gorm:"column:prompt;type:longtext" json:"prompt"`
	RawOutput  string      `gorm:"column:raw_output;type:longtext" json:"rawOutput"`
	ExitCode   int         `gorm:"column:exit_code;default:0" json:"exitCode"`
	Success    bool        `gorm:"column:success;default:false" json:"success"`
	WorkDir    string      `gorm:"column:work_dir;type:varchar(500)" json:"workDir"`
	DurationMs int64       `gorm:"column:duration_ms;default:0" json:"durationMs"`
	ErrorMsg   string      `gorm:"column:error_msg;type:text" json:"errorMsg"`
	CreatedAt  time.Time   `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
}

func (LlmCallLog) TableName() string { return "llm_call_log" }

// AllModels 返回所有需要迁移的模型，供 AutoMigrate 使用。
func AllModels() []interface{} {
	return []interface{}{
		&Project{},
		&Module{},
		&Task{},
		&TaskState{},
		&ChecklistItem{},
		&SliceHistory{},
		&DecisionLog{},
		&DependencyGraph{},
		&EnvironmentState{},
		&Checkpoint{},
		&PromptTemplate{},
		&SystemConfig{},
		&ModuleFixHistory{},
		&TddRetryChain{},
		// 部署运维相关模型
		&ProjectDeployment{},
		&ProjectDatabase{},
		&PortAllocation{},
		&SmsConfig{},
		&TestRun{},
		&ExecutionPlan{},
			&LlmCallLog{},
	}
}
