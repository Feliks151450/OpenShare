package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func AdjustFolderStatsTx(tx *gorm.DB, folderID *string, sizeDelta, downloadDelta, fileCountDelta int64) error {
	if tx == nil || folderID == nil {
		return nil
	}

	currentID := strings.TrimSpace(*folderID)
	if currentID == "" {
		return nil
	}
	if sizeDelta == 0 && downloadDelta == 0 && fileCountDelta == 0 {
		return nil
	}

	visited := make(map[string]struct{})
	for currentID != "" {
		if _, seen := visited[currentID]; seen {
			return fmt.Errorf("detected folder cycle while adjusting stats")
		}
		visited[currentID] = struct{}{}

		var folder Folder
		if err := tx.Model(&Folder{}).
			Select("id, parent_id").
			Where("id = ?", currentID).
			Take(&folder).Error; err != nil {
			return fmt.Errorf("load folder stats target: %w", err)
		}

		updates := map[string]any{}
		// MAX(0, ...) 钳制：负增量不得把统计扣成负数（历史漂移或 OR 优先级
		// 等异常路径造成的多扣在此兜底，启动时全量重建会进一步纠偏）
		if sizeDelta != 0 {
			updates["total_size"] = gorm.Expr("MAX(0, total_size + ?)", sizeDelta)
		}
		if downloadDelta != 0 {
			updates["download_count"] = gorm.Expr("MAX(0, download_count + ?)", downloadDelta)
		}
		if fileCountDelta != 0 {
			updates["file_count"] = gorm.Expr("MAX(0, file_count + ?)", fileCountDelta)
		}
		if len(updates) > 0 {
			if err := tx.Model(&Folder{}).
				Where("id = ?", currentID).
				UpdateColumns(updates).Error; err != nil {
				return fmt.Errorf("adjust folder stats: %w", err)
			}
		}

		if folder.ParentID == nil {
			break
		}
		currentID = strings.TrimSpace(*folder.ParentID)
	}

	return nil
}
