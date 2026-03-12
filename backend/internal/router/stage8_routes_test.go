package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
)

func TestAnnouncementLifecycleAndPublicVisibility(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "ann-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageAnnouncements},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	body := bytes.NewBufferString(`{"title":"维护通知","content":"今晚 10 点维护","status":"published"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/announcements", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create announcement: expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create announcement response: %v", err)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/api/public/announcements", nil)
	publicRec := httptest.NewRecorder()
	engine.ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK {
		t.Fatalf("list public announcements: expected 200, got %d", publicRec.Code)
	}
	var publicResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(publicRec.Body.Bytes(), &publicResp); err != nil {
		t.Fatalf("decode public announcement response: %v", err)
	}
	if len(publicResp.Items) != 1 || publicResp.Items[0].ID != created.ID {
		t.Fatalf("expected published announcement to be public, got %+v", publicResp.Items)
	}

	updateReq := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/announcements/"+created.ID,
		bytes.NewBufferString(`{"title":"维护通知","content":"今晚 10 点维护","status":"hidden"}`),
	)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateRec := httptest.NewRecorder()
	engine.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("hide announcement: expected 200, got %d, body=%s", updateRec.Code, updateRec.Body.String())
	}

	publicRec = httptest.NewRecorder()
	engine.ServeHTTP(publicRec, publicReq)
	if err := json.Unmarshal(publicRec.Body.Bytes(), &publicResp); err != nil {
		t.Fatalf("decode hidden public announcement response: %v", err)
	}
	if len(publicResp.Items) != 0 {
		t.Fatalf("expected hidden announcement to disappear from public list, got %+v", publicResp.Items)
	}
}

func TestSuperAdminCanManageAdminsAndNormalAdminCannot(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	superAdmin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "plain-admin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	superCookie := mustCreateSession(t, manager, superAdmin)
	adminCookie := mustCreateSession(t, manager, admin)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/admins",
		bytes.NewBufferString(`{"username":"ops","password":"password123","permissions":["manage_tags"]}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(superCookie)
	createRec := httptest.NewRecorder()
	engine.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("super admin create admin: expected 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}

	forbiddenReq := httptest.NewRequest(http.MethodGet, "/api/admin/admins", nil)
	forbiddenReq.AddCookie(adminCookie)
	forbiddenRec := httptest.NewRecorder()
	engine.ServeHTTP(forbiddenRec, forbiddenReq)
	if forbiddenRec.Code != http.StatusForbidden {
		t.Fatalf("normal admin should be forbidden from admin management, got %d", forbiddenRec.Code)
	}
}

func TestSuperAdminCanPersistSystemSettings(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	superAdmin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, superAdmin)

	body := bytes.NewBufferString(`{
		"guest":{"allow_direct_publish":true,"extra_permissions_enabled":true,"allow_guest_resource_edit":false,"allow_guest_resource_delete":false},
		"upload":{"max_file_size_bytes":1048576,"max_tag_count":8,"allowed_extensions":[".pdf",".md"]},
		"search":{"enable_fuzzy_match":true,"enable_tag_filter":true,"enable_folder_scope":true,"result_window":25}
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/system/settings", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save system settings: expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/system/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	engine.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get system settings: expected 200, got %d, body=%s", getRec.Code, getRec.Body.String())
	}

	var response struct {
		Guest struct {
			AllowDirectPublish bool `json:"allow_direct_publish"`
		} `json:"guest"`
		Upload struct {
			MaxTagCount int `json:"max_tag_count"`
		} `json:"upload"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode system settings response: %v", err)
	}
	if !response.Guest.AllowDirectPublish || response.Upload.MaxTagCount != 8 {
		t.Fatalf("unexpected system settings response: %+v", response)
	}
}

func TestResourceManagementCanUpdateAndDeleteFile(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "resource-admin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionEditResources,
			model.AdminPermissionDeleteResources,
		},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	fileID := createTestFile(t, db, nil)
	diskPath := filepath.Join(t.TempDir(), "resource.pdf")
	if err := os.WriteFile(diskPath, []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("write resource file: %v", err)
	}
	if err := db.Model(&model.File{}).Where("id = ?", fileID).Updates(map[string]any{
		"disk_path": diskPath,
	}).Error; err != nil {
		t.Fatalf("update resource disk path: %v", err)
	}

	updateReq := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources/files/"+fileID,
		bytes.NewBufferString(`{"title":"更新后的标题","description":"新的描述","tags":["Go","Math"]}`),
	)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateRec := httptest.NewRecorder()
	engine.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusNoContent {
		t.Fatalf("update resource: expected 204, got %d, body=%s", updateRec.Code, updateRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/resources/files/"+fileID, nil)
	deleteReq.AddCookie(cookie)
	deleteRec := httptest.NewRecorder()
	engine.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete resource: expected 204, got %d, body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	var file model.File
	if err := db.Where("id = ?", fileID).Take(&file).Error; err != nil {
		t.Fatalf("reload resource file: %v", err)
	}
	if file.Status != model.ResourceStatusDeleted {
		t.Fatalf("expected file status deleted, got %s", file.Status)
	}
}
