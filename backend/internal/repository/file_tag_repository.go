package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type FileTagRepository struct {
	db *gorm.DB
}

func NewFileTagRepository(db *gorm.DB) *FileTagRepository {
	return &FileTagRepository{db: db}
}

type PublicFileTagRow struct {
	ID    string
	Name  string
	Color string
}

type fileTagJoinRow struct {
	FileID    string
	TagID     string
	TagName   string
	TagColor  string
	SortOrder int
}

func (r *FileTagRepository) ListAllDefinitions(ctx context.Context) ([]model.FileTag, error) {
	var rows []model.FileTag
	if err := r.db.WithContext(ctx).
		Model(&model.FileTag{}).
		Order("sort_order ASC, name ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list file tags: %w", err)
	}
	return rows, nil
}

func (r *FileTagRepository) FindDefinitionByID(ctx context.Context, tagID string) (*model.FileTag, error) {
	trimmed := strings.TrimSpace(tagID)
	if trimmed == "" {
		return nil, nil
	}
	var row model.FileTag
	if err := r.db.WithContext(ctx).Where("id = ?", trimmed).Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find file tag: %w", err)
	}
	return &row, nil
}

func (r *FileTagRepository) ExistsDefinitions(ctx context.Context, tagIDs []string) (bool, error) {
	unique := dedupeNonEmpty(tagIDs)
	if len(unique) == 0 {
		return true, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.FileTag{}).Where("id IN ?", unique).Count(&count).Error; err != nil {
		return false, fmt.Errorf("count file tags: %w", err)
	}
	return int(count) == len(unique), nil
}

func (r *FileTagRepository) CreateDefinition(ctx context.Context, tag *model.FileTag) error {
	if err := r.db.WithContext(ctx).Create(tag).Error; err != nil {
		return fmt.Errorf("create file tag: %w", err)
	}
	return nil
}

func (r *FileTagRepository) UpdateDefinition(ctx context.Context, tagID string, name, color string, sortOrder int, now time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.FileTag{}).
		Where("id = ?", strings.TrimSpace(tagID)).
		Updates(map[string]any{
			"name":       strings.TrimSpace(name),
			"color":      color,
			"sort_order": sortOrder,
			"updated_at": now,
		})
	if res.Error != nil {
		return fmt.Errorf("update file tag: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *FileTagRepository) DeleteDefinition(ctx context.Context, tagID string) error {
	tid := strings.TrimSpace(tagID)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tag_id = ?", tid).Delete(&model.FileTagAssignment{}).Error; err != nil {
			return fmt.Errorf("clear assignments for tag: %w", err)
		}
		res := tx.Where("id = ?", tid).Delete(&model.FileTag{})
		if res.Error != nil {
			return fmt.Errorf("delete file tag: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *FileTagRepository) MapTagsByFileIDs(ctx context.Context, fileIDs []string) (map[string][]PublicFileTagRow, error) {
	out := make(map[string][]PublicFileTagRow)
	if len(fileIDs) == 0 {
		return out, nil
	}
	var rows []fileTagJoinRow
	if err := r.db.WithContext(ctx).Table("file_tag_assignments AS a").
		Select("a.file_id, t.id AS tag_id, t.name AS tag_name, t.color AS tag_color, t.sort_order").
		Joins("JOIN file_tags t ON t.id = a.tag_id").
		Where("a.file_id IN ?", fileIDs).
		Order("t.sort_order ASC, t.name ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list tags for files: %w", err)
	}
	for _, row := range rows {
		out[row.FileID] = append(out[row.FileID], PublicFileTagRow{
			ID:    row.TagID,
			Name:  row.TagName,
			Color: row.TagColor,
		})
	}
	return out, nil
}

func (r *FileTagRepository) ListTagRowsForFile(ctx context.Context, fileID string) ([]PublicFileTagRow, error) {
	fid := strings.TrimSpace(fileID)
	if fid == "" {
		return []PublicFileTagRow{}, nil
	}
	m, err := r.MapTagsByFileIDs(ctx, []string{fid})
	if err != nil {
		return nil, err
	}
	return m[fid], nil
}

func (r *FileTagRepository) ReplaceFileTags(ctx context.Context, fileID string, tagIDs []string) error {
	fid := strings.TrimSpace(fileID)
	if fid == "" {
		return fmt.Errorf("empty file id")
	}
	unique := dedupeNonEmpty(tagIDs)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("file_id = ?", fid).Delete(&model.FileTagAssignment{}).Error; err != nil {
			return fmt.Errorf("clear file tags: %w", err)
		}
		for _, tid := range unique {
			row := model.FileTagAssignment{FileID: fid, TagID: tid}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("insert file tag assignment: %w", err)
			}
		}
		return nil
	})
}

func dedupeNonEmpty(ids []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
