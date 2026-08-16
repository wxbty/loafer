package service

import (
	"fmt"
	"os"
	"testing"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 回归测试（集成）：重新部署已有记录时 deployment 结构体只回填 ID，CreatedAt 为零值，
// saveDeployment 更新路径不得写 created_at，否则 MySQL 严格模式报 Error 1292（'0000-00-00'）。
// 需要真实 MySQL：设置 LOAFER_TEST_MYSQL_DSN 后运行，默认跳过。
func TestSaveDeployment_UpdateOmitsCreatedAt(t *testing.T) {
	dsn := os.Getenv("LOAFER_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未设置 LOAFER_TEST_MYSQL_DSN，跳过集成测试")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ProjectDeployment{}); err != nil {
		t.Fatal(err)
	}

	s := &DeployService{db: db}

	// 使用不存在的 project_id，测试结束后清理（Unscoped 硬删：唯一索引不含软删标记）
	const testProjectID = 999999001
	defer db.Unscoped().Where("project_id = ?", testProjectID).Delete(&model.ProjectDeployment{})
	db.Unscoped().Where("project_id = ?", testProjectID).Delete(&model.ProjectDeployment{})

	// 1. 首次写入（Create 路径）
	d := &model.ProjectDeployment{
		ProjectID: testProjectID,
		Status:    DeployStatusDeploying,
	}
	if err := s.saveDeployment(d); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if d.ID == 0 {
		t.Fatal("Create 后 ID 仍为 0")
	}
	createdAt := d.CreatedAt

	// 2. 模拟重新部署：新结构体只回填 ID，CreatedAt 为零值（原 bug 场景）
	redeploy := &model.ProjectDeployment{
		ID:        d.ID,
		ProjectID: testProjectID,
		Status:    DeployStatusFailed,
	}
	if err := s.saveDeployment(redeploy); err != nil {
		t.Fatalf("更新失败（原 bug: Error 1292 created_at '0000-00-00'）: %v", err)
	}

	// 3. 校验 created_at 未被覆盖，status 已更新
	var got model.ProjectDeployment
	if err := db.First(&got, d.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != DeployStatusFailed {
		t.Fatalf("status 未更新: %s", got.Status)
	}
	if got.CreatedAt.Sub(createdAt) > time.Second || createdAt.Sub(got.CreatedAt) > time.Second {
		t.Fatalf("created_at 被覆盖: 原 %v, 现 %v", createdAt, got.CreatedAt)
	}
}

// 回归测试（集成）：ProvisionDatabase 复用既有非 ready 记录时，
// 新结构体只回填 ID，更新路径不得写 created_at（同类 Error 1292）。
// 需要真实 MySQL：设置 LOAFER_TEST_MYSQL_DSN 后运行，默认跳过。
func TestProvisionDatabase_UpdateOmitsCreatedAt(t *testing.T) {
	dsn := os.Getenv("LOAFER_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未设置 LOAFER_TEST_MYSQL_DSN，跳过集成测试")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ProjectDatabase{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, Username: "root"}
	dp := NewDatabaseProvisioner(db, cfg)

	const testProjectID = 999999002
	dbName := fmt.Sprintf("loafer_proj_%d", testProjectID)
	defer db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
	defer db.Unscoped().Where("project_id = ?", testProjectID).Delete(&model.ProjectDatabase{})
	db.Unscoped().Where("project_id = ?", testProjectID).Delete(&model.ProjectDatabase{})

	// 预置一条非 ready 记录（触发更新路径）
	pre := &model.ProjectDatabase{ProjectID: testProjectID, DBName: dbName, Status: "pending"}
	if err := db.Create(pre).Error; err != nil {
		t.Fatal(err)
	}
	createdAt := pre.CreatedAt

	// 原 bug：更新路径 Save 全量写 created_at='0000-00-00' → Error 1292
	got, err := dp.ProvisionDatabase(testProjectID)
	if err != nil {
		t.Fatalf("ProvisionDatabase 失败（原 bug: Error 1292 created_at '0000-00-00'）: %v", err)
	}
	if got.ID != pre.ID {
		t.Fatalf("未复用既有记录: 原 ID %d, 现 %d", pre.ID, got.ID)
	}
	if got.Status != "ready" {
		t.Fatalf("status 未更新为 ready: %s", got.Status)
	}

	var inDB model.ProjectDatabase
	if err := db.First(&inDB, pre.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inDB.CreatedAt.Sub(createdAt) > time.Second || createdAt.Sub(inDB.CreatedAt) > time.Second {
		t.Fatalf("created_at 被覆盖: 原 %v, 现 %v", createdAt, inDB.CreatedAt)
	}
}
