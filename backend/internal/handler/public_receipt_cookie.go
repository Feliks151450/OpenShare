package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const publicReceiptCookieName = "openshare_receipt_code"
const publicReceiptCookieMaxAgeSeconds = 180 * 24 * 60 * 60

func readPublicReceiptCode(ctx *gin.Context) string {
	value, err := ctx.Cookie(publicReceiptCookieName)
	if err != nil {
		return ""
	}
	return value
}

// writePublicReceiptCode 写入回执码 cookie。
// httpOnly=true 阻止页面脚本读取（防 XSS 窃取/伪造回执归属）；
// SameSite=Lax 保持同站表单/导航场景的正常携带。
// 注意：前端 receiptCode.ts 有 cookie 兜底读取逻辑，httpOnly 后该兜底读不到
// cookie，但 sessionStorage 主路径不受影响，功能无损。
func writePublicReceiptCode(ctx *gin.Context, receiptCode string) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(publicReceiptCookieName, receiptCode, publicReceiptCookieMaxAgeSeconds, "/", "", false, true)
}
