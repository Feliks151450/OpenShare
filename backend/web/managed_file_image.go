package webui

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// managedFileIDPath 匹配站内 Markdown 图址约定 `/files/<fileID>`（与前端 internalFileCoverRe 同一形状）。
var managedFileIDPath = regexp.MustCompile(`^/files/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})/?$`)

// redirectManagedFileImage 让 `/files/<fileID>` 作为图片地址可直接访问，不带前端改写的渲染器
// （如 vditor 编辑器）也能显示站内图。HTML 导航请求放行给 SPA 路由，避免抢掉 /files/:fileID 详情页。
func redirectManagedFileImage(ctx *gin.Context, requestPath string) bool {
	match := managedFileIDPath.FindStringSubmatch(requestPath)
	if match == nil {
		return false
	}
	if strings.Contains(ctx.GetHeader("Accept"), "text/html") {
		return false
	}
	ctx.Redirect(http.StatusTemporaryRedirect, "/api/public/files/"+url.PathEscape(match[1])+"/download")
	return true
}
