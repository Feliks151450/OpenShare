package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// GuestKeyContextKey 是 ctx 中提取访客密钥的 key 名。
const GuestKeyContextKey = "openshare.guest_key_value"

// GuestKeyHeader 是访客密钥在浏览器请求中的 HTTP header 名称。
const GuestKeyHeader = "X-OpenShare-Guest-Key"

// GuestKeyQueryParam 是访客密钥在 URL 中的兜底参数名（用于 /dl/* 等无法附 header 的场景）。
const GuestKeyQueryParam = "guest_key"

// ExtractGuestKeyHeader 解析访客密钥并写入 ctx。
// 仅在浏览类公共路由组挂载；下载端与 /dl/* 不挂。
func ExtractGuestKeyHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader(GuestKeyHeader))
		if key == "" {
			key = strings.TrimSpace(c.Query(GuestKeyQueryParam))
		}
		if key != "" {
			c.Set(GuestKeyContextKey, key)
		}
		c.Next()
	}
}

// GuestKeyFromContext 安全地从 ctx 中取出访客密钥；空时返回 ""。
func GuestKeyFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(GuestKeyContextKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
