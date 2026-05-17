package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/config"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

const systemPolicyKey = "system_policy"

// DefaultLargeDownloadConfirmBytes 超过该大小的单文件在访客端下载前会弹出确认（文件夹打包下载始终确认）。可由超级管理员在系统设置中调整。
const DefaultLargeDownloadConfirmBytes int64 = 1024 * 1024 * 1024

var ErrInvalidDownloadPolicyInput = errors.New("invalid download policy input")

type UploadPolicy struct {
	MaxUploadTotalBytes int64 `json:"max_upload_total_bytes"`
}

func (p *UploadPolicy) UnmarshalJSON(data []byte) error {
	type uploadPolicyAlias struct {
		MaxUploadTotalBytes int64 `json:"max_upload_total_bytes"`
		MaxFileSizeBytes    int64 `json:"max_file_size_bytes"`
	}

	var raw uploadPolicyAlias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.MaxUploadTotalBytes = raw.MaxUploadTotalBytes
	if p.MaxUploadTotalBytes <= 0 {
		p.MaxUploadTotalBytes = raw.MaxFileSizeBytes
	}
	return nil
}

type DownloadPolicy struct {
	LargeDownloadConfirmBytes int64  `json:"large_download_confirm_bytes"`
	WideLayoutExtensions      string `json:"wide_layout_extensions"`
	CdnMode                   bool   `json:"cdn_mode"`
	GlobalCdnUrl              string `json:"global_cdn_url"`
}

type SystemPolicy struct {
	Upload         UploadPolicy   `json:"upload"`
	Download       DownloadPolicy `json:"download"`
	CoverUploadDir string         `json:"cover_upload_dir"`
}

type SystemSettingService struct {
	repo          *repository.SystemSettingRepository
	defaultPolicy SystemPolicy
	nowFunc       func() time.Time
}

func defaultSystemPolicy(uploadCfg config.UploadConfig, coverUploadDir string) SystemPolicy {
	return SystemPolicy{
		Upload: UploadPolicy{
			MaxUploadTotalBytes: uploadCfg.MaxUploadTotalBytes,
		},
		Download: DownloadPolicy{
			LargeDownloadConfirmBytes: DefaultLargeDownloadConfirmBytes,
		},
		CoverUploadDir: coverUploadDir,
	}
}

func NewSystemSettingService(repo *repository.SystemSettingRepository, cfg config.Config) *SystemSettingService {
	return &SystemSettingService{
		repo:          repo,
		defaultPolicy: defaultSystemPolicy(cfg.Upload, cfg.Storage.CoverUploadDir),
		nowFunc:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *SystemSettingService) GetPolicy(ctx context.Context) (*SystemPolicy, error) {
	item, err := s.repo.FindByKey(ctx, systemPolicyKey)
	if err != nil {
		return nil, err
	}
	if item == nil || strings.TrimSpace(item.Value) == "" {
		policy := s.defaultPolicy
		return &policy, nil
	}

	var policy SystemPolicy
	if err := json.Unmarshal([]byte(item.Value), &policy); err != nil {
		return nil, fmt.Errorf("decode system policy: %w", err)
	}
	if policy.Download.LargeDownloadConfirmBytes <= 0 {
		policy.Download.LargeDownloadConfirmBytes = s.defaultPolicy.Download.LargeDownloadConfirmBytes
	}
	if policy.CoverUploadDir == "" {
		policy.CoverUploadDir = s.defaultPolicy.CoverUploadDir
	}
	return &policy, nil
}

func validateLargeDownloadConfirmBytes(v int64) error {
	const maxBytes = 1024 * 1024 * 1024 * 1024 * 1024 // 1 PiB
	if v < 1 {
		return ErrInvalidDownloadPolicyInput
	}
	if v > maxBytes {
		return ErrInvalidDownloadPolicyInput
	}
	return nil
}

func (s *SystemSettingService) SavePolicy(ctx context.Context, incoming SystemPolicy, operatorID string, operatorIP string) (*SystemPolicy, error) {
	baseline, err := s.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	policy := *baseline

	if incoming.Upload.MaxUploadTotalBytes > 0 {
		policy.Upload = incoming.Upload
	}
	if incoming.Download.LargeDownloadConfirmBytes > 0 {
		policy.Download.LargeDownloadConfirmBytes = incoming.Download.LargeDownloadConfirmBytes
	}
	policy.Download.WideLayoutExtensions = incoming.Download.WideLayoutExtensions
	policy.Download.CdnMode = incoming.Download.CdnMode
	policy.Download.GlobalCdnUrl = incoming.Download.GlobalCdnUrl
	if incoming.CoverUploadDir != "" || policy.CoverUploadDir == "" {
		policy.CoverUploadDir = incoming.CoverUploadDir
	}

	if policy.Upload.MaxUploadTotalBytes <= 0 {
		return nil, ErrInvalidUploadInput
	}
	if policy.Download.LargeDownloadConfirmBytes <= 0 {
		policy.Download.LargeDownloadConfirmBytes = baseline.Download.LargeDownloadConfirmBytes
	}
	if err := validateLargeDownloadConfirmBytes(policy.Download.LargeDownloadConfirmBytes); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("encode system policy: %w", err)
	}
	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate system policy log id: %w", err)
	}
	if err := s.repo.UpsertWithLog(ctx, systemPolicyKey, string(payload), operatorID, operatorIP, logID, s.nowFunc()); err != nil {
		return nil, fmt.Errorf("save system policy: %w", err)
	}
	return &policy, nil
}
