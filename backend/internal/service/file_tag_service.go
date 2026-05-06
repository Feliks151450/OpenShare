package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

var (
	ErrFileTagInvalidInput = errors.New("invalid file tag input")
	ErrFileTagNotFound     = errors.New("file tag not found")
	ErrFileTagNameConflict = errors.New("file tag name already exists")
)

var fileTagHexColorRe = regexp.MustCompile(`^#([0-9a-f]{3}|[0-9a-f]{6})$`)

func normalizePresetTagColor(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return "#64748b", nil
	}
	if !fileTagHexColorRe.MatchString(s) {
		return "", ErrFileTagInvalidInput
	}
	if len(s) == 4 {
		return fmt.Sprintf("#%c%c%c%c%c%c", s[1], s[1], s[2], s[2], s[3], s[3]), nil
	}
	return s, nil
}

type PublicFileTag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type FileTagDefinitionDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

type FileTagService struct {
	tags      *repository.FileTagRepository
	resources *repository.ResourceManagementRepository
}

func NewFileTagService(
	tags *repository.FileTagRepository,
	resources *repository.ResourceManagementRepository,
) *FileTagService {
	return &FileTagService{tags: tags, resources: resources}
}

func rowToPublicTag(row repository.PublicFileTagRow) PublicFileTag {
	return PublicFileTag{ID: row.ID, Name: row.Name, Color: row.Color}
}

func (s *FileTagService) ListDefinitions(ctx context.Context) ([]FileTagDefinitionDTO, error) {
	if s == nil || s.tags == nil {
		return []FileTagDefinitionDTO{}, nil
	}
	rows, err := s.tags.ListAllDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FileTagDefinitionDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, FileTagDefinitionDTO{
			ID:        row.ID,
			Name:      row.Name,
			Color:     row.Color,
			SortOrder: row.SortOrder,
		})
	}
	return out, nil
}

func (s *FileTagService) MapTagsByFileIDs(ctx context.Context, fileIDs []string) (map[string][]PublicFileTag, error) {
	if s == nil || s.tags == nil || len(fileIDs) == 0 {
		return map[string][]PublicFileTag{}, nil
	}
	raw, err := s.tags.MapTagsByFileIDs(ctx, fileIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]PublicFileTag, len(raw))
	for fid, rows := range raw {
		tags := make([]PublicFileTag, 0, len(rows))
		for _, r := range rows {
			tags = append(tags, rowToPublicTag(r))
		}
		out[fid] = tags
	}
	return out, nil
}

func (s *FileTagService) ListTagsForFile(ctx context.Context, fileID string) ([]PublicFileTag, error) {
	if s == nil || s.tags == nil {
		return []PublicFileTag{}, nil
	}
	rows, err := s.tags.ListTagRowsForFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	out := make([]PublicFileTag, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToPublicTag(r))
	}
	return out, nil
}

func (s *FileTagService) AdminCreateTag(ctx context.Context, name, color string, sortOrder int) (*FileTagDefinitionDTO, error) {
	if s == nil || s.tags == nil || s.resources == nil {
		return nil, fmt.Errorf("file tag service unavailable")
	}
	n := strings.TrimSpace(name)
	if n == "" {
		return nil, ErrFileTagInvalidInput
	}
	c, err := normalizePresetTagColor(color)
	if err != nil {
		return nil, err
	}
	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate tag id: %w", err)
	}
	tag := &model.FileTag{
		ID:        id,
		Name:      n,
		Color:     c,
		SortOrder: sortOrder,
	}
	if err := s.tags.CreateDefinition(ctx, tag); err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrFileTagNameConflict
		}
		return nil, err
	}
	return &FileTagDefinitionDTO{
		ID:        tag.ID,
		Name:      tag.Name,
		Color:     tag.Color,
		SortOrder: tag.SortOrder,
	}, nil
}

func (s *FileTagService) AdminUpdateTag(ctx context.Context, tagID, name, color string, sortOrder int) error {
	if s == nil || s.tags == nil {
		return fmt.Errorf("file tag service unavailable")
	}
	n := strings.TrimSpace(name)
	if n == "" {
		return ErrFileTagInvalidInput
	}
	c, err := normalizePresetTagColor(color)
	if err != nil {
		return err
	}
	tid := strings.TrimSpace(tagID)
	if tid == "" {
		return ErrFileTagInvalidInput
	}
	if err := s.tags.UpdateDefinition(ctx, tid, n, c, sortOrder, time.Now().UTC()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrFileTagNotFound
		}
		if isUniqueConstraintErr(err) {
			return ErrFileTagNameConflict
		}
		return err
	}
	return nil
}

func (s *FileTagService) AdminDeleteTag(ctx context.Context, tagID string) error {
	if s == nil || s.tags == nil {
		return fmt.Errorf("file tag service unavailable")
	}
	tid := strings.TrimSpace(tagID)
	if tid == "" {
		return ErrFileTagInvalidInput
	}
	if err := s.tags.DeleteDefinition(ctx, tid); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrFileTagNotFound
		}
		return err
	}
	return nil
}

func (s *FileTagService) ReplaceManagedFileTags(ctx context.Context, fileID string, tagIDs []string) error {
	if s == nil || s.tags == nil || s.resources == nil {
		return fmt.Errorf("file tag service unavailable")
	}
	fid := strings.TrimSpace(fileID)
	if fid == "" {
		return ErrManagedFileNotFound
	}
	current, err := s.resources.FindFileByID(ctx, fid)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}
	ok, err := s.tags.ExistsDefinitions(ctx, tagIDs)
	if err != nil {
		return err
	}
	if !ok {
		return ErrFileTagInvalidInput
	}
	return s.tags.ReplaceFileTags(ctx, fid, tagIDs)
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") && strings.Contains(msg, "file_tags")
}
