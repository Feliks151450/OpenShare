package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

func (s *ResourceManagementService) UpdateFolderDescription(ctx context.Context, folderID string, input UpdateManagedFolderDescriptionInput) error {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return ErrManagedFolderNotFound
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ErrInvalidResourceEdit
	}

	current, err := s.repo.FindFolderByID(ctx, folderID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFolderNotFound
	}

	if strings.TrimSpace(current.Name) != name {
		folderConflict, err := s.repo.FolderNameExists(ctx, current.ParentID, name, current.ID)
		if err != nil {
			return err
		}
		fileConflict, err := s.repo.FileNameExists(ctx, current.ParentID, name, "")
		if err != nil {
			return err
		}
		if folderConflict || fileConflict {
			return ErrManagedFolderConflict
		}
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate folder update log id: %w", err)
	}

	description := strings.TrimSpace(input.Description)
	remark := normalizeManagedRemark(input.Remark)
	coverURL, err := normalizeOptionalCoverURL(input.CoverURL)
	if err != nil {
		return ErrInvalidResourceEdit
	}
	prefix, err := normalizeOptionalHTTPURL(input.DirectLinkPrefix)
	if err != nil {
		return ErrInvalidResourceEdit
	}

	applyDl, allowDl, err := parseDownloadPolicy(input.DownloadPolicy)
	if err != nil {
		return ErrInvalidResourceEdit
	}

	if current.SourcePath == nil || strings.TrimSpace(*current.SourcePath) == "" || current.Name == name {
		if err := s.repo.UpdateFolderMetadata(ctx, folderID, name, description, remark, coverURL, prefix, applyDl, allowDl, input.OperatorID, input.OperatorIP, logID, s.nowFunc()); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrManagedFolderNotFound
			}
			return fmt.Errorf("update folder metadata: %w", err)
		}
		return nil
	}

	folders, err := s.repo.ListFolderPaths(ctx)
	if err != nil {
		return fmt.Errorf("list folder paths: %w", err)
	}

	oldRootPath := strings.TrimSpace(*current.SourcePath)
	newRootPath, err := s.storage.RenameManagedDirectory(oldRootPath, name)
	if err != nil {
		if errors.Is(err, storage.ErrManagedDirectoryConflict) {
			return ErrManagedFolderConflict
		}
		return fmt.Errorf("rename managed directory: %w", err)
	}

	folderSourcePaths := map[string]string{
		current.ID: newRootPath,
	}
	for _, folder := range folders {
		if folder.SourcePath == nil {
			continue
		}
		sourcePath := strings.TrimSpace(*folder.SourcePath)
		if sourcePath == "" || sourcePath == oldRootPath || !isPathWithinRoot(sourcePath, oldRootPath) {
			continue
		}
		relative, relErr := filepath.Rel(oldRootPath, sourcePath)
		if relErr != nil {
			return fmt.Errorf("resolve folder relative path: %w", relErr)
		}
		folderSourcePaths[folder.ID] = filepath.Join(newRootPath, relative)
	}

	if err := s.repo.UpdateFolderTreePaths(ctx, folderID, name, description, remark, coverURL, prefix, applyDl, allowDl, folderSourcePaths, input.OperatorID, input.OperatorIP, logID, s.nowFunc()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFolderNotFound
		}
		return fmt.Errorf("update folder tree paths: %w", err)
	}
	return nil
}

func normalizePathPointer(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func isPathWithinRoot(path, root string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	root = filepath.Clean(strings.TrimSpace(root))
	if path == "" || root == "" {
		return false
	}
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

type CreateFolderInput struct {
	Name       string `json:"name"`
	ParentID   string `json:"parent_id"`
	OperatorID string
	OperatorIP string
}

func (s *ResourceManagementService) CreateFolder(ctx context.Context, input CreateFolderInput) (*model.Folder, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrInvalidResourceEdit
	}
	parentID := strings.TrimSpace(input.ParentID)

	// Validate parent exists and resolve managed root
	var parentIDPtr *string
	var folderSourcePath *string
	if parentID != "" {
		parent, err := s.repo.FindFolderByID(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("find parent folder: %w", err)
		}
		if parent == nil {
			return nil, ErrManagedFolderNotFound
		}
		parentIDPtr = &parent.ID

		// Resolve the managed root source path by walking up the tree
		folderSourcePath = resolveNewFolderSourcePath(parent, name)
		if folderSourcePath != nil {
			diskPath := strings.TrimSpace(*folderSourcePath)
			if err := s.storage.EnsureManagedDirectory(diskPath); err != nil {
				return nil, fmt.Errorf("create directory on disk: %w", err)
			}
		}
	}

	// Check name conflict
	conflict, err := s.repo.FolderNameExists(ctx, parentIDPtr, name, "")
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, ErrManagedFolderConflict
	}
	fileConflict, err := s.repo.FileNameExists(ctx, parentIDPtr, name, "")
	if err != nil {
		return nil, err
	}
	if fileConflict {
		return nil, ErrManagedFolderConflict
	}

	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate folder id: %w", err)
	}

	now := s.nowFunc()
	folder := &model.Folder{
		ID:         id,
		ParentID:   parentIDPtr,
		Name:       name,
		SourcePath: folderSourcePath,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.repo.CreateFolder(ctx, folder, input.OperatorID, input.OperatorIP, now); err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	return folder, nil
}

// resolveNewFolderSourcePath builds the on-disk path for a new folder named childName
// under the given parent. Returns nil if the parent is not part of a managed tree.
func resolveNewFolderSourcePath(parent *model.Folder, childName string) *string {
	if parent == nil {
		return nil
	}
	if parent.SourcePath == nil || strings.TrimSpace(*parent.SourcePath) == "" {
		return nil
	}
	rootPath := filepath.Clean(strings.TrimSpace(*parent.SourcePath))
	result := filepath.Join(rootPath, childName)
	return &result
}

// CreateVirtualFolder 创建虚拟目录（无物理磁盘路径，仅存数据库，子文件通过 CDN 直链提供）。
func (s *ResourceManagementService) CreateVirtualFolder(ctx context.Context, input CreateFolderInput) (*model.Folder, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrInvalidResourceEdit
	}
	parentID := strings.TrimSpace(input.ParentID)

	var parentIDPtr *string
	if parentID != "" {
		parent, err := s.repo.FindFolderByID(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("find parent folder: %w", err)
		}
		if parent == nil {
			return nil, ErrManagedFolderNotFound
		}
		parentIDPtr = &parent.ID
	}

	conflict, err := s.repo.FolderNameExists(ctx, parentIDPtr, name, "")
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, ErrManagedFolderConflict
	}
	fileConflict, err := s.repo.FileNameExists(ctx, parentIDPtr, name, "")
	if err != nil {
		return nil, err
	}
	if fileConflict {
		return nil, ErrManagedFolderConflict
	}

	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate folder id: %w", err)
	}

	now := s.nowFunc()
	folder := &model.Folder{
		ID:         id,
		ParentID:   parentIDPtr,
		Name:       name,
		IsVirtual:  true,
		SourcePath: nil,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.repo.CreateFolder(ctx, folder, input.OperatorID, input.OperatorIP, now); err != nil {
		return nil, fmt.Errorf("create virtual folder: %w", err)
	}
	return folder, nil
}

// CreateVirtualFileInput 创建虚拟文件入参（仅用于虚拟目录下的文件）。
type CreateVirtualFileInput struct {
	Name        string
	FolderID    string
	PlaybackURL string
	Description string
	Remark      string
	OperatorID  string
	OperatorIP  string
}

// CreateVirtualFile 在虚拟目录下创建虚拟文件（通过 CDN 直链提供下载）。
func (s *ResourceManagementService) CreateVirtualFile(ctx context.Context, input CreateVirtualFileInput) (*model.File, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.FolderID = strings.TrimSpace(input.FolderID)
	input.PlaybackURL = strings.TrimSpace(input.PlaybackURL)

	if input.Name == "" || input.FolderID == "" {
		return nil, ErrInvalidResourceEdit
	}

	// 校验 CDN 直链
	candidate, err := normalizeOptionalHTTPURL(input.PlaybackURL)
	if err != nil || candidate == "" {
		return nil, ErrInvalidResourceEdit
	}

	// 确认父文件夹存在且为虚拟目录
	folder, err := s.repo.FindFolderByID(ctx, input.FolderID)
	if err != nil {
		return nil, fmt.Errorf("find parent folder: %w", err)
	}
	if folder == nil {
		return nil, ErrManagedFolderNotFound
	}
	if !folder.IsVirtual {
		return nil, ErrInvalidResourceEdit
	}

	// 检查文件名冲突
	conflict, err := s.repo.FileNameExists(ctx, &folder.ID, input.Name, "")
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, ErrManagedFileConflict
	}

	// HEAD 请求获取文件大小
	fileSize := int64(0)
	client := &http.Client{Timeout: 10 * time.Second}
	headResp, err := client.Head(input.PlaybackURL)
	if err == nil && headResp != nil {
		headResp.Body.Close()
		if headResp.ContentLength > 0 {
			fileSize = headResp.ContentLength
		}
	}

	// 解析文件名和扩展名
	name, extension, ok := model.NormalizeManagedFileName(input.Name)
	if !ok {
		return nil, ErrInvalidResourceEdit
	}

	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate file id: %w", err)
	}

	now := s.nowFunc()
	file := &model.File{
		ID:          id,
		FolderID:    &folder.ID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Remark:      normalizeManagedRemark(input.Remark),
		Extension:   extension,
		PlaybackURL: candidate,
		Size:        fileSize,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateFile(ctx, file, input.OperatorID, input.OperatorIP, now); err != nil {
		return nil, fmt.Errorf("create virtual file: %w", err)
	}
	return file, nil
}

func (s *ResourceManagementService) PatchFolderCdnUrl(ctx context.Context, folderID string, cdnURL string, operatorID string, operatorIP string) error {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return ErrManagedFolderNotFound
	}
	current, err := s.repo.FindFolderByID(ctx, folderID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFolderNotFound
	}
	if current.SourcePath == nil || strings.TrimSpace(*current.SourcePath) == "" {
		return ErrInvalidResourceEdit
	}
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate cdn url update log id: %w", err)
	}
	return s.repo.PatchFolderCdnUrl(ctx, folderID, cdnURL, operatorID, operatorIP, logID, s.nowFunc())
}
