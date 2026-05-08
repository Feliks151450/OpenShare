package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

// ExportHandler 导出公开数据为静态 JSON，供 CDN 直链加载。
type ExportHandler struct {
	announcement   *service.AnnouncementService
	publicCatalog  *service.PublicCatalogService
	systemSetting  *service.SystemSettingService
	fileTag        *service.FileTagService
	imports        *service.ImportService
	publicDownload *service.PublicDownloadService
}

func NewExportHandler(
	announcement *service.AnnouncementService,
	publicCatalog *service.PublicCatalogService,
	systemSetting *service.SystemSettingService,
	fileTag *service.FileTagService,
	imports *service.ImportService,
	publicDownload *service.PublicDownloadService,
) *ExportHandler {
	return &ExportHandler{
		announcement:   announcement,
		publicCatalog:  publicCatalog,
		systemSetting:  systemSetting,
		fileTag:        fileTag,
		imports:        imports,
		publicDownload: publicDownload,
	}
}

// ─── 全局数据导出 ────────────────────────────────────────────────

type downloadPolicyExport struct {
	LargeDownloadConfirmBytes int64  `json:"large_download_confirm_bytes"`
	WideLayoutExtensions      string `json:"wide_layout_extensions"`
}

type GlobalExportData struct {
	Version        int                  `json:"version"`
	ExportedAt     string               `json:"exported_at"`
	Announcements  interface{}          `json:"announcements"`
	HotFiles       interface{}          `json:"hot_files"`
	LatestFiles    interface{}          `json:"latest_files"`
	RootFolders    interface{}          `json:"root_folders"`
	DownloadPolicy downloadPolicyExport `json:"download_policy"`
	FileTags       interface{}          `json:"file_tags"`
}

func (h *ExportHandler) ExportGlobal(ctx *gin.Context) {
	ctx2 := ctx.Request.Context()

	announcements, err := h.announcement.ListPublic(ctx2)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load announcements: %v", err)})
		return
	}

	hotFiles, err := h.publicCatalog.ListHotFiles(ctx2, 20)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load hot files: %v", err)})
		return
	}

	latestFiles, err := h.publicCatalog.ListLatestFiles(ctx2, 20)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load latest files: %v", err)})
		return
	}

	rootFolders, err := h.publicCatalog.ListPublicFolders(ctx2, "")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load root folders: %v", err)})
		return
	}

	policy, err := h.systemSetting.GetPolicy(ctx2)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load download policy: %v", err)})
		return
	}

	fileTags, err := h.fileTag.ListDefinitions(ctx2)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load file tags: %v", err)})
		return
	}

	data := GlobalExportData{
		Version:        1,
		ExportedAt:     time.Now().UTC().Format(time.RFC3339),
		Announcements:  announcements,
		HotFiles:       hotFiles,
		LatestFiles:    latestFiles,
		RootFolders:    rootFolders,
		DownloadPolicy: downloadPolicyExport{
			LargeDownloadConfirmBytes: policy.Download.LargeDownloadConfirmBytes,
			WideLayoutExtensions:      policy.Download.WideLayoutExtensions,
		},
		FileTags: fileTags,
	}

	ctx.JSON(http.StatusOK, data)
}

// ─── 托管目录导出 ──────────────────────────────────────────────

type DirectoryExportEntry struct {
	Detail      interface{}            `json:"detail"`
	Folders     interface{}            `json:"folders"`
	Files       interface{}            `json:"files"`
	FileDetails map[string]interface{} `json:"file_details"`
}

type DirectoryExportData struct {
	Version     int                            `json:"version"`
	ExportedAt  string                         `json:"exported_at"`
	ManagedRoot DirectoryExportManagedRoot     `json:"managed_root"`
	Directories map[string]DirectoryExportEntry `json:"directories"`
}

type DirectoryExportManagedRoot struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	CoverURL        string `json:"cover_url"`
	SourcePath      string `json:"source_path"`
	DownloadAllowed bool   `json:"download_allowed"`
	FileCount       int64  `json:"file_count"`
	DownloadCount   int64  `json:"download_count"`
	TotalSize       int64  `json:"total_size"`
	UpdatedAt       string `json:"updated_at"`
}

func (h *ExportHandler) ExportDirectory(ctx *gin.Context) {
	folderID := strings.TrimSpace(ctx.Param("folderID"))
	if folderID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing folder id"})
		return
	}

	ctx2 := ctx.Request.Context()

	tree, err := h.imports.GetFolderTree(ctx2)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load folder tree: %v", err)})
		return
	}

	// 找到指定的托管根
	target := findFolderInTree(tree, folderID)
	if target == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "managed directory not found"})
		return
	}

	// 收集所有子目录 ID
	allIDs := collectFolderIDs(target.Folders)

	rootDetail, err := h.publicCatalog.GetPublicFolderDetail(ctx2, folderID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load root detail: %v", err)})
		return
	}

	export := DirectoryExportData{
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		ManagedRoot: DirectoryExportManagedRoot{
			ID:              target.ID,
			Name:            target.Name,
			SourcePath:      target.SourcePath,
			DownloadAllowed: true,
			FileCount:       rootDetail.FileCount,
			DownloadCount:   rootDetail.DownloadCount,
			TotalSize:       rootDetail.TotalSize,
			UpdatedAt:       rootDetail.UpdatedAt.Format(time.RFC3339),
		},
		Directories: make(map[string]DirectoryExportEntry),
	}

	// 根目录详情
	rootFolders, _ := h.publicCatalog.ListPublicFolders(ctx2, folderID)
	rootFiles, _ := getAllPublicFolderFiles(ctx2, h.publicCatalog, folderID)
	export.Directories[folderID] = DirectoryExportEntry{
		Detail:      rootDetail,
		Folders:     rootFolders,
		Files:       rootFiles,
		FileDetails: buildFileDetails(ctx2, h.publicDownload, rootFiles),
	}

	// 每个子目录
	for _, id := range allIDs {
		folders, fErr := h.publicCatalog.ListPublicFolders(ctx2, id)
		if fErr != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load folders for %s: %v", id, fErr)})
			return
		}
		if folders == nil {
			folders = []service.PublicFolderItem{}
		}

		detail, dErr := h.publicCatalog.GetPublicFolderDetail(ctx2, id)
		if dErr != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load folder detail for %s: %v", id, dErr)})
			return
		}

		filesResult, flErr := getAllPublicFolderFiles(ctx2, h.publicCatalog, id)
		if flErr != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("load files for %s: %v", id, flErr)})
			return
		}

		export.Directories[id] = DirectoryExportEntry{
			Detail:      detail,
			Folders:     folders,
			Files:       filesResult,
			FileDetails: buildFileDetails(ctx2, h.publicDownload, filesResult),
		}
	}

	ctx.JSON(http.StatusOK, export)
}

func findFolderInTree(nodes []service.FolderTreeNode, id string) *service.FolderTreeNode {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
		if found := findFolderInTree(nodes[i].Folders, id); found != nil {
			return found
		}
	}
	return nil
}

func collectFolderIDs(nodes []service.FolderTreeNode) []string {
	var ids []string
	for _, n := range nodes {
		ids = append(ids, n.ID)
		if len(n.Folders) > 0 {
			ids = append(ids, collectFolderIDs(n.Folders)...)
		}
	}
	return ids
}

// getAllPublicFolderFiles 分页拉取文件夹下全部文件。
func getAllPublicFolderFiles(ctx context.Context, catalog *service.PublicCatalogService, folderID string) ([]service.PublicFileItem, error) {
	const pageSize = 100
	var all []service.PublicFileItem
	page := 1
	for {
		result, err := catalog.ListPublicFolderFiles(ctx, service.PublicFolderFileListInput{
			FolderID: folderID,
			Page:     page,
			PageSize: pageSize,
			Sort:     "name_asc",
		})
		if err != nil {
			return nil, err
		}
		all = append(all, result.Items...)
		if int64(len(all)) >= result.Total || len(result.Items) < pageSize {
			break
		}
		page++
	}
	return all, nil
}

// buildFileDetails 为文件列表中的每个文件获取完整详情。
func buildFileDetails(ctx context.Context, download *service.PublicDownloadService, files []service.PublicFileItem) map[string]interface{} {
	if download == nil || len(files) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(files))
	for _, f := range files {
		detail, err := download.GetFileDetail(ctx, f.ID)
		if err != nil {
			continue
		}
		result[f.ID] = detail
	}
	return result
}
