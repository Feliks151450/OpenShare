package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
)

func TestListPendingSubmissions(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	createPendingModerationRecord(t, cfg, db, "PENDING01")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/submissions/pending", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			ReceiptCode string `json:"receipt_code"`
			Status      string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("expected 1 pending submission, got %d", len(response.Items))
	}
	if response.Items[0].ReceiptCode != "PENDING01" {
		t.Fatalf("unexpected receipt code %q", response.Items[0].ReceiptCode)
	}
}

func TestApproveSubmissionMovesFileAndUpdatesStatus(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	submission, file := createPendingModerationRecord(t, cfg, db, "APPROVE01")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/submissions/"+submission.ID+"/approve", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updatedSubmission model.Submission
	if err := db.Where("id = ?", submission.ID).Take(&updatedSubmission).Error; err != nil {
		t.Fatalf("reload submission failed: %v", err)
	}
	if updatedSubmission.Status != model.SubmissionStatusApproved {
		t.Fatalf("expected approved status, got %q", updatedSubmission.Status)
	}

	var updatedFile model.File
	if err := db.Where("id = ?", file.ID).Take(&updatedFile).Error; err != nil {
		t.Fatalf("reload file failed: %v", err)
	}
	if updatedFile.Status != model.ResourceStatusActive {
		t.Fatalf("expected active file status, got %q", updatedFile.Status)
	}
	if _, err := os.Stat(updatedFile.DiskPath); err != nil {
		t.Fatalf("expected repository file to exist: %v", err)
	}
	if _, err := os.Stat(file.DiskPath); !os.IsNotExist(err) {
		t.Fatalf("expected staged file to be moved, stat err=%v", err)
	}

	var logCount int64
	if err := db.Model(&model.OperationLog{}).Where("target_id = ? AND action = ?", submission.ID, "submission_approved").Count(&logCount).Error; err != nil {
		t.Fatalf("count operation logs failed: %v", err)
	}
	if logCount != 1 {
		t.Fatalf("expected 1 approval log, got %d", logCount)
	}
}

func TestRejectSubmissionDeletesStagedFileAndStoresReason(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	submission, file := createPendingModerationRecord(t, cfg, db, "REJECT01")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"reject_reason":"文件内容不完整"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/submissions/"+submission.ID+"/reject", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updatedSubmission model.Submission
	if err := db.Where("id = ?", submission.ID).Take(&updatedSubmission).Error; err != nil {
		t.Fatalf("reload submission failed: %v", err)
	}
	if updatedSubmission.Status != model.SubmissionStatusRejected {
		t.Fatalf("expected rejected status, got %q", updatedSubmission.Status)
	}
	if updatedSubmission.RejectReason != "文件内容不完整" {
		t.Fatalf("unexpected reject reason %q", updatedSubmission.RejectReason)
	}
	if _, err := os.Stat(file.DiskPath); !os.IsNotExist(err) {
		t.Fatalf("expected staged file to be deleted, stat err=%v", err)
	}

	var updatedFile model.File
	if err := db.Where("id = ?", file.ID).Take(&updatedFile).Error; err != nil {
		t.Fatalf("reload file failed: %v", err)
	}
	if updatedFile.DiskPath != "" {
		t.Fatalf("expected rejected file disk path to be cleared, got %q", updatedFile.DiskPath)
	}

	var logCount int64
	if err := db.Model(&model.OperationLog{}).Where("target_id = ? AND action = ?", submission.ID, "submission_rejected").Count(&logCount).Error; err != nil {
		t.Fatalf("count operation logs failed: %v", err)
	}
	if logCount != 1 {
		t.Fatalf("expected 1 rejection log, got %d", logCount)
	}
}

func createPendingModerationRecord(t *testing.T, cfg config.Config, db *gorm.DB, receiptCode string) (*model.Submission, *model.File) {
	t.Helper()

	now := time.Date(2026, 3, 12, 12, 0, 0, 0, time.UTC)
	submissionID := mustNewID(t)
	storedName := mustNewID(t) + ".pdf"
	stagingPath := filepath.Join(cfg.Storage.Root, cfg.Storage.Staging, storedName)
	if err := os.WriteFile(stagingPath, []byte("%PDF-1.4 staged file"), 0o644); err != nil {
		t.Fatalf("write staged file failed: %v", err)
	}

	submission := &model.Submission{
		ID:                  submissionID,
		ReceiptCode:         receiptCode,
		TitleSnapshot:       "离散数学",
		DescriptionSnapshot: "待审核",
		TagsSnapshot:        `["数学"]`,
		Status:              model.SubmissionStatusPending,
		UploaderIP:          "127.0.0.1",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create submission failed: %v", err)
	}

	file := &model.File{
		ID:            mustNewID(t),
		SubmissionID:  &submissionID,
		Title:         submission.TitleSnapshot,
		OriginalName:  "math.pdf",
		StoredName:    storedName,
		Extension:     ".pdf",
		MimeType:      "application/pdf",
		Size:          1024,
		DiskPath:      stagingPath,
		Status:        model.ResourceStatusOffline,
		DownloadCount: 0,
		UploaderIP:    "127.0.0.1",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	return submission, file
}
