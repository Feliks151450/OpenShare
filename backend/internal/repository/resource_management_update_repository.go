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

func (r *ResourceManagementRepository) UpdateFileMetadata(
	ctx context.Context,
	fileID string,
	name string,
	extension string,
	description string,
	remark string,
	playbackURL string,
	playbackFallbackURL string,
	coverURL string,
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
			"cover_url":             coverURL,
			"updated_at":            now,
		}
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
	coverURL string,
	directLinkPrefix string,
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
			"cover_url":          coverURL,
			"direct_link_prefix": directLinkPrefix,
			"updated_at":         now,
		}
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
	coverURL string,
	directLinkPrefix string,
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
			"cover_url":          coverURL,
			"direct_link_prefix": directLinkPrefix,
			"updated_at":         now,
		}
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
		if err := tx.Create(folder).Error; err != nil {
			return fmt.Errorf("create folder: %w", err)
		}
		logID, err := identity.NewID()
		if err != nil {
			return fmt.Errorf("generate log id: %w", err)
		}
		return createOperationLogTx(tx, logID, operatorID, "folder_created", "folder", folder.ID, folder.Name, operatorIP, now)
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
