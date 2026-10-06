package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

var (
	ErrInvalidUploadInput   = errors.New("invalid upload input")
	ErrUploadReceiptExists  = errors.New("receipt code already exists")
	ErrUploadTooLarge       = errors.New("upload too large")
	ErrUploadEmptyFile      = errors.New("upload file is empty")
	ErrUploadNameConflict   = errors.New("upload name conflict")
	ErrFileIDConflict       = errors.New("file_id already exists")
	ErrReceiptCodeGenerate  = errors.New("failed to generate receipt code")
	ErrUploadFolderRequired = errors.New("upload target folder is required")
	ErrUploadFolderNotFound = errors.New("upload target folder not found")
)

const maxGeneratedReceiptAttempts = 5

type PublicUploadService struct {
	config        config.UploadConfig
	repository    *repository.UploadRepository
	receiptCodes  *ReceiptCodeService
	storage       *storage.Service
	systemSetting *SystemSettingService
	nowFunc       func() time.Time
}

type PublicUploadInput struct {
	Description string
	ReceiptCode string
	FolderID    string
	UploaderIP  string
	Files       []PublicUploadFileInput
	Overwrite   bool
	OverwriteID bool
}

type PublicUploadFileInput struct {
	Name         string
	RelativePath string
	File         io.Reader
	ID           string
}

type PublicUploadResult struct {
	ReceiptCode string                 `json:"receipt_code"`
	Status      model.SubmissionStatus `json:"status"`
	ItemCount   int                    `json:"item_count"`
	Names       []string               `json:"names"`
	UploadedAt  time.Time              `json:"uploaded_at"`
}

func NewPublicUploadService(
	cfg config.UploadConfig,
	repository *repository.UploadRepository,
	receiptCodes *ReceiptCodeService,
	storageService *storage.Service,
	systemSettingService *SystemSettingService,
) *PublicUploadService {
	return &PublicUploadService{
		config:        cfg,
		repository:    repository,
		receiptCodes:  receiptCodes,
		storage:       storageService,
		systemSetting: systemSettingService,
		nowFunc:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *PublicUploadService) CreateSubmission(ctx context.Context, input PublicUploadInput) (*PublicUploadResult, error) {
	policy := s.effectivePolicy(ctx)
	actor, canDirectPublish := publicUploadActorFromContext(ctx)
	normalized, err := s.normalizeInput(input)
	if err != nil {
		return nil, err
	}
	if len(normalized.Files) == 0 {
		return nil, ErrInvalidUploadInput
	}
	if normalized.FolderID == "" {
		return nil, ErrUploadFolderRequired
	}

	rootFolder, err := s.repository.FindManagedFolderByID(ctx, normalized.FolderID)
	if err != nil {
		return nil, fmt.Errorf("validate upload folder: %w", err)
	}
	if rootFolder == nil || rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, ErrUploadFolderNotFound
	}

	rootFolderDisplayPath, err := s.repository.BuildFolderDisplayPath(ctx, &rootFolder.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve upload folder path: %w", err)
	}
	if err := s.resolveUploadFileNames(ctx, rootFolder, rootFolderDisplayPath, normalized.Files, normalized.Overwrite); err != nil {
		return nil, err
	}

	receiptCode, err := s.resolveReceiptCode(ctx, normalized.ReceiptCode)
	if err != nil {
		return nil, err
	}

	now := s.nowFunc()

	submissions := make([]model.Submission, 0, len(normalized.Files))
	stagedPaths := make([]string, 0, len(normalized.Files))
	names := make([]string, 0, len(normalized.Files))
	folderID := normalized.FolderID

	for _, entry := range normalized.Files {
		bufferedReader := bufio.NewReader(entry.File)
		head, inspectErr := bufferedReader.Peek(512)
		if inspectErr != nil && !errors.Is(inspectErr, io.EOF) {
			err = fmt.Errorf("inspect upload file: %w", inspectErr)
			break
		}

		detectedMIME := strings.ToLower(strings.TrimSpace(http.DetectContentType(head)))

		maxUploadTotalBytes := s.config.MaxUploadTotalBytes
		if policy.Upload.MaxUploadTotalBytes > 0 {
			maxUploadTotalBytes = policy.Upload.MaxUploadTotalBytes
		}
		stagedFile, saveErr := s.storage.SaveToStaging(bufferedReader, entry.Extension, maxUploadTotalBytes)
		if saveErr != nil {
			switch {
			case errors.Is(saveErr, storage.ErrFileTooLarge):
				err = ErrUploadTooLarge
			case strings.Contains(strings.ToLower(saveErr.Error()), "empty file"):
				err = ErrUploadEmptyFile
			default:
				err = fmt.Errorf("save upload to staging: %w", saveErr)
			}
			break
		}
		stagedPaths = append(stagedPaths, stagedFile.DiskPath)

		submissionID, idErr := identity.NewID()
		if idErr != nil {
			err = fmt.Errorf("generate submission id: %w", idErr)
			break
		}

		folderRef := folderID
		submission := model.Submission{
			ID:           submissionID,
			ReceiptCode:  receiptCode,
			FolderID:     &folderRef,
			Name:         entry.Name,
			Description:  normalized.Description,
			RelativePath: entry.RelativePath,
			Extension:    entry.Extension,
			MimeType:     detectedMIME,
			Size:         stagedFile.Size,
			StagingPath:  stagedFile.DiskPath,
			Status:       model.SubmissionStatusPending,
			UploaderIP:   normalized.UploaderIP,
			FileInputID:  entry.ID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		submissions = append(submissions, submission)
		names = append(names, submission.Name)
	}

	if err != nil {
		s.cleanupStagedPaths(stagedPaths)
		return nil, err
	}

	if canDirectPublish {
		result, publishErr := s.publishDirectUploadBatch(ctx, rootFolder, submissions, actor.AdminID, normalized.UploaderIP, now, normalized.Overwrite, normalized.OverwriteID)
		if publishErr != nil {
			s.cleanupStagedSubmissions(submissions)
			return nil, publishErr
		}
		return result, nil
	}

	if createErr := s.repository.CreateUploadBatch(ctx, submissions); createErr != nil {
		s.cleanupStagedSubmissions(submissions)
		return nil, fmt.Errorf("persist upload submission: %w", createErr)
	}

	return &PublicUploadResult{
		ReceiptCode: receiptCode,
		Status:      model.SubmissionStatusPending,
		ItemCount:   len(submissions),
		Names:       names,
		UploadedAt:  now,
	}, nil
}

type directPublishedUpload struct {
	submission        model.Submission
	finalPath         string
	finalName         string
	finalRelativePath string
}

// trashedOverwrittenFile 记录被覆盖旧文件的回收站位置与原路径，
// 发布流程失败时用于把旧文件还原，保证覆盖操作可回滚。
type trashedOverwrittenFile struct {
	trashPath    string
	originalPath string
}

func (s *PublicUploadService) publishDirectUploadBatch(
	ctx context.Context,
	rootFolder *model.Folder,
	submissions []model.Submission,
	adminID string,
	operatorIP string,
	reviewedAt time.Time,
	overwrite bool,
	overwriteID bool,
) (*PublicUploadResult, error) {
	if rootFolder == nil || rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, ErrUploadFolderNotFound
	}

	published := make([]directPublishedUpload, 0, len(submissions))
	trashedOldFiles := make([]trashedOverwrittenFile, 0)
	newItems := make([]repository.ApprovedUploadBatchItem, 0)
	newSubmissions := make([]model.Submission, 0)
	allNames := make([]string, 0, len(submissions))
	rootSourcePath := strings.TrimSpace(*rootFolder.SourcePath)

	for _, submission := range submissions {
		relativeDir := repository.NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Dir(submission.RelativePath)))
		targetDir := rootSourcePath
		if relativeDir != "" {
			targetDir = filepath.Join(rootSourcePath, filepath.FromSlash(relativeDir))
		}
		if err := s.storage.EnsureManagedDirectory(targetDir); err != nil {
			rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
			if rollbackErr != nil {
				return nil, fmt.Errorf("ensure direct upload target directory failed (%v); rollback failed: %w", err, rollbackErr)
			}
			return nil, fmt.Errorf("ensure direct upload target directory: %w", err)
		}

		// 定位本次上传目标目录对应的 DB 目录记录：
		// 根目录必然存在；子目录记录缺失时说明磁盘文件无 DB 记录可覆盖，按新文件处理。
		targetFolderID := rootFolder.ID
		if relativeDir != "" {
			resolvedID, resolveErr := s.repository.FindFolderIDBySourcePath(ctx, targetDir)
			if resolveErr != nil {
				rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
				if rollbackErr != nil {
					return nil, fmt.Errorf("resolve target folder failed (%v); rollback failed: %w", resolveErr, rollbackErr)
				}
				return nil, fmt.Errorf("resolve target folder for overwrite: %w", resolveErr)
			}
			if resolvedID != "" {
				targetFolderID = resolvedID
			}
		}

		// 覆盖模式：检查是否已有同名文件，有则替换内容而非创建新记录。
		// 顺序：先更新 DB，再把旧磁盘文件移入回收站（可恢复），最后移入新文件。
		replaced := false
		if overwrite {
			destPath := filepath.Join(targetDir, filepath.Base(submission.Name))
			if _, statErr := os.Stat(destPath); statErr == nil {
				// 数据库：找到旧记录，更新元数据而非创建新记录
				existingID, updateErr := s.repository.UpdateExistingFileMetadata(
					ctx, &targetFolderID, submission.Name,
					submission.Size, submission.Extension, submission.MimeType, reviewedAt,
				)
				if updateErr != nil {
					rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
					if rollbackErr != nil {
						return nil, fmt.Errorf("update existing file metadata failed (%v); rollback failed: %w", updateErr, rollbackErr)
					}
					return nil, fmt.Errorf("update existing file metadata: %w", updateErr)
				}
				// 磁盘：旧文件移入回收站（不再永久删除），后续失败时回滚还原
				trashPath, trashErr := s.storage.MoveManagedFileToTrash(destPath)
				if trashErr != nil {
					rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
					if rollbackErr != nil {
						return nil, fmt.Errorf("overwrite existing file on disk failed (%v); rollback failed: %w", trashErr, rollbackErr)
					}
					return nil, fmt.Errorf("move overwritten file to trash: %w", trashErr)
				}
				if trashPath != "" {
					trashedOldFiles = append(trashedOldFiles, trashedOverwrittenFile{trashPath: trashPath, originalPath: destPath})
				}
				if existingID != "" {
					replaced = true
				}
			}
		}

		// 指定 file_id 冲突处理
		if submission.FileInputID != "" && !replaced {
			exists, existsErr := s.repository.FileIDExists(ctx, submission.FileInputID)
			if existsErr != nil {
				return nil, fmt.Errorf("check file id existence: %w", existsErr)
			}
			if exists {
				if !overwriteID {
					return nil, ErrFileIDConflict
				}
				// 覆盖模式：更新现有文件元数据（记录迁移到本次目标目录，跨目录调整统计）
				oldName, oldFolderPath, updateErr := s.repository.UpdateExistingFileByID(
					ctx, submission.FileInputID, &submission, targetFolderID, reviewedAt,
				)
				if updateErr != nil {
					rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
					if rollbackErr != nil {
						return nil, fmt.Errorf("update existing file by id failed (%v); rollback failed: %w", updateErr, rollbackErr)
					}
					return nil, fmt.Errorf("update existing file by id: %w", updateErr)
				}
				// 旧磁盘文件已不再被记录引用（记录已指向本次目标目录），
				// 名称不同或目录不同都需要清理；移入回收站以保留恢复能力。
				if oldName != "" && oldFolderPath != "" {
					oldPath := filepath.Join(oldFolderPath, filepath.Base(oldName))
					stale := oldFolderPath != targetDir || oldName != submission.Name
					if stale {
						if _, statErr := os.Stat(oldPath); statErr == nil {
							trashPath, trashErr := s.storage.MoveManagedFileToTrash(oldPath)
							if trashErr != nil {
								rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
								if rollbackErr != nil {
									return nil, fmt.Errorf("remove old file for id overwrite failed (%v); rollback failed: %w", trashErr, rollbackErr)
								}
								return nil, fmt.Errorf("move stale file of id overwrite to trash: %w", trashErr)
							}
							if trashPath != "" {
								trashedOldFiles = append(trashedOldFiles, trashedOverwrittenFile{trashPath: trashPath, originalPath: oldPath})
							}
						}
					}
				}
				replaced = true
			}
		}

		finalPath, finalName, err := s.storage.MoveStagedFileToFolder(submission.StagingPath, targetDir, submission.Name)
		if err != nil {
			rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
			if rollbackErr != nil {
				return nil, fmt.Errorf("direct upload move failed (%v); rollback failed: %w", err, rollbackErr)
			}
			return nil, fmt.Errorf("move direct upload into managed folder: %w", err)
		}

		finalRelativePath := replaceRelativePathBase(submission.RelativePath, finalName)
		published = append(published, directPublishedUpload{
			submission:        submission,
			finalPath:         finalPath,
			finalName:         finalName,
			finalRelativePath: finalRelativePath,
		})
		allNames = append(allNames, finalName)

		if replaced {
			// 已替换现有文件：不创建新的 DB 记录
			continue
		}
		newItems = append(newItems, repository.ApprovedUploadBatchItem{
			SubmissionID:      submission.ID,
			FinalName:         finalName,
			FinalRelativePath: finalRelativePath,
		})
		newSubmissions = append(newSubmissions, submission)
	}

	if len(newItems) > 0 {
		if err := s.repository.CreateApprovedUploadBatch(ctx, rootFolder, newSubmissions, newItems, adminID, operatorIP, reviewedAt); err != nil {
			rollbackErr := s.rollbackDirectPublishedUploads(published, trashedOldFiles)
			if rollbackErr != nil {
				return nil, fmt.Errorf("persist direct upload failed (%v); rollback failed: %w", err, rollbackErr)
			}
			return nil, fmt.Errorf("persist direct upload: %w", err)
		}
	}

	return &PublicUploadResult{
		ReceiptCode: submissions[0].ReceiptCode,
		Status:      model.SubmissionStatusApproved,
		ItemCount:   len(allNames),
		Names:       allNames,
		UploadedAt:  reviewedAt,
	}, nil
}

// rollbackDirectPublishedUploads 把已发布的新文件移回暂存区、把被覆盖的旧文件从回收站
// 还原到原位。任何单项失败都会记录并继续处理剩余项（errors.Join 汇总），避免中途放弃
// 留下更多无主文件。
func (s *PublicUploadService) rollbackDirectPublishedUploads(published []directPublishedUpload, trashedOldFiles []trashedOverwrittenFile) error {
	var errs []error
	for i := len(published) - 1; i >= 0; i-- {
		entry := published[i]
		if _, err := s.storage.MoveFileBackToStaging(entry.finalPath, entry.submission.StagingPath); err != nil {
			errs = append(errs, fmt.Errorf("rollback published file %s: %w", entry.finalPath, err))
		}
	}
	for i := len(trashedOldFiles) - 1; i >= 0; i-- {
		entry := trashedOldFiles[i]
		if err := storage.MoveRegularFile(entry.trashPath, entry.originalPath); err != nil {
			errs = append(errs, fmt.Errorf("restore overwritten file %s from trash: %w", entry.originalPath, err))
		}
	}
	return errors.Join(errs...)
}

func (s *PublicUploadService) cleanupStagedSubmissions(submissions []model.Submission) {
	for _, submission := range submissions {
		if strings.TrimSpace(submission.StagingPath) != "" {
			_ = s.storage.DeleteStagedFile(submission.StagingPath)
		}
	}
}

func (s *PublicUploadService) cleanupStagedPaths(paths []string) {
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			_ = s.storage.DeleteStagedFile(path)
		}
	}
}

func (s *PublicUploadService) MaxUploadTotalBytes(ctx context.Context) int64 {
	policy := s.effectivePolicy(ctx)
	if policy.Upload.MaxUploadTotalBytes > 0 {
		return policy.Upload.MaxUploadTotalBytes
	}
	return s.config.MaxUploadTotalBytes
}
