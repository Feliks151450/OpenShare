package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

// trashLogDetail 把回收站路径写入操作日志详情（JSON），使"移到回收站"可人工追溯恢复。
func trashLogDetail(name string, trashPath string) string {
	if strings.TrimSpace(trashPath) == "" {
		return name
	}
	payload, err := json.Marshal(map[string]string{
		"name":       name,
		"trash_path": trashPath,
	})
	if err != nil {
		return name
	}
	return string(payload)
}

// DeleteFile 删除单个托管文件。
// 一致性编排：事务开启 → DB 删除（未提交）→ 磁盘删除 → 提交。
//   - 磁盘失败：回滚事务，DB 无变化，磁盘无变化；
//   - 提交失败（磁盘已删）：回收站分支把文件从回收站还原；永久删除分支已无补偿
//     手段（文件本体已销毁），仅返回错误——此时 DB 记录保留（可重新尝试删除），
//     磁盘孤儿可由重新扫描清理。
func (s *ResourceManagementService) DeleteFile(ctx context.Context, fileID string, operatorID string, operatorIP string, moveToTrash bool) error {
	current, err := s.repo.FindFileByID(ctx, strings.TrimSpace(fileID))
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}

	folder, err := s.repo.FindFolderByID(ctx, modelValue(current.FolderID))
	if err != nil {
		return err
	}

	filePath := ""
	if folder != nil && !folder.IsVirtual {
		filePath = model.BuildManagedFilePath(folderSourcePath(folder), current.Name)
		if filePath == "" {
			return ErrManagedFileNotFound
		}
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate resource delete log id: %w", err)
	}
	now := s.nowFunc()

	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin delete transaction: %w", tx.Error)
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	if err := repository.DeleteFileWithLogTx(tx, current.ID, operatorID, operatorIP, logID, current.Name, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFileNotFound
		}
		return fmt.Errorf("delete managed file: %w", err)
	}

	trashPath := ""
	if filePath != "" {
		if moveToTrash {
			trashPath, err = s.storage.MoveManagedFileToTrash(filePath)
			if err != nil {
				return fmt.Errorf("move managed file to trash: %w", err)
			}
		} else {
			if err := s.storage.RemoveManagedFilePermanently(filePath); err != nil {
				return fmt.Errorf("remove managed file: %w", err)
			}
		}
	}

	// 磁盘操作成功后提交；回收站路径补记进日志（先删旧日志再写，简单且低频）
	if err := tx.Commit().Error; err != nil {
		if moveToTrash && trashPath != "" {
			if _, restoreErr := s.storageMoveRestore(trashPath, filePath); restoreErr != nil {
				return fmt.Errorf("commit delete failed (%v); restore from trash failed: %w", err, restoreErr)
			}
		}
		return fmt.Errorf("commit delete: %w", err)
	}
	committed = true

	if trashPath != "" {
		s.appendTrashPathToLog(ctx, logID, trashLogDetail(current.Name, trashPath))
	}
	return nil
}

// DeleteFolder 删除整棵托管目录树。编排策略同 DeleteFile。
func (s *ResourceManagementService) DeleteFolder(ctx context.Context, folderID string, operatorID string, operatorIP string, moveToTrash bool) error {
	current, err := s.repo.FindFolderByID(ctx, strings.TrimSpace(folderID))
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFolderNotFound
	}

	folderIDs, err := s.repo.ListFolderTreeIDs(ctx, current.ID)
	if err != nil {
		return err
	}
	if len(folderIDs) == 0 {
		return ErrManagedFolderNotFound
	}

	srcPath := ""
	if current.SourcePath != nil {
		srcPath = strings.TrimSpace(*current.SourcePath)
	}

	now := s.nowFunc()
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate folder delete log id: %w", err)
	}

	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin delete transaction: %w", tx.Error)
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	if err := repository.DeleteFolderTreeWithLogTx(tx, current.ID, folderIDs, operatorID, operatorIP, logID, current.Name, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFolderNotFound
		}
		return fmt.Errorf("delete managed folder: %w", err)
	}

	trashPath := ""
	if srcPath != "" {
		if moveToTrash {
			trashPath, err = s.storage.MoveManagedDirectoryToTrash(srcPath)
			if err != nil {
				return fmt.Errorf("move managed folder to trash: %w", err)
			}
		} else {
			if err := s.storage.RemoveManagedDirectoryPermanently(srcPath); err != nil {
				return fmt.Errorf("remove managed folder: %w", err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		if moveToTrash && trashPath != "" {
			if restoreErr := s.storageRestoreDirectory(trashPath, srcPath); restoreErr != nil {
				return fmt.Errorf("commit delete failed (%v); restore from trash failed: %w", err, restoreErr)
			}
		}
		return fmt.Errorf("commit delete: %w", err)
	}
	committed = true

	if trashPath != "" {
		s.appendTrashPathToLog(ctx, logID, trashLogDetail(current.Name, trashPath))
	}
	return nil
}
