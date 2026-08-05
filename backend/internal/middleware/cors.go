package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// PublicDownloadCORSKey 是上游公开 CORS 中间件写入的标记值，用于让白名单 CORS 识别并跳过 /dl/*。
const PublicDownloadCORSKey = "X-OpenShare-Public-Download-CORS"

// PublicDownloadCORS 仅为 /dl/* 下载直链提供公开的跨域响应头。
// - 任意来源均可访问，响应头统一返回 Access-Control-Allow-Origin: *；
// - 显式不返回 Access-Control-Allow-Credentials，因此调用方必须使用 credentials: "omit"（或默认无 Cookie）；
// - 仅允许 GET/HEAD/OPTIONS 与 Range/If-Range 头，便于媒体客户端探测与断点续传；
// - 通过 Access-Control-Expose-Headers 暴露 Content-Disposition/Content-Length/Content-Range/Accept-Ranges，
//   让前端 fetch 能读到文件名与分块信息；
// - 在 ctx 中写入 PublicDownloadCORSKey 标记，避免后续白名单 CORS 再覆盖 Access-Control-Allow-Origin。
// 注意：应注册在白名单 CORS 之前，使得 /dl/* 请求的公开策略优先生效。
func PublicDownloadCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅对 /dl 与 /dl/ 前缀的请求生效；其它路径交给后续白名单 CORS 处理。
		path := c.Request.URL.Path
		if path != "/dl" && !strings.HasPrefix(path, "/dl/") {
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Range, If-Range, Origin, Content-Type, Accept")
		h.Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, Content-Range, Accept-Ranges")
		h.Set("Access-Control-Max-Age", "86400")
		h.Set(PublicDownloadCORSKey, "1")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// CORS 为浏览器跨域请求添加响应头。allowedOrigins 为空时不做任何处理（与未配置 CORS 时行为一致）。
// 列表中的字符串须为完整 Origin（例如 https://qiniu.feliks.top），须与浏览器发送的 Origin 完全一致。
// 当上游 PublicDownloadCORS 已经标记过当前请求（例如 /dl/*），本中间件不会再写入 Access-Control-Allow-Origin，
// 以避免白名单 CORS 覆盖公开 CORS 头。
func CORS(allowedOrigins []string) gin.HandlerFunc {
	if len(allowedOrigins) == 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		allow[o] = struct{}{}
	}
	if len(allow) == 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		// /dl/* 已经由 PublicDownloadCORS 写入 * 策略，白名单 CORS 不再覆盖。
		if c.Writer.Header().Get(PublicDownloadCORSKey) == "1" {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allow[origin]; ok {
				h := c.Writer.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")
				h.Set("Access-Control-Max-Age", "86400")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
