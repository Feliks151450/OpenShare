package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/session"
)

// ---------------------------------------------------------------------------
// 6.1 Tag CRUD
// ---------------------------------------------------------------------------

func TestCreateTag(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	body := `{"name":"数学"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "数学" {
		t.Fatalf("expected name '数学', got %q", resp.Name)
	}
	if resp.ID == "" {
		t.Fatal("expected non-empty id")
	}
}

func TestCreateTagDuplicateCaseInsensitive(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	// Create first tag
	createTagViaAPI(t, engine, cookie, "Mathematics")

	// Attempt duplicate with different case
	body := `{"name":"mathematics"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateTag(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	tagID := createTagViaAPI(t, engine, cookie, "OldName")

	body := `{"name":"NewName"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/tags/"+tagID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "NewName" {
		t.Fatalf("expected 'NewName', got %q", resp.Name)
	}
}

func TestDeleteTag(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	tagID := createTagViaAPI(t, engine, cookie, "ToDelete")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tags/"+tagID, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// Verify tag is soft-deleted
	var tag model.Tag
	if err := db.Where("id = ?", tagID).Take(&tag).Error; err != nil {
		t.Fatalf("reload tag: %v", err)
	}
	if tag.DeletedAt == nil {
		t.Fatal("expected tag to be soft-deleted")
	}
}

func TestListTags(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	createTagViaAPI(t, engine, cookie, "Alpha")
	createTagViaAPI(t, engine, cookie, "Beta")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tags", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// 6.2 File / Folder tag binding
// ---------------------------------------------------------------------------

func TestBindFileTags(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	fileID := createTestFile(t, db, nil)

	body := `{"tags":["Go","Rust"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/files/"+fileID+"/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// Verify tags were created and bound
	var count int64
	db.Model(&model.FileTag{}).Where("file_id = ?", fileID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 file_tags, got %d", count)
	}
}

func TestBindFolderTags(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	folderID := createTestFolder(t, db, nil)

	body := `{"tags":["PDF","教材"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/folders/"+folderID+"/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var count int64
	db.Model(&model.FolderTag{}).Where("folder_id = ?", folderID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 folder_tags, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// 6.3 Tag inheritance
// ---------------------------------------------------------------------------

func TestGetFileTagsWithInheritance(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	parentFolderID := createTestFolder(t, db, nil)
	childFolderID := createTestFolder(t, db, &parentFolderID)
	fileID := createTestFile(t, db, &childFolderID)

	// Bind tags to parent folder
	bindTagsViaAPI(t, engine, cookie, "PUT", "/api/admin/folders/"+parentFolderID+"/tags", []string{"课程"})
	// Bind tags to child folder
	bindTagsViaAPI(t, engine, cookie, "PUT", "/api/admin/folders/"+childFolderID+"/tags", []string{"期末"})
	// Bind direct tags to file
	bindTagsViaAPI(t, engine, cookie, "PUT", "/api/admin/files/"+fileID+"/tags", []string{"答案"})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/files/"+fileID+"/tags", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		DirectTags    []string `json:"direct_tags"`
		InheritedTags []string `json:"inherited_tags"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.DirectTags) != 1 || resp.DirectTags[0] != "答案" {
		t.Fatalf("expected direct_tags=[答案], got %v", resp.DirectTags)
	}
	if len(resp.InheritedTags) != 2 {
		t.Fatalf("expected 2 inherited tags, got %v", resp.InheritedTags)
	}
}

func TestGetFolderTagsWithInheritance(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	parentFolderID := createTestFolder(t, db, nil)
	childFolderID := createTestFolder(t, db, &parentFolderID)

	bindTagsViaAPI(t, engine, cookie, "PUT", "/api/admin/folders/"+parentFolderID+"/tags", []string{"根目录Tag"})
	bindTagsViaAPI(t, engine, cookie, "PUT", "/api/admin/folders/"+childFolderID+"/tags", []string{"子目录Tag"})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/folders/"+childFolderID+"/tags", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		DirectTags    []string `json:"direct_tags"`
		InheritedTags []string `json:"inherited_tags"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.DirectTags) != 1 || resp.DirectTags[0] != "子目录Tag" {
		t.Fatalf("expected direct_tags=[子目录Tag], got %v", resp.DirectTags)
	}
	if len(resp.InheritedTags) != 1 || resp.InheritedTags[0] != "根目录Tag" {
		t.Fatalf("expected inherited_tags=[根目录Tag], got %v", resp.InheritedTags)
	}
}

// ---------------------------------------------------------------------------
// 6.4 User candidate tag submissions
// ---------------------------------------------------------------------------

func TestSubmitCandidateTag(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	body := `{"proposed_name":"新标签"}`
	req := httptest.NewRequest(http.MethodPost, "/api/public/tag-submissions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ID           string `json:"id"`
		ProposedName string `json:"proposed_name"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ProposedName != "新标签" {
		t.Fatalf("expected proposed_name '新标签', got %q", resp.ProposedName)
	}
	if resp.Status != "pending" {
		t.Fatalf("expected status 'pending', got %q", resp.Status)
	}
}

func TestApproveCandidateTag(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	// Submit a candidate tag
	submissionID := submitCandidateTagViaAPI(t, engine, "候选Tag")

	// Approve it
	body := `{"final_name":"候选Tag正式版"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tag-submissions/"+submissionID+"/approve", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// Verify submission got approved
	var ts model.TagSubmission
	if err := db.Where("id = ?", submissionID).Take(&ts).Error; err != nil {
		t.Fatalf("reload submission: %v", err)
	}
	if ts.Status != model.TagSubmissionStatusApproved {
		t.Fatalf("expected approved, got %q", ts.Status)
	}

	// Verify formal tag was created
	var tag model.Tag
	if err := db.Where("id = ?", *ts.TagID).Take(&tag).Error; err != nil {
		t.Fatalf("load formal tag: %v", err)
	}
	if tag.Name != "候选Tag正式版" {
		t.Fatalf("expected name '候选Tag正式版', got %q", tag.Name)
	}
}

func TestRejectCandidateTag(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	submissionID := submitCandidateTagViaAPI(t, engine, "垃圾标签")

	body := `{"reject_reason":"名称不规范"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tag-submissions/"+submissionID+"/reject", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var ts model.TagSubmission
	if err := db.Where("id = ?", submissionID).Take(&ts).Error; err != nil {
		t.Fatalf("reload submission: %v", err)
	}
	if ts.Status != model.TagSubmissionStatusRejected {
		t.Fatalf("expected rejected, got %q", ts.Status)
	}
	if ts.RejectReason != "名称不规范" {
		t.Fatalf("expected reject reason '名称不规范', got %q", ts.RejectReason)
	}
}

// ---------------------------------------------------------------------------
// 6.5 Tag merge
// ---------------------------------------------------------------------------

func TestMergeTags(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "tag-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageTags},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	sourceID := createTagViaAPI(t, engine, cookie, "SourceTag")
	targetID := createTagViaAPI(t, engine, cookie, "TargetTag")

	// Bind source tag to a file
	fileID := createTestFile(t, db, nil)
	bindTagsViaAPI(t, engine, cookie, "PUT", "/api/admin/files/"+fileID+"/tags", []string{"SourceTag"})

	// Merge source into target
	body, _ := json.Marshal(map[string]string{
		"source_tag_id": sourceID,
		"target_tag_id": targetID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tags/merge", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// Verify source is soft-deleted
	var source model.Tag
	if err := db.Where("id = ?", sourceID).Take(&source).Error; err != nil {
		t.Fatalf("reload source: %v", err)
	}
	if source.DeletedAt == nil {
		t.Fatal("expected source tag to be soft-deleted")
	}

	// Verify the file_tag now points to target
	var ft model.FileTag
	if err := db.Where("file_id = ?", fileID).Take(&ft).Error; err != nil {
		t.Fatalf("reload file_tag: %v", err)
	}
	if ft.TagID != targetID {
		t.Fatalf("expected file_tag to reference target %s, got %s", targetID, ft.TagID)
	}
}

// ---------------------------------------------------------------------------
// Permission guard tests
// ---------------------------------------------------------------------------

func TestTagEndpointsRequireManageTagsPermission(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "no-perm",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionReviewSubmissions}, // No manage_tags
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/tags"},
		{http.MethodGet, "/api/admin/tags"},
		{http.MethodPut, "/api/admin/tags/fake-id"},
		{http.MethodDelete, "/api/admin/tags/fake-id"},
		{http.MethodPost, "/api/admin/tags/merge"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403, got %d", ep.method, ep.path, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustCreateSession(t *testing.T, manager *session.Manager, admin *model.Admin) *http.Cookie {
	t.Helper()
	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"}
}

func createTagViaAPI(t *testing.T, engine http.Handler, cookie *http.Cookie, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tags", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag %q: expected 201, got %d, body=%s", name, rec.Code, rec.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode tag create response: %v", err)
	}
	return resp.ID
}

func submitCandidateTagViaAPI(t *testing.T, engine http.Handler, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"proposed_name": name})
	req := httptest.NewRequest(http.MethodPost, "/api/public/tag-submissions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit candidate tag %q: expected 201, got %d, body=%s", name, rec.Code, rec.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.ID
}

func bindTagsViaAPI(t *testing.T, engine http.Handler, cookie *http.Cookie, method, path string, tags []string) {
	t.Helper()
	body, _ := json.Marshal(map[string][]string{"tags": tags})
	req := httptest.NewRequest(method, path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("bind tags %s %s: expected 204, got %d, body=%s", method, path, rec.Code, rec.Body.String())
	}
}

func createTestFolder(t *testing.T, db *gorm.DB, parentID *string) string {
	t.Helper()
	id := mustNewID(t)
	now := time.Now().UTC()
	folder := &model.Folder{
		ID:        id,
		ParentID:  parentID,
		Name:      "folder-" + id[:8],
		Status:    model.ResourceStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create test folder: %v", err)
	}
	return id
}

func createTestFile(t *testing.T, db *gorm.DB, folderID *string) string {
	t.Helper()
	id := mustNewID(t)
	now := time.Now().UTC()
	file := &model.File{
		ID:           id,
		FolderID:     folderID,
		Title:        "file-" + id[:8],
		OriginalName: "file-" + id[:8] + ".pdf",
		StoredName:   id + ".pdf",
		Extension:    ".pdf",
		MimeType:     "application/pdf",
		Size:         1024,
		DiskPath:     "/tmp/" + id + ".pdf",
		Status:       model.ResourceStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create test file: %v", err)
	}
	return id
}
