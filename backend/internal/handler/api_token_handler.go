package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type ApiTokenHandler struct {
	service *service.ApiTokenService
}

func NewApiTokenHandler(service *service.ApiTokenService) *ApiTokenHandler {
	return &ApiTokenHandler{service: service}
}

func (h *ApiTokenHandler) List(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	tokens, err := h.service.List(ctx.Request.Context(), identity.AdminID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list api tokens"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": tokens})
}

type createApiTokenRequest struct {
	Name string `json:"name"`
}

func (h *ApiTokenHandler) Create(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req createApiTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	result, err := h.service.Create(ctx.Request.Context(), identity.AdminID, req.Name)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create api token"})
		return
	}

	ctx.JSON(http.StatusCreated, result)
}

func (h *ApiTokenHandler) Delete(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	if err := h.service.Delete(ctx.Request.Context(), ctx.Param("tokenID"), identity.AdminID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "api token not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete api token"})
		return
	}

	ctx.Status(http.StatusNoContent)
}
