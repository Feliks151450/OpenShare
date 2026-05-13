package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	coverURL, err := normalizeOptionalCoverURL(input.CoverURL)
	if err != nil {
		return ErrInvalidResourceEdit
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
