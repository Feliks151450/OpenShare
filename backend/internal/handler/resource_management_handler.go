package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type ResourceManagementHandler struct {
	service     *service.ResourceManagementService
	authService *service.AdminAuthService
}

type updateManagedFileRequest struct {
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	Remark              string  `json:"remark"`
	PlaybackURL         string  `json:"playback_url"`
	PlaybackFallbackURL string  `json:"playback_fallback_url"`
	CoverURL            string  `json:"cover_url"`
	DownloadPolicy      *string `json:"download_policy"`
}

type updateManagedFolderDescriptionRequest struct {
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Remark           string  `json:"remark"`
	CoverURL         string  `json:"cover_url"`
	DirectLinkPrefix string  `json:"direct_link_prefix"`
	DownloadPolicy   *string `json:"download_policy"`
}

type createManagedFolderRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
}

type deleteManagedResourceRequest struct {
	Password    string `json:"password"`
	MoveToTrash *bool  `json:"move_to_trash"`
}

func NewResourceManagementHandler(service *service.ResourceManagementService, authService *service.AdminAuthService) *ResourceManagementHandler {
	return &ResourceManagementHandler{service: service, authService: authService}
}

func (h *ResourceManagementHandler) ListFiles(ctx *gin.Context) {
	items, err := h.service.ListFiles(ctx.Request.Context(), service.ListManagedFilesInput{
		Query: ctx.Query("q"),
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
		Name:                req.Name,
		Description:         req.Description,
		Remark:              req.Remark,
		PlaybackURL:         req.PlaybackURL,
		PlaybackFallbackURL: req.PlaybackFallbackURL,
		CoverURL:            req.CoverURL,
		DownloadPolicy:      req.DownloadPolicy,
		OperatorID:          identity.AdminID,
		OperatorIP:          ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidResourceEdit):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource input"})
		case errors.Is(err, service.ErrManagedFileConflict):
			ctx.JSON(http.StatusConflict, gin.H{"error": "file name already exists"})
		case errors.Is(err, service.ErrManagedFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update resource"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

type patchFolderCatalogVisibilityRequest struct {
	HidePublicCatalog bool `json:"hide_public_catalog"`
}

func (h *ResourceManagementHandler) UpdateFolderDescription(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req updateManagedFolderDescriptionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.UpdateFolderDescription(ctx.Request.Context(), ctx.Param("folderID"), service.UpdateManagedFolderDescriptionInput{
		Name:             req.Name,
		Description:      req.Description,
		Remark:           req.Remark,
		CoverURL:         req.CoverURL,
		DirectLinkPrefix: req.DirectLinkPrefix,
		DownloadPolicy:   req.DownloadPolicy,
		OperatorID:       identity.AdminID,
		OperatorIP:       ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidResourceEdit):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder input"})
		case errors.Is(err, service.ErrManagedFolderConflict):
			ctx.JSON(http.StatusConflict, gin.H{"error": "folder name already exists"})
		case errors.Is(err, service.ErrManagedFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update folder"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *ResourceManagementHandler) PatchFolderCatalogVisibility(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req patchFolderCatalogVisibilityRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.PatchRootFolderHidePublicCatalog(
		ctx.Request.Context(),
		ctx.Param("folderID"),
		req.HidePublicCatalog,
		identity.AdminID,
		ctx.ClientIP(),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagedFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrInvalidResourceEdit):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "managed root folder required"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update folder visibility"})
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

	var req deleteManagedResourceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.authService.VerifyPassword(ctx.Request.Context(), identity.AdminID, req.Password); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAdminCredentials):
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify password"})
		}
		return
	}

	moveToTrash := true
	if req.MoveToTrash != nil {
		moveToTrash = *req.MoveToTrash
	}

	err := h.service.DeleteFile(ctx.Request.Context(), ctx.Param("fileID"), identity.AdminID, ctx.ClientIP(), moveToTrash)
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

func (h *ResourceManagementHandler) DeleteFolder(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req deleteManagedResourceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.authService.VerifyPassword(ctx.Request.Context(), identity.AdminID, req.Password); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAdminCredentials):
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify password"})
		}
		return
	}

	moveToTrash := true
	if req.MoveToTrash != nil {
		moveToTrash = *req.MoveToTrash
	}

	err := h.service.DeleteFolder(ctx.Request.Context(), ctx.Param("folderID"), identity.AdminID, ctx.ClientIP(), moveToTrash)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagedFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete folder"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *ResourceManagementHandler) CreateFolder(ctx *gin.Context) {
	adminIdentity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req createManagedFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	folder, err := h.service.CreateFolder(ctx.Request.Context(), service.CreateFolderInput{
		Name:       req.Name,
		ParentID:   req.ParentID,
		OperatorID: adminIdentity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidResourceEdit):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder name"})
		case errors.Is(err, service.ErrManagedFolderConflict):
			ctx.JSON(http.StatusConflict, gin.H{"error": "folder name already exists"})
		case errors.Is(err, service.ErrManagedFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "parent folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create folder"})
		}
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"id": folder.ID, "name": folder.Name})
}
