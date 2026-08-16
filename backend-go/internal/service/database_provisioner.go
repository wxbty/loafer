package service

import (
	"errors"
	"fmt"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// DatabaseProvisioner 数据库供给服务。
//
// 为每个项目在主 MySQL 实例上创建独立的数据库，数据库名形如 loafer_proj_<projectID>，
// 复用主库连接执行 CREATE / DROP DATABASE 原生 SQL，并将凭据信息持久化到
// project_database 表。各项目使用同一 MySQL 主机/端口，但拥有独立的数据库名。
type DatabaseProvisioner struct {
	db  *gorm.DB
	cfg *config.DatabaseConfig
}

// NewDatabaseProvisioner 构造数据库供给服务。
func NewDatabaseProvisioner(db *gorm.DB, cfg *config.DatabaseConfig) *DatabaseProvisioner {
	return &DatabaseProvisioner{
		db:  db,
		cfg: cfg,
	}
}

// ProvisionDatabase 为项目创建独立数据库并写入（或更新）ProjectDatabase 记录。
//
// 若项目已有 status=ready 的记录则直接返回（幂等）；否则执行
// CREATE DATABASE IF NOT EXISTS，并复用主库凭据记录连接信息。
func (dp *DatabaseProvisioner) ProvisionDatabase(projectID int64) (*model.ProjectDatabase, error) {
	// 幂等：已存在且就绪则直接返回
	existing, err := dp.GetProjectDatabase(projectID)
	if err != nil {
		return nil, fmt.Errorf("查询项目 %d 数据库记录失败: %w", projectID, err)
	}
	if existing != nil && existing.Status == "ready" {
		return existing, nil
	}

	dbName := fmt.Sprintf("loafer_proj_%d", projectID)

	// 创建数据库（反引号包裹名称，统一使用 utf8mb4 字符集）
	createSQL := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		dbName,
	)
	if err := dp.db.Exec(createSQL).Error; err != nil {
		return nil, fmt.Errorf("创建数据库 %s 失败: %w", dbName, err)
	}

	// 复用主库连接凭据（同一 MySQL 实例，主库用户对各项目库具备权限）
	record := &model.ProjectDatabase{
		ProjectID:  projectID,
		DBName:     dbName,
		DBHost:     dp.cfg.Host,
		DBPort:     dp.cfg.Port,
		DBUsername: dp.cfg.Username,
		DBPassword: dp.cfg.Password,
		Status:     "ready",
	}

	if existing != nil {
		// 复用既有记录行，全量更新（不覆盖 created_at：本结构体为新建，CreatedAt 为零值，
		// 全量写入会触发 MySQL 严格模式 Error 1292）
		record.ID = existing.ID
		if err := dp.db.Model(record).Select("*").Omit("created_at").Updates(record).Error; err != nil {
			return nil, fmt.Errorf("更新项目数据库记录失败: %w", err)
		}
	} else {
		if err := dp.db.Create(record).Error; err != nil {
			return nil, fmt.Errorf("写入项目数据库记录失败: %w", err)
		}
	}

	return record, nil
}

// DropDatabase 删除项目的独立数据库（DROP DATABASE IF EXISTS），
// 并将对应记录状态更新为 dropped。无记录时仅执行 SQL，幂等。
func (dp *DatabaseProvisioner) DropDatabase(projectID int64) error {
	dbName := fmt.Sprintf("loafer_proj_%d", projectID)

	dropSQL := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)
	if err := dp.db.Exec(dropSQL).Error; err != nil {
		return fmt.Errorf("删除数据库 %s 失败: %w", dbName, err)
	}

	// 更新记录状态为 dropped
	record, err := dp.GetProjectDatabase(projectID)
	if err != nil {
		return fmt.Errorf("查询项目 %d 数据库记录失败: %w", projectID, err)
	}
	if record != nil {
		if err := dp.db.Model(&model.ProjectDatabase{}).
			Where("id = ?", record.ID).
			Update("status", "dropped").Error; err != nil {
			return fmt.Errorf("更新数据库记录状态失败: %w", err)
		}
	}

	return nil
}

// GetProjectDatabase 查询指定项目的数据库供给记录。
// 不存在时返回 (nil, nil)，便于调用方区分“无记录”与“查询出错”。
func (dp *DatabaseProvisioner) GetProjectDatabase(projectID int64) (*model.ProjectDatabase, error) {
	var record model.ProjectDatabase
	err := dp.db.Where("project_id = ?", projectID).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}
