package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/model"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type AnnouncementHandler struct {
	service *service.AnnouncementService
}

type saveAnnouncementRequest struct {
	Title    string                   `json:"title"`
	Content  string                   `json:"content"`
	Status   model.AnnouncementStatus `json:"status"`
	IsPinned *bool                    `json:"is_pinned,omitempty"`
	IsHome   *bool                    `json:"is_home,omitempty"`
}

func NewAnnouncementHandler(service *service.AnnouncementService) *AnnouncementHandler {
	return &AnnouncementHandler{service: service}
}

func (h *AnnouncementHandler) ListPublic(ctx *gin.Context) {
	items, err := h.service.ListPublic(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list announcements"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AnnouncementHandler) GetHomeAnnouncement(ctx *gin.Context) {
	item, err := h.service.FindHomeAnnouncement(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get home announcement"})
		return
	}
	if item == nil {
		ctx.JSON(http.StatusOK, gin.H{"announcement": nil})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"announcement": item})
}

func (h *AnnouncementHandler) ListAdmin(ctx *gin.Context) {
	items, err := h.service.ListAdmin(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list announcements"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AnnouncementHandler) Create(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req saveAnnouncementRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.service.Create(ctx.Request.Context(), service.SaveAnnouncementInput{
		Title:      req.Title,
		Content:    req.Content,
		Status:     req.Status,
		IsPinned:   req.IsPinned,
		IsHome:     req.IsHome,
		OperatorID: identity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		if errors.Is(err, service.ErrAnnouncementInvalidInput) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid announcement"})
			return
		}
		if errors.Is(err, service.ErrAnnouncementPinDenied) {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "only super admin can pin announcements"})
			return
		}
		if errors.Is(err, service.ErrAnnouncementHomeDenied) {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "only super admin can set homepage announcement"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create announcement"})
		return
	}
	ctx.JSON(http.StatusCreated, item)
}

func (h *AnnouncementHandler) Update(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req saveAnnouncementRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.service.Update(ctx.Request.Context(), ctx.Param("announcementID"), service.SaveAnnouncementInput{
		Title:      req.Title,
		Content:    req.Content,
		Status:     req.Status,
		IsPinned:   req.IsPinned,
		IsHome:     req.IsHome,
		OperatorID: identity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAnnouncementInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid announcement"})
		case errors.Is(err, service.ErrAnnouncementPinDenied):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "only super admin can pin announcements"})
		case errors.Is(err, service.ErrAnnouncementHomeDenied):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "only super admin can set homepage announcement"})
		case errors.Is(err, service.ErrAnnouncementUpdateDenied):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "cannot update this announcement"})
		case errors.Is(err, service.ErrAnnouncementNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "announcement not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update announcement"})
		}
		return
	}
	ctx.JSON(http.StatusOK, item)
}

func (h *AnnouncementHandler) Delete(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	err := h.service.Delete(ctx.Request.Context(), ctx.Param("announcementID"), identity.AdminID, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAnnouncementNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "announcement not found"})
		case errors.Is(err, service.ErrAnnouncementDeleteDenied):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "cannot delete this announcement"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete announcement"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}
