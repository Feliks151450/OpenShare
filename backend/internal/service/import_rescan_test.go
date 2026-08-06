package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

// TestRescanManagedDirectoryOnSubfolder 验证重新扫描可作用于任一非虚拟托管目录（不再要求根目录）。
// 扫描入口为子目录时：
//  1. 入口子目录在数据库中保留原 parentID，避免被拍平到根；
//  2. 入口子目录自身及其后代会被纳入 diff（新增/删除/更新）；
//  3. 扫描范围之外的兄弟目录/文件不会被误删。
func TestRescanManagedDirectoryOnSubfolder(t *testing.T) {
	_, db, storageSvc := newUploadTestDeps(t)

	// 在磁盘上构造：diskroot/sub/inner/ 与 diskroot/sub/keep.txt，外加 diskroot/other.txt
	diskRoot := filepath.Join(t.TempDir(), "diskroot")
	subDir := filepath.Join(diskRoot, "sub")
	innerDir := filepath.Join(subDir, "inner")
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		t.Fatalf("mkdir inner: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(diskRoot, "other.txt"), []byte("other"), 0o644); err != nil {
		t.Fatalf("write other.txt: %v", err)
	}

	// 提前在数据库里建好 root 与 sub 两个目录（sub.parent_id = root.id），
	// 表示 root 早已托管，sub 是一个已有的真实磁盘子目录。
	rootID := mustNewID(t)
	subID := mustNewID(t)

	rootFolder := &model.Folder{
		ID:         rootID,
		ParentID:   nil,
		Name:       "root",
		SourcePath: stringPtr(diskRoot),
	}
	if err := db.Create(rootFolder).Error; err != nil {
		t.Fatalf("create root folder: %v", err)
	}
	subFolder := &model.Folder{
		ID:         subID,
		ParentID:   &rootID,
		Name:       "sub",
		SourcePath: stringPtr(subDir),
	}
	if err := db.Create(subFolder).Error; err != nil {
		t.Fatalf("create sub folder: %v", err)
	}

	repo := repository.NewImportRepository(db)
	svc := NewImportService(repo, storageSvc)

	// 对 sub 触发重新扫描
	result, err := svc.RescanManagedDirectory(context.Background(), subID, "admin-test", "127.0.0.1")
	if err != nil {
		t.Fatalf("rescan sub folder: %v", err)
	}

	// 应至少新增 sub/inner 目录与 sub/keep.txt
	if result.AddedFolders < 1 {
		t.Errorf("expected at least 1 added folder (inner), got %d", result.AddedFolders)
	}
	if result.AddedFiles < 1 {
		t.Errorf("expected at least 1 added file (keep.txt), got %d", result.AddedFiles)
	}

	// 入口子目录的 parentID 必须保留为 rootID
	var refreshedSub model.Folder
	if err := db.Where("id = ?", subID).Take(&refreshedSub).Error; err != nil {
		t.Fatalf("reload sub folder: %v", err)
	}
	if refreshedSub.ParentID == nil || *refreshedSub.ParentID != rootID {
		t.Errorf("expected sub.parent_id to be %q, got %v", rootID, refreshedSub.ParentID)
	}

	// 扫描范围不应新增直接挂在 root 下的额外目录（其他文件/目录在 root 之下，但不在 sub 子树内）
	countDirectlyUnderRoot := 0
	var allFolders []model.Folder
	if err := db.Find(&allFolders).Error; err != nil {
		t.Fatalf("list folders: %v", err)
	}
	for _, f := range allFolders {
		if f.ParentID != nil && *f.ParentID == rootID && f.ID != subID {
			countDirectlyUnderRoot++
		}
	}
	if countDirectlyUnderRoot != 0 {
		t.Errorf("expected no extra folders directly under root from this scan, got %d", countDirectlyUnderRoot)
	}

	// 验证 inner 目录被建出来，且 parent 为 sub
	var innerFolder model.Folder
	if err := db.Where("name = ? AND parent_id = ?", "inner", subID).Take(&innerFolder).Error; err != nil {
		t.Fatalf("expected inner folder under sub: %v", err)
	}
	if innerFolder.SourcePath == nil || filepath.Base(*innerFolder.SourcePath) != "inner" {
		t.Errorf("expected inner folder source_path to end with inner, got %v", innerFolder.SourcePath)
	}

	// 验证 keep.txt 落在 sub 目录下
	var keepFile model.File
	if err := db.Where("name = ? AND folder_id = ?", "keep.txt", subID).Take(&keepFile).Error; err != nil {
		t.Fatalf("expected keep.txt under sub: %v", err)
	}
}

func mustNewID(t *testing.T) string {
	t.Helper()
	id, err := identity.NewID()
	if err != nil {
		t.Fatalf("generate id failed: %v", err)
	}
	return id
}
