package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type ApiTokenRepository struct {
	db *gorm.DB
}

func NewApiTokenRepository(db *gorm.DB) *ApiTokenRepository {
	return &ApiTokenRepository{db: db}
}

func (r *ApiTokenRepository) Create(ctx context.Context, token *model.ApiToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *ApiTokenRepository) ListByAdminID(ctx context.Context, adminID string) ([]model.ApiToken, error) {
	var tokens []model.ApiToken
	if err := r.db.WithContext(ctx).
		Where("admin_id = ?", adminID).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	return tokens, nil
}

func (r *ApiTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*model.ApiToken, error) {
	var token model.ApiToken
	if err := r.db.WithContext(ctx).
		Preload("Admin").
		Where("token_hash = ?", tokenHash).
		Take(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find api token by hash: %w", err)
	}
	return &token, nil
}

func (r *ApiTokenRepository) UpdateLastUsed(ctx context.Context, tokenID string, now time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.ApiToken{}).
		Where("id = ?", tokenID).
		Update("last_used_at", now).Error
}

func (r *ApiTokenRepository) Delete(ctx context.Context, tokenID string, adminID string) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND admin_id = ?", tokenID, adminID).
		Delete(&model.ApiToken{})
	if result.Error != nil {
		return fmt.Errorf("delete api token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
