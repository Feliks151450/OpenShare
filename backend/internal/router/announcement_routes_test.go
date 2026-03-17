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
)

func TestPublicAnnouncementsPrioritizePinnedItems(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	now := time.Now().UTC()
	createAnnouncementForTest(t, db, announcementSeed{
		title:       "普通公告",
		status:      model.AnnouncementStatusPublished,
		publishedAt: now.Add(-2 * time.Hour),
	})
	createAnnouncementForTest(t, db, announcementSeed{
		title:       "置顶公告",
		status:      model.AnnouncementStatusPublished,
		isPinned:    true,
		publishedAt: now.Add(-24 * time.Hour),
	})
	createAnnouncementForTest(t, db, announcementSeed{
		title:       "隐藏公告",
		status:      model.AnnouncementStatusHidden,
		publishedAt: now,
	})

	request := httptest.NewRequest(http.MethodGet, "/api/public/announcements", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Title    string `json:"title"`
			IsPinned bool   `json:"is_pinned"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Items) != 2 {
		t.Fatalf("expected 2 public announcements, got %d", len(response.Items))
	}
	if response.Items[0].Title != "置顶公告" || !response.Items[0].IsPinned {
		t.Fatalf("expected pinned announcement first, got %+v", response.Items[0])
	}
}

func TestAdminAnnouncementCreateRejectsPinnedForNormalAdmin(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "announcer",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionAnnouncements},
	})
	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"title":"普通公告","content":"正文","status":"published","is_pinned":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/announcements", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminAnnouncementCreateAllowsPinnedForSuperAdmin(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	admin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"title":"置顶公告","content":"正文","status":"published","is_pinned":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/announcements", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var stored model.Announcement
	if err := db.Where("title = ?", "置顶公告").Take(&stored).Error; err != nil {
		t.Fatalf("query announcement failed: %v", err)
	}
	if !stored.IsPinned {
		t.Fatal("expected announcement to be pinned")
	}
}

type announcementSeed struct {
	title       string
	status      model.AnnouncementStatus
	isPinned    bool
	publishedAt time.Time
}

func createAnnouncementForTest(t *testing.T, db *gorm.DB, seed announcementSeed) {
	t.Helper()

	admin := createRouterTestAdmin(t, db, seed.title+"-author", "pass-123456")
	item := &model.Announcement{
		ID:          mustNewID(t),
		Title:       seed.title,
		Content:     seed.title + " content",
		Status:      seed.status,
		IsPinned:    seed.isPinned,
		CreatedByID: admin.ID,
		CreatedAt:   seed.publishedAt,
		UpdatedAt:   seed.publishedAt,
	}
	if seed.status == model.AnnouncementStatusPublished {
		item.PublishedAt = &seed.publishedAt
	}
	if err := db.Create(item).Error; err != nil {
		t.Fatalf("create announcement failed: %v", err)
	}
}
