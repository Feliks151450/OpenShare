package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"openshare/backend/internal/middleware"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
)

func TestPublicResolveCatalogPathResolvesRelativeFolderAndFile(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)

	dataID := createPublicTestFolder(t, db, "data")
	ncID := createPublicTestFolderWithParent(t, db, publicTestFolder{name: "nc", parentID: &dataID})
	plan := createPublicTestFile(t, db, publicTestFile{title: "NC_CONVERSION_PLAN", folderID: &dataID})
	engine := New(db, cfg, newRouterSessionManager(db))

	folderResult := resolveCatalogPathRequest(t, engine, dataID, "nc/")
	if folderResult.Type != "folder" || folderResult.FolderID != ncID {
		t.Fatalf("expected folder %q, got %+v", ncID, folderResult)
	}

	// 尾斜杠缺省时同名文件优先，与 /dl/* 的语义保持一致。
	fileResult := resolveCatalogPathRequest(t, engine, dataID, "NC_CONVERSION_PLAN.pdf")
	if fileResult.Type != "file" || fileResult.FileID != plan.ID {
		t.Fatalf("expected file %q, got %+v", plan.ID, fileResult)
	}

	parentResult := resolveCatalogPathRequest(t, engine, ncID, "../")
	if parentResult.Type != "folder" || parentResult.FolderID != dataID {
		t.Fatalf("expected parent folder %q, got %+v", dataID, parentResult)
	}
}

func TestPublicResolveCatalogPathRejectsUnknownAndMissingPath(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	dataID := createPublicTestFolder(t, db, "data")
	engine := New(db, cfg, newRouterSessionManager(db))

	request := httptest.NewRequest(http.MethodGet, "/api/public/resolve-catalog-path?path="+url.QueryEscape("missing.md")+"&folder_id="+dataID, nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/public/resolve-catalog-path?folder_id="+dataID, nil)
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicResolveCatalogPathEnforcesGuestKeyOnAnchor(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)

	dataID := createPublicTestFolder(t, db, "data")
	createPublicTestFile(t, db, publicTestFile{title: "NC_CONVERSION_PLAN", folderID: &dataID})
	if err := db.Model(&model.Folder{}).Where("id = ?", dataID).Update("guest_key_required", true).Error; err != nil {
		t.Fatalf("mark folder guest key required failed: %v", err)
	}

	ctx := context.Background()
	settings := service.NewSystemSettingService(repository.NewSystemSettingRepository(db), cfg)
	operator := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "system-op",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
	})
	keyID := mustNewID(t)
	if _, err := settings.SaveGuestAccess(ctx, service.GuestAccessPolicy{
		Enabled: true,
		Keys:    []service.GuestAccessKey{{ID: keyID, Name: "研究组", Value: "secret-key"}},
	}, operator.ID, "127.0.0.1"); err != nil {
		t.Fatalf("save guest access policy failed: %v", err)
	}
	if err := repository.NewGuestKeyAssignmentRepository(db).ReplaceAssignments(ctx, dataID, []string{keyID}); err != nil {
		t.Fatalf("assign guest key to folder failed: %v", err)
	}

	engine := New(db, cfg, newRouterSessionManager(db))
	target := "/api/public/resolve-catalog-path?path=" + url.QueryEscape("NC_CONVERSION_PLAN.pdf") + "&folder_id=" + dataID

	request := httptest.NewRequest(http.MethodGet, target, nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without guest key, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set(middleware.GuestKeyHeader, "secret-key")
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 with guest key, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

type catalogPathResponse struct {
	Type     string `json:"type"`
	FolderID string `json:"folder_id"`
	FileID   string `json:"file_id"`
	Name     string `json:"name"`
}

func resolveCatalogPathRequest(t *testing.T, engine http.Handler, anchorFolderID, path string) catalogPathResponse {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/api/public/resolve-catalog-path?path="+url.QueryEscape(path)+"&folder_id="+anchorFolderID, nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for path %q, got %d, body=%s", path, recorder.Code, recorder.Body.String())
	}

	var response catalogPathResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response for path %q failed: %v", path, err)
	}
	return response
}
