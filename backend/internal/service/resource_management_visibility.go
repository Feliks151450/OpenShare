package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/pkg/identity"
)

// PatchRootFolderHidePublicCatalog 仅允许托管根目录：控制是否出现在访客 GET /public/folders 根列表。
func (s *ResourceManagementService) PatchRootFolderHidePublicCatalog(
	ctx context.Context,
	folderID string,
	hide bool,
	operatorID string,
	operatorIP string,
) error {
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
	if current.ParentID != nil {
		return ErrInvalidResourceEdit
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate folder visibility log id: %w", err)
	}
	if err := s.repo.UpdateRootFolderHidePublicCatalog(ctx, folderID, hide, operatorID, operatorIP, logID, s.nowFunc()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFolderNotFound
		}
		return fmt.Errorf("update root folder catalog visibility: %w", err)
	}
	return nil
}
