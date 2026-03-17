package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"

	"openshare/backend/internal/bootstrap"
	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/database"
	"openshare/backend/pkg/identity"
)

func TestCreateSubmissionReusesExistingReceiptCode(t *testing.T) {
	cfg, db, storageService := newUploadTestDeps(t)
	repo := repository.NewUploadRepository(db)
	service := NewPublicUploadService(cfg.Upload, repo, NewReceiptCodeService(repository.NewReceiptCodeRepository(db), cfg.Upload.ReceiptCodeLength), storageService, nil)
	folderID := createUploadTargetFolder(t, db)

	createExistingSubmission(t, db, "CUSTOM123")

	result, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		ReceiptCode: "CUSTOM123",
		FolderID:    folderID,
		Files: []PublicUploadFileInput{
			{
				OriginalName: "notes.pdf",
				DeclaredMIME: "application/pdf",
				File:         strings.NewReader("%PDF-1.4 test document"),
			},
		},
	})
	if err != nil {
		t.Fatalf("expected success when reusing receipt code, got %v", err)
	}
	if result.ReceiptCode != "CUSTOM123" {
		t.Fatalf("expected receipt code CUSTOM123, got %s", result.ReceiptCode)
	}
}

func TestCreateSubmissionReturnsReceiptGenerationError(t *testing.T) {
	cfg, db, storageService := newUploadTestDeps(t)
	repo := repository.NewUploadRepository(db)
	service := NewPublicUploadService(cfg.Upload, repo, NewReceiptCodeService(repository.NewReceiptCodeRepository(db), cfg.Upload.ReceiptCodeLength), storageService, nil)
	folderID := createUploadTargetFolder(t, db)
	service.receiptCodes.codeGen = func(int) (string, error) {
		return "", errors.New("entropy unavailable")
	}

	_, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		FolderID: folderID,
		Files: []PublicUploadFileInput{
			{
				OriginalName: "notes.pdf",
				DeclaredMIME: "application/pdf",
				File:         strings.NewReader("%PDF-1.4 test document"),
			},
		},
	})
	if !errors.Is(err, ErrReceiptCodeGenerate) {
		t.Fatalf("expected receipt generation error, got %v", err)
	}
}

func TestCreateSubmissionIgnoresDSStoreFiles(t *testing.T) {
	cfg, db, storageService := newUploadTestDeps(t)
	repo := repository.NewUploadRepository(db)
	service := NewPublicUploadService(cfg.Upload, repo, NewReceiptCodeService(repository.NewReceiptCodeRepository(db), cfg.Upload.ReceiptCodeLength), storageService, nil)
	folderID := createUploadTargetFolder(t, db)

	result, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		FolderID: folderID,
		Files: []PublicUploadFileInput{
			{
				OriginalName: ".DS_Store",
				File:         strings.NewReader("ignored"),
			},
			{
				OriginalName: "notes.pdf",
				DeclaredMIME: "application/pdf",
				File:         strings.NewReader("%PDF-1.4 test document"),
			},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.ItemCount != 1 {
		t.Fatalf("expected only 1 uploaded item after filtering, got %d", result.ItemCount)
	}

	var count int64
	if err := db.Model(&model.Submission{}).Count(&count).Error; err != nil {
		t.Fatalf("count submissions failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 stored submission, got %d", count)
	}
}

func newUploadTestDeps(t *testing.T) (config.Config, *gorm.DB, *storage.Service) {
	t.Helper()

	cfg := config.Default()
	cfg.Session.Secret = "test-secret"
	cfg.Storage.Root = filepath.Join(t.TempDir(), "storage")
	cfg.Database.Path = filepath.Join(t.TempDir(), "openshare-upload.db")

	if err := storage.EnsureLayout(cfg.Storage); err != nil {
		t.Fatalf("ensure storage layout failed: %v", err)
	}

	db, err := database.NewSQLite(database.Options{
		Path:      cfg.Database.Path,
		LogLevel:  "silent",
		EnableWAL: true,
		Pragmas: []database.Pragma{
			{Name: "foreign_keys", Value: "ON"},
			{Name: "busy_timeout", Value: "5000"},
		},
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := bootstrap.EnsureSchema(db); err != nil {
		t.Fatalf("ensure schema failed: %v", err)
	}

	return cfg, db, storage.NewService(cfg.Storage)
}

func createExistingSubmission(t *testing.T, db *gorm.DB, receiptCode string) {
	t.Helper()

	submission := &model.Submission{
		ID:            mustNewUploadID(t),
		ReceiptCode:   receiptCode,
		TitleSnapshot: "existing",
		Status:        model.SubmissionStatusPending,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create existing submission failed: %v", err)
	}
}

func createUploadTargetFolder(t *testing.T, db *gorm.DB) string {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(sourcePath, 0o755); err != nil {
		t.Fatalf("ensure upload target folder path failed: %v", err)
	}

	folderID := mustNewUploadID(t)
	folder := &model.Folder{
		ID:         folderID,
		Name:       "upload-target",
		SourcePath: &sourcePath,
		Status:     model.ResourceStatusActive,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create upload target folder failed: %v", err)
	}
	return folderID
}

func mustNewUploadID(t *testing.T) string {
	t.Helper()

	id, err := identity.NewID()
	if err != nil {
		t.Fatalf("generate id failed: %v", err)
	}
	return id
}
