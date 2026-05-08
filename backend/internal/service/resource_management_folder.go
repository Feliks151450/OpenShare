package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

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
