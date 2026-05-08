package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

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
	if err := s.repo.UpdateFileMetadata(ctx, fileID, name, extension, description, remark, playbackURL, playbackFallbackURL, coverURL, applyDl, allowDl, input.OperatorID, input.OperatorIP, logID, s.nowFunc()); err != nil {
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
