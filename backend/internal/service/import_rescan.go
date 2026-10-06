package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

type ManagedDirectoryRescanResult struct {
	RootPath       string `json:"root_path"`
	AddedFolders   int    `json:"added_folders"`
	AddedFiles     int    `json:"added_files"`
	UpdatedFolders int    `json:"updated_folders"`
	UpdatedFiles   int    `json:"updated_files"`
	DeletedFolders int    `json:"deleted_folders"`
	DeletedFiles   int    `json:"deleted_files"`
}

// rescanMutex 串行化全部重新扫描：磁盘快照与 DB 快照之间的 diff 无锁，
// 并发 rescan 同一/嵌套目录会产生互相污染的删除集。
var rescanMutex sync.Mutex

func (s *ImportService) RescanManagedDirectory(ctx context.Context, folderID, adminID, operatorIP string) (*ManagedDirectoryRescanResult, error) {
	rescanMutex.Lock()
	defer rescanMutex.Unlock()

	// 重新扫描支持任意非虚拟托管目录：根目录或子目录均可，扫描范围包含该目录及其所有后代。
	// 虚拟目录没有磁盘对应，按"目录不可用"返回。
	rootFolder, err := s.repository.FindFolderByID(ctx, strings.TrimSpace(folderID))
	if err != nil {
		return nil, fmt.Errorf("find managed root: %w", err)
	}
	if rootFolder == nil {
		return nil, ErrFolderTreeNotFound
	}
	if rootFolder.IsVirtual {
		return nil, &ManagedDirectoryUnavailableError{}
	}
	if rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, &ManagedDirectoryUnavailableError{}
	}

	rootPath := filepath.Clean(strings.TrimSpace(*rootFolder.SourcePath))
	info, err := os.Stat(rootPath)
	if err != nil || !info.IsDir() {
		return nil, &ManagedDirectoryUnavailableError{Path: rootPath}
	}

	// 磁盘扫描时刻：快照窗口内（此刻之后）新落库的文件不参与删除判定，
	// 防止扫描期间并发上传/审批的记录被误判为"磁盘不存在"而从 DB 删除。
	scanStartedAt := s.nowFunc()

	entries, err := s.storage.ScanDirectory(rootPath)
	if err != nil {
		return nil, fmt.Errorf("scan managed directory: %w", err)
	}

	folders, err := s.repository.ListManagedSubtreeFolders(ctx, rootFolder.ID)
	if err != nil {
		return nil, fmt.Errorf("list managed subtree folders: %w", err)
	}
	files, err := s.repository.ListManagedSubtreeFiles(ctx, rootFolder.ID)
	if err != nil {
		return nil, fmt.Errorf("list managed subtree files: %w", err)
	}

	now := s.nowFunc()
	result := &ManagedDirectoryRescanResult{RootPath: rootPath}

	fsFolderPaths, fsFiles := buildFilesystemSnapshot(rootPath, entries)
	existingFoldersByPath := make(map[string]repository.ManagedSubtreeFolderRow, len(folders))
	folderSourcePathByID := make(map[string]string, len(folders))
	// 虚拟目录 ID 集合，重新扫描时跳过删除
	virtualFolderIDs := make(map[string]struct{})
	for _, folder := range folders {
		if folder.IsVirtual {
			virtualFolderIDs[folder.ID] = struct{}{}
		}
		if folder.SourcePath == nil || strings.TrimSpace(*folder.SourcePath) == "" {
			continue
		}
		normalized := normalizeRescanPath(*folder.SourcePath)
		existingFoldersByPath[normalized] = folder
		folderSourcePathByID[folder.ID] = normalized
	}

	existingFilesByPath := make(map[string]repository.ManagedSubtreeFileRow, len(files))
	for _, file := range files {
		folderPath := folderSourcePathByID[optionalStringValue(file.FolderID)]
		if folderPath == "" || strings.TrimSpace(file.Name) == "" {
			continue
		}
		existingFilesByPath[normalizeRescanPath(filepath.Join(folderPath, file.Name))] = file
	}

	matchedFolderIDs := make(map[string]struct{}, len(folders))
	pathToFolderID := make(map[string]string, len(fsFolderPaths))
	updatedFolders := make([]repository.ManagedFolderUpdate, 0)
	addedFolders := make([]*model.Folder, 0)

	for _, folderPath := range fsFolderPaths {
		normalizedPath := normalizeRescanPath(folderPath)
		var parentID *string
		if normalizedPath == normalizeRescanPath(rootPath) {
			// 扫描起点对应的目录：保留其在数据库中现存的父级，避免被拍平到根或孤立。
			parentID = rootFolder.ParentID
		} else {
			parentPath := normalizeRescanPath(filepath.Dir(normalizedPath))
			parentValue := pathToFolderID[parentPath]
			parentID = &parentValue
		}
		name := filepath.Base(normalizedPath)

		if existing, ok := existingFoldersByPath[normalizedPath]; ok {
			matchedFolderIDs[existing.ID] = struct{}{}
			pathToFolderID[normalizedPath] = existing.ID
			if existing.ParentID == nil && parentID == nil &&
				existing.Name == name &&
				normalizeOptionalPath(existing.SourcePath) == normalizedPath {
				continue
			}
			if optionalStringValue(existing.ParentID) == optionalStringValue(parentID) &&
				existing.Name == name &&
				normalizeOptionalPath(existing.SourcePath) == normalizedPath {
				continue
			}
			updatedFolders = append(updatedFolders, repository.ManagedFolderUpdate{
				ID:         existing.ID,
				ParentID:   parentID,
				Name:       name,
				SourcePath: normalizedPath,
			})
			result.UpdatedFolders++
			continue
		}

		id, err := identity.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate rescanned folder id: %w", err)
		}
		addedFolders = append(addedFolders, &model.Folder{
			ID:          id,
			ParentID:    parentID,
			SourcePath:  stringPtr(normalizedPath),
			Name:        name,
			Description: "",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		pathToFolderID[normalizedPath] = id
		result.AddedFolders++
	}

	matchedFileIDs := make(map[string]struct{}, len(files))
	updatedFiles := make([]repository.ManagedFileUpdate, 0)
	addedFiles := make([]*model.File, 0)

	filePaths := make([]string, 0, len(fsFiles))
	for path := range fsFiles {
		filePaths = append(filePaths, path)
	}
	sort.Strings(filePaths)

	for _, filePath := range filePaths {
		entry := fsFiles[filePath]
		parentPath := normalizeRescanPath(filepath.Dir(filePath))
		parentValue := pathToFolderID[parentPath]
		folderID := &parentValue

		if existing, ok := matchManagedFileByPath(filePath, existingFilesByPath, matchedFileIDs); ok {
			matchedFileIDs[existing.ID] = struct{}{}
			description := existing.Description
			if optionalStringValue(existing.FolderID) == optionalStringValue(folderID) &&
				existing.Name == entry.Name &&
				existing.Extension == entry.Extension &&
				existing.MimeType == entry.MimeType &&
				existing.Size == entry.Size {
				continue
			}
			updatedFiles = append(updatedFiles, repository.ManagedFileUpdate{
				ID:          existing.ID,
				FolderID:    folderID,
				Name:        entry.Name,
				Description: description,
				Extension:   entry.Extension,
				MimeType:    entry.MimeType,
				Size:        entry.Size,
			})
			result.UpdatedFiles++
			continue
		}

		id, err := identity.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate rescanned file id: %w", err)
		}
		addedFiles = append(addedFiles, &model.File{
			ID:            id,
			FolderID:      folderID,
			Name:          entry.Name,
			Description:   "",
			Extension:     entry.Extension,
			MimeType:      entry.MimeType,
			Size:          entry.Size,
			DownloadCount: 0,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		result.AddedFiles++
	}

	deletedFileIDs := make([]string, 0)
	for _, file := range files {
		if _, ok := matchedFileIDs[file.ID]; ok {
			continue
		}
		// 跳过虚拟目录下的文件：没有磁盘对应，不应被重新扫描删除
		if file.FolderID != nil {
			if _, isVirtual := virtualFolderIDs[*file.FolderID]; isVirtual {
				continue
			}
		}
		// 跳过扫描开始后才落库的记录：它们可能来自扫描期间的并发上传/审批，
		// 其磁盘文件未进入本次快照，删除会造成"记录消失 + 磁盘孤儿"。
		if !file.CreatedAt.Before(scanStartedAt) {
			continue
		}
		deletedFileIDs = append(deletedFileIDs, file.ID)
	}
	result.DeletedFiles = len(deletedFileIDs)

	deletedFolderIDs := make([]string, 0)
	for _, folder := range folders {
		if folder.ID == rootFolder.ID {
			continue
		}
		if _, ok := matchedFolderIDs[folder.ID]; ok {
			continue
		}
		// 跳过虚拟目录：没有磁盘对应，不应被重新扫描删除
		if folder.IsVirtual {
			continue
		}
		// 跳过非虚拟但 SourcePath 为空的目录（异常数据）：不应被静默删除
		if folder.SourcePath == nil || strings.TrimSpace(*folder.SourcePath) == "" {
			continue
		}
		// 同文件：扫描窗口内新建的目录记录不删
		if !folder.CreatedAt.Before(scanStartedAt) {
			continue
		}
		deletedFolderIDs = append(deletedFolderIDs, folder.ID)
	}
	result.DeletedFolders = len(deletedFolderIDs)

	detail, _ := json.Marshal(result)
	if err := s.repository.ApplyRescanSync(ctx, repository.RescanSyncInput{
		RootFolderID:     rootFolder.ID,
		OperatorID:       adminID,
		OperatorIP:       operatorIP,
		Detail:           string(detail),
		Now:              now,
		AddedFolders:     addedFolders,
		UpdatedFolders:   updatedFolders,
		DeletedFolderIDs: deletedFolderIDs,
		AddedFiles:       addedFiles,
		UpdatedFiles:     updatedFiles,
		DeletedFileIDs:   deletedFileIDs,
	}); err != nil {
		return nil, fmt.Errorf("apply managed directory rescan: %w", err)
	}

	return result, nil
}
