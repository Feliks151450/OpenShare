package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

var (
	ErrInvalidPublicFileQuery = errors.New("invalid public file query")
	ErrFolderNotFound         = errors.New("folder not found")
)

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
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	Remark                  string    `json:"remark"`
	Extension               string    `json:"extension"`
	CoverURL                string    `json:"cover_url"`
	PlaybackURL             string    `json:"playback_url"`
	FolderDirectDownloadURL string    `json:"folder_direct_download_url"`
	DownloadAllowed         bool      `json:"download_allowed"`
	UploadedAt              time.Time `json:"uploaded_at"`
	DownloadCount           int64     `json:"download_count"`
	Size                    int64     `json:"size"`
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

	files, total, err := s.repository.ListPublicFolderFiles(ctx, repository.PublicFolderFileListQuery{
		FolderID: normalized.FolderID,
		Offset:   (normalized.Page - 1) * normalized.PageSize,
		Limit:    normalized.PageSize,
		OrderBy:  normalized.OrderBy,
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
	if s.download != nil {
		var err error
		dlAllowed, err = s.download.EffectiveDownloadAllowedForFolder(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("resolve folder download policy: %w", err)
		}
	}

	detail := PublicFolderDetail{
		ID:               current.ID,
		Name:             current.Name,
		Description:      current.Description,
		Remark:           current.Remark,
		CoverURL:         strings.TrimSpace(current.CoverURL),
		ParentID:         current.ParentID,
		Breadcrumbs:      breadcrumbs,
		FileCount:        current.FileCount,
		DownloadCount:    current.DownloadCount,
		TotalSize:        current.TotalSize,
		UpdatedAt:        current.UpdatedAt,
		DirectLinkPrefix: strings.TrimSpace(current.DirectLinkPrefix),
		DownloadAllowed:  dlAllowed,
		DownloadPolicy:   DownloadPolicyString(current.AllowDownload),
		IsVirtual:        current.IsVirtual,
	}
	if current.ParentID == nil {
		h := current.HidePublicCatalog
		detail.HidePublicCatalog = &h
	}
	return &detail, nil
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
	case "download_count_desc":
		return []string{"download_count DESC", "created_at DESC", "id DESC"}, nil
	case "name_asc":
		return []string{"name ASC", "created_at DESC", "id DESC"}, nil
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
