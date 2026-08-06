package service

import (
	"context"
	"errors"
	"strings"

	"openshare/backend/internal/repository"
)

// GuestAccessRequirement 描述某个目录对访客密钥的要求。
// Required=false 表示无需密钥；否则需要 AllowedKeyIDs 中的密钥匹配。
type GuestAccessRequirement struct {
	Required      bool
	AllowedKeyIDs []string
}

// GuestAccessService 集中处理访客密钥的解析与校验。
// 通过 SystemSettingService 读取全局策略（启用 / 密钥池），
// 通过 GuestKeyAssignmentRepository 读取每目录允许的密钥子集。
type GuestAccessService struct {
	settings *SystemSettingService
	keyRepo  *repository.GuestKeyAssignmentRepository
}

func NewGuestAccessService(settings *SystemSettingService, keyRepo *repository.GuestKeyAssignmentRepository) *GuestAccessService {
	return &GuestAccessService{settings: settings, keyRepo: keyRepo}
}

// ResolveRequirement 读取目录是否要求密钥以及允许的密钥 ID 列表。
// 全局 Enabled=false 时一律返回 Required=false。
func (s *GuestAccessService) ResolveRequirement(ctx context.Context, folderID string) (GuestAccessRequirement, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return GuestAccessRequirement{}, nil
	}
	policy, err := s.settings.GetPolicy(ctx)
	if err != nil {
		return GuestAccessRequirement{}, err
	}
	if !policy.GuestAccess.Enabled {
		return GuestAccessRequirement{}, nil
	}
	// 仅当该目录本身或某个上级目录设置 guest_key_required=true 时要求密钥。
	// 直接读单目录字段：上层开关由 ListPublicFolders 过滤保证根列表本身已过滤，
	// 此处用于直链命中（folder detail / file detail）时的精确判定。
	required, err := s.folderOrAncestorRequiresKey(ctx, folderID)
	if err != nil {
		return GuestAccessRequirement{}, err
	}
	if !required {
		return GuestAccessRequirement{}, nil
	}
	allowed, err := s.keyRepo.ListAllowedKeyIDsByFolderID(ctx, folderID)
	if err != nil {
		return GuestAccessRequirement{}, err
	}
	return GuestAccessRequirement{Required: true, AllowedKeyIDs: allowed}, nil
}

// ValidateForFolder 给定 (folderID, candidateKey)，校验 candidateKey 是否能解锁该目录。
// 命中规则：candidateKey 命中全局密钥池中的某个 key，且该 key ID 在该目录允许列表中。
// 返回 (allowed, hint)：allowed=true 则通过；hint 在密钥无效或全局禁用时返回任一配置提示。
func (s *GuestAccessService) ValidateForFolder(ctx context.Context, folderID, candidateKey string) (allowed bool, hint string) {
	requirement, err := s.ResolveRequirement(ctx, folderID)
	if err != nil || !requirement.Required {
		return true, "" // 未启用或目录不要求时一律放行
	}
	if strings.TrimSpace(candidateKey) == "" {
		return false, firstHint(s.settings, ctx)
	}
	matchedKeyID, genericHint, matched := s.settings.FindGuestAccessKeyByValue(ctx, candidateKey)
	if !matched {
		return false, genericHint
	}
	for _, id := range requirement.AllowedKeyIDs {
		if id == matchedKeyID {
			return true, ""
		}
	}
	return false, genericHint
}

// ValidateValue 公开 validate 端点：返回该密钥值匹配的全站密钥 ID，以及它能解锁的全部目录 ID。
// 返回 (keyID, unlockedFolderIDs, hint, ok)。ok=false 时 hint 为全局提示（非具体泄露）。
func (s *GuestAccessService) ValidateValue(ctx context.Context, value string) (string, []string, string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil, firstHint(s.settings, ctx), false
	}
	keyID, genericHint, ok := s.settings.FindGuestAccessKeyByValue(ctx, trimmed)
	if !ok {
		return "", nil, genericHint, false
	}
	folderIDs, err := s.keyRepo.ListFolderIDsByKeyID(ctx, keyID)
	if err != nil {
		return "", nil, genericHint, false
	}
	return keyID, folderIDs, "", true
}

// folderOrAncestorRequiresKey 检查目录自身或任意祖先是否设置了 guest_key_required=true。
func (s *GuestAccessService) folderOrAncestorRequiresKey(ctx context.Context, folderID string) (bool, error) {
	const recursionCTE = `
		WITH RECURSIVE ancestors(id, parent_id, required) AS (
			SELECT id, parent_id, guest_key_required FROM folders WHERE id = ?
			UNION ALL
			SELECT f.id, f.parent_id, f.guest_key_required FROM folders f
			INNER JOIN ancestors a ON f.id = a.parent_id
		)
	`
	var anyRequired bool
	err := s.keyRepo.RawAncestorHasGuestKey(ctx, folderID, recursionCTE, &anyRequired)
	if err != nil {
		return false, err
	}
	return anyRequired, nil
}

// firstHint 取出所有 key 中第一条 hint（用于全局禁用 / 错误输入时的通用提示）。
func firstHint(s *SystemSettingService, ctx context.Context) string {
	policy, err := s.GetPolicy(ctx)
	if err != nil || policy == nil {
		return ""
	}
	for _, k := range policy.GuestAccess.Keys {
		if k.Hint != "" {
			return k.Hint
		}
	}
	return ""
}

// ErrGuestAccessDisabled 全局功能未启用时返回（语义化错误）。
var ErrGuestAccessDisabled = errors.New("guest access is not enabled")
