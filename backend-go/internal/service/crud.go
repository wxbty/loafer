package service

import (
	"gorm.io/gorm"
)

// CrudService 是泛型 CRUD 服务，对应 MyBatis-Plus 的 ServiceImpl<T>，
// 提供 getById / list / page / save / update / removeById 等基础能力。
// 各实体 Service 嵌入本结构体即可复用，并按需追加自定义查询方法。
type CrudService[T any] struct {
	DB *gorm.DB
}

// NewCrudService 构造泛型 CRUD 服务。
func NewCrudService[T any](db *gorm.DB) *CrudService[T] {
	return &CrudService[T]{DB: db}
}

// GetByID 按 ID 查询单条（对应 serviceImpl.getById）。
func (s *CrudService[T]) GetByID(id int64) (*T, error) {
	var entity T
	if err := s.DB.First(&entity, id).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// List 查询列表，query 回调可追加 where/order 等条件（对应 serviceImpl.list(wrapper)）。
func (s *CrudService[T]) List(query func(*gorm.DB) *gorm.DB) ([]T, error) {
	var list []T
	tx := s.DB
	if query != nil {
		tx = query(tx)
	}
	if err := tx.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Page 分页查询，返回记录列表与总数（对应 serviceImpl.page(page, wrapper)）。
func (s *CrudService[T]) Page(page, size int64, query func(*gorm.DB) *gorm.DB) ([]T, int64, error) {
	var list []T
	var total int64
	tx := s.DB.Model(new(T))
	if query != nil {
		tx = query(tx)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if err := tx.Offset(int((page - 1) * size)).Limit(int(size)).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Save 新增（对应 serviceImpl.save）。entity 须为指针以便回填自增 ID。
func (s *CrudService[T]) Save(entity *T) error {
	return s.DB.Create(entity).Error
}

// Update 按主键全量更新（对应 serviceImpl.updateById 的整体保存语义）。
func (s *CrudService[T]) Update(entity *T) error {
	return s.DB.Save(entity).Error
}

// UpdateByID 按指定 ID 更新非零字段（对应 MyBatis-Plus updateById 仅更新非 null 字段）。
func (s *CrudService[T]) UpdateByID(id int64, entity *T) error {
	return s.DB.Model(new(T)).Where("id = ?", id).Updates(entity).Error
}

// UpdateColumns 按主键更新非零字段（对应 updateById 的部分更新语义）。
func (s *CrudService[T]) UpdateColumns(id int64, entity map[string]interface{}) error {
	return s.DB.Model(new(T)).Where("id = ?", id).Updates(entity).Error
}

// Delete 软删除（对应 serviceImpl.removeById，GORM softDelete 自动置 deleted）。
func (s *CrudService[T]) Delete(id int64) error {
	return s.DB.Delete(new(T), id).Error
}
