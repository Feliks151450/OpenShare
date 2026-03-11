package model

import (
	"slices"
	"strings"
)

const (
	AdminRoleSuperAdmin = "super_admin"
	AdminRoleAdmin      = "admin"
)

type AdminPermission string

const (
	AdminPermissionReviewSubmissions   AdminPermission = "review_submissions"
	AdminPermissionManageAnnouncements AdminPermission = "manage_announcements"
	AdminPermissionEditResources       AdminPermission = "edit_resources"
	AdminPermissionDeleteResources     AdminPermission = "delete_resources"
	AdminPermissionManageTags          AdminPermission = "manage_tags"
	AdminPermissionManageAdmins        AdminPermission = "manage_admins"
	AdminPermissionManageSystem        AdminPermission = "manage_system"
)

var validAdminPermissions = map[AdminPermission]struct{}{
	AdminPermissionReviewSubmissions:   {},
	AdminPermissionManageAnnouncements: {},
	AdminPermissionEditResources:       {},
	AdminPermissionDeleteResources:     {},
	AdminPermissionManageTags:          {},
	AdminPermissionManageAdmins:        {},
	AdminPermissionManageSystem:        {},
}

func (a Admin) PermissionList() []AdminPermission {
	return ParseAdminPermissions(a.Permissions)
}

func (a Admin) HasPermission(permission AdminPermission) bool {
	if a.Role == AdminRoleSuperAdmin {
		return true
	}

	return slices.Contains(a.PermissionList(), permission)
}

func NormalizeAdminPermissions(permissions []AdminPermission) string {
	seen := make(map[AdminPermission]struct{}, len(permissions))
	normalized := make([]string, 0, len(permissions))

	for _, permission := range permissions {
		permission = AdminPermission(strings.TrimSpace(string(permission)))
		if _, ok := validAdminPermissions[permission]; !ok {
			continue
		}
		if _, exists := seen[permission]; exists {
			continue
		}

		seen[permission] = struct{}{}
		normalized = append(normalized, string(permission))
	}

	slices.Sort(normalized)
	return strings.Join(normalized, ",")
}

func ParseAdminPermissions(raw string) []AdminPermission {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	permissions := make([]AdminPermission, 0, len(parts))
	seen := make(map[AdminPermission]struct{}, len(parts))

	for _, part := range parts {
		permission := AdminPermission(strings.TrimSpace(part))
		if _, ok := validAdminPermissions[permission]; !ok {
			continue
		}
		if _, exists := seen[permission]; exists {
			continue
		}

		seen[permission] = struct{}{}
		permissions = append(permissions, permission)
	}

	slices.Sort(permissions)
	return permissions
}
