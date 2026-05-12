package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
)

var (
	ErrDownloadFileNotFound    = errors.New("download file not found")
	ErrDownloadFolderNotFound  = errors.New("download folder not found")
	ErrDownloadFileUnavailable = errors.New("download file unavailable")
	ErrBatchDownloadInvalid    = errors.New("invalid batch download request")
	ErrDownloadForbidden       = errors.New("download not allowed")
)

type PublicDownloadService struct {
	repository *repository.PublicDownloadRepository
	storage    *storage.Service
	fileTags   *FileTagService
}

type DownloadableFile struct {
	FileID   string
	FileName string
	MimeType string
	Size     int64
	ModTime  time.Time
	Content  *os.File
	// RedirectURL 非空表示虚拟文件，应 302 跳转到 CDN 直链，不读本地磁盘。
	RedirectURL string
	// PlaybackInlineOnly 为 true 时表示策略禁止「下载」，但仍允许浏览器内嵌播放音/视频（inline），不计入下载次数。
	PlaybackInlineOnly bool
}

type PublicFileDetail struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Extension           string `json:"extension"`
	FolderID            string `json:"folder_id"`
	Path                string `json:"path"`
	StoragePath         string `json:"storage_path,omitempty"`
	Description         string `json:"description"`
	Remark              string `json:"remark"`
	MimeType            string `json:"mime_type"`
	PlaybackURL         string `json:"playback_url"`
	PlaybackFallbackURL string `json:"playback_fallback_url"`
	CoverURL            string `json:"cover_url"`
	// FolderDirectDownloadURL 由文件夹直链前缀 + 相对路径生成；不含 playback_url。前端优先使用 playback_url。
	FolderDirectDownloadURL string `json:"folder_direct_download_url"`
	DownloadAllowed         bool   `json:"download_allowed"`
	// DownloadPolicy 本节点存储：inherit | allow | deny（不含继承解析）
	DownloadPolicy string    `json:"download_policy"`
	Size           int64     `json:"size"`
	UploadedAt     time.Time `json:"uploaded_at"`
	DownloadCount  int64     `json:"download_count"`
	Tags           []PublicFileTag `json:"tags"`
}

type BatchDownloadFile struct {
	FileID   string
	FileName string
	DiskPath string
	ZipPath  string
}

type FolderDownload struct {
	FolderID   string
	FolderName string
	Items      []BatchDownloadFile
}

func NewPublicDownloadService(
	repository *repository.PublicDownloadRepository,
	storageService *storage.Service,
	fileTags *FileTagService,
) *PublicDownloadService {
	return &PublicDownloadService{
		repository: repository,
		storage:    storageService,
		fileTags:   fileTags,
	}
}

// DownloadPolicyString 将库中的可空布尔转为管理端/前端三态字符串。
func DownloadPolicyString(p *bool) string {
	if p == nil {
		return "inherit"
	}
	if *p {
		return "allow"
	}
	return "deny"
}

// inlinePlaybackAllowedWhenDownloadForbidden 策略禁止下载时仍允许通过本站 URL 内嵌播放音/视频，以及浏览器内预览的常见图文/CSV/PDF。
func inlinePlaybackAllowedWhenDownloadForbidden(mimeType, fileName string) bool {
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mt, "video/") || strings.HasPrefix(mt, "audio/") {
		return true
	}
	if strings.HasPrefix(mt, "image/") {
		return true
	}
	switch mt {
	case "application/pdf", "text/plain", "text/markdown", "text/csv", "text/tab-separated-values":
		return true
	default:
		if strings.Contains(mt, "markdown") {
			return true
		}
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".mp4", ".webm", ".mov", ".m4v", ".ogv", ".mkv", ".avi",
		".mp3", ".wav", ".aac", ".m4a", ".oga", ".ogg", ".opus", ".flac",
		".png", ".jpg", ".jpeg", ".jfif", ".gif", ".webp", ".svg", ".bmp",
		".pdf", ".txt", ".md", ".markdown", ".csv", ".tsv", ".nc":
		return true
	default:
		return false
	}
}

// InlineEmbedDispositionAllowed 是否允许在带 ?inline=1 时使用 Content-Disposition: inline（PDF、图片等内嵌预览）。
func InlineEmbedDispositionAllowed(mimeType, fileName string) bool {
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	if mt == "application/pdf" || strings.HasPrefix(mt, "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".pdf", ".png", ".jpg", ".jpeg", ".jfif", ".gif", ".webp", ".svg", ".bmp":
		return true
	default:
		return false
	}
}

// EffectiveDownloadAllowedForFile 解析文件是否允许下载：文件单独设置优先，否则自文件所在文件夹向根查找，均未设置则默认允许。
func (s *PublicDownloadService) EffectiveDownloadAllowedForFile(ctx context.Context, file *model.File) (bool, error) {
	if file == nil {
		return true, nil
	}
	if file.AllowDownload != nil {
		return *file.AllowDownload, nil
	}
	if file.FolderID == nil || strings.TrimSpace(*file.FolderID) == "" {
		return true, nil
	}
	chain, err := s.repository.ListFolderAncestorsFromLeaf(ctx, strings.TrimSpace(*file.FolderID))
	if err != nil {
		return false, err
	}
	for i := range chain {
		if chain[i].AllowDownload != nil {
			return *chain[i].AllowDownload, nil
		}
	}
	return true, nil
}

// folderAncestorCache 批量预取文件夹祖先链缓存，避免 N+1 查询。
type folderAncestorCache struct {
	chains map[string][]model.Folder
}

// newFolderAncestorCache 对给定文件夹 ID 集合每个仅查一次祖先链。
func (s *PublicDownloadService) newFolderAncestorCache(ctx context.Context, folderIDs []string) (*folderAncestorCache, error) {
	c := &folderAncestorCache{chains: make(map[string][]model.Folder, len(folderIDs))}
	for _, fid := range folderIDs {
		if fid == "" {
			continue
		}
		chain, err := s.repository.ListFolderAncestorsFromLeaf(ctx, fid)
		if err != nil {
			return nil, err
		}
		c.chains[fid] = chain
	}
	return c, nil
}

func (c *folderAncestorCache) effectiveDownloadAllowedForFile(file *model.File) bool {
	if file == nil {
		return true
	}
	if file.AllowDownload != nil {
		return *file.AllowDownload
	}
	fid := ""
	if file.FolderID != nil {
		fid = strings.TrimSpace(*file.FolderID)
	}
	if fid == "" {
		return true
	}
	for _, f := range c.chains[fid] {
		if f.AllowDownload != nil {
			return *f.AllowDownload
		}
	}
	return true
}

func (c *folderAncestorCache) folderDirectDownloadURLForFile(file model.File) string {
	if file.FolderID == nil {
		return ""
	}
	fid := strings.TrimSpace(*file.FolderID)
	chain := c.chains[fid]
	if len(chain) == 0 {
		return ""
	}
	for i := 0; i < len(chain); i++ {
		prefix := strings.TrimSpace(chain[i].DirectLinkPrefix)
		if prefix == "" {
			continue
		}
		return folderDirectFileURL(prefix, chain, i, file.Name)
	}
	return ""
}

// EffectiveDownloadAllowedForFolder 解析文件夹是否允许打包下载：本文件夹设置优先，否则向上查找祖先，均未设置则默认允许。
func (s *PublicDownloadService) EffectiveDownloadAllowedForFolder(ctx context.Context, folder *model.Folder) (bool, error) {
	if folder == nil {
		return true, nil
	}
	if folder.AllowDownload != nil {
		return *folder.AllowDownload, nil
	}
	chain, err := s.repository.ListFolderAncestorsFromLeaf(ctx, folder.ID)
	if err != nil {
		return false, err
	}
	for i := range chain {
		if chain[i].AllowDownload != nil {
			return *chain[i].AllowDownload, nil
		}
	}
	return true, nil
}

func (s *PublicDownloadService) PrepareDownload(ctx context.Context, fileID string) (*DownloadableFile, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, ErrDownloadFileNotFound
	}

	file, err := s.repository.FindManagedFileByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("find downloadable file: %w", err)
	}
	if file == nil {
		return nil, ErrDownloadFileNotFound
	}

	// 虚拟目录下的文件：通过 CDN 直链（playback_url）302 跳转，不读本地磁盘
	if file.FolderID != nil && strings.TrimSpace(*file.FolderID) != "" {
		if virtual, checkErr := s.isFileInVirtualFolder(ctx, *file); checkErr == nil && virtual {
			redirectURL := strings.TrimSpace(file.PlaybackURL)
			if redirectURL == "" {
				return nil, ErrDownloadFileUnavailable
			}
			return &DownloadableFile{
				FileID:      file.ID,
				FileName:    file.Name,
				MimeType:    file.MimeType,
				Size:        file.Size,
				RedirectURL: redirectURL,
			}, nil
		}
	}

	allowed, err := s.EffectiveDownloadAllowedForFile(ctx, file)
	if err != nil {
		return nil, err
	}
	playbackInlineOnly := false
	if !allowed {
		if !inlinePlaybackAllowedWhenDownloadForbidden(file.MimeType, file.Name) {
			return nil, ErrDownloadForbidden
		}
		playbackInlineOnly = true
	}

	diskPath, err := s.resolveManagedFilePath(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("resolve downloadable file path: %w", err)
	}

	opened, err := s.storage.OpenManagedFile(diskPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDownloadFileUnavailable
		}
		return nil, fmt.Errorf("open downloadable file: %w", err)
	}

	return &DownloadableFile{
		FileID:             file.ID,
		FileName:           file.Name,
		MimeType:           file.MimeType,
		Size:               opened.Info.Size(),
		ModTime:            opened.Info.ModTime(),
		Content:            opened.File,
		PlaybackInlineOnly: playbackInlineOnly,
	}, nil
}

// isFileInVirtualFolder 检查文件所属目录是否为虚拟目录。
func (s *PublicDownloadService) isFileInVirtualFolder(ctx context.Context, file model.File) (bool, error) {
	folders, err := s.repository.ListManagedFoldersByIDs(ctx, []string{strings.TrimSpace(*file.FolderID)})
	if err != nil {
		return false, err
	}
	if len(folders) == 0 {
		return false, nil
	}
	return folders[0].IsVirtual, nil
}

func (s *PublicDownloadService) GetFileDetail(ctx context.Context, fileID string) (*PublicFileDetail, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, ErrDownloadFileNotFound
	}
	file, err := s.repository.FindManagedFileByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("find public file detail: %w", err)
	}
	if file == nil {
		return nil, ErrDownloadFileNotFound
	}

	fullPath, err := s.buildFilePath(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("build public file path: %w", err)
	}

	storagePath := ""
	if resolvedStorage, err := s.resolveManagedFilePath(ctx, file); err == nil {
		storagePath = strings.TrimSpace(resolvedStorage)
	}

	dlAllowed, err := s.EffectiveDownloadAllowedForFile(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("resolve download policy: %w", err)
	}

	tags := []PublicFileTag{}
	if s.fileTags != nil {
		if loaded, err := s.fileTags.ListTagsForFile(ctx, file.ID); err == nil {
			tags = loaded
		}
	}

	return &PublicFileDetail{
		ID:                      file.ID,
		Name:                    file.Name,
		Extension:               file.Extension,
		FolderID:                strings.TrimSpace(optionalString(file.FolderID)),
		Path:                    fullPath,
		StoragePath:             storagePath,
		Description:             file.Description,
		Remark:                  file.Remark,
		MimeType:                file.MimeType,
		PlaybackURL:             strings.TrimSpace(file.PlaybackURL),
		PlaybackFallbackURL:     strings.TrimSpace(file.PlaybackFallbackURL),
		CoverURL:                effectiveFileCoverURL(file.CoverURL, file.Extension, file.ID),
		FolderDirectDownloadURL: s.FolderDirectDownloadURLForFile(ctx, *file),
		DownloadAllowed:         dlAllowed,
		DownloadPolicy:          DownloadPolicyString(file.AllowDownload),
		Size:                    file.Size,
		UploadedAt:              file.CreatedAt,
		DownloadCount:           file.DownloadCount,
		Tags:                    tags,
	}, nil
}

// FolderDirectDownloadURLForFile 仅根据祖先文件夹中「最靠近文件」的直链前缀拼接相对路径，不含文件单独配置的 playback_url。
func (s *PublicDownloadService) FolderDirectDownloadURLForFile(ctx context.Context, file model.File) string {
	if file.FolderID == nil {
		return ""
	}
	chain, err := s.repository.ListFolderAncestorsFromLeaf(ctx, strings.TrimSpace(*file.FolderID))
	if err != nil || len(chain) == 0 {
		return ""
	}
	for i := 0; i < len(chain); i++ {
		prefix := strings.TrimSpace(chain[i].DirectLinkPrefix)
		if prefix == "" {
			continue
		}
		return folderDirectFileURL(prefix, chain, i, file.Name)
	}
	return ""
}

func folderDirectFileURL(prefix string, chain []model.Folder, baseIndex int, fileName string) string {
	var segments []string
	if baseIndex > 0 {
		for j := baseIndex - 1; j >= 0; j-- {
			segments = append(segments, chain[j].Name)
		}
	}
	segments = append(segments, fileName)
	return joinURLPrefixWithPathSegments(prefix, segments)
}

func joinURLPrefixWithPathSegments(prefix string, segments []string) string {
	p := strings.TrimRight(strings.TrimSpace(prefix), "/")
	var b strings.Builder
	b.WriteString(p)
	for _, seg := range segments {
		b.WriteString("/")
		b.WriteString(url.PathEscape(seg))
	}
	return b.String()
}

func (s *PublicDownloadService) buildFilePath(ctx context.Context, file *model.File) (string, error) {
	if file.FolderID == nil || strings.TrimSpace(*file.FolderID) == "" {
		return "主页根目录", nil
	}

	folderIDs := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	currentID := strings.TrimSpace(*file.FolderID)

	for currentID != "" {
		if _, ok := seen[currentID]; ok {
			break
		}
		seen[currentID] = struct{}{}
		folderIDs = append(folderIDs, currentID)

		folders, err := s.repository.ListManagedFoldersByIDs(ctx, []string{currentID})
		if err != nil {
			return "", err
		}
		if len(folders) == 0 || folders[0].ParentID == nil {
			break
		}
		currentID = strings.TrimSpace(*folders[0].ParentID)
	}

	folders, err := s.repository.ListManagedFoldersByIDs(ctx, folderIDs)
	if err != nil {
		return "", err
	}

	byID := make(map[string]repository.ManagedFolderNode, len(folders))
	for _, folder := range folders {
		byID[folder.ID] = folder
	}

	segments := make([]string, 0, len(folderIDs)+1)
	currentID = strings.TrimSpace(*file.FolderID)
	for currentID != "" {
		folder, ok := byID[currentID]
		if !ok {
			break
		}
		segments = append([]string{folder.Name}, segments...)
		if folder.ParentID == nil {
			break
		}
		currentID = strings.TrimSpace(*folder.ParentID)
	}

	if len(segments) == 0 {
		return "主页根目录", nil
	}
	return strings.Join(segments, " / "), nil
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *PublicDownloadService) resolveManagedFilePath(ctx context.Context, file *model.File) (string, error) {
	if file == nil {
		return "", ErrDownloadFileNotFound
	}
	if file.FolderID == nil || strings.TrimSpace(*file.FolderID) == "" {
		return "", ErrDownloadFileUnavailable
	}

	folders, err := s.repository.ListManagedFoldersByIDs(ctx, []string{strings.TrimSpace(*file.FolderID)})
	if err != nil {
		return "", err
	}
	if len(folders) == 0 {
		return "", ErrDownloadFileUnavailable
	}

	return s.resolveManagedFilePathFromFolderMap(*file, map[string]repository.ManagedFolderNode{
		folders[0].ID: folders[0],
	})
}

func (s *PublicDownloadService) resolveManagedFilePathFromFolderMap(file model.File, folderByID map[string]repository.ManagedFolderNode) (string, error) {
	if file.FolderID == nil || strings.TrimSpace(*file.FolderID) == "" {
		return "", ErrDownloadFileUnavailable
	}

	folder, ok := folderByID[strings.TrimSpace(*file.FolderID)]
	if !ok {
		return "", ErrDownloadFileUnavailable
	}

	filePath := model.BuildManagedFilePath(folder.SourcePath, file.Name)
	if filePath == "" {
		return "", ErrDownloadFileUnavailable
	}
	return filePath, nil
}

func (s *PublicDownloadService) PrepareBatchDownload(ctx context.Context, fileIDs []string) ([]BatchDownloadFile, error) {
	normalized := normalizeBatchFileIDs(fileIDs)
	if len(normalized) == 0 {
		return nil, ErrBatchDownloadInvalid
	}

	files, err := s.repository.ListManagedFilesByIDs(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("list batch download files: %w", err)
	}
	if len(files) != len(normalized) {
		return nil, ErrDownloadFileNotFound
	}

	for i := range files {
		allowed, err := s.EffectiveDownloadAllowedForFile(ctx, &files[i])
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrDownloadForbidden
		}
	}

	folderByID, err := s.folderMapForFiles(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("load folder download paths: %w", err)
	}

	items := make([]BatchDownloadFile, 0, len(files))
	for _, file := range files {
		filePath, err := s.resolveManagedFilePathFromFolderMap(file, folderByID)
		if err != nil {
			return nil, err
		}

		opened, err := s.storage.OpenManagedFile(filePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, ErrDownloadFileUnavailable
			}
			return nil, fmt.Errorf("validate batch download file: %w", err)
		}
		opened.File.Close()

		items = append(items, BatchDownloadFile{
			FileID:   file.ID,
			FileName: file.Name,
			DiskPath: filePath,
			ZipPath:  file.Name,
		})
	}
	return items, nil
}

func (s *PublicDownloadService) PrepareResourceBatchDownload(ctx context.Context, fileIDs []string, folderIDs []string) ([]BatchDownloadFile, error) {
	normalizedFiles := normalizeBatchFileIDs(fileIDs)
	normalizedFolders := normalizeBatchFileIDs(folderIDs)
	if len(normalizedFiles) == 0 && len(normalizedFolders) == 0 {
		return nil, ErrBatchDownloadInvalid
	}

	items := make([]BatchDownloadFile, 0, len(normalizedFiles))
	if len(normalizedFiles) > 0 {
		files, err := s.PrepareBatchDownload(ctx, normalizedFiles)
		if err != nil {
			return nil, err
		}
		items = append(items, files...)
	}

	for _, folderID := range normalizedFolders {
		folderDownload, err := s.PrepareFolderDownload(ctx, folderID)
		if err != nil {
			return nil, err
		}
		items = append(items, folderDownload.Items...)
	}

	if len(items) == 0 {
		return nil, ErrBatchDownloadInvalid
	}

	return items, nil
}

func (s *PublicDownloadService) PrepareFolderDownload(ctx context.Context, folderID string) (*FolderDownload, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return nil, ErrDownloadFolderNotFound
	}

	root, err := s.repository.FindManagedFolderByID(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("find downloadable folder: %w", err)
	}
	if root == nil {
		return nil, ErrDownloadFolderNotFound
	}

	// 虚拟目录无本地文件，不支持整目录下载
	if root.IsVirtual {
		return nil, ErrDownloadFolderNotFound
	}

	allowed, err := s.EffectiveDownloadAllowedForFolder(ctx, root)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrDownloadForbidden
	}

	parentByFolder := map[string]string{root.ID: ""}
	nameByFolder := map[string]string{root.ID: root.Name}
	folderByID := map[string]repository.ManagedFolderNode{
		root.ID: {
			ID:         root.ID,
			ParentID:   root.ParentID,
			Name:       root.Name,
			SourcePath: root.SourcePath,
		},
	}
	allFolderIDs := []string{root.ID}
	currentLevel := []string{root.ID}

	for len(currentLevel) > 0 {
		children, err := s.repository.ListManagedFoldersByParentIDs(ctx, currentLevel)
		if err != nil {
			return nil, fmt.Errorf("list descendant folders: %w", err)
		}

		nextLevel := make([]string, 0, len(children))
		for _, child := range children {
			nameByFolder[child.ID] = child.Name
			folderByID[child.ID] = child
			if child.ParentID != nil {
				parentByFolder[child.ID] = *child.ParentID
			}
			allFolderIDs = append(allFolderIDs, child.ID)
			nextLevel = append(nextLevel, child.ID)
		}
		currentLevel = nextLevel
	}

	files, err := s.repository.ListManagedFilesByFolderIDs(ctx, allFolderIDs)
	if err != nil {
		return nil, fmt.Errorf("list folder download files: %w", err)
	}

	items := make([]BatchDownloadFile, 0, len(files))
	for _, file := range files {
		allowedFile, err := s.EffectiveDownloadAllowedForFile(ctx, &file)
		if err != nil {
			return nil, err
		}
		if !allowedFile {
			continue
		}

		filePath, err := s.resolveManagedFilePathFromFolderMap(file, folderByID)
		if err != nil {
			return nil, err
		}

		opened, err := s.storage.OpenManagedFile(filePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, ErrDownloadFileUnavailable
			}
			return nil, fmt.Errorf("validate folder download file: %w", err)
		}
		opened.File.Close()

		items = append(items, BatchDownloadFile{
			FileID:   file.ID,
			FileName: file.Name,
			DiskPath: filePath,
			ZipPath:  buildFolderZipPath(file.Name, file.FolderID, parentByFolder, nameByFolder),
		})
	}

	if len(items) == 0 && len(files) > 0 {
		return nil, ErrDownloadForbidden
	}

	return &FolderDownload{
		FolderID:   root.ID,
		FolderName: root.Name,
		Items:      items,
	}, nil
}

func (s *PublicDownloadService) RecordDownload(ctx context.Context, fileID string) error {
	return s.repository.IncrementDownloadCount(ctx, fileID)
}

func (s *PublicDownloadService) RecordBatchDownload(ctx context.Context, fileIDs []string) error {
	normalized := normalizeBatchFileIDs(fileIDs)
	if len(normalized) == 0 {
		return nil
	}
	return s.repository.IncrementDownloadCounts(ctx, normalized)
}

func normalizeBatchFileIDs(fileIDs []string) []string {
	normalized := make([]string, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		fileID = strings.TrimSpace(fileID)
		if fileID == "" || slices.Contains(normalized, fileID) {
			continue
		}
		normalized = append(normalized, fileID)
	}
	return normalized
}

func (s *PublicDownloadService) folderMapForFiles(ctx context.Context, files []model.File) (map[string]repository.ManagedFolderNode, error) {
	folderIDs := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if file.FolderID == nil || strings.TrimSpace(*file.FolderID) == "" {
			continue
		}
		folderID := strings.TrimSpace(*file.FolderID)
		if _, ok := seen[folderID]; ok {
			continue
		}
		seen[folderID] = struct{}{}
		folderIDs = append(folderIDs, folderID)
	}

	rows, err := s.repository.ListManagedFoldersByIDs(ctx, folderIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[string]repository.ManagedFolderNode, len(rows))
	for _, row := range rows {
		result[row.ID] = row
	}
	return result, nil
}

func buildFolderZipPath(fileName string, folderID *string, parentByFolder map[string]string, nameByFolder map[string]string) string {
	parts := []string{fileName}
	if folderID == nil {
		return fileName
	}

	currentID := *folderID
	for currentID != "" {
		name := nameByFolder[currentID]
		if name != "" {
			parts = append([]string{name}, parts...)
		}
		currentID = parentByFolder[currentID]
	}

	return strings.Join(parts, "/")
}
