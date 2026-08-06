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

// GetDB 暴露底层 gorm.DB 句柄（用于自定义轻量查询场景）。
func (r *PublicCatalogRepository) GetDB() *gorm.DB {
	return r.db
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
		Scopes(FilesNotUnderHiddenPublicCatalogRoot(), FilesNotUnderGuestKeyRequired())
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
		Scopes(FilesNotUnderHiddenPublicCatalogRoot(), FilesNotUnderGuestKeyRequired()).
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
	// 访客密钥访问：凡是自身或任意祖先设置了 guest_key_required 的目录都对访客不可见。
	query = query.Scopes(FoldersNotUnderGuestKeyRequired())

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

// FindFolderByParentAndName 根据父文件夹 ID 和名称查找子文件夹。
// parentID 为 nil 时查找根目录（不过滤 hide_public_catalog，与直链直达策略一致）。
// name 匹配为精确匹配（区分大小写，与 SQLite 默认行为一致）。
func (r *PublicCatalogRepository) FindFolderByParentAndName(ctx context.Context, parentID *string, name string) (*model.Folder, error) {
	query := r.db.WithContext(ctx).Model(&model.Folder{})
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	query = query.Where("name = ?", name)

	var folder model.Folder
	if err := query.Take(&folder).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find folder by parent and name: %w", err)
	}
	return &folder, nil
}

// FindFileByFolderAndName 根据文件夹 ID 和文件名查找文件。
// name 为精确匹配（含扩展名，如 "report.pdf"）。
func (r *PublicCatalogRepository) FindFileByFolderAndName(ctx context.Context, folderID string, name string) (*model.File, error) {
	var file model.File
	err := r.db.WithContext(ctx).
		Where("folder_id = ? AND name = ?", folderID, name).
		Take(&file).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find file by folder and name: %w", err)
	}
	return &file, nil
}
