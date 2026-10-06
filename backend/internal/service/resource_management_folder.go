package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
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
	var coverURL *string
	if input.CoverURL != nil {
		normalized, err := normalizeOptionalCoverURL(*input.CoverURL)
		if err != nil {
			return ErrInvalidResourceEdit
		}
		coverURL = &normalized
	}
	prefix, err := normalizeOptionalHTTPURL(input.DirectLinkPrefix)
	if err != nil {
		return ErrInvalidResourceEdit
	}

	applyDl, allowDl, err := parseDownloadPolicy(input.DownloadPolicy)
	if err != nil {
		return ErrInvalidResourceEdit
	}

	applyHide, hideExtension, err := parseHideFileExtensionPolicy(input.HideFileExtension)
	if err != nil {
		return ErrInvalidResourceEdit
	}

	// 校验并处理 custom_path
	customPath := strings.TrimSpace(input.CustomPath)
	if err := ValidateCustomPath(customPath); err != nil {
		return fmt.Errorf("%w: custom_path 必须以英文字母开头，且不能与 upload/admin/files/api/public 等保留路径冲突", ErrInvalidResourceEdit)
	}

	// 检查 custom_path 唯一性（排除当前文件夹自身）
	cpConflict, err := s.repo.CustomPathExists(ctx, customPath, folderID)
	if err != nil {
		return fmt.Errorf("check custom path uniqueness: %w", err)
	}
	if cpConflict {
		return fmt.Errorf("%w: custom_path %q 已被其他文件夹使用", ErrManagedFolderConflict, customPath)
	}

	if current.SourcePath == nil || strings.TrimSpace(*current.SourcePath) == "" || current.Name == name {
		if err := s.repo.UpdateFolderMetadata(ctx, folderID, name, description, remark, coverURL, prefix, customPath, applyDl, allowDl, applyHide, hideExtension, input.OperatorID, input.OperatorIP, logID, s.nowFunc()); err != nil {
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

	// 一致性编排：事务开启 → DB 更新（未提交）→ 磁盘改名 → 提交；
	// 提交失败时把目录名改回旧名，避免整棵子树路径与磁盘脱节。
	now := s.nowFunc()
	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin folder update transaction: %w", tx.Error)
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

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

	if err := repository.UpdateFolderTreePathsTx(tx, folderID, name, description, remark, coverURL, prefix, customPath, applyDl, allowDl, folderSourcePaths, input.OperatorID, input.OperatorIP, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFolderNotFound
		}
		return fmt.Errorf("update folder tree paths: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		// 磁盘已改名，DB 未提交：把目录名还原回旧名
		_, _ = s.storage.RenameManagedDirectory(newRootPath, current.Name)
		return fmt.Errorf("commit folder update: %w", err)
	}
	committed = true
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
	name, ok := model.NormalizeManagedFolderName(input.Name)
	if !ok {
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

	// DB 记录落库后再建磁盘目录；失败时删除刚建的记录，避免留下"有记录无目录"或
	// 冲突检查前的空目录残留。
	if folderSourcePath != nil {
		if err := s.storage.EnsureManagedDirectory(strings.TrimSpace(*folderSourcePath)); err != nil {
			if delErr := s.repo.DeleteFolderRecord(ctx, folder.ID); delErr != nil {
				return nil, fmt.Errorf("create directory on disk failed (%v); cleanup db record failed: %w", err, delErr)
			}
			return nil, fmt.Errorf("create directory on disk: %w", err)
		}
	}
	return folder, nil
}

// resolveNewFolderSourcePath builds the on-disk path for a new folder named childName
// under the given parent. Returns nil if the parent is not part of a managed tree.
// childName 必须已经过 NormalizeManagedFolderName 校验，这里再做一次 Base 防御。
func resolveNewFolderSourcePath(parent *model.Folder, childName string) *string {
	if parent == nil {
		return nil
	}
	if parent.SourcePath == nil || strings.TrimSpace(*parent.SourcePath) == "" {
		return nil
	}
	if validated, ok := model.NormalizeManagedFolderName(childName); !ok || validated != childName {
		return nil
	}
	rootPath := filepath.Clean(strings.TrimSpace(*parent.SourcePath))
	result := filepath.Join(rootPath, childName)
	return &result
}

// CreateVirtualFolder 创建虚拟目录（无物理磁盘路径，仅存数据库，子文件通过 CDN 直链提供）。
func (s *ResourceManagementService) CreateVirtualFolder(ctx context.Context, input CreateFolderInput) (*model.Folder, error) {
	name, ok := model.NormalizeManagedFolderName(input.Name)
	if !ok {
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
	Name              string
	FolderID          string
	PlaybackURL       string // CDN 前台直链（非代理时也可作为直链播放地址）
	PlaybackFallbackURL string // CDN 前台备用直链
	ProxySourceURL    string // 服务端代理拉取的目标地址（仅 proxy_download=true 时使用）
	ProxyDownload     bool
	Description       string
	Remark            string
	OperatorID        string
	OperatorIP        string
}

// CreateVirtualFile 在虚拟目录下创建虚拟文件（通过 CDN 直链提供下载）。
func (s *ResourceManagementService) CreateVirtualFile(ctx context.Context, input CreateVirtualFileInput) (*model.File, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.FolderID = strings.TrimSpace(input.FolderID)
	input.PlaybackURL = strings.TrimSpace(input.PlaybackURL)
	input.PlaybackFallbackURL = strings.TrimSpace(input.PlaybackFallbackURL)
	input.ProxySourceURL = strings.TrimSpace(input.ProxySourceURL)

	if input.Name == "" || input.FolderID == "" {
		return nil, ErrInvalidResourceEdit
	}

	// 代理模式下必须有代理源地址，否则必须有前台直链
	if input.ProxyDownload {
		if input.ProxySourceURL == "" {
			return nil, ErrInvalidResourceEdit
		}
	} else {
		if input.PlaybackURL == "" {
			return nil, ErrInvalidResourceEdit
		}
	}

	// 校验前台直链（可选）
	playbackURL := input.PlaybackURL
	if playbackURL != "" {
		candidate, err := normalizeOptionalHTTPURL(playbackURL)
		if err != nil {
			return nil, ErrInvalidResourceEdit
		}
		playbackURL = candidate
	}

	// 校验前台备用直链（可选）
	playbackFallbackURL := input.PlaybackFallbackURL
	if playbackFallbackURL != "" {
		candidate, err := normalizeOptionalHTTPURL(playbackFallbackURL)
		if err != nil {
			return nil, ErrInvalidResourceEdit
		}
		playbackFallbackURL = candidate
	}

	// 校验代理源地址（可选）
	proxySourceURL := input.ProxySourceURL
	if proxySourceURL != "" {
		candidate, err := normalizeOptionalHTTPURL(proxySourceURL)
		if err != nil {
			return nil, ErrInvalidResourceEdit
		}
		proxySourceURL = candidate
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

	// HEAD 请求获取文件大小：代理模式用代理源地址（proxy 模式允许内网存储），
	// 否则用前台直链（strict 模式拒绝内网目标）。
	// 探测失败仅跳过大小探测（size 落库为 0），错误打印日志，不阻断创建。
	headURL := proxySourceURL
	headMode := SSRFModeStrict
	if headURL == "" {
		headURL = playbackURL
	} else if input.ProxyDownload {
		headMode = SSRFModeProxy
	}
	fileSize := int64(0)
	if headURL != "" {
		if validateErr := validateSSRFURL(headURL, headMode); validateErr == nil {
			client := NewSSRFFetchClient(headMode, 10*time.Second)
			headResp, err := client.Head(headURL)
			if err == nil && headResp != nil {
				headResp.Body.Close()
				if headResp.ContentLength > 0 {
					fileSize = headResp.ContentLength
				}
			} else if err != nil {
				fmt.Printf("[warn] virtual file HEAD probe failed for %q: %v\n", headURL, err)
			}
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
		ID:                  id,
		FolderID:            &folder.ID,
		Name:                name,
		Description:         strings.TrimSpace(input.Description),
		Remark:              normalizeManagedRemark(input.Remark),
		Extension:           extension,
		PlaybackURL:         playbackURL,
		PlaybackFallbackURL: playbackFallbackURL,
		ProxySourceURL:      proxySourceURL,
		ProxyDownload:       input.ProxyDownload,
		Size:                fileSize,
		CreatedAt:           now,
		UpdatedAt:           now,
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

// PatchFolderGuestKeys 切换目录的访客密钥访问要求并替换允许的密钥 ID 列表。
// 该字段对托管根和子目录都生效，不要求根目录。
func (s *ResourceManagementService) PatchFolderGuestKeys(ctx context.Context, folderID string, required bool, allowedKeyIDs []string, operatorID string, operatorIP string) error {
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
	// 简单校验：去重后非空 keyIDs 全部 trim
	cleaned := make([]string, 0, len(allowedKeyIDs))
	for _, k := range allowedKeyIDs {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		cleaned = append(cleaned, k)
	}
	if required && len(cleaned) == 0 {
		return ErrInvalidResourceEdit
	}
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate guest key log id: %w", err)
	}
	return s.repo.PatchFolderGuestKeys(ctx, folderID, required, cleaned, operatorID, operatorIP, logID, s.nowFunc())
}
