package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

// TestGetPublicFolderDetailReadmeFallback 验证未配置简介时，目录详情会带上该目录下
// README.md 的正文；一旦配置了简介，README 不再返回（简介优先）。
func TestGetPublicFolderDetailReadmeFallback(t *testing.T) {
	_, db, _ := newUploadTestDeps(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# 目录说明\n\n来自 README 的简介。"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	folderID := mustNewID(t)
	folder := &model.Folder{
		ID:          folderID,
		Name:        "readme-folder",
		SourcePath:  stringPtr(dir),
		Description: "",
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}
	readmeFile := &model.File{
		ID:        mustNewID(t),
		FolderID:  &folderID,
		Name:      "README.md",
		Extension: ".md",
	}
	if err := db.Create(readmeFile).Error; err != nil {
		t.Fatalf("create readme file: %v", err)
	}

	svc := NewPublicCatalogService(repository.NewPublicCatalogRepository(db), nil, nil)
	ctx := context.Background()

	detail, err := svc.GetPublicFolderDetail(ctx, folderID)
	if err != nil {
		t.Fatalf("get folder detail: %v", err)
	}
	if detail.Readme == "" {
		t.Fatalf("expected README content when description is empty")
	}

	folder.Description = "管理员填写的简介"
	if err := db.Save(folder).Error; err != nil {
		t.Fatalf("update folder description: %v", err)
	}
	detail, err = svc.GetPublicFolderDetail(ctx, folderID)
	if err != nil {
		t.Fatalf("get folder detail after edit: %v", err)
	}
	if detail.Readme != "" {
		t.Fatalf("expected no README when description is set, got %q", detail.Readme)
	}
}
