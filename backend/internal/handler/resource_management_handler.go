package handler

import (
	"errors"
	"strings"
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
	ProxySourceURL      string  `json:"proxy_source_url"`
	CoverURL            string  `json:"cover_url"`
	CustomPath          string  `json:"custom_path"`
	DownloadPolicy      *string `json:"download_policy"`
}

type updateManagedFolderDescriptionRequest struct {
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Remark           string  `json:"remark"`
	CoverURL         string  `json:"cover_url"`
	DirectLinkPrefix string  `json:"direct_link_prefix"`
	CdnURL           string  `json:"cdn_url"`
	CustomPath       string  `json:"custom_path"`
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
		ProxySourceURL:      req.ProxySourceURL,
		CoverURL:            req.CoverURL,
		CustomPath:          req.CustomPath,
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
		CdnURL:           req.CdnURL,
		CustomPath:       req.CustomPath,
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

// CreateVirtualFolder 创建虚拟目录（无物理磁盘路径，子文件通过 CDN 直链提供）。
func (h *ResourceManagementHandler) CreateVirtualFolder(ctx *gin.Context) {
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

	folder, err := h.service.CreateVirtualFolder(ctx.Request.Context(), service.CreateFolderInput{
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
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create virtual folder"})
		}
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"id": folder.ID, "name": folder.Name})
}

type createVirtualFileRequest struct {
	Name                string `json:"name"`
	FolderID            string `json:"folder_id"`
	PlaybackURL         string `json:"playback_url"`
	PlaybackFallbackURL string `json:"playback_fallback_url"`
	ProxySourceURL      string `json:"proxy_source_url"`
	ProxyDownload       bool   `json:"proxy_download"`
	Description         string `json:"description"`
	Remark              string `json:"remark"`
}

// CreateVirtualFile 在虚拟目录下创建虚拟文件。
func (h *ResourceManagementHandler) CreateVirtualFile(ctx *gin.Context) {
	adminIdentity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req createVirtualFileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	file, err := h.service.CreateVirtualFile(ctx.Request.Context(), service.CreateVirtualFileInput{
		Name:                req.Name,
		FolderID:            req.FolderID,
		PlaybackURL:         req.PlaybackURL,
		PlaybackFallbackURL: req.PlaybackFallbackURL,
		ProxySourceURL:      req.ProxySourceURL,
		ProxyDownload:       req.ProxyDownload,
		Description:         req.Description,
		Remark:              req.Remark,
		OperatorID:          adminIdentity.AdminID,
		OperatorIP:          ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidResourceEdit):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		case errors.Is(err, service.ErrManagedFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrManagedFileConflict):
			ctx.JSON(http.StatusConflict, gin.H{"error": "file name already exists"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create virtual file"})
		}
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"id": file.ID, "name": file.Name})
}

type patchFolderCdnUrlRequest struct {
	CdnURL string `json:"cdn_url"`
}

func (h *ResourceManagementHandler) PatchFolderCdnUrl(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req patchFolderCdnUrlRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	err := h.service.PatchFolderCdnUrl(ctx.Request.Context(), ctx.Param("folderID"), strings.TrimSpace(req.CdnURL), identity.AdminID, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagedFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update cdn url"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

type probeURLResponse struct {
	OK           bool   `json:"ok"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
	FileName     string `json:"file_name"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ProbeURL 由服务端发起 HEAD 请求检测 URL 可达性、文件大小和建议文件名。
func (h *ResourceManagementHandler) ProbeURL(ctx *gin.Context) {
	_, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	result := service.ProbeRemoteURL(ctx.Request.Context(), strings.TrimSpace(req.URL))
	ctx.JSON(http.StatusOK, result)
}
