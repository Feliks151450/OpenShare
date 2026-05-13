//go:build noembed

package webui

import "github.com/gin-gonic/gin"

// Register is a noop when built with -tags noembed.
// Static files are served externally (Nginx / Caddy).
func Register(_ *gin.Engine) {}
