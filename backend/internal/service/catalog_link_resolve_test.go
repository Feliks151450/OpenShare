package service

import (
	"context"
	"strings"
	"testing"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

// newCatalogLinkTestTree 构造：
//
//	data/                 根目录
//	  nc/                 子目录
//	    2020/             孙目录
//	    a.nc
//	  nc                  与子目录同名的文件（用于验证文件优先 / 结尾斜杠强制目录）
//	  NC_CONVERSION_PLAN.md
//	  README.md
func newCatalogLinkTestTree(t *testing.T) (svc *PublicCatalogService, dataID, ncID string) {
	t.Helper()
	_, db, _ := newUploadTestDeps(t)

	dataID = mustNewID(t)
	ncID = mustNewID(t)
	yearID := mustNewID(t)

	folders := []*model.Folder{
		{ID: dataID, Name: "data"},
		{ID: ncID, Name: "nc", ParentID: &dataID},
		{ID: yearID, Name: "2020", ParentID: &ncID},
	}
	// 与生产写入路径一致：空 custom_path 跳过插入，避免 UNIQUE 索引冲突
	for _, f := range folders {
		if err := db.Omit("custom_path").Create(f).Error; err != nil {
			t.Fatalf("create folder %s: %v", f.Name, err)
		}
	}

	files := []*model.File{
		{ID: mustNewID(t), FolderID: &ncID, Name: "a.nc", Extension: ".nc"},
		{ID: mustNewID(t), FolderID: &dataID, Name: "nc", Extension: ""},
		{ID: mustNewID(t), FolderID: &dataID, Name: "NC_CONVERSION_PLAN.md", Extension: ".md"},
		{ID: mustNewID(t), FolderID: &dataID, Name: "README.md", Extension: ".md"},
	}
	for _, f := range files {
		if err := db.Omit("custom_path").Create(f).Error; err != nil {
			t.Fatalf("create file %s: %v", f.Name, err)
		}
	}

	return NewPublicCatalogService(repository.NewPublicCatalogRepository(db), nil, nil), dataID, ncID
}

func TestResolveCatalogRelativePath(t *testing.T) {
	svc, dataID, ncID := newCatalogLinkTestTree(t)
	ctx := context.Background()

	ncFileID := ""
	ncSameNameFileID := ""
	planFileID := ""
	{
		var files []model.File
		if err := svc.repository.GetDB().Find(&files).Error; err != nil {
			t.Fatalf("list files: %v", err)
		}
		for _, f := range files {
			switch f.Name {
			case "a.nc":
				ncFileID = f.ID
			case "nc":
				ncSameNameFileID = f.ID
			case "NC_CONVERSION_PLAN.md":
				planFileID = f.ID
			}
		}
	}

	for _, tc := range []struct {
		name       string
		anchor     string
		path       string
		wantType   string
		wantFolder string
		wantFile   string
	}{
		{"结尾斜杠强制解析为目录", dataID, "nc/", "folder", ncID, ""},
		{"无斜杠时文件优先", dataID, "nc", "file", "", ncSameNameFileID},
		{"同级文件", dataID, "NC_CONVERSION_PLAN.md", "file", "", planFileID},
		{"./ 前缀", dataID, "./README.md", "file", "", ""},
		{"多级路径", dataID, "nc/a.nc", "file", "", ncFileID},
		{"点号段被忽略", dataID, "nc/./a.nc", "file", "", ncFileID},
		{"上级目录", ncID, "../NC_CONVERSION_PLAN.md", "file", "", planFileID},
		{"上跳后重新进入兄弟目录", ncID, "../nc/2020", "folder", "", ""},
		{"越过根目录不再上溯", dataID, "../../README.md", "", "", ""},
		{"纯上溯指向父目录", ncID, "..", "folder", dataID, ""},
		{"纯上溯带结尾斜杠", ncID, "../", "folder", dataID, ""},
		{"点号指向锚点自身", ncID, ".", "folder", ncID, ""},
		{"点号斜杠指向锚点自身", ncID, "./", "folder", ncID, ""},
		{"根之上的纯上溯无对应目录", "", "..", "", "", ""},
		{"单独的根斜杠无对应目录", dataID, "/", "", "", ""},
		{"绝对路径从根开始", dataID, "/data/nc/a.nc", "file", "", ncFileID},
		{"绝对路径不落到锚点之外", dataID, "/nc/a.nc", "", "", ""},
		{"中间的上跳段拒绝猜测", dataID, "nc/../README.md", "", "", ""},
		{"不存在的名称", dataID, "nope", "", "", ""},
		{"空路径", dataID, "", "", "", ""},
		{"空白锚点按根目录", "", "data", "folder", dataID, ""},
		{"不存在的锚点", mustNewID(t), "nc/", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ResolveCatalogRelativePath(ctx, tc.anchor, tc.path)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if tc.wantType == "" {
				if got != nil {
					t.Fatalf("want not found, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want type %s, got nil", tc.wantType)
			}
			if got.Type != tc.wantType {
				t.Errorf("type=%q, want %q", got.Type, tc.wantType)
			}
			if tc.wantFolder != "" && got.FolderID != tc.wantFolder {
				t.Errorf("folder_id=%q, want %q", got.FolderID, tc.wantFolder)
			}
			if tc.wantFile != "" && got.FileID != tc.wantFile {
				t.Errorf("file_id=%q, want %q", got.FileID, tc.wantFile)
			}
		})
	}
}

// TestResolveCatalogRelativePathLimitsInputSize 保证公开端点上的长路径不会变成任意多次查询。
func TestResolveCatalogRelativePathLimitsInputSize(t *testing.T) {
	svc, dataID, _ := newCatalogLinkTestTree(t)
	ctx := context.Background()

	deep := strings.Repeat("nc/", maxCatalogLinkPathSegments+1) + "a.nc"
	if got, err := svc.ResolveCatalogRelativePath(ctx, dataID, deep); err != nil || got != nil {
		t.Fatalf("超长路径应被拒绝, got=%+v err=%v", got, err)
	}

	long := strings.Repeat("x", maxCatalogLinkPathBytes+1)
	if got, err := svc.ResolveCatalogRelativePath(ctx, dataID, long); err != nil || got != nil {
		t.Fatalf("超长字符数应被拒绝, got=%+v err=%v", got, err)
	}
}

// TestResolvePathSegmentsStillStartsAtRoot 锁定 /dl/* 的原有语义未被锚点改造影响。
func TestResolvePathSegmentsStillStartsAtRoot(t *testing.T) {
	svc, _, _ := newCatalogLinkTestTree(t)
	ctx := context.Background()

	got, err := svc.ResolvePathSegments(ctx, []string{"data", "nc", "a.nc"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.Type != "file" {
		t.Fatalf("want file at /data/nc/a.nc, got %+v", got)
	}
	if got, err := svc.ResolvePathSegments(ctx, []string{"nc", "a.nc"}); err != nil || got != nil {
		t.Fatalf("根下不应存在 nc/a.nc, got=%+v err=%v", got, err)
	}
}
