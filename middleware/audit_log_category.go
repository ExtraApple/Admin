package middleware

import (
	"net/http"
	"strings"

	"admin/service"
)

func auditCategory(method, path string) string {
	normalizedPath := normalizeAuditPath(path)

	if normalizedPath == "/api/login" {
		return service.AuditCategoryLogin
	}
	if isAuditReadMethod(method) {
		return service.AuditCategoryDataAccess
	}
	if isAuditPermissionMutation(method, normalizedPath) {
		return service.AuditCategoryPermission
	}
	if isAuditWriteMethod(method) {
		return service.AuditCategoryOperation
	}
	return service.AuditCategoryAPI
}

func normalizeAuditPath(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func isAuditReadMethod(method string) bool {
	return method == http.MethodGet
}

func isAuditWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isAuditPermissionMutation(method, path string) bool {
	if !isAuditWriteMethod(method) {
		return false
	}

	if strings.HasPrefix(path, "/api/admin/permissions") ||
		strings.HasPrefix(path, "/api/admin/permission-groups") ||
		strings.HasPrefix(path, "/api/admin/menus") {
		return true
	}

	return strings.HasPrefix(path, "/api/admin/roles/") &&
		(strings.HasSuffix(path, "/permissions") ||
			strings.HasSuffix(path, "/menus") ||
			strings.HasSuffix(path, "/users"))
}
