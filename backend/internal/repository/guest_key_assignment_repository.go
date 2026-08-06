package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type GuestKeyAssignmentRepository struct {
	db *gorm.DB
}

func NewGuestKeyAssignmentRepository(db *gorm.DB) *GuestKeyAssignmentRepository {
	return &GuestKeyAssignmentRepository{db: db}
}

// ListFolderIDsByKeyID 列出允许使用给定密钥的所有目录 ID。
func (r *GuestKeyAssignmentRepository) ListFolderIDsByKeyID(ctx context.Context, keyID string) ([]string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).
		Model(&model.FolderGuestKeyAssignment{}).
		Where("key_id = ?", keyID).
		Pluck("folder_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list folder ids by key: %w", err)
	}
	return ids, nil
}

// ListAllowedKeyIDsByFolderID 列出该目录允许的密钥 ID。
func (r *GuestKeyAssignmentRepository) ListAllowedKeyIDsByFolderID(ctx context.Context, folderID string) ([]string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).
		Model(&model.FolderGuestKeyAssignment{}).
		Where("folder_id = ?", folderID).
		Pluck("key_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list allowed key ids by folder: %w", err)
	}
	return ids, nil
}

// ListAllowedKeyIDsByFolderIDs 批量获取多个目录允许的密钥 ID，map[folderID] -> []keyID。
func (r *GuestKeyAssignmentRepository) ListAllowedKeyIDsByFolderIDs(ctx context.Context, folderIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(folderIDs))
	if len(folderIDs) == 0 {
		return out, nil
	}
	var rows []model.FolderGuestKeyAssignment
	if err := r.db.WithContext(ctx).
		Where("folder_id IN ?", folderIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list allowed key ids by folder ids: %w", err)
	}
	for _, row := range rows {
		out[row.FolderID] = append(out[row.FolderID], row.KeyID)
	}
	return out, nil
}

// ReplaceAssignments 在事务中替换目录允许的密钥：required=false 时清空；required=true 时 upsert 完整列表。
func (r *GuestKeyAssignmentRepository) ReplaceAssignments(ctx context.Context, folderID string, keyIDs []string) error {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return fmt.Errorf("empty folder id")
	}
	cleaned := dedupeNonEmpty(keyIDs)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("folder_id = ?", folderID).Delete(&model.FolderGuestKeyAssignment{}).Error; err != nil {
			return fmt.Errorf("clear folder guest keys: %w", err)
		}
		for _, kid := range cleaned {
			row := model.FolderGuestKeyAssignment{FolderID: folderID, KeyID: kid}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("insert folder guest key: %w", err)
			}
		}
		return nil
	})
}

// RawAncestorHasGuestKey 从指定目录向上递归祖先链，查询是否任一节点设置了 guest_key_required=true。
// sqlCte 形如 "WITH RECURSIVE ancestors(...) AS (...)"，由调用方提供以便复用。
func (r *GuestKeyAssignmentRepository) RawAncestorHasGuestKey(ctx context.Context, folderID, sqlCte string, out *bool) error {
	q := sqlCte + ` SELECT EXISTS(SELECT 1 FROM ancestors WHERE required != 0) AS any_required`
	row := r.db.WithContext(ctx).Raw(q, folderID).Row()
	if row == nil {
		return fmt.Errorf("ancestor query returned no row")
	}
	return row.Scan(out)
}
