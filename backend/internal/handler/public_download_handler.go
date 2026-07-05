package handler

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

type PublicDownloadHandler struct {
	service    *service.PublicDownloadService
	catalogSvc *service.PublicCatalogService
}

type batchDownloadRequest struct {
	FileIDs []string `json:"file_ids"`
}

type resourceBatchDownloadRequest struct {
	FileIDs   []string `json:"file_ids"`
	FolderIDs []string `json:"folder_ids"`
}

func NewPublicDownloadHandler(service *service.PublicDownloadService, catalogSvc *service.PublicCatalogService) *PublicDownloadHandler {
	return &PublicDownloadHandler{service: service, catalogSvc: catalogSvc}
}

func (h *PublicDownloadHandler) DownloadFile(ctx *gin.Context) {
	download, err := h.service.PrepareDownload(ctx.Request.Context(), ctx.Param("fileID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "file is unavailable"})
		case errors.Is(err, service.ErrDownloadForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "download not allowed"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download file"})
		}
		return
	}

	// CDN 直链：302 跳转
	if download.RedirectURL != "" {
		ctx.Redirect(http.StatusFound, download.RedirectURL)
		return
	}

	// 服务端代理模式：反向代理到上游 URL，透传 Range 头以支持断点续传和视频拖动
	if download.ProxyURL != "" {
		h.proxyDownload(ctx, download)
		return
	}

	defer download.Content.Close()

	if download.MimeType != "" {
		ctx.Header("Content-Type", download.MimeType)
	}
	inlineQuery := strings.ToLower(strings.TrimSpace(ctx.Query("inline")))
	wantInlineEmbed := inlineQuery == "1" || inlineQuery == "true" || inlineQuery == "yes"
	inlineDisposition := download.PlaybackInlineOnly ||
		(wantInlineEmbed && service.InlineEmbedDispositionAllowed(download.MimeType, download.FileName))
	if inlineDisposition {
		ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", download.FileName))
	} else {
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.FileName))
	}
	ctx.Header("Content-Length", strconv.FormatInt(download.Size, 10))

	shouldRecord := !download.PlaybackInlineOnly
	if shouldRecord && wantInlineEmbed && inlineDisposition && service.InlineEmbedDispositionAllowed(download.MimeType, download.FileName) {
		shouldRecord = false
	}
	if shouldRecord {
		if err := h.service.RecordDownload(ctx.Request.Context(), download.FileID); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
			return
		}
	}

	http.ServeContent(ctx.Writer, ctx.Request, download.FileName, download.ModTime, download.Content)
}

// DownloadByPath 根据文件夹层级路径下载文件或打包下载文件夹。
// URL 格式：/api/public/dl/文件夹/子文件夹/文件名.pdf
// 路径段按 "/" 分割，逐级匹配文件夹名称，最后一段匹配文件名或文件夹名。
// 支持 ?inline=1 参数对支持的文件类型以内嵌方式返回。
func (h *PublicDownloadHandler) DownloadByPath(ctx *gin.Context) {
	rawPath := strings.TrimPrefix(ctx.Param("path"), "/")
	if rawPath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	// URL 解码（处理中文、空格等编码字符）
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid path encoding"})
		return
	}

	// 按 "/" 分割为路径段，过滤空段
	segments := strings.Split(decodedPath, "/")
	nonEmpty := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			nonEmpty = append(nonEmpty, seg)
		}
	}
	if len(nonEmpty) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	// 逐级解析路径
	result, err := h.catalogSvc.ResolvePathSegments(ctx.Request.Context(), nonEmpty)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve path"})
		return
	}
	if result == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "path not found"})
		return
	}

	// 文件夹：打包下载为 ZIP
	if result.Type == "folder" {
		h.downloadFolderByID(ctx, result.FolderID, result.Name)
		return
	}

	// 文件：复用完整的下载逻辑（CDN 重定向 / 代理 / 本地流式）
	h.downloadFileByID(ctx, result.FileID)
}

// downloadFileByID 根据文件 ID 执行下载，复用 PrepareDownload 的完整逻辑。
func (h *PublicDownloadHandler) downloadFileByID(ctx *gin.Context, fileID string) {
	download, err := h.service.PrepareDownload(ctx.Request.Context(), fileID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "file is unavailable"})
		case errors.Is(err, service.ErrDownloadForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "download not allowed"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download file"})
		}
		return
	}

	// CDN 直链：302 跳转
	if download.RedirectURL != "" {
		ctx.Redirect(http.StatusFound, download.RedirectURL)
		return
	}

	// 服务端代理模式
	if download.ProxyURL != "" {
		h.proxyDownload(ctx, download)
		return
	}

	defer download.Content.Close()

	if download.MimeType != "" {
		ctx.Header("Content-Type", download.MimeType)
	}
	inlineQuery := strings.ToLower(strings.TrimSpace(ctx.Query("inline")))
	wantInlineEmbed := inlineQuery == "1" || inlineQuery == "true" || inlineQuery == "yes"
	inlineDisposition := download.PlaybackInlineOnly ||
		(wantInlineEmbed && service.InlineEmbedDispositionAllowed(download.MimeType, download.FileName))
	if inlineDisposition {
		ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", download.FileName))
	} else {
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.FileName))
	}
	ctx.Header("Content-Length", strconv.FormatInt(download.Size, 10))

	shouldRecord := !download.PlaybackInlineOnly
	if shouldRecord && wantInlineEmbed && inlineDisposition && service.InlineEmbedDispositionAllowed(download.MimeType, download.FileName) {
		shouldRecord = false
	}
	if shouldRecord {
		if err := h.service.RecordDownload(ctx.Request.Context(), download.FileID); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
			return
		}
	}

	http.ServeContent(ctx.Writer, ctx.Request, download.FileName, download.ModTime, download.Content)
}

// downloadFolderByID 根据文件夹 ID 打包下载为 ZIP。
func (h *PublicDownloadHandler) downloadFolderByID(ctx *gin.Context, folderID string, folderName string) {
	download, err := h.service.PrepareFolderDownload(ctx.Request.Context(), folderID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrDownloadForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "download not allowed"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download folder"})
		}
		return
	}

	fileIDs := make([]string, 0, len(download.Items))
	for _, item := range download.Items {
		fileIDs = append(fileIDs, item.FileID)
	}
	if err := h.service.RecordBatchDownload(ctx.Request.Context(), fileIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.FolderName+".zip"))
	zipWriter := zip.NewWriter(ctx.Writer)
	usedNames := make(map[string]int, len(download.Items))

	for _, item := range download.Items {
		opened, openErr := h.service.PrepareDownload(ctx.Request.Context(), item.FileID)
		if openErr != nil {
			zipWriter.Close()
			return
		}
		entryName := uniqueZipEntryName(item.ZipPath, usedNames)
		entry, createErr := zipWriter.Create(entryName)
		if createErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		if _, copyErr := io.Copy(entry, opened.Content); copyErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		opened.Content.Close()
	}
	_ = zipWriter.Close()
}

// proxyDownload 反向代理上游 URL，透传 Range/If-Range 头，原样返回上游响应。
func (h *PublicDownloadHandler) proxyDownload(ctx *gin.Context, download *service.DownloadableFile) {
	proxyReq, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodGet, download.ProxyURL, nil)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
		return
	}

	// 透传 Range 头，支持断点续传和视频拖动进度条
	if rangeHdr := ctx.GetHeader("Range"); rangeHdr != "" {
		proxyReq.Header.Set("Range", rangeHdr)
	}
	if ifRange := ctx.GetHeader("If-Range"); ifRange != "" {
		proxyReq.Header.Set("If-Range", ifRange)
	}

	client := &http.Client{Timeout: 0} // 无超时，支持大文件和长时间流式传输
	resp, err := client.Do(proxyReq)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "upstream unreachable"})
		return
	}
	defer resp.Body.Close()

	// 透传上游响应头（跳过 hop-by-hop 头）
	hopByHop := map[string]bool{
		"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
		"Proxy-Authorization": true, "Te": true, "Trailer": true,
		"Transfer-Encoding": true, "Upgrade": true,
	}
	upstreamHasContentDisp := false
	for key, values := range resp.Header {
		if hopByHop[key] {
			continue
		}
		for _, v := range values {
			ctx.Header(key, v)
		}
		if strings.EqualFold(key, "Content-Disposition") {
			upstreamHasContentDisp = true
		}
	}

	// 上游未设置 Content-Disposition 时补一个
	if !upstreamHasContentDisp {
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.FileName))
	}

	ctx.Status(resp.StatusCode)
	io.Copy(ctx.Writer, resp.Body)
}

func (h *PublicDownloadHandler) DownloadFolder(ctx *gin.Context) {
	download, err := h.service.PrepareFolderDownload(ctx.Request.Context(), ctx.Param("folderID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "one or more files are unavailable"})
		case errors.Is(err, service.ErrDownloadForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "download not allowed"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download folder"})
		}
		return
	}

	fileIDs := make([]string, 0, len(download.Items))
	for _, item := range download.Items {
		fileIDs = append(fileIDs, item.FileID)
	}
	if err := h.service.RecordBatchDownload(ctx.Request.Context(), fileIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.FolderName+".zip"))
	zipWriter := zip.NewWriter(ctx.Writer)
	usedNames := make(map[string]int, len(download.Items))

	for _, item := range download.Items {
		opened, openErr := h.service.PrepareDownload(ctx.Request.Context(), item.FileID)
		if openErr != nil {
			zipWriter.Close()
			return
		}

		entryName := uniqueZipEntryName(item.ZipPath, usedNames)
		entry, createErr := zipWriter.Create(entryName)
		if createErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		if _, copyErr := io.Copy(entry, opened.Content); copyErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		opened.Content.Close()
	}
	_ = zipWriter.Close()
}

func (h *PublicDownloadHandler) GetNetCDFDump(ctx *gin.Context) {
	text, structure, truncated, err := h.service.PrepareNetCDFDump(ctx.Request.Context(), ctx.Param("fileID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		case errors.Is(err, service.ErrNetCDFNotApplicable):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is not a NetCDF (.nc) file"})
		case errors.Is(err, service.ErrDownloadForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "download not allowed"})
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to read NetCDF file"})
		}
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"text": text, "structure": structure, "truncated": truncated})
}

// GetNetCDFDumpFallback 使用系统 ncdump 命令作为回退方案获取 NetCDF 头部信息。
func (h *PublicDownloadHandler) GetNetCDFDumpFallback(ctx *gin.Context) {
	text, truncated, err := h.service.PrepareNetCDFDumpFallback(ctx.Request.Context(), ctx.Param("fileID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		case errors.Is(err, service.ErrNetCDFNotApplicable):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is not a NetCDF (.nc) file"})
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to read NetCDF file with ncdump fallback"})
		}
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"text": text, "truncated": truncated})
}

func (h *PublicDownloadHandler) GetFileDetail(ctx *gin.Context) {
	detail, err := h.service.GetFileDetail(ctx.Request.Context(), ctx.Param("fileID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load file detail"})
		}
		return
	}
	ctx.JSON(http.StatusOK, detail)
}

func (h *PublicDownloadHandler) DownloadBatch(ctx *gin.Context) {
	var req batchDownloadRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	files, err := h.service.PrepareBatchDownload(ctx.Request.Context(), req.FileIDs)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBatchDownloadInvalid):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file_ids is required"})
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "one or more files were not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "one or more files are unavailable"})
		case errors.Is(err, service.ErrDownloadForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "download not allowed"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare batch download"})
		}
		return
	}

	fileIDs := make([]string, 0, len(files))
	for _, item := range files {
		fileIDs = append(fileIDs, item.FileID)
	}
	if err := h.service.RecordBatchDownload(ctx.Request.Context(), fileIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", `attachment; filename="openshare-batch.zip"`)
	zipWriter := zip.NewWriter(ctx.Writer)
	usedNames := make(map[string]int, len(files))

	for _, item := range files {
		opened, openErr := h.service.PrepareDownload(ctx.Request.Context(), item.FileID)
		if openErr != nil {
			zipWriter.Close()
			return
		}

		entryName := uniqueZipEntryName(item.FileName, usedNames)
		entry, createErr := zipWriter.Create(entryName)
		if createErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		if _, copyErr := io.Copy(entry, opened.Content); copyErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		opened.Content.Close()
	}
	_ = zipWriter.Close()
}

func (h *PublicDownloadHandler) DownloadResourceBatch(ctx *gin.Context) {
	var req resourceBatchDownloadRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	files, err := h.service.PrepareResourceBatchDownload(ctx.Request.Context(), req.FileIDs, req.FolderIDs)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBatchDownloadInvalid):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file_ids or folder_ids is required"})
		case errors.Is(err, service.ErrDownloadFileNotFound), errors.Is(err, service.ErrDownloadFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "one or more resources were not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "one or more files are unavailable"})
		case errors.Is(err, service.ErrDownloadForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"error": "download not allowed"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare batch download"})
		}
		return
	}

	fileIDs := make([]string, 0, len(files))
	for _, item := range files {
		fileIDs = append(fileIDs, item.FileID)
	}
	if err := h.service.RecordBatchDownload(ctx.Request.Context(), fileIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", `attachment; filename="openshare-selection.zip"`)
	zipWriter := zip.NewWriter(ctx.Writer)
	usedNames := make(map[string]int, len(files))

	for _, item := range files {
		opened, openErr := h.service.PrepareDownload(ctx.Request.Context(), item.FileID)
		if openErr != nil {
			zipWriter.Close()
			return
		}

		entryName := uniqueZipEntryName(item.ZipPath, usedNames)
		entry, createErr := zipWriter.Create(entryName)
		if createErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		if _, copyErr := io.Copy(entry, opened.Content); copyErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		opened.Content.Close()
	}
	_ = zipWriter.Close()
}

func uniqueZipEntryName(originalName string, used map[string]int) string {
	originalName = strings.TrimSpace(originalName)
	if originalName == "" {
		originalName = "file"
	}
	if _, exists := used[originalName]; !exists {
		used[originalName] = 1
		return originalName
	}

	ext := ""
	base := originalName
	if dot := strings.LastIndex(originalName, "."); dot > 0 {
		base = originalName[:dot]
		ext = originalName[dot:]
	}
	next := used[originalName]
	used[originalName] = next + 1
	return fmt.Sprintf("%s_%d%s", base, next, ext)
}
