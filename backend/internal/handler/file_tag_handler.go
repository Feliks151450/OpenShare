package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type FileTagHandler struct {
	tags *service.FileTagService
}

func NewFileTagHandler(tags *service.FileTagService) *FileTagHandler {
	return &FileTagHandler{tags: tags}
}

func (h *FileTagHandler) ListPublicDefinitions(ctx *gin.Context) {
	if h.tags == nil {
		ctx.JSON(http.StatusOK, gin.H{"items": []any{}})
		return
	}
	items, err := h.tags.ListDefinitions(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list file tags"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

type createFileTagRequest struct {
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder *int   `json:"sort_order"`
}

type updateFileTagRequest struct {
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder *int   `json:"sort_order"`
}

type replaceFileTagsRequest struct {
	TagIDs []string `json:"tag_ids"`
}

func (h *FileTagHandler) AdminCreateTag(ctx *gin.Context) {
	if h.tags == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "file tag service unavailable"})
		return
	}
	if _, ok := session.GetAdminIdentity(ctx); !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req createFileTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	row, err := h.tags.AdminCreateTag(ctx.Request.Context(), req.Name, req.Color, sortOrder)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFileTagInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag name or color"})
		case errors.Is(err, service.ErrFileTagNameConflict):
			ctx.JSON(http.StatusConflict, gin.H{"error": "tag name already exists"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tag"})
		}
		return
	}
	ctx.JSON(http.StatusCreated, row)
}

func (h *FileTagHandler) AdminUpdateTag(ctx *gin.Context) {
	if h.tags == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "file tag service unavailable"})
		return
	}
	if _, ok := session.GetAdminIdentity(ctx); !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req updateFileTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	tagID := strings.TrimSpace(ctx.Param("tagID"))
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	err := h.tags.AdminUpdateTag(ctx.Request.Context(), tagID, req.Name, req.Color, sortOrder)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFileTagInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag name or color"})
		case errors.Is(err, service.ErrFileTagNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		case errors.Is(err, service.ErrFileTagNameConflict):
			ctx.JSON(http.StatusConflict, gin.H{"error": "tag name already exists"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tag"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *FileTagHandler) AdminDeleteTag(ctx *gin.Context) {
	if h.tags == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "file tag service unavailable"})
		return
	}
	if _, ok := session.GetAdminIdentity(ctx); !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	tagID := strings.TrimSpace(ctx.Param("tagID"))
	err := h.tags.AdminDeleteTag(ctx.Request.Context(), tagID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFileTagInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag id"})
		case errors.Is(err, service.ErrFileTagNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tag"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *FileTagHandler) AdminReplaceFileTags(ctx *gin.Context) {
	if h.tags == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "file tag service unavailable"})
		return
	}
	if _, ok := session.GetAdminIdentity(ctx); !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req replaceFileTagsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.TagIDs == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids is required"})
		return
	}
	fileID := strings.TrimSpace(ctx.Param("fileID"))
	err := h.tags.ReplaceManagedFileTags(ctx.Request.Context(), fileID, req.TagIDs)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagedFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		case errors.Is(err, service.ErrFileTagInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "one or more tag ids are invalid"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update file tags"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}
