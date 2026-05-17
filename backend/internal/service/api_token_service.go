package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/session"
	"openshare/backend/pkg/identity"
)

type ApiTokenListItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ApiTokenCreateResult struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"` // 仅创建时返回一次
	CreatedAt time.Time `json:"created_at"`
}

type ApiTokenService struct {
	repo *repository.ApiTokenRepository
}

func NewApiTokenService(repo *repository.ApiTokenRepository) *ApiTokenService {
	return &ApiTokenService{repo: repo}
}

func (s *ApiTokenService) List(ctx context.Context, adminID string) ([]ApiTokenListItem, error) {
	tokens, err := s.repo.ListByAdminID(ctx, adminID)
	if err != nil {
		return nil, err
	}
	items := make([]ApiTokenListItem, len(tokens))
	for i, t := range tokens {
		items[i] = ApiTokenListItem{
			ID:         t.ID,
			Name:       t.Name,
			LastUsedAt: t.LastUsedAt,
			CreatedAt:  t.CreatedAt,
		}
	}
	return items, nil
}

func (s *ApiTokenService) Create(ctx context.Context, adminID, name string) (*ApiTokenCreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "API Token"
	}

	rawToken, err := generateApiToken()
	if err != nil {
		return nil, fmt.Errorf("generate api token: %w", err)
	}

	tokenID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate api token id: %w", err)
	}

	now := time.Now().UTC()
	tokenModel := &model.ApiToken{
		ID:        tokenID,
		AdminID:   adminID,
		Name:      name,
		TokenHash: hashApiToken(rawToken),
		CreatedAt: now,
	}
	if err := s.repo.Create(ctx, tokenModel); err != nil {
		return nil, fmt.Errorf("create api token: %w", err)
	}

	return &ApiTokenCreateResult{
		ID:        tokenID,
		Name:      name,
		Token:     rawToken,
		CreatedAt: now,
	}, nil
}

func (s *ApiTokenService) Delete(ctx context.Context, tokenID, adminID string) error {
	return s.repo.Delete(ctx, tokenID, adminID)
}

// ResolveByTokenHash validates a bearer token against the api_tokens table.
// Returns the associated admin identity, or nil if not found.
func (s *ApiTokenService) ResolveByTokenHash(ctx context.Context, rawToken string) (*session.AdminIdentity, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, nil
	}

	tokenModel, err := s.repo.FindByTokenHash(ctx, hashApiToken(rawToken))
	if err != nil {
		return nil, fmt.Errorf("resolve api token: %w", err)
	}
	if tokenModel == nil {
		return nil, nil
	}
	if tokenModel.Admin == nil || tokenModel.Admin.Status != model.AdminStatusActive {
		return nil, nil
	}

	// 更新最后使用时间（非关键路径，失败不阻断）
	_ = s.repo.UpdateLastUsed(ctx, tokenModel.ID, time.Now().UTC())

	return &session.AdminIdentity{
		AdminID:     tokenModel.Admin.ID,
		Username:    tokenModel.Admin.Username,
		Role:        tokenModel.Admin.Role,
		Status:      tokenModel.Admin.Status,
		Permissions: tokenModel.Admin.PermissionList(),
	}, nil
}

func generateApiToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "osk_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func hashApiToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
