package service

import (
	"context"
	"fmt"
	"strings"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

// AdminFolderItem is a lightweight folder entry for the admin folder picker.
type AdminFolderItem struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
}

// ListAllFolders returns all non-virtual folders without public catalog filtering.
func (s *ResourceManagementService) ListAllFolders(ctx context.Context) ([]AdminFolderItem, error) {
	rows, err := s.repo.ListAllFolders(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AdminFolderItem, len(rows))
	for i, row := range rows {
		items[i] = AdminFolderItem{
			ID:       row.ID,
			Name:     row.Name,
			ParentID: row.ParentID,
		}
	}
	return items, nil
}

// MoveFileInput is the input for moving a file to a different folder.
type MoveFileInput struct {
	FileID         string
	TargetFolderID string
	OperatorID     string
	OperatorIP     string
}

// MoveFile 移动文件到目标目录。
// 一致性编排：事务开启 → DB 移动（未提交）→ 磁盘移动 → 提交；
// 磁盘失败回滚事务；提交失败时把磁盘文件移回源目录。
// 物理文件移入虚拟目录时旧磁盘文件改入回收站（原为永久删除，DB 失败即丢数据）。
func (s *ResourceManagementService) MoveFile(ctx context.Context, input MoveFileInput) error {
	fileID := strings.TrimSpace(input.FileID)
	targetFolderID := strings.TrimSpace(input.TargetFolderID)
	if fileID == "" || targetFolderID == "" {
		return fmt.Errorf("file id and target folder id must not be empty")
	}

	file, err := s.repo.FindFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if file == nil {
		return ErrManagedFileNotFound
	}

	// 检查源文件夹存在
	srcFolder, err := s.repo.FindFolderByID(ctx, modelValue(file.FolderID))
	if err != nil {
		return err
	}
	if srcFolder == nil {
		return ErrManagedFolderNotFound
	}

	// 检查目标文件夹
	dstFolder, err := s.repo.FindFolderByID(ctx, targetFolderID)
	if err != nil {
		return err
	}
	if dstFolder == nil {
		return ErrManagedFolderNotFound
	}
	if dstFolder.ID == modelValue(file.FolderID) {
		return fmt.Errorf("file already in the target folder")
	}

	// 预解析磁盘路径（磁盘操作发生在事务窗口内）
	srcPath := ""
	if !srcFolder.IsVirtual {
		srcPath = model.BuildManagedFilePath(folderSourcePath(srcFolder), file.Name)
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate move log id: %w", err)
	}
	now := s.nowFunc()

	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin move transaction: %w", tx.Error)
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	if err := repository.MoveFileToFolderTx(tx, fileID, targetFolderID, input.OperatorID, input.OperatorIP, logID, now); err != nil {
		return fmt.Errorf("move file in database: %w", err)
	}

	trashPath := ""
	switch {
	case !srcFolder.IsVirtual && !dstFolder.IsVirtual:
		// 两个都是物理文件夹：移动磁盘文件
		if srcPath == "" {
			return fmt.Errorf("cannot resolve source file path")
		}
		if err := s.storage.MoveManagedFileToFolder(
			folderSourcePath(srcFolder),
			folderSourcePath(dstFolder),
			file.Name,
		); err != nil {
			return fmt.Errorf("move managed file on disk: %w", err)
		}
	case !srcFolder.IsVirtual && dstFolder.IsVirtual:
		// 物理文件移动到虚拟目录：旧磁盘文件移入回收站（保留恢复能力）
		if srcPath != "" {
			trashPath, err = s.storage.MoveManagedFileToTrash(srcPath)
			if err != nil {
				return fmt.Errorf("move source file to trash when moving to virtual folder: %w", err)
			}
		}
		// 虚拟→物理、虚拟→虚拟：仅 DB，无磁盘操作
	}

	if err := tx.Commit().Error; err != nil {
		// 提交失败：把磁盘状态还原
		if !srcFolder.IsVirtual && !dstFolder.IsVirtual && srcPath != "" {
			// 物理→物理：文件已在新目录，移回源目录
			_ = s.storage.MoveManagedFileToFolder(folderSourcePath(dstFolder), folderSourcePath(srcFolder), file.Name)
		}
		if trashPath != "" && srcPath != "" {
			if _, restoreErr := s.storageMoveRestore(trashPath, srcPath); restoreErr != nil {
				return fmt.Errorf("commit move failed (%v); restore from trash failed: %w", err, restoreErr)
			}
		}
		return fmt.Errorf("commit move: %w", err)
	}
	committed = true
	return nil
}
