package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/session"
	"openshare/backend/pkg/identity"
)

var ErrInvalidAdminCredentials = errors.New("invalid admin credentials")
var ErrInvalidAdminPasswordChange = errors.New("invalid admin password change")
var ErrInvalidAdminProfileUpdate = errors.New("invalid admin profile update")
var ErrAdminDisplayNameTaken = errors.New("admin display name already exists")

const maxAdminAvatarDataURLLength = 2_800_000

type AdminAuthService struct {
	db             *gorm.DB
	adminRepo      *repository.AdminRepository
	sessionManager *session.Manager
}

type AuthenticatedAdmin struct {
	Admin    *model.Admin
	Identity session.AdminIdentity
	Cookie   string
}

func NewAdminAuthService(
	db *gorm.DB,
	adminRepo *repository.AdminRepository,
	sessionManager *session.Manager,
) *AdminAuthService {
	return &AdminAuthService{
		db:             db,
		adminRepo:      adminRepo,
		sessionManager: sessionManager,
	}
}

func (s *AdminAuthService) Login(ctx context.Context, username, password string) (*AuthenticatedAdmin, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrInvalidAdminCredentials
	}

	admin, err := s.adminRepo.FindByUsername(s.db.WithContext(ctx), username)
	if err != nil {
		return nil, fmt.Errorf("find admin by username: %w", err)
	}
	if admin == nil {
		return nil, ErrInvalidAdminCredentials
	}
	if admin.Status != model.AdminStatusActive {
		return nil, ErrInvalidAdminCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, ErrInvalidAdminCredentials
		}
		return nil, fmt.Errorf("compare admin password: %w", err)
	}

	cookieValue, identity, err := s.sessionManager.Create(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("create admin session: %w", err)
	}

	return &AuthenticatedAdmin{
		Admin:    admin,
		Identity: identity,
		Cookie:   cookieValue,
	}, nil
}

func (s *AdminAuthService) Logout(ctx context.Context, cookieValue string) error {
	if err := s.sessionManager.Destroy(ctx, cookieValue); err != nil {
		return fmt.Errorf("destroy admin session: %w", err)
	}

	return nil
}

func (s *AdminAuthService) ChangePassword(ctx context.Context, adminID, newPassword, operatorIP string) error {
	adminID = strings.TrimSpace(adminID)
	if adminID == "" || len(newPassword) < 8 {
		return ErrInvalidAdminPasswordChange
	}

	admin, err := s.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return fmt.Errorf("find admin for password change: %w", err)
	}
	if admin == nil || admin.Status != model.AdminStatusActive {
		return ErrInvalidAdminCredentials
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new admin password: %w", err)
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate password change log id: %w", err)
	}
	now := time.Now().UTC()
	if err := s.adminRepo.UpdateAdminWithLog(ctx, admin.ID, map[string]any{
		"password_hash": string(hashed),
		"updated_at":    now,
	}, admin.ID, operatorIP, "admin_password_changed", admin.Username, logID, now); err != nil {
		return fmt.Errorf("persist admin password change: %w", err)
	}
	return nil
}

func (s *AdminAuthService) GetProfile(ctx context.Context, adminID string) (*model.Admin, error) {
	adminID = strings.TrimSpace(adminID)
	if adminID == "" {
		return nil, ErrInvalidAdminCredentials
	}

	admin, err := s.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("find admin profile: %w", err)
	}
	if admin == nil || admin.Status != model.AdminStatusActive {
		return nil, nil
	}
	return admin, nil
}

func (s *AdminAuthService) VerifyPassword(ctx context.Context, adminID, password string) error {
	adminID = strings.TrimSpace(adminID)
	if adminID == "" || password == "" {
		return ErrInvalidAdminCredentials
	}

	admin, err := s.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return fmt.Errorf("find admin for password verify: %w", err)
	}
	if admin == nil || admin.Status != model.AdminStatusActive {
		return ErrInvalidAdminCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidAdminCredentials
		}
		return fmt.Errorf("compare admin password: %w", err)
	}

	return nil
}

func (s *AdminAuthService) UpdateProfile(ctx context.Context, adminID, displayName, avatarURL, operatorIP string) (*model.Admin, error) {
	adminID = strings.TrimSpace(adminID)
	displayName = strings.TrimSpace(displayName)
	avatarURL = strings.TrimSpace(avatarURL)
	if adminID == "" || len([]rune(displayName)) == 0 || len([]rune(displayName)) > 40 || !isValidAvatarURL(avatarURL) {
		return nil, ErrInvalidAdminProfileUpdate
	}

	admin, err := s.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("find admin for profile update: %w", err)
	}
	if admin == nil || admin.Status != model.AdminStatusActive {
		return nil, ErrInvalidAdminCredentials
	}

	exists, err := s.adminRepo.DisplayNameExists(ctx, displayName, admin.ID)
	if err != nil {
		return nil, fmt.Errorf("check duplicate admin display name: %w", err)
	}
	if exists {
		return nil, ErrAdminDisplayNameTaken
	}

	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate profile update log id: %w", err)
	}

	now := time.Now().UTC()
	if err := s.adminRepo.UpdateAdminWithLog(ctx, admin.ID, map[string]any{
		"display_name": displayName,
		"avatar_url":   avatarURL,
		"updated_at":   now,
	}, admin.ID, operatorIP, "admin_profile_updated", displayName, logID, now); err != nil {
		return nil, fmt.Errorf("persist admin profile update: %w", err)
	}

	updated, err := s.adminRepo.FindByID(ctx, admin.ID)
	if err != nil {
		return nil, fmt.Errorf("reload updated admin profile: %w", err)
	}
	return updated, nil
}

func isValidAvatarURL(raw string) bool {
	if raw == "" {
		return true
	}
	// data:image/... base64
	if strings.HasPrefix(raw, "data:image/") {
		return len(raw) <= maxAdminAvatarDataURLLength
	}
	// http(s) 直链
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return len(raw) <= maxAdminAvatarDataURLLength
	}
	return false
}
