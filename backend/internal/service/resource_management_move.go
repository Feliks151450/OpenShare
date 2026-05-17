package service

import (
	"context"
	"fmt"
	"strings"

	"openshare/backend/internal/model"
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

	// 虚拟目录 / 非虚拟目录处理
	if !srcFolder.IsVirtual && !dstFolder.IsVirtual {
		// 两个都是物理文件夹：移动磁盘文件
		srcPath := model.BuildManagedFilePath(folderSourcePath(srcFolder), file.Name)
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
	} else if srcFolder.IsVirtual && !dstFolder.IsVirtual {
		// 虚拟文件移动到物理文件夹：仅更新 DB，无磁盘操作
	} else if !srcFolder.IsVirtual && dstFolder.IsVirtual {
		// 物理文件移动到虚拟目录：删除磁盘文件，仅保留 DB 记录
		srcPath := model.BuildManagedFilePath(folderSourcePath(srcFolder), file.Name)
		if srcPath != "" {
			if err := s.storage.RemoveManagedFilePermanently(srcPath); err != nil {
				return fmt.Errorf("remove source file when moving to virtual folder: %w", err)
			}
		}
	}
	// 两个都是虚拟目录：仅更新 DB

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate move log id: %w", err)
	}
	now := s.nowFunc()
	if err := s.repo.MoveFileToFolder(ctx, fileID, targetFolderID, input.OperatorID, input.OperatorIP, logID, now); err != nil {
		return fmt.Errorf("move file in database: %w", err)
	}
	return nil
}
