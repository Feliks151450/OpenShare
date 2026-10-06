package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRedirectManagedFileImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const fileID = "2fbd8a3f-1111-2222-3333-444455556666"

	cases := []struct {
		name         string
		path         string
		accept       string
		wantRedirect bool
	}{
		{name: "图片请求重定向到下载端点", path: "/files/" + fileID, accept: "image/avif,image/webp,*/*", wantRedirect: true},
		{name: "尾斜杠同样命中", path: "/files/" + fileID + "/", accept: "*/*", wantRedirect: true},
		{name: "HTML 导航留给 SPA 详情页", path: "/files/" + fileID, accept: "text/html,application/xhtml+xml", wantRedirect: false},
		{name: "自定义路径不被抢走", path: "/files/my-doc", accept: "*/*", wantRedirect: false},
		{name: "其他路径放行", path: "/about", accept: "*/*", wantRedirect: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			ctx.Request.Header.Set("Accept", tc.accept)

			if got := redirectManagedFileImage(ctx, tc.path); got != tc.wantRedirect {
				t.Fatalf("redirectManagedFileImage() = %v, want %v", got, tc.wantRedirect)
			}
			if !tc.wantRedirect {
				return
			}
			if recorder.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTemporaryRedirect)
			}
			location := recorder.Header().Get("Location")
			if !strings.HasPrefix(location, "/api/public/files/") || !strings.HasSuffix(location, "/download") {
				t.Fatalf("Location = %q, want the public download endpoint", location)
			}
		})
	}
}
