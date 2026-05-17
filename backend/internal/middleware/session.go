package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

func SessionLoader(manager *session.Manager, apiTokenService *service.ApiTokenService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 优先尝试 Authorization: Bearer <api_token>
		authHeader := ctx.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawToken := strings.TrimPrefix(authHeader, "Bearer ")
			if rawToken != "" && apiTokenService != nil {
				identity, resolveErr := apiTokenService.ResolveByTokenHash(ctx.Request.Context(), rawToken)
				if resolveErr == nil && identity != nil {
					session.SetAdminIdentity(ctx, *identity)
					ctx.Next()
					return
				}
			}
		}

		// 其次尝试 Cookie（浏览器访问）
		cookieValue, err := ctx.Cookie(managerCookieName(manager))
		if err != nil {
			ctx.Next()
			return
		}

		result, resolveErr := manager.Resolve(ctx.Request.Context(), cookieValue)
		if resolveErr != nil {
			if result != nil && result.ShouldClear {
				manager.ClearCookie(ctx.Writer)
			}
			if errors.Is(resolveErr, session.ErrNoSession) ||
				errors.Is(resolveErr, session.ErrInvalidSession) ||
				errors.Is(resolveErr, session.ErrExpiredSession) ||
				errors.Is(resolveErr, session.ErrInactiveAdmin) {
				ctx.Next()
				return
			}

			ctx.Error(resolveErr)
			ctx.Next()
			return
		}

		session.SetAdminIdentity(ctx, result.Identity)
		if result.Renewed {
			manager.WriteCookie(ctx.Writer, result.CookieValue, result.Identity.ExpiresAt)
		}

		ctx.Next()
	}
}

func managerCookieName(manager cookieNameProvider) string {
	return manager.CookieName()
}

type cookieNameProvider interface {
	CookieName() string
}
