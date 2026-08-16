package db

import (
	"fmt"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Init 建立数据库连接并执行表结构迁移（对应原 Liquibase 启动时自动建表）。
func Init(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	gormLogLevel := logger.Warn
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger:                 logger.Default.LogMode(gormLogLevel),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层 sql.DB 失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Minute)
	sqlDB.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTime) * time.Minute)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库 Ping 失败: %w", err)
	}

	// 自动迁移（仅创建缺失的表/列/索引，不删除已有结构，等价于 Liquibase 的 CREATE TABLE IF NOT EXISTS）
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	return db, nil
}
