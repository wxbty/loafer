package service

import (
	"fmt"
	"sync"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// PortAllocator 端口分配服务。
//
// 从可配置的端口范围（默认 40410-40500）内为项目分配端口，
// 分配记录持久化到 port_allocation 表，支持按项目查询与释放。
// 同一端口在释放后可被再次分配（复用既有记录行，规避 port 唯一索引冲突）。
type PortAllocator struct {
	db  *gorm.DB
	cfg *config.InfraConfig

	// mu 保护“查找可用端口 + 写入记录”的临界区，避免并发分配到同一端口。
	mu sync.Mutex
}

// NewPortAllocator 构造端口分配服务。
func NewPortAllocator(db *gorm.DB, cfg *config.InfraConfig) *PortAllocator {
	return &PortAllocator{
		db:  db,
		cfg: cfg,
	}
}

// AllocatePort 为指定项目分配一个可用端口。
//
// portType 标识端口用途（如 frontend / backend），desc 为可读描述。
// 在配置的端口范围内查找第一个未被占用（无 allocated 状态记录）的端口并写入分配记录。
// 若该端口存在历史（已释放）记录，则就地更新复用，避免违反 port 唯一索引。
func (pa *PortAllocator) AllocatePort(projectID int64, portType string, desc string) (int, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	if pa.cfg.PortRangeStart <= 0 || pa.cfg.PortRangeEnd <= 0 || pa.cfg.PortRangeStart > pa.cfg.PortRangeEnd {
		return 0, fmt.Errorf("端口范围配置无效: start=%d end=%d", pa.cfg.PortRangeStart, pa.cfg.PortRangeEnd)
	}

	// 查询当前处于 allocated 状态的端口集合
	allocated, err := pa.GetAllocatedPorts()
	if err != nil {
		return 0, fmt.Errorf("查询已分配端口失败: %w", err)
	}

	used := make(map[int]struct{}, len(allocated))
	for _, p := range allocated {
		used[p] = struct{}{}
	}

	// 在范围内查找第一个可用端口
	var available int
	for p := pa.cfg.PortRangeStart; p <= pa.cfg.PortRangeEnd; p++ {
		if _, ok := used[p]; !ok {
			available = p
			break
		}
	}
	if available == 0 {
		return 0, fmt.Errorf("端口范围 %d-%d 已全部占用", pa.cfg.PortRangeStart, pa.cfg.PortRangeEnd)
	}

	now := time.Now()

	// 复用该端口的历史记录（已释放），避免违反 port 唯一索引
	var existing model.PortAllocation
	findErr := pa.db.Where("port = ?", available).First(&existing).Error
	if findErr == nil {
		// 就地更新：归属新项目、重置为 allocated、清空释放时间
		if err := pa.db.Model(&model.PortAllocation{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"project_id":   projectID,
			"port_type":    portType,
			"description":  desc,
			"status":       "allocated",
			"allocated_at": now,
			"released_at":  nil,
		}).Error; err != nil {
			return 0, fmt.Errorf("复用端口 %d 分配记录失败: %w", available, err)
		}
		return available, nil
	}
	if !isRecordNotFound(findErr) {
		return 0, fmt.Errorf("查询端口 %d 现有记录失败: %w", available, findErr)
	}

	// 无历史记录，新建分配记录；port 唯一索引作为并发安全兜底
	record := &model.PortAllocation{
		ProjectID:   projectID,
		Port:        available,
		PortType:    portType,
		Description: desc,
		Status:      "allocated",
	}
	if err := pa.db.Create(record).Error; err != nil {
		return 0, fmt.Errorf("写入端口分配记录失败(port=%d): %w", available, err)
	}

	return available, nil
}

// ReleasePort 释放指定端口，将其状态置为 released 并记录释放时间。
// 对未分配或已释放的端口调用是幂等的。
func (pa *PortAllocator) ReleasePort(port int) error {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	now := time.Now()
	result := pa.db.Model(&model.PortAllocation{}).
		Where("port = ? AND status = ?", port, "allocated").
		Updates(map[string]interface{}{
			"status":      "released",
			"released_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("释放端口 %d 失败: %w", port, result.Error)
	}
	// RowsAffected == 0 表示端口未分配或已释放，视为幂等成功
	return nil
}

// GetProjectPorts 查询指定项目的全部端口分配记录（含已释放），按端口号升序返回。
func (pa *PortAllocator) GetProjectPorts(projectID int64) ([]model.PortAllocation, error) {
	var list []model.PortAllocation
	if err := pa.db.Where("project_id = ?", projectID).Order("port asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询项目 %d 端口分配记录失败: %w", projectID, err)
	}
	return list, nil
}

// GetAllocatedPorts 返回当前处于 allocated 状态的所有端口号列表（升序）。
func (pa *PortAllocator) GetAllocatedPorts() ([]int, error) {
	var ports []int
	if err := pa.db.Model(&model.PortAllocation{}).
		Where("status = ?", "allocated").
		Order("port asc").
		Pluck("port", &ports).Error; err != nil {
		return nil, fmt.Errorf("查询已分配端口列表失败: %w", err)
	}
	return ports, nil
}

// PeekNextPorts 预览下两个可用端口（前端 + 后端），不实际写入分配记录。
// 用于项目创建前的上下文预览。若剩余端口不足 2 个，返回错误。
func (pa *PortAllocator) PeekNextPorts() (int, int) {
	allocated, err := pa.GetAllocatedPorts()
	if err != nil {
		// 查询失败时返回范围起始的两个端口作为兜底
		return pa.cfg.PortRangeStart, pa.cfg.PortRangeStart + 1
	}

	used := make(map[int]struct{}, len(allocated))
	for _, p := range allocated {
		used[p] = struct{}{}
	}

	var ports []int
	for p := pa.cfg.PortRangeStart; p <= pa.cfg.PortRangeEnd; p++ {
		if _, ok := used[p]; !ok {
			ports = append(ports, p)
			if len(ports) >= 2 {
				break
			}
		}
	}

	if len(ports) >= 2 {
		return ports[0], ports[1]
	}
	if len(ports) == 1 {
		return ports[0], ports[0] + 1
	}
	return pa.cfg.PortRangeStart, pa.cfg.PortRangeStart + 1
}

// isRecordNotFound 判断是否为 GORM 记录未找到错误。
func isRecordNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
