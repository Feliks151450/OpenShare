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
var ErrInvalidGuestAccessInput = errors.New("invalid guest access input")

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

// GuestAccessKey 管理员配置的访客密钥条目。
// ID 由后端生成（UUID）；Value 是访客需要在网页端输入的明文密钥；
// Hint 是可选的失败提示文案，访客输入错误时由后端 401 响应带回。
type GuestAccessKey struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Hint  string `json:"hint,omitempty"`
}

// GuestAccessPolicy 访客密钥访问的全局策略。
// Enabled 为 false 时即使目录设置了 guest_key_required 也不在校验链路中触发密钥输入。
type GuestAccessPolicy struct {
	Enabled bool             `json:"enabled"`
	Keys    []GuestAccessKey `json:"keys"`
}

type SystemPolicy struct {
	Upload         UploadPolicy       `json:"upload"`
	Download       DownloadPolicy     `json:"download"`
	CoverUploadDir string             `json:"cover_upload_dir"`
	GuestAccess    GuestAccessPolicy  `json:"guest_access"`
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
		GuestAccess: GuestAccessPolicy{
			Enabled: false,
			Keys:    nil,
		},
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
	// 访客密钥访问：必须通过专用端点 PUT /api/admin/system/guest-access 管理。
	// 在通用 SavePolicy 中保留 baseline，不随上传/下载策略一起被覆盖。
	policy.GuestAccess = baseline.GuestAccess

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

// SaveGuestAccess 替换 SystemPolicy.GuestAccess 字段，其他字段保持不变。
// 配套校验：每个密钥必须拥有非空 ID/Name/Value；ID 若缺失则由调用方预生成。
func (s *SystemSettingService) SaveGuestAccess(ctx context.Context, incoming GuestAccessPolicy, operatorID string, operatorIP string) (*GuestAccessPolicy, error) {
	if err := normalizeGuestAccess(&incoming); err != nil {
		return nil, err
	}
	baseline, err := s.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	policy := *baseline
	policy.GuestAccess = incoming

	payload, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("encode system policy: %w", err)
	}
	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate guest access log id: %w", err)
	}
	if err := s.repo.UpsertWithLog(ctx, systemPolicyKey, string(payload), operatorID, operatorIP, logID, s.nowFunc()); err != nil {
		return nil, fmt.Errorf("save guest access policy: %w", err)
	}
	return &policy.GuestAccess, nil
}

// normalizeGuestAccess 校验密钥条目：ID/Name/Value 缺一不可；hint 与 value 都不允许换行（防止注入）。
func normalizeGuestAccess(p *GuestAccessPolicy) error {
	if p == nil {
		return ErrInvalidGuestAccessInput
	}
	seen := make(map[string]struct{}, len(p.Keys))
	for i := range p.Keys {
		k := &p.Keys[i]
		k.ID = strings.TrimSpace(k.ID)
		k.Name = strings.TrimSpace(k.Name)
		k.Value = strings.TrimSpace(k.Value)
		k.Hint = strings.TrimSpace(k.Hint)
		if strings.ContainsAny(k.Value, "\r\n") || strings.ContainsAny(k.Hint, "\r\n") {
			return ErrInvalidGuestAccessInput
		}
		if k.ID == "" || k.Name == "" || k.Value == "" {
			return ErrInvalidGuestAccessInput
		}
		if _, dup := seen[k.ID]; dup {
			return ErrInvalidGuestAccessInput
		}
		seen[k.ID] = struct{}{}
	}
	return nil
}

// FindGuestAccessKeyByValue 在启用策略中按值匹配密钥，返回其 ID（用于 validate 端点）。
// disabled 策略下不返回任何匹配。
func (s *SystemSettingService) FindGuestAccessKeyByValue(ctx context.Context, value string) (keyID string, hint string, ok bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", "", false
	}
	policy, err := s.GetPolicy(ctx)
	if err != nil || policy == nil || !policy.GuestAccess.Enabled {
		return "", "", false
	}
	for _, k := range policy.GuestAccess.Keys {
		if k.Value == trimmed {
			return k.ID, k.Hint, true
		}
	}
	// 仅展示 hint 时（hint 非空且没有任何 key 匹配），仍展示配置的 hint。
	if len(policy.GuestAccess.Keys) > 0 {
		// 取第一条 hint 作为通用提示（避免泄露具体格式）
		for _, k := range policy.GuestAccess.Keys {
			if k.Hint != "" {
				return "", k.Hint, false
			}
		}
	}
	return "", "", false
}
