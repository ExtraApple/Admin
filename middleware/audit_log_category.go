package middleware

import (
	"net/http"
	"strings"

	"admin/service"
)

// auditCategory 根据请求方法和路径把审计日志归类，供管理端分类型查询。
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

// normalizeAuditPath 统一路径大小写和尾部斜杠，降低分类规则的匹配误差。
func normalizeAuditPath(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}

// isAuditReadMethod 判断请求方法是否属于读取类数据访问。
func isAuditReadMethod(method string) bool {
	return method == http.MethodGet
}

// isAuditWriteMethod 判断请求方法是否可能产生业务状态变更。
func isAuditWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isAuditPermissionMutation 判断请求是否属于角色、权限或菜单相关的变更操作。
func isAuditPermissionMutation(method, path string) bool {
	if !isAuditWriteMethod(method) {
		return false
	}

	if strings.HasPrefix(path, "/api/admin/permissions") ||
		strings.HasPrefix(path, "/api/admin/permission-groups") ||
		strings.HasPrefix(path, "/api/admin/menus") ||
		strings.HasPrefix(path, "/api/admin/apis") {
		return true
	}

	return strings.HasPrefix(path, "/api/admin/roles/") &&
		(strings.HasSuffix(path, "/permissions") ||
			strings.HasSuffix(path, "/menus") ||
			strings.HasSuffix(path, "/users"))
}
