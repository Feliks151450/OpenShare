package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

// GuestAccessHandler 处理访客密钥访问相关的端点：
//   - POST /api/public/guest-access/validate（公开）：访客提交密钥值校验。
//   - GET  /api/admin/system/guest-access（超管）：读取全局密钥池。
//   - PUT  /api/admin/system/guest-access（超管）：替换全局密钥池。
type GuestAccessHandler struct {
	systemSetting *service.SystemSettingService
	guestAccess   *service.GuestAccessService
}

func NewGuestAccessHandler(systemSetting *service.SystemSettingService, guestAccess *service.GuestAccessService) *GuestAccessHandler {
	return &GuestAccessHandler{systemSetting: systemSetting, guestAccess: guestAccess}
}

type validateGuestAccessRequest struct {
	Value string `json:"value"`
}

// Validate 公开端点：访客输入密钥值，校验成功后返回能解锁的目录 ID 列表。
func (h *GuestAccessHandler) Validate(ctx *gin.Context) {
	var req validateGuestAccessRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	keyID, folderIDs, hint, ok := h.guestAccess.ValidateValue(ctx.Request.Context(), req.Value)
	if !ok {
		ctx.JSON(http.StatusOK, gin.H{"valid": false, "hint": hint})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"valid":               true,
		"key_id":              keyID,
		"unlocked_folder_ids": folderIDs,
	})
}

// AdminGet 全局密钥池读取（超管）。
func (h *GuestAccessHandler) AdminGet(ctx *gin.Context) {
	policy, err := h.systemSetting.GetPolicy(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load guest access policy"})
		return
	}
	ctx.JSON(http.StatusOK, policy.GuestAccess)
}

// AdminPut 全局密钥池替换（超管）。value 字段明文校验。
func (h *GuestAccessHandler) AdminPut(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req service.GuestAccessPolicy
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// 空 value 视为非法；service 层会兜底校验。
	for _, k := range req.Keys {
		if strings.TrimSpace(k.ID) == "" || strings.TrimSpace(k.Name) == "" || strings.TrimSpace(k.Value) == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "key id, name and value are required"})
			return
		}
	}

	updated, err := h.systemSetting.SaveGuestAccess(ctx.Request.Context(), req, identity.AdminID, ctx.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrInvalidGuestAccessInput) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid guest access input"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save guest access policy"})
		return
	}
	ctx.JSON(http.StatusOK, updated)
}
