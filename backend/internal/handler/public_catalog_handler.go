package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

type PublicCatalogHandler struct {
	service       *service.PublicCatalogService
	systemSetting *service.SystemSettingService
}

func NewPublicCatalogHandler(service *service.PublicCatalogService, systemSetting *service.SystemSettingService) *PublicCatalogHandler {
	return &PublicCatalogHandler{service: service, systemSetting: systemSetting}
}

func (h *PublicCatalogHandler) ListPublicFolderFiles(ctx *gin.Context) {
	page, err := parseIntQuery(ctx.Query("page"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
		return
	}

	pageSize, err := parseIntQuery(ctx.Query("page_size"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid page_size"})
		return
	}

	result, err := h.service.ListPublicFolderFiles(ctx.Request.Context(), service.PublicFolderFileListInput{
		FolderID: ctx.Param("folderID"),
		Page:     page,
		PageSize: pageSize,
		Sort:     ctx.Query("sort"),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidPublicFileQuery):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		case errors.Is(err, service.ErrFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list public files"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (h *PublicCatalogHandler) ListHotFiles(ctx *gin.Context) {
	limit, err := parseIntQuery(ctx.Query("limit"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
		return
	}

	result, err := h.service.ListHotFiles(ctx.Request.Context(), limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list hot files"})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *PublicCatalogHandler) ListLatestFiles(ctx *gin.Context) {
	limit, err := parseIntQuery(ctx.Query("limit"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
		return
	}

	result, err := h.service.ListLatestFiles(ctx.Request.Context(), limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list latest files"})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *PublicCatalogHandler) ListPublicFolders(ctx *gin.Context) {
	parentID := ctx.Query("parent_id")
	items, err := h.service.ListPublicFolders(ctx.Request.Context(), parentID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list folders"})
		}
		return
	}

	resp := gin.H{"items": items}

	// 根目录请求时附带 download_policy，避免前端额外请求
	if strings.TrimSpace(parentID) == "" && h.systemSetting != nil {
		if policy, pErr := h.systemSetting.GetPolicy(ctx.Request.Context()); pErr == nil {
			resp["download_policy"] = gin.H{
				"large_download_confirm_bytes": policy.Download.LargeDownloadConfirmBytes,
				"wide_layout_extensions":       policy.Download.WideLayoutExtensions,
				"cdn_mode":                     policy.Download.CdnMode,
			}
		}
	}

	ctx.JSON(http.StatusOK, resp)
}

// ResolveCustomPath 根据自定义路径解析到文件夹或文件信息。
func (h *PublicCatalogHandler) ResolveCustomPath(ctx *gin.Context) {
	path := strings.TrimSpace(ctx.Query("path"))
	if path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	result, err := h.service.ResolveCustomPathFull(ctx.Request.Context(), path)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve custom path"})
		return
	}
	if result == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "custom path not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"type":      result.Type,
		"folder_id": result.FolderID,
		"file_id":   result.FileID,
		"name":      result.Name,
	})
}

func (h *PublicCatalogHandler) GetPublicFolderDetail(ctx *gin.Context) {
	detail, err := h.service.GetPublicFolderDetail(ctx.Request.Context(), ctx.Param("folderID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get folder detail"})
		}
		return
	}

	ctx.JSON(http.StatusOK, detail)
}

func parseIntQuery(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	return value, nil
}
