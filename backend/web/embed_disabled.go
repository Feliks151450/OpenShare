//go:build noembed

package webui

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CustomPathHandler is called when a request path doesn't match any API route.
// It should return true if it handled the request (wrote a response).
// 与 embed 构建保持一致的类型定义（noembed 下没有静态文件概念）。
type CustomPathHandler func(ctx *gin.Context, requestPath string) bool

// Register 在 noembed（前后端分离）构建下仅保留自定义路径 HTML 渲染：
// 该功能需要查库解析短路径，静态文件服务器（Nginx/Caddy）无法承担，
// 因此即使静态资源由外部托管，未命中 /api 的路径仍交回本处理链尝试
// 解析为托管 HTML 文件；未命中时返回 404（由部署方决定是否覆写）。
func Register(engine *gin.Engine, customPathHandler CustomPathHandler) {
	if customPathHandler == nil {
		return
	}
	engine.NoRoute(func(ctx *gin.Context) {
		requestPath := ctx.Request.URL.Path
		if strings.HasPrefix(requestPath, "/api/") || requestPath == "/api" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if redirectManagedFileImage(ctx, requestPath) {
			return
		}
		if customPathHandler(ctx, requestPath) {
			return
		}
		ctx.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
}
