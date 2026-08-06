package service

import (
	"context"
	"fmt"
)

type FolderTreeNode struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	SourcePath        string           `json:"source_path"`
	HidePublicCatalog bool             `json:"hide_public_catalog"`
	CdnURL            string           `json:"cdn_url"`
	GuestKeyRequired  bool             `json:"guest_key_required"`
	AllowedGuestKeyIDs []string        `json:"allowed_guest_key_ids"`
	Folders           []FolderTreeNode `json:"folders"`
	Files             []FolderTreeFile `json:"files"`
}

type FolderTreeFile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	DownloadCount int64  `json:"download_count"`
}

func (s *ImportService) GetFolderTree(ctx context.Context) ([]FolderTreeNode, error) {
	folders, err := s.repository.ListFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	files, err := s.repository.ListFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	// 批量取每个目录允许的密钥 ID（仅涉及一次性 INNER，避免逐目录 N+1）
	folderIDs := make([]string, 0, len(folders))
	for _, folder := range folders {
		folderIDs = append(folderIDs, folder.ID)
	}
	allowedKeysByFolder, err := s.listAllowedGuestKeysByFolderIDs(ctx, folderIDs)
	if err != nil {
		return nil, fmt.Errorf("list allowed guest keys: %w", err)
	}

	nodes := make(map[string]*FolderTreeNode, len(folders))
	childrenByParent := make(map[string][]string)
	rootIDs := make([]string, 0)

	for _, folder := range folders {
		nodes[folder.ID] = &FolderTreeNode{
			ID:                folder.ID,
			Name:              folder.Name,
			SourcePath:        derefString(folder.SourcePath),
			HidePublicCatalog: folder.HidePublicCatalog,
			CdnURL:            folder.CdnURL,
			GuestKeyRequired:  folder.GuestKeyRequired,
			AllowedGuestKeyIDs: allowedKeysByFolder[folder.ID],
			Folders:           []FolderTreeNode{},
			Files:             []FolderTreeFile{},
		}
	}
	for _, folder := range folders {
		if folder.ParentID == nil {
			rootIDs = append(rootIDs, folder.ID)
			continue
		}
		childrenByParent[*folder.ParentID] = append(childrenByParent[*folder.ParentID], folder.ID)
	}
	for _, file := range files {
		if file.FolderID == nil {
			continue
		}
		parent := nodes[*file.FolderID]
		if parent == nil {
			continue
		}
		parent.Files = append(parent.Files, FolderTreeFile{
			ID:            file.ID,
			Name:          file.Name,
			Size:          file.Size,
			DownloadCount: file.DownloadCount,
		})
	}

	var build func(string) FolderTreeNode
	build = func(folderID string) FolderTreeNode {
		node := nodes[folderID]
		result := *node
		result.Folders = make([]FolderTreeNode, 0, len(childrenByParent[folderID]))
		for _, childID := range childrenByParent[folderID] {
			result.Folders = append(result.Folders, build(childID))
		}
		return result
	}

	result := make([]FolderTreeNode, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		result = append(result, build(rootID))
	}

	return result, nil
}

// listAllowedGuestKeysByFolderIDs 批量取每个目录允许的密钥 ID；缺失目录以空 slice 占位。
func (s *ImportService) listAllowedGuestKeysByFolderIDs(ctx context.Context, folderIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(folderIDs))
	for _, id := range folderIDs {
		out[id] = []string{}
	}
	if len(folderIDs) == 0 {
		return out, nil
	}
	// 通过 ImportRepository 关联的 GORM 实例直接查询关联表，避免新建 repository 依赖。
	rows, err := s.repository.GetDB().WithContext(ctx).
		Table("folder_guest_keys").
		Where("folder_id IN ?", folderIDs).
		Rows()
	if err != nil {
		return nil, fmt.Errorf("query folder guest keys: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var folderID, keyID string
		if scanErr := rows.Scan(&folderID, &keyID); scanErr != nil {
			return nil, fmt.Errorf("scan folder guest key row: %w", scanErr)
		}
		out[folderID] = append(out[folderID], keyID)
	}
	return out, nil
}
