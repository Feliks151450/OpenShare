package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

func (s *ResourceManagementService) UpdateFile(ctx context.Context, fileID string, input UpdateManagedFileInput) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return ErrManagedFileNotFound
	}

	current, err := s.repo.FindFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}

	name, extension, ok := model.NormalizeManagedFileName(input.Name)
	if !ok {
		return ErrInvalidResourceEdit
	}
	description := normalizeTrimmedString(input.Description)
	remark := normalizeManagedRemark(input.Remark)
	playbackURL, err := normalizeOptionalHTTPURL(input.PlaybackURL)
	if err != nil {
		return ErrInvalidResourceEdit
	}
	playbackFallbackURL, err := normalizeOptionalHTTPURL(input.PlaybackFallbackURL)
	proxySourceURL := strings.TrimSpace(input.ProxySourceURL)
	if err != nil {
		return ErrInvalidResourceEdit
	}
	if playbackFallbackURL != "" && playbackURL == "" {
		return ErrInvalidResourceEdit
	}
	var coverURL *string
	if input.CoverURL != nil {
		normalized, err := normalizeOptionalCoverURL(*input.CoverURL)
		if err != nil {
			return ErrInvalidResourceEdit
		}
		coverURL = &normalized
	}

	applyDl, allowDl, err := parseDownloadPolicy(input.DownloadPolicy)
	if err != nil {
		return ErrInvalidResourceEdit
	}

	// 校验并处理 custom_path
	customPath := strings.TrimSpace(input.CustomPath)
	if err := ValidateCustomPath(customPath); err != nil {
		return fmt.Errorf("%w: custom_path 必须以英文字母开头，且不能与保留路径冲突", ErrInvalidResourceEdit)
	}

	// 检查 custom_path 唯一性（同时检查 folders 和 files 表，排除当前文件自身）
	cpConflict, err := s.repo.CustomPathExists(ctx, customPath, fileID)
	if err != nil {
		return fmt.Errorf("check custom path uniqueness: %w", err)
	}
	if cpConflict {
		return fmt.Errorf("%w: custom_path %q 已被使用", ErrManagedFileConflict, customPath)
	}

	if current.Name != name {
		fileConflict, err := s.repo.FileNameExists(ctx, current.FolderID, name, current.ID)
		if err != nil {
			return err
		}
		folderConflict, err := s.repo.FolderNameExists(ctx, current.FolderID, name, "")
		if err != nil {
			return err
		}
		if fileConflict || folderConflict {
			return ErrManagedFileConflict
		}
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate resource update log id: %w", err)
	}
	if current.Name != name {
		folder, err := s.repo.FindFolderByID(ctx, normalizeTrimmedString(modelValue(current.FolderID)))
		if err != nil {
			return err
		}
		currentPath := model.BuildManagedFilePath(folderSourcePath(folder), current.Name)
		if currentPath == "" {
			return ErrManagedFileNotFound
		}
		if _, err := s.storage.RenameManagedFile(currentPath, name); err != nil {
			if errors.Is(err, storage.ErrManagedFileConflict) {
				return ErrManagedFileConflict
			}
			return fmt.Errorf("rename managed file: %w", err)
		}
	}
	if err := s.repo.UpdateFileMetadata(ctx, fileID, name, extension, description, remark, playbackURL, playbackFallbackURL, proxySourceURL, coverURL, customPath, applyDl, allowDl, input.OperatorID, input.OperatorIP, logID, s.nowFunc()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFileNotFound
		}
		return fmt.Errorf("update managed file: %w", err)
	}
	return nil
}

// parseDownloadPolicy 解析管理端 download_policy：nil 表示不改数据库字段；inherit 写入 NULL。
func parseDownloadPolicy(raw *string) (apply bool, allowDownload *bool, err error) {
	if raw == nil {
		return false, nil, nil
	}
	s := strings.ToLower(strings.TrimSpace(*raw))
	if s == "" {
		return false, nil, nil
	}
	switch s {
	case "inherit":
		return true, nil, nil
	case "allow":
		v := true
		return true, &v, nil
	case "deny":
		v := false
		return true, &v, nil
	default:
		return false, nil, ErrInvalidResourceEdit
	}
}

func normalizeOptionalHTTPURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("url must use http or https")
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("url must include host")
	}
	return u.String(), nil
}

var internalFileCoverPathPattern = regexp.MustCompile(`^/files/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func normalizeOptionalCoverURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if internalFileCoverPathPattern.MatchString(s) {
		return s, nil
	}
	return normalizeOptionalHTTPURL(s)
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".jfif": true,
	".gif": true, ".webp": true, ".svg": true, ".bmp": true,
}

func effectiveFileCoverURL(storedCoverURL, extension, fileID string) string {
	if c := strings.TrimSpace(storedCoverURL); c != "" {
		return c
	}
	if !imageExtensions[strings.ToLower(strings.TrimSpace(extension))] {
		return ""
	}
	return "/files/" + fileID
}

const maxManagedRemarkRunes = 500

// normalizeManagedRemark 将备注规范为单行纯文本（换行等折叠为空格），最长 maxManagedRemarkRunes 个 Unicode 字符。
func normalizeManagedRemark(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = collapseSearchWhitespace(s)
	fields := strings.Fields(s)
	s = strings.Join(fields, " ")
	r := []rune(s)
	if len(r) > maxManagedRemarkRunes {
		return string(r[:maxManagedRemarkRunes])
	}
	return s
}

// ProbeURLResult 服务端 URL 探测结果。
type ProbeURLResult struct {
	OK           bool   `json:"ok"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
	FileName     string `json:"file_name"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ProbeRemoteURL 由服务端发起 HEAD 请求，检测 URL 可达性、文件大小和建议文件名。
func ProbeRemoteURL(ctx context.Context, rawURL string) ProbeURLResult {
	candidate, err := normalizeOptionalHTTPURL(rawURL)
	if err != nil || candidate == "" {
		return ProbeURLResult{ErrorMessage: "URL 格式无效"}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Head(candidate)
	if err != nil {
		return ProbeURLResult{ErrorMessage: fmt.Sprintf("无法连接: %s", err.Error())}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ProbeURLResult{ErrorMessage: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	fileName := extractSuggestedFileName(resp)
	return ProbeURLResult{
		OK:          true,
		Size:        resp.ContentLength,
		ContentType: resp.Header.Get("Content-Type"),
		FileName:    fileName,
	}
}

// extractSuggestedFileName 从 Content-Disposition 头或 URL 路径中提取建议文件名。
func extractSuggestedFileName(resp *http.Response) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		// 解析 attachment; filename="xxx" 或 inline; filename=xxx
		for _, part := range strings.Split(cd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(strings.ToLower(part), "filename=") {
				fn := strings.TrimPrefix(part, "filename=")
				fn = strings.TrimPrefix(fn, `"`)
				fn = strings.TrimSuffix(fn, `"`)
				fn = strings.TrimPrefix(fn, `'`)
				fn = strings.TrimSuffix(fn, `'`)
				fn = strings.TrimSpace(fn)
				if fn != "" {
					return fn
				}
			}
		}
	}
	// 从 URL 路径最后一段提取
	u, err := url.Parse(resp.Request.URL.String())
	if err == nil {
		path := strings.TrimSpace(u.Path)
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			seg := strings.TrimSpace(path[idx+1:])
			if seg != "" {
				decoded, err := url.PathUnescape(seg)
				if err == nil && decoded != "" {
					return decoded
				}
				return seg
			}
		}
	}
	return ""
}

// 封面图片上传限制：最大 10 MB，仅允许常见图片格式。
const maxCoverImageBytes = 10 << 20

var coverImageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".jfif": true,
	".gif": true, ".webp": true, ".svg": true, ".bmp": true,
}

// UploadCoverImage 将上传的封面图片保存到封面存储目录，并创建为托管文件返回站内链接。
func (s *ResourceManagementService) UploadCoverImage(ctx context.Context, reader io.Reader, originalName string, coverUploadDir string, operatorID string, operatorIP string) (string, error) {
	coverUploadDir = filepath.Clean(strings.TrimSpace(coverUploadDir))
	if coverUploadDir == "" || coverUploadDir == "." || coverUploadDir == "/" {
		return "", fmt.Errorf("%w: 封面存储目录未配置或无效", ErrInvalidResourceEdit)
	}

	ext := strings.ToLower(filepath.Ext(originalName))
	if !coverImageExtensions[ext] {
		return "", fmt.Errorf("%w: 不支持的图片格式: %s", ErrInvalidResourceEdit, ext)
	}

	// 确保封面目录在磁盘上存在。
	if err := s.storage.EnsureManagedDirectory(coverUploadDir); err != nil {
		return "", fmt.Errorf("ensure cover upload directory: %w", err)
	}

	// 查找或创建隐藏托管根目录。
	folderID, err := s.ensureCoverManagedRoot(ctx, coverUploadDir, operatorID, operatorIP, s.nowFunc())
	if err != nil {
		return "", fmt.Errorf("ensure cover managed root: %w", err)
	}

	// 生成文件 ID 并保存到磁盘。
	fileID, err := identity.NewID()
	if err != nil {
		return "", fmt.Errorf("generate cover file id: %w", err)
	}
	storedName := fileID + ext
	destPath := filepath.Join(coverUploadDir, storedName)

	srcFile, err := os.CreateTemp("", "openshare-cover-*")
	if err != nil {
		return "", fmt.Errorf("create cover temp file: %w", err)
	}
	tmpPath := srcFile.Name()
	defer os.Remove(tmpPath)

	written, err := io.Copy(srcFile, io.LimitReader(reader, maxCoverImageBytes+1))
	if err != nil {
		srcFile.Close()
		return "", fmt.Errorf("write cover temp file: %w", err)
	}
	if written == 0 {
		srcFile.Close()
		return "", fmt.Errorf("%w: 封面图片为空", ErrInvalidResourceEdit)
	}
	if written > maxCoverImageBytes {
		srcFile.Close()
		return "", fmt.Errorf("%w: 封面图片超过 10 MB 限制", ErrInvalidResourceEdit)
	}
	if err := srcFile.Close(); err != nil {
		return "", fmt.Errorf("close cover temp file: %w", err)
	}

	if err := storage.MoveRegularFile(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("move cover file to destination: %w", err)
	}

	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 磁盘文件名为 <fileID><ext>，DB name 必须与之匹配才能正确构建下载路径。
	// CustomPath 列有唯一索引，空字符串不能重复，封面文件无需短链接访问，用文件 ID 占位。
	now := s.nowFunc()
	coverCustomPath := "cover_" + fileID
	file := &model.File{
		ID:         fileID,
		FolderID:   &folderID,
		Name:       storedName,
		Extension:  ext,
		MimeType:   mimeType,
		Size:       written,
		CustomPath: coverCustomPath,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.CreateFile(ctx, file, operatorID, operatorIP, now); err != nil {
		// 清理已保存的磁盘文件。
		_ = os.Remove(destPath)
		return "", fmt.Errorf("create cover file record: %w", err)
	}

	return "/files/" + fileID, nil
}

// ensureCoverManagedRoot 查找或创建一个隐藏的托管根目录用于存储封面图片。
func (s *ResourceManagementService) ensureCoverManagedRoot(ctx context.Context, sourcePath string, operatorID string, operatorIP string, now time.Time) (string, error) {
	folder, err := s.repo.FindFolderBySourcePath(ctx, sourcePath)
	if err != nil {
		return "", err
	}
	if folder != nil {
		return folder.ID, nil
	}

	folderID, err := identity.NewID()
	if err != nil {
		return "", fmt.Errorf("generate cover folder id: %w", err)
	}
	dirName := filepath.Base(sourcePath)
	sourcePathCopy := sourcePath
	// CustomPath 有唯一索引，空串不能重复。封面托管根目录不需要短链接，用 folder ID 占位。
	folderCustomPath := "cover_root_" + folderID
	newFolder := &model.Folder{
		ID:                folderID,
		ParentID:          nil,
		SourcePath:        &sourcePathCopy,
		Name:              dirName,
		HidePublicCatalog: true,
		CustomPath:        folderCustomPath,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.CreateFolder(ctx, newFolder, operatorID, operatorIP, now); err != nil {
		return "", fmt.Errorf("create cover managed root folder: %w", err)
	}
	return folderID, nil
}

// ReplaceFile overwrites a managed file on disk with the uploaded content.
// Only works for managed files that have a physical disk path (non-virtual).
func (s *ResourceManagementService) ReplaceFile(ctx context.Context, fileID string, fileHeader *multipart.FileHeader, operatorID, operatorIP string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return ErrManagedFileNotFound
	}

	current, err := s.repo.FindFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}

	// Only managed files with a physical path can be replaced
	if current.FolderID == nil {
		return fmt.Errorf("%w: cannot replace a file without a parent folder", ErrInvalidResourceEdit)
	}
	folder, err := s.repo.FindFolderByID(ctx, *current.FolderID)
	if err != nil || folder == nil {
		return ErrManagedFileNotFound
	}
	if folder.IsVirtual {
		return fmt.Errorf("%w: cannot replace files in virtual directories", ErrInvalidResourceEdit)
	}

	// Build the disk path matching the original file name
	diskPath := model.BuildManagedFilePath(folder.SourcePath, current.Name)
	if diskPath == "" {
		return fmt.Errorf("%w: cannot resolve file disk path", ErrInvalidResourceEdit)
	}

	// Verify the uploaded file name matches the original (case-insensitive check for Windows)
	src, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open uploaded file: %w", err)
	}
	defer src.Close()

	// Write to a temp file first, then rename to avoid partial writes
	tmpPath := diskPath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write file content: %w", err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close destination file: %w", err)
	}

	// Atomic rename to replace the original
	if err := os.Rename(tmpPath, diskPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace file on disk: %w", err)
	}

	// Update file metadata (extension unchanged since file name stays the same)
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate log id: %w", err)
	}
	now := s.nowFunc()
	// Only update size and timestamp; keep other metadata unchanged
	_ = filepath.Ext(current.Name) // extension unchanged
	if err := s.repo.UpdateFileSize(ctx, fileID, written, operatorID, operatorIP, logID, now); err != nil {
		return fmt.Errorf("update file metadata after replace: %w", err)
	}

	return nil
}
