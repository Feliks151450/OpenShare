package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type SystemSettingHandler struct {
	service     *service.SystemSettingService
	importRepo  *repository.ImportRepository
}

func NewSystemSettingHandler(service *service.SystemSettingService, importRepo *repository.ImportRepository) *SystemSettingHandler {
	return &SystemSettingHandler{service: service, importRepo: importRepo}
}

func (h *SystemSettingHandler) GetPolicy(ctx *gin.Context) {
	policy, err := h.service.GetPolicy(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load system settings"})
		return
	}
	ctx.JSON(http.StatusOK, policy)
}

func (h *SystemSettingHandler) GetPublicDownloadPolicy(ctx *gin.Context) {
	policy, err := h.service.GetPolicy(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load download policy"})
		return
	}
	resp := gin.H{
		"large_download_confirm_bytes": policy.Download.LargeDownloadConfirmBytes,
		"cdn_mode":                     policy.Download.CdnMode,
		"wide_layout_extensions":       policy.Download.WideLayoutExtensions,
		"pdf_preview_method":           policy.Download.PdfPreviewMethod,
	}
	if policy.Download.CdnMode {
		resp["global_cdn_url"] = policy.Download.GlobalCdnUrl
	}

	if policy.Download.CdnMode && h.importRepo != nil {
		if rows, rErr := h.importRepo.ListManagedRootCdnUrls(ctx.Request.Context()); rErr == nil {
			m := make(map[string]string, len(rows))
			for _, row := range rows {
				m[row.ID] = row.CdnURL
			}
			resp["directory_cdn_urls"] = m
		}
	}
	ctx.JSON(http.StatusOK, resp)
}

func (h *SystemSettingHandler) SavePolicy(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req service.SystemPolicy
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	policy, err := h.service.SavePolicy(ctx.Request.Context(), req, identity.AdminID, ctx.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrInvalidUploadInput) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid system settings"})
			return
		}
		if errors.Is(err, service.ErrInvalidDownloadPolicyInput) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid download policy"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save system settings"})
		return
	}
	ctx.JSON(http.StatusOK, policy)
}
