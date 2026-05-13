package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type PublicCatalogRepository struct {
	db *gorm.DB
}

type PublicFolderFileListQuery struct {
	FolderID string
	Offset   int
	Limit    int
	OrderBy  []string
}

type PublicFileFeedQuery struct {
	Limit   int
	OrderBy []string
}

type PublicHotFileFeedQuery struct {
	SinceDay string
	Limit    int
}

type PublicFolderRow struct {
	ID            string
	ParentID      *string
	Name          string
	Description   string
	Remark        string
	CoverURL      string
	CdnURL        string
	AllowDownload *bool
	IsVirtual     bool
	UpdatedAt     time.Time
	FileCount     int64
	DownloadCount int64
	TotalSize     int64
}

func NewPublicCatalogRepository(db *gorm.DB) *PublicCatalogRepository {
	return &PublicCatalogRepository{db: db}
}

func (r *PublicCatalogRepository) ListPublicFolderFiles(ctx context.Context, query PublicFolderFileListQuery) ([]model.File, int64, error) {
	base := r.db.WithContext(ctx).
		Model(&model.File{}).
		Where("folder_id = ?", query.FolderID)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count public files: %w", err)
	}

	listQuery := base
	for _, orderBy := range query.OrderBy {
		listQuery = listQuery.Order(orderBy)
	}

	var files []model.File
	if err := listQuery.Offset(query.Offset).Limit(query.Limit).Find(&files).Error; err != nil {
		return nil, 0, fmt.Errorf("list public files: %w", err)
	}

	return files, total, nil
}

func (r *PublicCatalogRepository) ListManagedFileFeed(ctx context.Context, query PublicFileFeedQuery) ([]model.File, error) {
	listQuery := r.db.WithContext(ctx).
		Model(&model.File{}).
		Scopes(FilesNotUnderHiddenPublicCatalogRoot())
	for _, orderBy := range query.OrderBy {
		listQuery = listQuery.Order(orderBy)
	}

	var files []model.File
	if err := listQuery.Limit(query.Limit).Find(&files).Error; err != nil {
		return nil, fmt.Errorf("list managed file feed: %w", err)
	}
	return files, nil
}

func (r *PublicCatalogRepository) ListRecentHotManagedFiles(ctx context.Context, query PublicHotFileFeedQuery) ([]model.File, error) {
	aggregated := r.db.WithContext(ctx).
		Model(&model.FileDailyDownload{}).
		Select("file_id, SUM(downloads) AS hot_downloads").
		Where("day >= ?", query.SinceDay).
		Group("file_id")

	var files []model.File
	if err := r.db.WithContext(ctx).
		Model(&model.File{}).
		Scopes(FilesNotUnderHiddenPublicCatalogRoot()).
		Select("files.*").
		Joins("JOIN (?) AS hot ON hot.file_id = files.id", aggregated).
		Order("hot.hot_downloads DESC").
		Order("files.created_at DESC").
		Order("files.id DESC").
		Limit(query.Limit).
		Find(&files).Error; err != nil {
		return nil, fmt.Errorf("list recent hot managed files: %w", err)
	}
	return files, nil
}

func (r *PublicCatalogRepository) FolderExists(ctx context.Context, folderID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Where("id = ?", folderID).
		Count(&count).
		Error
	if err != nil {
		return false, fmt.Errorf("check folder existence: %w", err)
	}

	return count > 0, nil
}

func (r *PublicCatalogRepository) ListPublicFolders(ctx context.Context, parentID *string) ([]PublicFolderRow, error) {
	query := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Select("id, parent_id, name, description, remark, cover_url, cdn_url, allow_download, is_virtual, updated_at, file_count, download_count, total_size")

	if parentID == nil {
		query = query.Where("parent_id IS NULL AND hide_public_catalog = ?", false)
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	var rows []PublicFolderRow
	if err := query.Order("name ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list public folders: %w", err)
	}

	return rows, nil
}

// FindPublicFolderByCustomPath 通过自定义路径查找文件夹。path 为空或未找到时返回 nil。
func (r *PublicCatalogRepository) FindPublicFolderByCustomPath(ctx context.Context, customPath string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("custom_path = ?", customPath).
		Take(&folder).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find public folder by custom path: %w", err)
	}
	return &folder, nil
}

// FindPublicFileByCustomPath 通过自定义路径查找文件。path 为空或未找到时返回 nil。
func (r *PublicCatalogRepository) FindPublicFileByCustomPath(ctx context.Context, customPath string) (*model.File, error) {
	var file model.File
	err := r.db.WithContext(ctx).
		Where("custom_path = ?", customPath).
		Take(&file).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find public file by custom path: %w", err)
	}
	return &file, nil
}

// CustomPathExists 检查 custom_path 是否已被使用（同时检查 folders 和 files 表，可排除指定 ID）。
func (r *PublicCatalogRepository) CustomPathExists(ctx context.Context, customPath string, excludeID string) (bool, error) {
	if customPath == "" {
		return false, nil
	}
	// 检查 folders 表
	{
		var count int64
		query := r.db.WithContext(ctx).Model(&model.Folder{}).Where("custom_path = ?", customPath)
		if excludeID != "" {
			query = query.Where("id != ?", excludeID)
		}
		if err := query.Count(&count).Error; err != nil {
			return false, fmt.Errorf("check custom path in folders: %w", err)
		}
		if count > 0 {
			return true, nil
		}
	}
	// 检查 files 表
	{
		var count int64
		query := r.db.WithContext(ctx).Model(&model.File{}).Where("custom_path = ?", customPath)
		if excludeID != "" {
			query = query.Where("id != ?", excludeID)
		}
		if err := query.Count(&count).Error; err != nil {
			return false, fmt.Errorf("check custom path in files: %w", err)
		}
		return count > 0, nil
	}
}

func (r *PublicCatalogRepository) FindPublicFolderByID(ctx context.Context, folderID string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("id = ?", folderID).
		Take(&folder).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find public folder: %w", err)
	}

	return &folder, nil
}
