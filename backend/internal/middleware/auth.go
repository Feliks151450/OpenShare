package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/model"
	"openshare/backend/internal/session"
)

func RequireAdminAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if _, ok := session.GetAdminIdentity(ctx); !ok {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		ctx.Next()
	}
}

func RequireAdminPermission(permission model.AdminPermission) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		identity, ok := session.GetAdminIdentity(ctx)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		if !identity.HasPermission(permission) {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":               "permission denied",
				"required_permission": permission,
			})
			return
		}

		ctx.Next()
	}
}
