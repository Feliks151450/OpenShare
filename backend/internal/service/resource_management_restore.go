package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/storage"
)

// storageMoveRestore 把回滚需要的文件从回收站移回原位。
// 目标路径的父目录若已被删除（如跨卷恢复），先重建目录。
func (s *ResourceManagementService) storageMoveRestore(trashPath, originalPath string) (string, error) {
	if dir := filepath.Dir(originalPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("restore original directory: %w", err)
		}
	}
	return originalPath, storage.MoveRegularFile(trashPath, originalPath)
}

// storageRestoreDirectory 把整棵目录树从回收站移回原位。
func (s *ResourceManagementService) storageRestoreDirectory(trashPath, originalPath string) error {
	if dir := filepath.Dir(originalPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("restore original directory: %w", err)
		}
	}
	return storage.MoveRegularFile(trashPath, originalPath)
}

// storageReplaceRestore 把替换前的备份还原回原文件（替换目标已存在时直接覆盖）。
func (s *ResourceManagementService) storageReplaceRestore(backupPath, originalPath string) (string, error) {
	if err := os.Remove(originalPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove replaced file before restore: %w", err)
	}
	return originalPath, storage.MoveRegularFile(backupPath, originalPath)
}

// appendTrashPathToLog 把回收站路径补写进已提交的删除日志。
// 失败仅打印日志，不影响删除结果（回收站文件本体仍在磁盘上）。
func (s *ResourceManagementService) appendTrashPathToLog(ctx context.Context, logID string, detail string) {
	updates := map[string]any{"detail": detail, "updated_at": time.Now().UTC()}
	if err := s.repo.DB().WithContext(ctx).Model(&model.OperationLog{}).
		Where("id = ?", logID).Updates(updates).Error; err != nil {
		fmt.Printf("[warn] record trash path into operation log %s failed: %v\n", logID, err)
	}
}
