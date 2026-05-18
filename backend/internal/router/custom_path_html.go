package router

import (
	"fmt"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	webui "openshare/backend/web"
)

// newCustomPathHTMLHandler 创建自定义路径处理器：当访问路径匹配到 HTML 文件的自定义路径时，直接返回 HTML 内容。
func newCustomPathHTMLHandler(
	catalogSvc *service.PublicCatalogService,
	downloadSvc *service.PublicDownloadService,
) webui.CustomPathHandler {
	return func(ctx *gin.Context, requestPath string) bool {
		cleanPath := strings.TrimPrefix(path.Clean("/"+requestPath), "/")
		log.Printf("[custom-path-html] requestPath=%q cleanPath=%q", requestPath, cleanPath)

		result, err := catalogSvc.ResolveCustomPathFull(ctx.Request.Context(), cleanPath)
		if err != nil {
			log.Printf("[custom-path-html] resolve error: %v", err)
			return false
		}
		if result == nil {
			log.Printf("[custom-path-html] no match for path %q", cleanPath)
			return false
		}
		log.Printf("[custom-path-html] resolved type=%s name=%q fileID=%s", result.Type, result.Name, result.FileID)

		if result.Type != "file" {
			return false
		}
		ext := strings.ToLower(filepath.Ext(result.Name))
		if ext != ".html" && ext != ".htm" {
			log.Printf("[custom-path-html] not HTML: ext=%q", ext)
			return false
		}

		dl, err := downloadSvc.PrepareDownload(ctx.Request.Context(), result.FileID)
		if err != nil {
			log.Printf("[custom-path-html] prepare download error: %v", err)
			return false
		}
		if dl == nil {
			log.Printf("[custom-path-html] download is nil")
			return false
		}
		// 虚拟文件（代理拉取 / CDN 直链）暂不处理
		if dl.ProxyURL != "" || dl.RedirectURL != "" {
			log.Printf("[custom-path-html] virtual file, skip")
			return false
		}
		defer dl.Content.Close()

		log.Printf("[custom-path-html] serving HTML file %q (%d bytes)", dl.FileName, dl.Size)
		ctx.Header("Content-Type", "text/html; charset=utf-8")
		ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", dl.FileName))
		ctx.Header("Content-Length", strconv.FormatInt(dl.Size, 10))
		http.ServeContent(ctx.Writer, ctx.Request, dl.FileName, dl.ModTime, dl.Content)
		return true
	}
}
