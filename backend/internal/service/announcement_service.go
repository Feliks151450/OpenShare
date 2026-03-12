package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

var (
	ErrAnnouncementNotFound     = errors.New("announcement not found")
	ErrAnnouncementInvalidInput = errors.New("invalid announcement input")
)

type AnnouncementService struct {
	repo    *repository.AnnouncementRepository
	nowFunc func() time.Time
}

type AnnouncementItem struct {
	ID          string                     `json:"id"`
	Title       string                     `json:"title"`
	Content     string                     `json:"content"`
	Status      model.AnnouncementStatus   `json:"status"`
	PublishedAt *time.Time                 `json:"published_at,omitempty"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}

type SaveAnnouncementInput struct {
	Title      string
	Content    string
	Status     model.AnnouncementStatus
	OperatorID string
	OperatorIP string
}

func NewAnnouncementService(repo *repository.AnnouncementRepository) *AnnouncementService {
	return &AnnouncementService{
		repo:    repo,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
}

func (s *AnnouncementService) ListPublic(ctx context.Context) ([]AnnouncementItem, error) {
	items, err := s.repo.ListPublic(ctx)
	if err != nil {
		return nil, err
	}
	return mapAnnouncements(items), nil
}

func (s *AnnouncementService) ListAdmin(ctx context.Context) ([]AnnouncementItem, error) {
	items, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return mapAnnouncements(items), nil
}

func (s *AnnouncementService) Create(ctx context.Context, input SaveAnnouncementInput) (*AnnouncementItem, error) {
	title, content, status, err := normalizeAnnouncementInput(input.Title, input.Content, input.Status)
	if err != nil {
		return nil, err
	}

	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate announcement id: %w", err)
	}
	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate announcement log id: %w", err)
	}
	now := s.nowFunc()
	item := &model.Announcement{
		ID:          id,
		Title:       title,
		Content:     content,
		Status:      status,
		CreatedByID: input.OperatorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if status == model.AnnouncementStatusPublished {
		item.PublishedAt = &now
	}

	if err := s.repo.CreateWithLog(ctx, item, input.OperatorID, input.OperatorIP, logID); err != nil {
		return nil, fmt.Errorf("create announcement: %w", err)
	}

	result := mapAnnouncements([]model.Announcement{*item})
	return &result[0], nil
}

func (s *AnnouncementService) Update(ctx context.Context, id string, input SaveAnnouncementInput) (*AnnouncementItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrAnnouncementNotFound
	}
	current, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrAnnouncementNotFound
	}

	title, content, status, err := normalizeAnnouncementInput(input.Title, input.Content, input.Status)
	if err != nil {
		return nil, err
	}

	now := s.nowFunc()
	updates := map[string]any{
		"title":      title,
		"content":    content,
		"status":     status,
		"updated_at": now,
	}
	switch status {
	case model.AnnouncementStatusPublished:
		if current.PublishedAt == nil {
			updates["published_at"] = &now
		}
	case model.AnnouncementStatusDraft, model.AnnouncementStatusHidden:
		updates["published_at"] = current.PublishedAt
	}

	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate announcement log id: %w", err)
	}
	if err := s.repo.UpdateWithLog(ctx, id, updates, input.OperatorID, input.OperatorIP, title, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, fmt.Errorf("update announcement: %w", err)
	}

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	result := mapAnnouncements([]model.Announcement{*updated})
	return &result[0], nil
}

func (s *AnnouncementService) Delete(ctx context.Context, id string, operatorID string, operatorIP string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrAnnouncementNotFound
	}
	current, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrAnnouncementNotFound
	}
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate announcement log id: %w", err)
	}
	now := s.nowFunc()
	if err := s.repo.SoftDeleteWithLog(ctx, id, operatorID, operatorIP, current.Title, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAnnouncementNotFound
		}
		return fmt.Errorf("delete announcement: %w", err)
	}
	return nil
}

func normalizeAnnouncementInput(title string, content string, status model.AnnouncementStatus) (string, string, model.AnnouncementStatus, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return "", "", "", ErrAnnouncementInvalidInput
	}
	switch status {
	case model.AnnouncementStatusDraft, model.AnnouncementStatusPublished, model.AnnouncementStatusHidden:
	default:
		return "", "", "", ErrAnnouncementInvalidInput
	}
	return title, content, status, nil
}

func mapAnnouncements(items []model.Announcement) []AnnouncementItem {
	result := make([]AnnouncementItem, 0, len(items))
	for _, item := range items {
		result = append(result, AnnouncementItem{
			ID:          item.ID,
			Title:       item.Title,
			Content:     item.Content,
			Status:      item.Status,
			PublishedAt: item.PublishedAt,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	return result
}
