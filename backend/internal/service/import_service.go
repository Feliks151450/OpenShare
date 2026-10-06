package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
)

var (
	ErrInvalidImportPath   = errors.New("invalid import path")
	ErrFolderTreeNotFound  = errors.New("folder not found")
	ErrManagedRootRequired = errors.New("managed root folder required")
)

type ManagedDirectoryConflictError struct {
	Message string
}

func (e *ManagedDirectoryConflictError) Error() string {
	if e == nil {
		return "managed directory conflict"
	}
	return e.Message
}

type ManagedDirectoryUnavailableError struct {
	Path string
}

func (e *ManagedDirectoryUnavailableError) Error() string {
	if e == nil || strings.TrimSpace(e.Path) == "" {
		return "managed directory path is unavailable"
	}
	return fmt.Sprintf("托管目录不可用：%s", e.Path)
}

type ImportService struct {
	repository *repository.ImportRepository
	storage    *storage.Service
	nowFunc    func() time.Time
	// allowedRoots 非空时限制本地目录浏览/导入的可访问范围（绝对路径前缀白名单）。
	// 空表示不限制（保持旧行为）。
	allowedRoots []string
}

func NewImportService(repository *repository.ImportRepository, storageService *storage.Service, allowedRoots []string) *ImportService {
	normalized := make([]string, 0, len(allowedRoots))
	for _, root := range allowedRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		normalized = append(normalized, filepath.Clean(root))
	}
	return &ImportService{
		repository:   repository,
		storage:      storageService,
		nowFunc:      func() time.Time { return time.Now().UTC() },
		allowedRoots: normalized,
	}
}

// pathAllowedByRoots 判定路径是否落在白名单内（白名单为空表示不限制）。
func (s *ImportService) pathAllowedByRoots(path string) bool {
	if len(s.allowedRoots) == 0 {
		return true
	}
	cleaned := filepath.Clean(strings.TrimSpace(path))
	for _, root := range s.allowedRoots {
		if cleaned == root || strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
