package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestPublicDownloadCORS_GETAndHEADAllowAnyOrigin 验证 /dl/* 的 GET 与 HEAD 对任意 Origin 都返回公开 CORS 头，
// 且不返回 Access-Control-Allow-Credentials。
func TestPublicDownloadCORS_GETAndHEADAllowAnyOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(PublicDownloadCORS())
	engine.GET("/dl/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	engine.HEAD("/dl/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request := httptest.NewRequest(method, "/dl/sample.txt", nil)
		request.Header.Set("Origin", "https://attacker.example")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("[%s] expected 200, got %d", method, recorder.Code)
		}
		if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("[%s] expected ACAO=*, got %q", method, got)
		}
		if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("[%s] expected no Allow-Credentials, got %q", method, got)
		}
		expose := recorder.Header().Get("Access-Control-Expose-Headers")
		for _, want := range []string{"Content-Disposition", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if !strings.Contains(expose, want) {
				t.Fatalf("[%s] expected Expose-Headers to contain %q, got %q", method, want, expose)
			}
		}
	}
}

// TestPublicDownloadCORS_OptionsPreflight 验证 OPTIONS 预检来自任意 Origin 都返回 204 与完整公开头。
func TestPublicDownloadCORS_OptionsPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(PublicDownloadCORS())
	engine.GET("/dl/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	request := httptest.NewRequest(http.MethodOptions, "/dl/sample.txt", nil)
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Access-Control-Request-Method", "GET")
	request.Header.Set("Access-Control-Request-Headers", "range, if-range")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected ACAO=*, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") || !strings.Contains(got, "HEAD") || !strings.Contains(got, "OPTIONS") {
		t.Fatalf("expected Allow-Methods to include GET/HEAD/OPTIONS, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Range") {
		t.Fatalf("expected Allow-Headers to include Range, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("expected no Allow-Credentials, got %q", got)
	}
}

// TestPublicDownloadCORS_OnlyAppliesToDLPath 验证公开策略仅作用于 /dl/*，对其它路径不写头。
func TestPublicDownloadCORS_OnlyAppliesToDLPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(PublicDownloadCORS())
	engine.GET("/api/public/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"pong": true})
	})

	request := httptest.NewRequest(http.MethodGet, "/api/public/ping", nil)
	request.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no ACAO for non-/dl path, got %q", got)
	}
}

// TestPublicDownloadCORSPreventsWhitelistOverwrite 验证组合注册时，PublicDownloadCORS 必须先于白名单 CORS，
// 否则 /dl/* 的白名单命中会覆盖为具体 Origin，破坏通配符策略。
func TestPublicDownloadCORSPreventsWhitelistOverwrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(PublicDownloadCORS())
	engine.Use(CORS([]string{"https://qiniu.feliks.top"}))
	engine.GET("/dl/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	request := httptest.NewRequest(http.MethodGet, "/dl/sample.txt", nil)
	request.Header.Set("Origin", "https://qiniu.feliks.top")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected ACAO=* for /dl/*, got %q", got)
	}
}
