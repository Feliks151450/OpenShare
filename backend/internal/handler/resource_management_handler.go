package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type ResourceManagementHandler struct {
	service *service.ResourceManagementService
}

type updateManagedFileRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func NewResourceManagementHandler(service *service.ResourceManagementService) *ResourceManagementHandler {
	return &ResourceManagementHandler{service: service}
}

func (h *ResourceManagementHandler) ListFiles(ctx *gin.Context) {
	items, err := h.service.ListFiles(ctx.Request.Context(), service.ListManagedFilesInput{
		Query:  ctx.Query("q"),
		Status: ctx.Query("status"),
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list resources"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *ResourceManagementHandler) UpdateFile(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req updateManagedFileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.UpdateFile(ctx.Request.Context(), ctx.Param("fileID"), service.UpdateManagedFileInput{
		Title:       req.Title,
		Description: req.Description,
		Tags:        req.Tags,
		OperatorID:  identity.AdminID,
		OperatorIP:  ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidResourceEdit):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource input"})
		case errors.Is(err, service.ErrManagedFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update resource"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *ResourceManagementHandler) OfflineFile(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	err := h.service.OfflineFile(ctx.Request.Context(), ctx.Param("fileID"), identity.AdminID, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagedFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to offline resource"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *ResourceManagementHandler) DeleteFile(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	err := h.service.DeleteFile(ctx.Request.Context(), ctx.Param("fileID"), identity.AdminID, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagedFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete resource"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}
