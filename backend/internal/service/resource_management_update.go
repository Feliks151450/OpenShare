package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

func (s *ResourceManagementService) UpdateFile(ctx context.Context, fileID string, input UpdateManagedFileInput) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return ErrManagedFileNotFound
	}

	current, err := s.repo.FindFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}

	name, extension, ok := model.NormalizeManagedFileName(input.Name)
	if !ok {
		return ErrInvalidResourceEdit
	}
	description := normalizeTrimmedString(input.Description)
	playbackURL, err := normalizeOptionalHTTPURL(input.PlaybackURL)
	if err != nil {
		return ErrInvalidResourceEdit
	}
	coverURL, err := normalizeOptionalHTTPURL(input.CoverURL)
	if err != nil {
		return ErrInvalidResourceEdit
	}

	if current.Name != name {
		fileConflict, err := s.repo.FileNameExists(ctx, current.FolderID, name, current.ID)
		if err != nil {
			return err
		}
		folderConflict, err := s.repo.FolderNameExists(ctx, current.FolderID, name, "")
		if err != nil {
			return err
		}
		if fileConflict || folderConflict {
			return ErrManagedFileConflict
		}
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate resource update log id: %w", err)
	}
	if current.Name != name {
		folder, err := s.repo.FindFolderByID(ctx, normalizeTrimmedString(modelValue(current.FolderID)))
		if err != nil {
			return err
		}
		currentPath := model.BuildManagedFilePath(folderSourcePath(folder), current.Name)
		if currentPath == "" {
			return ErrManagedFileNotFound
		}
		if _, err := s.storage.RenameManagedFile(currentPath, name); err != nil {
			if errors.Is(err, storage.ErrManagedFileConflict) {
				return ErrManagedFileConflict
			}
			return fmt.Errorf("rename managed file: %w", err)
		}
	}
	if err := s.repo.UpdateFileMetadata(ctx, fileID, name, extension, description, playbackURL, coverURL, input.OperatorID, input.OperatorIP, logID, s.nowFunc()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFileNotFound
		}
		return fmt.Errorf("update managed file: %w", err)
	}
	return nil
}

func normalizeOptionalHTTPURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("url must use http or https")
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("url must include host")
	}
	return u.String(), nil
}
