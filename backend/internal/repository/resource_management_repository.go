package repository

import (
	"gorm.io/gorm"
)

type ResourceManagementRepository struct {
	db *gorm.DB
}

func NewResourceManagementRepository(db *gorm.DB) *ResourceManagementRepository {
	return &ResourceManagementRepository{db: db}
}

// DB 暴露底层 *gorm.DB，供 service 层编排"事务开启 → 磁盘操作 → 提交"的一致性窗口。
func (r *ResourceManagementRepository) DB() *gorm.DB {
	return r.db
}
