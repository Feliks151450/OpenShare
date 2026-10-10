package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

var (
	ErrInvalidPublicFileQuery = errors.New("invalid public file query")
	ErrFolderNotFound         = errors.New("folder not found")
	ErrCustomPathInvalid      = errors.New("custom path is invalid")
	ErrCustomPathConflict     = errors.New("custom path is already in use")
)

// customPathPattern 自定义路径必须以英文字母开头，后续可包含字母、数字、下划线、连字符、斜杠。
var customPathPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*(/[a-zA-Z][a-zA-Z0-9_-]*)*$`)

// reservedPaths 与现有路由冲突的路径前缀。
var reservedPaths = []string{"upload", "admin", "files", "api", "public", "healthz", "dl"}

// ValidateCustomPath 校验 custom_path 格式并检查是否与保留路由冲突。
func ValidateCustomPath(customPath string) error {
	customPath = strings.TrimSpace(customPath)
	if customPath == "" {
		return nil // 空值表示未设置，合法
	}
	if !customPathPattern.MatchString(customPath) {
		return ErrCustomPathInvalid
	}
	// 检查首段是否与保留路由冲突
	firstSegment := customPath
	if idx := strings.Index(customPath, "/"); idx != -1 {
		firstSegment = customPath[:idx]
	}
	for _, reserved := range reservedPaths {
		if firstSegment == reserved {
			return ErrCustomPathInvalid
		}
	}
	return nil
}

const (
	defaultPublicFilePage     = 1
	defaultPublicFilePageSize = 20
	maxPublicFilePageSize     = 100
)

type PublicCatalogService struct {
	repository *repository.PublicCatalogRepository
	download   *PublicDownloadService
	fileTags   *FileTagService
}

type PublicFolderFileListInput struct {
	FolderID string
	Page     int
	PageSize int
	Sort     string
}

type PublicFolderFileListResult struct {
	Items    []PublicFileItem `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int64            `json:"total"`
}

type PublicFileFeedResult struct {
	Items []PublicFileItem `json:"items"`
}

type PublicFileItem struct {
	ID                      string          `json:"id"`
	Name                    string          `json:"name"`
	Description             string          `json:"description"`
	Remark                  string          `json:"remark"`
	Extension               string          `json:"extension"`
	CoverURL                string          `json:"cover_url"`
	PlaybackURL             string          `json:"playback_url"`
	ProxyDownload           bool            `json:"proxy_download"`
	ProxySourceURL          string          `json:"proxy_source_url"`
	FolderDirectDownloadURL string          `json:"folder_direct_download_url"`
	DownloadAllowed         bool            `json:"download_allowed"`
	UploadedAt              time.Time       `json:"uploaded_at"`
	DownloadCount           int64           `json:"download_count"`
	Size                    int64           `json:"size"`
	Tags                    []PublicFileTag `json:"tags"`
}

type PublicFolderItem struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Remark          string    `json:"remark"`
	CoverURL        string    `json:"cover_url"`
	CdnURL          string    `json:"cdn_url"`
	DownloadAllowed bool      `json:"download_allowed"`
	IsVirtual       bool      `json:"is_virtual"`
	UpdatedAt       time.Time `json:"updated_at"`
	FileCount       int64     `json:"file_count"`
	DownloadCount   int64     `json:"download_count"`
	TotalSize       int64     `json:"total_size"`
}

type PublicFolderBreadcrumbItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PublicFolderDetail struct {
	ID               string                       `json:"id"`
	Name             string                       `json:"name"`
	Description      string                       `json:"description"`
	Remark           string                       `json:"remark"`
	CoverURL         string                       `json:"cover_url"`
	ParentID         *string                      `json:"parent_id"`
	Breadcrumbs      []PublicFolderBreadcrumbItem `json:"breadcrumbs"`
	FileCount        int64                        `json:"file_count"`
	DownloadCount    int64                        `json:"download_count"`
	TotalSize        int64                        `json:"total_size"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	DirectLinkPrefix string                       `json:"direct_link_prefix"`
	DownloadAllowed  bool                         `json:"download_allowed"`
	DownloadPolicy   string                       `json:"download_policy"`
	// IsVirtual 为 true 时表示虚拟目录（无物理磁盘路径，文件通过 CDN 直链提供）。
	IsVirtual bool `json:"is_virtual"`
	// HidePublicCatalog 仅托管根目录返回：访客首页根列表是否隐藏该托管树。
	HidePublicCatalog *bool `json:"hide_public_catalog,omitempty"`
	// GuestKeyRequired 为 true 时表示该目录（或其祖先）配置了访客密钥访问，必须输入正确密钥才能看到详情。
	GuestKeyRequired bool `json:"guest_key_required"`
	// CustomPath 自定义访问路径，如 "doc" 对应 /doc 访问该文件夹。
	CustomPath string `json:"custom_path"`
	// HideFileExtension 原始设置值：inherit/hide/show
	HideFileExtension *bool `json:"hide_file_extension,omitempty"`
	// HideFileExtensionResolved 解析后的最终值：true 表示隐藏后缀，false 表示显示
	HideFileExtensionResolved bool `json:"hide_file_extension_resolved"`
	// Readme 该目录下 README 文件的正文；仅在 description 为空时填充，供前端简介区回退展示。
	Readme string `json:"readme,omitempty"`
}

func NewPublicCatalogService(
	repository *repository.PublicCatalogRepository,
	download *PublicDownloadService,
	fileTags *FileTagService,
) *PublicCatalogService {
	return &PublicCatalogService{repository: repository, download: download, fileTags: fileTags}
}

func (s *PublicCatalogService) ListPublicFolderFiles(ctx context.Context, input PublicFolderFileListInput) (*PublicFolderFileListResult, error) {
	normalized, err := normalizePublicFolderFileListInput(input)
	if err != nil {
		return nil, err
	}

	exists, err := s.repository.FolderExists(ctx, normalized.FolderID)
	if err != nil {
		return nil, fmt.Errorf("validate folder: %w", err)
	}
	if !exists {
		return nil, ErrFolderNotFound
	}

	// 自定义排序优先：sort_order ASC 作为第一排序键，未设置排序的文件 (sort_order=0) 自然排在后面
	orderBy := append([]string{"sort_order ASC"}, normalized.OrderBy...)

	files, total, err := s.repository.ListPublicFolderFiles(ctx, repository.PublicFolderFileListQuery{
		FolderID: normalized.FolderID,
		Offset:   (normalized.Page - 1) * normalized.PageSize,
		Limit:    normalized.PageSize,
		OrderBy:  orderBy,
	})
	if err != nil {
		return nil, fmt.Errorf("list public folder files: %w", err)
	}

	mapped, err := s.mapPublicFileItems(ctx, files)
	if err != nil {
		return nil, err
	}
	return &PublicFolderFileListResult{
		Items:    mapped,
		Page:     normalized.Page,
		PageSize: normalized.PageSize,
		Total:    total,
	}, nil
}

func (s *PublicCatalogService) ListHotFiles(ctx context.Context, limit int) (*PublicFileFeedResult, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 20
	}
	if normalizedLimit > maxPublicFilePageSize {
		normalizedLimit = maxPublicFilePageSize
	}

	files, err := s.repository.ListRecentHotManagedFiles(ctx, repository.PublicHotFileFeedQuery{
		SinceDay: time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02"),
		Limit:    normalizedLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list recent hot managed files: %w", err)
	}

	mapped, err := s.mapPublicFileItems(ctx, files)
	if err != nil {
		return nil, err
	}
	return &PublicFileFeedResult{
		Items: mapped,
	}, nil
}

func (s *PublicCatalogService) ListLatestFiles(ctx context.Context, limit int) (*PublicFileFeedResult, error) {
	return s.listManagedFileFeed(ctx, limit, []string{"created_at DESC", "id DESC"})
}

func (s *PublicCatalogService) ListPublicFolders(ctx context.Context, parentID string) ([]PublicFolderItem, error) {
	var parentPtr *string
	if trimmed := strings.TrimSpace(parentID); trimmed != "" {
		exists, err := s.repository.FolderExists(ctx, trimmed)
		if err != nil {
			return nil, fmt.Errorf("validate parent folder: %w", err)
		}
		if !exists {
			return nil, ErrFolderNotFound
		}
		parentPtr = &trimmed
	}

	rows, err := s.repository.ListPublicFolders(ctx, parentPtr)
	if err != nil {
		return nil, fmt.Errorf("list public folders: %w", err)
	}

	items := make([]PublicFolderItem, 0, len(rows))
	for _, row := range rows {
		allowed := true
		if s.download != nil {
			f := model.Folder{
				ID:            row.ID,
				ParentID:      row.ParentID,
				Name:          row.Name,
				AllowDownload: row.AllowDownload,
			}
			var err error
			allowed, err = s.download.EffectiveDownloadAllowedForFolder(ctx, &f)
			if err != nil {
				return nil, fmt.Errorf("resolve folder download policy: %w", err)
			}
		}
		items = append(items, PublicFolderItem{
			ID:              row.ID,
			Name:            row.Name,
			Description:     row.Description,
			Remark:          row.Remark,
			CoverURL:        strings.TrimSpace(row.CoverURL),
			CdnURL:          strings.TrimSpace(row.CdnURL),
			DownloadAllowed: allowed,
			IsVirtual:       row.IsVirtual,
			UpdatedAt:       row.UpdatedAt,
			FileCount:       row.FileCount,
			DownloadCount:   row.DownloadCount,
			TotalSize:       row.TotalSize,
		})
	}

	return items, nil
}

func (s *PublicCatalogService) GetPublicFolderDetail(ctx context.Context, folderID string) (*PublicFolderDetail, error) {
	trimmed := strings.TrimSpace(folderID)
	if trimmed == "" {
		return nil, ErrFolderNotFound
	}

	current, err := s.repository.FindPublicFolderByID(ctx, trimmed)
	if err != nil {
		return nil, fmt.Errorf("find public folder: %w", err)
	}
	if current == nil {
		return nil, ErrFolderNotFound
	}

	breadcrumbs := []PublicFolderBreadcrumbItem{{ID: current.ID, Name: current.Name}}
	parentID := current.ParentID
	for parentID != nil {
		parent, err := s.repository.FindPublicFolderByID(ctx, *parentID)
		if err != nil {
			return nil, fmt.Errorf("find public folder ancestor: %w", err)
		}
		if parent == nil {
			return nil, ErrFolderNotFound
		}
		breadcrumbs = append(breadcrumbs, PublicFolderBreadcrumbItem{
			ID:   parent.ID,
			Name: parent.Name,
		})
		parentID = parent.ParentID
	}

	for i, j := 0, len(breadcrumbs)-1; i < j; i, j = i+1, j-1 {
		breadcrumbs[i], breadcrumbs[j] = breadcrumbs[j], breadcrumbs[i]
	}

	dlAllowed := true
	hideExt := false
	if s.download != nil {
		var err error
		dlAllowed, err = s.download.EffectiveDownloadAllowedForFolder(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("resolve folder download policy: %w", err)
		}
		hideExt, err = s.download.EffectiveHideFileExtensionForFolder(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("resolve folder hide file extension: %w", err)
		}
	}

	detail := PublicFolderDetail{
		ID:                        current.ID,
		Name:                      current.Name,
		Description:               current.Description,
		Remark:                    current.Remark,
		CoverURL:                  strings.TrimSpace(current.CoverURL),
		ParentID:                  current.ParentID,
		Breadcrumbs:               breadcrumbs,
		FileCount:                 current.FileCount,
		DownloadCount:             current.DownloadCount,
		TotalSize:                 current.TotalSize,
		UpdatedAt:                 current.UpdatedAt,
		DirectLinkPrefix:          strings.TrimSpace(current.DirectLinkPrefix),
		DownloadAllowed:           dlAllowed,
		DownloadPolicy:            DownloadPolicyString(current.AllowDownload),
		IsVirtual:                 current.IsVirtual,
		GuestKeyRequired:          current.GuestKeyRequired,
		CustomPath:                strings.TrimSpace(current.CustomPath),
		HideFileExtension:         current.HideFileExtension,
		HideFileExtensionResolved: hideExt,
	}
	if current.ParentID == nil {
		h := current.HidePublicCatalog
		detail.HidePublicCatalog = &h
	}
	if strings.TrimSpace(detail.Description) == "" {
		detail.Readme = s.readFolderReadme(ctx, current)
	}
	return &detail, nil
}

// maxFolderReadmeBytes 自动加载 README 的大小上限：简介面板只放说明文档，
// 大文件不该整份塞进每次目录详情请求。
const maxFolderReadmeBytes = 256 << 10

// readFolderReadme 读取托管目录下 README 文件的正文，用于未配置简介时的回退展示。
// 虚拟目录与 CDN 目录没有本地文件，直接返回空串。
func (s *PublicCatalogService) readFolderReadme(ctx context.Context, folder *model.Folder) string {
	if folder.IsVirtual || strings.TrimSpace(folder.CdnURL) != "" {
		return ""
	}

	file, err := s.repository.FindFolderReadmeFile(ctx, folder.ID)
	if err != nil || file == nil {
		return ""
	}

	path := model.BuildManagedFilePath(folder.SourcePath, file.Name)
	if path == "" {
		return ""
	}
	handle, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer handle.Close()

	content, err := io.ReadAll(io.LimitReader(handle, maxFolderReadmeBytes))
	if err != nil {
		return ""
	}
	// 截断可能落在多字节字符中间，渲染前去掉残缺尾部
	return strings.ToValidUTF8(string(content), "")
}

// ResolveCustomPath 根据自定义路径解析到文件夹。未找到时返回 nil, nil。
func (s *PublicCatalogService) ResolveCustomPath(ctx context.Context, customPath string) (*model.Folder, error) {
	customPath = strings.TrimSpace(customPath)
	if customPath == "" {
		return nil, nil
	}
	return s.repository.FindPublicFolderByCustomPath(ctx, customPath)
}

// ResolveFileFolderID 拿到文件所属目录的 ID（用于自定义路径解析后的密钥校验）。
func (s *PublicCatalogService) ResolveFileFolderID(ctx context.Context, fileID string) (string, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return "", nil
	}
	var folderID *string
	if err := s.repository.GetDB().
		WithContext(ctx).
		Model(&model.File{}).
		Select("folder_id").
		Where("id = ?", fileID).
		Scan(&folderID).Error; err != nil {
		return "", fmt.Errorf("resolve file folder id: %w", err)
	}
	if folderID == nil {
		return "", nil
	}
	return *folderID, nil
}

// CustomPathResolveResult 自定义路径解析结果，含类型和 ID。
type CustomPathResolveResult struct {
	Type      string `json:"type"` // "folder" 或 "file"
	FolderID  string `json:"folder_id,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	Name      string `json:"name"`
	Extension string `json:"extension,omitempty"` // 文件扩展名，如 .html
}

// ResolveCustomPathFull 同时查找文件夹和文件的自定义路径，返回完整解析结果。
func (s *PublicCatalogService) ResolveCustomPathFull(ctx context.Context, customPath string) (*CustomPathResolveResult, error) {
	customPath = strings.TrimSpace(customPath)
	if customPath == "" {
		return nil, nil
	}
	// 先查文件夹
	folder, err := s.repository.FindPublicFolderByCustomPath(ctx, customPath)
	if err != nil {
		return nil, err
	}
	if folder != nil {
		return &CustomPathResolveResult{
			Type:     "folder",
			FolderID: folder.ID,
			Name:     folder.Name,
		}, nil
	}
	// 再查文件
	file, err := s.repository.FindPublicFileByCustomPath(ctx, customPath)
	if err != nil {
		return nil, err
	}
	if file != nil {
		return &CustomPathResolveResult{
			Type:      "file",
			FileID:    file.ID,
			Name:      file.Name,
			Extension: strings.TrimSpace(file.Extension),
		}, nil
	}
	return nil, nil
}

// PathResolveResult 基于文件夹层级路径的解析结果。
type PathResolveResult struct {
	// Type 为 "file" 或 "folder"。
	Type     string `json:"type"`
	FileID   string `json:"file_id,omitempty"`
	FolderID string `json:"folder_id,omitempty"`
	Name     string `json:"name"`
}

// ResolvePathSegments 根据文件夹层级路径逐级解析到文件或文件夹。
// segments 为按 "/" 分割的路径段，如 ["课程", "数学", "笔记.pdf"]。
// 解析逻辑：从根目录开始，前 N-1 段逐级匹配子文件夹名称，
// 最后一段同时尝试匹配文件名和文件夹名（文件优先）。
func (s *PublicCatalogService) ResolvePathSegments(ctx context.Context, segments []string) (*PathResolveResult, error) {
	return s.resolveSegmentsFrom(ctx, nil, segments, false)
}

// maxCatalogLinkPathBytes 与 maxCatalogLinkPathSegments 限制单次链接解析的工作量：
// 每段都要落一次查询，路径由链接文本提供，属于公开端点上的输入。
const (
	maxCatalogLinkPathBytes    = 1024
	maxCatalogLinkPathSegments = 32
)

// ResolveCatalogRelativePath 以 anchorFolderID 为锚点解析 Markdown 相对链接的路径。
// anchorFolderID 为空表示根目录；path 以 "/" 开头时同样按根目录解析（与 URL 语义一致）。
// 支持 "." 与 ".."（逐级上溯，越过根即停在根），结尾 "/" 强制解析为文件夹
// （如 [nc/](nc/) 指向子目录而非同名文件）。无法解析时返回 nil, nil。
func (s *PublicCatalogService) ResolveCatalogRelativePath(ctx context.Context, anchorFolderID, path string) (*PathResolveResult, error) {
	path = strings.TrimSpace(path)
	if path == "" || len(path) > maxCatalogLinkPathBytes {
		return nil, nil
	}

	folderOnly := strings.HasSuffix(path, "/")
	segments := splitCatalogLinkPath(path)
	if len(segments) > maxCatalogLinkPathSegments {
		return nil, nil
	}

	parentID := (*string)(nil)
	if !strings.HasPrefix(path, "/") {
		anchor := strings.TrimSpace(anchorFolderID)
		if anchor != "" {
			folder, err := s.repository.FindPublicFolderByID(ctx, anchor)
			if err != nil {
				return nil, err
			}
			if folder == nil {
				return nil, nil
			}
			id := folder.ID
			parentID = &id
		}
	}

	// 前导 ".."：逐级回到父目录，根目录之上仍留在根。
	for len(segments) > 0 && segments[0] == ".." {
		segments = segments[1:]
		if parentID == nil {
			continue
		}
		folder, err := s.repository.FindPublicFolderByID(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		if folder == nil {
			return nil, nil
		}
		parentID = folder.ParentID
	}
	// 中间出现的 ".." 意味着路径已越出锚点或被手写错位，不做猜测。
	for _, seg := range segments {
		if seg == ".." {
			return nil, nil
		}
	}

	// 只剩 "." 或 ".." 的链接（如 [父目录](../)）指向锚点或其上级目录本身。
	// 已上溯到根之外时没有对应的目录 ID，交由前端保持默认行为。
	if len(segments) == 0 {
		if parentID == nil {
			return nil, nil
		}
		folder, err := s.repository.FindPublicFolderByID(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		if folder == nil {
			return nil, nil
		}
		return &PathResolveResult{
			Type:     "folder",
			FolderID: folder.ID,
			Name:     folder.Name,
		}, nil
	}

	return s.resolveSegmentsFrom(ctx, parentID, segments, folderOnly)
}

// splitCatalogLinkPath 按 "/" 切分并丢弃空段与 "."；保留 ".." 交由调用方逐级上溯。
func splitCatalogLinkPath(path string) []string {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			continue
		}
		segments = append(segments, p)
	}
	return segments
}

// resolveSegmentsFrom 从 parentID（nil 为根）开始逐级匹配子文件夹名称，
// 最后一段默认文件优先；folderOnly 为 true 时只匹配文件夹。
func (s *PublicCatalogService) resolveSegmentsFrom(ctx context.Context, parentID *string, segments []string, folderOnly bool) (*PathResolveResult, error) {
	if len(segments) == 0 {
		return nil, nil
	}

	// 逐级查找中间的文件夹段
	for i := 0; i < len(segments)-1; i++ {
		folder, err := s.repository.FindFolderByParentAndName(ctx, parentID, segments[i])
		if err != nil {
			return nil, fmt.Errorf("resolve path segment %q: %w", segments[i], err)
		}
		if folder == nil {
			return nil, nil // 路径不存在
		}
		id := folder.ID
		parentID = &id
	}

	// 最后一段：优先查找文件，再查找文件夹
	lastSegment := segments[len(segments)-1]

	if parentID != nil && !folderOnly {
		file, err := s.repository.FindFileByFolderAndName(ctx, *parentID, lastSegment)
		if err != nil {
			return nil, fmt.Errorf("resolve file %q: %w", lastSegment, err)
		}
		if file != nil {
			return &PathResolveResult{
				Type:   "file",
				FileID: file.ID,
				Name:   file.Name,
			}, nil
		}
	}

	folder, err := s.repository.FindFolderByParentAndName(ctx, parentID, lastSegment)
	if err != nil {
		return nil, fmt.Errorf("resolve folder %q: %w", lastSegment, err)
	}
	if folder != nil {
		return &PathResolveResult{
			Type:     "folder",
			FolderID: folder.ID,
			Name:     folder.Name,
		}, nil
	}

	return nil, nil
}

type normalizedPublicFolderFileListInput struct {
	FolderID string
	Page     int
	PageSize int
	OrderBy  []string
}

func normalizePublicFolderFileListInput(input PublicFolderFileListInput) (*normalizedPublicFolderFileListInput, error) {
	folderID := strings.TrimSpace(input.FolderID)
	if folderID == "" {
		return nil, ErrInvalidPublicFileQuery
	}

	page := input.Page
	if page == 0 {
		page = defaultPublicFilePage
	}
	if page < 1 {
		return nil, ErrInvalidPublicFileQuery
	}

	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = defaultPublicFilePageSize
	}
	if pageSize < 1 || pageSize > maxPublicFilePageSize {
		return nil, ErrInvalidPublicFileQuery
	}

	orderBy, err := resolvePublicFileSort(input.Sort)
	if err != nil {
		return nil, err
	}

	return &normalizedPublicFolderFileListInput{
		FolderID: folderID,
		Page:     page,
		PageSize: pageSize,
		OrderBy:  orderBy,
	}, nil
}

func resolvePublicFileSort(sort string) ([]string, error) {
	switch strings.TrimSpace(sort) {
	case "", "created_at_desc":
		return []string{"created_at DESC", "id DESC"}, nil
	case "created_at_asc":
		return []string{"created_at ASC", "id ASC"}, nil
	case "download_count_desc":
		return []string{"download_count DESC", "created_at DESC", "id DESC"}, nil
	case "download_count_asc":
		return []string{"download_count ASC", "created_at ASC", "id ASC"}, nil
	case "name_asc":
		return []string{"name ASC", "created_at DESC", "id DESC"}, nil
	case "name_desc":
		return []string{"name DESC", "created_at DESC", "id DESC"}, nil
	default:
		return nil, ErrInvalidPublicFileQuery
	}
}

func (s *PublicCatalogService) listManagedFileFeed(ctx context.Context, limit int, orderBy []string) (*PublicFileFeedResult, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 20
	}
	if normalizedLimit > maxPublicFilePageSize {
		normalizedLimit = maxPublicFilePageSize
	}

	files, err := s.repository.ListManagedFileFeed(ctx, repository.PublicFileFeedQuery{
		Limit:   normalizedLimit,
		OrderBy: orderBy,
	})
	if err != nil {
		return nil, fmt.Errorf("list managed file feed: %w", err)
	}

	mapped, err := s.mapPublicFileItems(ctx, files)
	if err != nil {
		return nil, err
	}
	return &PublicFileFeedResult{
		Items: mapped,
	}, nil
}

func (s *PublicCatalogService) mapPublicFileItems(ctx context.Context, files []model.File) ([]PublicFileItem, error) {
	fileIDs := make([]string, 0, len(files))
	for _, file := range files {
		fileIDs = append(fileIDs, file.ID)
	}
	tagMap := map[string][]PublicFileTag{}
	if s.fileTags != nil && len(fileIDs) > 0 {
		m, err := s.fileTags.MapTagsByFileIDs(ctx, fileIDs)
		if err != nil {
			return nil, err
		}
		tagMap = m
	}

	// 批量预取文件夹祖先链，避免对同目录下的文件重复查询
	var ancestorCache *folderAncestorCache
	if s.download != nil && len(files) > 0 {
		folderIDSet := make(map[string]struct{}, len(files))
		for _, f := range files {
			if f.FolderID != nil {
				folderIDSet[strings.TrimSpace(*f.FolderID)] = struct{}{}
			}
		}
		folderIDs := make([]string, 0, len(folderIDSet))
		for fid := range folderIDSet {
			if fid != "" {
				folderIDs = append(folderIDs, fid)
			}
		}
		var err error
		ancestorCache, err = s.download.newFolderAncestorCache(ctx, folderIDs)
		if err != nil {
			return nil, err
		}
	}

	items := make([]PublicFileItem, 0, len(files))
	for _, file := range files {
		fd := ""
		allowed := true
		if ancestorCache != nil {
			fd = ancestorCache.folderDirectDownloadURLForFile(file)
			allowed = ancestorCache.effectiveDownloadAllowedForFile(&file)
		}
		tags := tagMap[file.ID]
		if tags == nil {
			tags = []PublicFileTag{}
		}
		items = append(items, PublicFileItem{
			ID:                      file.ID,
			Name:                    file.Name,
			Description:             file.Description,
			Remark:                  file.Remark,
			Extension:               file.Extension,
			CoverURL:                effectiveFileCoverURL(file.CoverURL, file.Extension, file.ID),
			PlaybackURL:             strings.TrimSpace(file.PlaybackURL),
			ProxyDownload:           file.ProxyDownload,
			ProxySourceURL:          strings.TrimSpace(file.ProxySourceURL),
			FolderDirectDownloadURL: fd,
			DownloadAllowed:         allowed,
			UploadedAt:              file.CreatedAt,
			DownloadCount:           file.DownloadCount,
			Size:                    file.Size,
			Tags:                    tags,
		})
	}
	return items, nil
}
