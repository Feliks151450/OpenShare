package repository

import (
	"errors"

	"gorm.io/gorm"
)

type ImportRepository struct {
	db *gorm.DB
}

var ErrManagedRootRequired = errors.New("managed root folder required")

func NewImportRepository(db *gorm.DB) *ImportRepository {
	return &ImportRepository{db: db}
}

// GetDB 暴露底层 gorm.DB 句柄（用于自定义轻量查询场景）。
func (r *ImportRepository) GetDB() *gorm.DB {
	return r.db
}

