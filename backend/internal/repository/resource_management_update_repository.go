package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

func appendAllowDownloadUpdate(updates map[string]any, apply bool, allowPtr *bool) {
	if !apply {
		return
	}
	if allowPtr == nil {
		updates["allow_download"] = gorm.Expr("NULL")
	} else {
		updates["allow_download"] = *allowPtr
	}
}

// appendCustomPathUpdate 将 custom_path 写入更新 map。空字符串时写 NULL 避免 UNIQUE 约束冲突。
func appendCustomPathUpdate(updates map[string]any, customPath string) {
	if customPath == "" {
		updates["custom_path"] = gorm.Expr("NULL")
	} else {
		updates["custom_path"] = customPath
	}
}

func (r *ResourceManagementRepository) UpdateFileMetadata(
	ctx context.Context,
	fileID string,
	name string,
	extension string,
	description string,
	remark string,
	playbackURL string,
	playbackFallbackURL string,
	proxySourceURL string,
	coverURL *string,
	customPath string,
	applyAllowDownload bool,
	allowDownload *bool,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"name":                  name,
			"extension":             extension,
			"description":           description,
			"remark":                remark,
			"playback_url":          playbackURL,
			"playback_fallback_url": playbackFallbackURL,
		"proxy_source_url":      proxySourceURL,
			"updated_at":            now,
		}
		if coverURL != nil {
			updates["cover_url"] = *coverURL
		}
		appendCustomPathUpdate(updates, customPath)
		appendAllowDownloadUpdate(updates, applyAllowDownload, allowDownload)
		result := tx.Model(&model.File{}).Where("id = ?", fileID).Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("update file metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return createOperationLogTx(tx, logID, operatorID, "resource_updated", "file", fileID, name, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) UpdateFolderMetadata(
	ctx context.Context,
	folderID string,
	name string,
	description string,
	remark string,
	coverURL *string,
	directLinkPrefix string,
	customPath string,
	applyAllowDownload bool,
	allowDownload *bool,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"name":               name,
			"description":        description,
			"remark":             remark,
			"direct_link_prefix": directLinkPrefix,
			"updated_at":         now,
		}
		if coverURL != nil {
			updates["cover_url"] = *coverURL
		}
		appendCustomPathUpdate(updates, customPath)
		appendAllowDownloadUpdate(updates, applyAllowDownload, allowDownload)
		result := tx.Model(&model.Folder{}).Where("id = ?", folderID).Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("update folder metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return createOperationLogTx(tx, logID, operatorID, "folder_updated", "folder", folderID, name, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) UpdateRootFolderHidePublicCatalog(
	ctx context.Context,
	folderID string,
	hide bool,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Folder{}).
			Where("id = ? AND parent_id IS NULL", folderID).
			Updates(map[string]any{
				"hide_public_catalog": hide,
				"updated_at":          now,
			})
		if result.Error != nil {
			return fmt.Errorf("update root folder catalog visibility: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		detail := "show"
		if hide {
			detail = "hide"
		}
		return createOperationLogTx(tx, logID, operatorID, "folder_catalog_visibility", "folder", folderID, detail, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) UpdateFolderTreePaths(
	ctx context.Context,
	folderID string,
	name string,
	description string,
	remark string,
	coverURL *string,
	directLinkPrefix string,
	customPath string,
	applyAllowDownload bool,
	allowDownload *bool,
	folderSourcePaths map[string]string,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rootUpdates := map[string]any{
			"name":               name,
			"description":        description,
			"remark":             remark,
			"direct_link_prefix": directLinkPrefix,
			"updated_at":         now,
		}
		if coverURL != nil {
			rootUpdates["cover_url"] = *coverURL
		}
		appendCustomPathUpdate(rootUpdates, customPath)
		appendAllowDownloadUpdate(rootUpdates, applyAllowDownload, allowDownload)
		if sourcePath, ok := folderSourcePaths[folderID]; ok {
			rootUpdates["source_path"] = sourcePath
		}

		result := tx.Model(&model.Folder{}).Where("id = ?", folderID).Updates(rootUpdates)
		if result.Error != nil {
			return fmt.Errorf("update root folder metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		for id, sourcePath := range folderSourcePaths {
			if id == folderID {
				continue
			}
			if err := tx.Model(&model.Folder{}).Where("id = ?", id).Updates(map[string]any{
				"source_path": sourcePath,
				"updated_at":  now,
			}).Error; err != nil {
				return fmt.Errorf("update child folder path: %w", err)
			}
		}

		return createOperationLogTx(tx, logID, operatorID, "folder_updated", "folder", folderID, name, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) CreateFolder(ctx context.Context, folder *model.Folder, operatorID, operatorIP string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 空 custom_path 跳过插入，避免 UNIQUE 约束冲突
		createSQL := tx
		if folder.CustomPath == "" {
			createSQL = tx.Omit("custom_path")
		}
		if err := createSQL.Create(folder).Error; err != nil {
			return fmt.Errorf("create folder: %w", err)
		}
		logID, err := identity.NewID()
		if err != nil {
			return fmt.Errorf("generate log id: %w", err)
		}
		return createOperationLogTx(tx, logID, operatorID, "folder_created", "folder", folder.ID, folder.Name, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) CreateFile(ctx context.Context, file *model.File, operatorID string, operatorIP string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 空 custom_path 跳过插入，避免 UNIQUE 约束冲突
		createSQL := tx
		if file.CustomPath == "" {
			createSQL = tx.Omit("custom_path")
		}
		if err := createSQL.Create(file).Error; err != nil {
			return fmt.Errorf("create file: %w", err)
		}
		logID, err := identity.NewID()
		if err != nil {
			return fmt.Errorf("generate log id: %w", err)
		}
		return createOperationLogTx(tx, logID, operatorID, "file_created", "file", file.ID, file.Name, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) PatchFolderCdnUrl(ctx context.Context, folderID string, cdnURL string, operatorID string, operatorIP string, logID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Folder{}).Where("id = ?", folderID).Update("cdn_url", cdnURL)
		if result.Error != nil {
			return fmt.Errorf("patch folder cdn url: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("folder not found: %s", folderID)
		}
		comment := fmt.Sprintf("CDN URL updated to: %s", cdnURL)
		if cdnURL == "" {
			comment = "CDN URL removed"
		}
		if err := createOperationLogTx(tx, logID, operatorID, "folder_cdn_url_updated", "folder", folderID, comment, operatorIP, now); err != nil {
			return fmt.Errorf("log folder cdn url update: %w", err)
		}
		return nil
	})
}

// FileOrderEntry represents a file's custom sort order entry.
type FileOrderEntry struct {
	FileID    string `json:"file_id"`
	SortOrder int64  `json:"sort_order"`
}

// UpdateFolderFileOrder batch-updates the custom sort order for files within a folder.
func (r *ResourceManagementRepository) UpdateFolderFileOrder(ctx context.Context, folderID string, orders []FileOrderEntry, operatorID, operatorIP string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Reset all file orders in this folder
		if err := tx.Model(&model.File{}).
			Where("folder_id = ?", folderID).
			Update("sort_order", 0).Error; err != nil {
			return fmt.Errorf("reset file order: %w", err)
		}
		// Write new orders
		for _, entry := range orders {
			if err := tx.Model(&model.File{}).
				Where("id = ? AND folder_id = ?", entry.FileID, folderID).
				Update("sort_order", entry.SortOrder).Error; err != nil {
				return fmt.Errorf("update file sort_order: %w", err)
			}
		}
		logID, err := identity.NewID()
		if err != nil {
			return fmt.Errorf("generate log id: %w", err)
		}
		return createOperationLogTx(tx, logID, operatorID, "folder_file_order_updated", "folder", folderID, "file order updated", operatorIP, now)
	})
}

// UpdateFileSize updates a file's size and updated_at after content replacement.
func (r *ResourceManagementRepository) UpdateFileSize(ctx context.Context, fileID string, newSize int64, operatorID, operatorIP, logID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.File{}).Where("id = ?", fileID).Updates(map[string]any{
			"size":       newSize,
			"updated_at": now,
		})
		if result.Error != nil {
			return fmt.Errorf("update file size: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return createOperationLogTx(tx, logID, operatorID, "file_replaced", "file", fileID, "file content replaced", operatorIP, now)
	})
}

// MoveFileToFolder updates a file's folder_id and adjusts folder stats for both
// the source and target folder trees.
func (r *ResourceManagementRepository) MoveFileToFolder(
	ctx context.Context,
	fileID string,
	targetFolderID string,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var file model.File
		if err := tx.Select("id, folder_id, name, size, download_count").
			Where("id = ?", fileID).
			Take(&file).Error; err != nil {
			return err
		}

		if file.FolderID == nil || *file.FolderID == targetFolderID {
			return fmt.Errorf("file already in target folder")
		}

		// 检查目标文件夹下是否已有同名文件
		var conflictCount int64
		if err := tx.Model(&model.File{}).
			Where("folder_id = ? AND name = ? AND id != ?", targetFolderID, file.Name, fileID).
			Count(&conflictCount).Error; err != nil {
			return fmt.Errorf("check file name conflict in target folder: %w", err)
		}
		if conflictCount > 0 {
			return fmt.Errorf("target folder already contains a file named %q", file.Name)
		}

		srcFolderID := *file.FolderID

		// 更新文件的 folder_id
		if err := tx.Model(&model.File{}).Where("id = ?", fileID).
			Update("folder_id", targetFolderID).Error; err != nil {
			return fmt.Errorf("update file folder_id: %w", err)
		}

		// 调整源文件夹统计（减）
		if err := model.AdjustFolderStatsTx(tx, &srcFolderID, -file.Size, -file.DownloadCount, -1); err != nil {
			return fmt.Errorf("adjust source folder stats: %w", err)
		}

		// 调整目标文件夹统计（加）
		if err := model.AdjustFolderStatsTx(tx, &targetFolderID, file.Size, file.DownloadCount, 1); err != nil {
			return fmt.Errorf("adjust target folder stats: %w", err)
		}

		detail := fmt.Sprintf("moved %q from folder %s to folder %s", file.Name, srcFolderID, targetFolderID)
		return createOperationLogTx(tx, logID, operatorID, "file_moved", "file", fileID, detail, operatorIP, now)
	})
}
