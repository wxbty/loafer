package model

import (
	"time"

	"gorm.io/gorm"
)

// ProjectDeployment 项目部署记录表
// 记录每个项目的部署信息：分配端口、Nginx配置路径、构建产物路径、后端端口等
type ProjectDeployment struct {
	ID              int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID       int64          `gorm:"column:project_id;not null;uniqueIndex:uk_project_deployment" json:"projectId"`
	FrontendPort    int            `gorm:"column:frontend_port" json:"frontendPort"`
	BackendPort     int            `gorm:"column:backend_port" json:"backendPort"`
	NginxConfigPath string         `gorm:"column:nginx_config_path;type:varchar(500)" json:"nginxConfigPath"`
	BuildDir        string         `gorm:"column:build_dir;type:varchar(500)" json:"buildDir"`
	BackendBinary   string         `gorm:"column:backend_binary;type:varchar(500)" json:"backendBinary"`
	BackendPID      int            `gorm:"column:backend_pid;default:0" json:"backendPid"`
	Status          string         `gorm:"column:status;type:varchar(50);default:'pending'" json:"status"`
	AccessURL       string         `gorm:"column:access_url;type:varchar(500)" json:"accessUrl"`
	DeployLog       string         `gorm:"column:deploy_log;type:longtext" json:"deployLog"`
	LastDeployedAt  *time.Time     `gorm:"column:last_deployed_at" json:"lastDeployedAt"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted         gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (ProjectDeployment) TableName() string { return "project_deployment" }

// ProjectDatabase 项目数据库供给记录表
// 记录为项目创建的独立数据库信息
type ProjectDatabase struct {
	ID           int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID    int64          `gorm:"column:project_id;not null;uniqueIndex:uk_project_database" json:"projectId"`
	DBName       string         `gorm:"column:db_name;type:varchar(100);not null" json:"dbName"`
	DBHost       string         `gorm:"column:db_host;type:varchar(255)" json:"dbHost"`
	DBPort       int            `gorm:"column:db_port" json:"dbPort"`
	DBUsername   string         `gorm:"column:db_username;type:varchar(100)" json:"dbUsername"`
	DBPassword   string         `gorm:"column:db_password;type:varchar(255)" json:"dbPassword"`
	Status       string         `gorm:"column:status;type:varchar(50);default:'pending'" json:"status"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted      gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (ProjectDatabase) TableName() string { return "project_database" }

// PortAllocation 端口分配记录表
// 记录端口段 40410-40500 的分配情况
type PortAllocation struct {
	ID           int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID    int64          `gorm:"column:project_id;not null;index" json:"projectId"`
	Port         int            `gorm:"column:port;not null;uniqueIndex:uk_port" json:"port"`
	PortType     string         `gorm:"column:port_type;type:varchar(50)" json:"portType"`
	Description  string         `gorm:"column:description;type:varchar(255)" json:"description"`
	AllocatedAt  time.Time      `gorm:"column:allocated_at;autoCreateTime" json:"allocatedAt"`
	ReleasedAt   *time.Time     `gorm:"column:released_at" json:"releasedAt"`
	Status       string         `gorm:"column:status;type:varchar(50);default:'allocated'" json:"status"`
	Deleted      gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (PortAllocation) TableName() string { return "port_allocation" }

// SmsConfig 短信服务配置表
type SmsConfig struct {
	ID           int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID    int64          `gorm:"column:project_id;index" json:"projectId"`
	AccessKey    string         `gorm:"column:access_key;type:varchar(255)" json:"accessKey"`
	SecretKey    string         `gorm:"column:secret_key;type:varchar(500)" json:"secretKey"`
	AccountID    string         `gorm:"column:account_id;type:varchar(100)" json:"accountId"`
	SignName     string         `gorm:"column:sign_name;type:varchar(200)" json:"signName"`
	TemplateID   string         `gorm:"column:template_id;type:varchar(100)" json:"templateId"`
	Enabled      int            `gorm:"column:enabled;default:0" json:"enabled"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted      gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (SmsConfig) TableName() string { return "sms_config" }

// TestRun 测试运行记录表
type TestRun struct {
	ID           int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID    int64          `gorm:"column:project_id;not null;index" json:"projectId"`
	ModuleID     *int64         `gorm:"column:module_id;index" json:"moduleId"`
	TaskID       *int64         `gorm:"column:task_id;index" json:"taskId"`
	TestType     string         `gorm:"column:test_type;type:varchar(50)" json:"testType"`
	Status       string         `gorm:"column:status;type:varchar(50);default:'pending'" json:"status"`
	Output       string         `gorm:"column:output;type:longtext" json:"output"`
	PassCount    int            `gorm:"column:pass_count;default:0" json:"passCount"`
	FailCount    int            `gorm:"column:fail_count;default:0" json:"failCount"`
	Duration     int            `gorm:"column:duration;default:0" json:"duration"`
	StartedAt    *time.Time     `gorm:"column:started_at" json:"startedAt"`
	CompletedAt  *time.Time     `gorm:"column:completed_at" json:"completedAt"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted      gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (TestRun) TableName() string { return "test_run" }

// ExecutionPlan 执行计划表
// 存储AI生成的执行计划，包含原始需求、生成的plan.md内容、状态等
type ExecutionPlan struct {
	ID              int64          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID       int64          `gorm:"column:project_id;not null;index" json:"projectId"`
	OriginalRequest string         `gorm:"column:original_request;type:longtext" json:"originalRequest"`
	PlanContent     string         `gorm:"column:plan_content;type:longtext" json:"planContent"`
	Status          string         `gorm:"column:status;type:varchar(50);default:'draft'" json:"status"`
	// draft → confirmed → decomposing → decomposed → executing → completed
	SessionID       string         `gorm:"column:session_id;type:varchar(255)" json:"sessionId"`
	ConfirmedAt     *time.Time     `gorm:"column:confirmed_at" json:"confirmedAt"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	Deleted         gorm.DeletedAt `gorm:"column:deleted;softDelete:flag" json:"-"`
}

func (ExecutionPlan) TableName() string { return "execution_plan" }

// DeploymentModels 返回部署相关的所有模型
func DeploymentModels() []interface{} {
	return []interface{}{
		&ProjectDeployment{},
		&ProjectDatabase{},
		&PortAllocation{},
		&SmsConfig{},
		&TestRun{},
		&ExecutionPlan{},
	}
}
