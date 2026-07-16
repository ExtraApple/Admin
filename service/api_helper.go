package service

import (
	"errors"
	"net/http"
	"strings"

	"admin/dto"
	"admin/global"
	"admin/model"
)

var supportedAPIMethodList = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
	http.MethodHead,
}

var supportedAPIMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
	http.MethodHead:    {},
}

// normalizeAPIMethod 规范化并校验 HTTP 方法。
func normalizeAPIMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "", errors.New("请求方法不能为空")
	}
	if _, ok := supportedAPIMethods[method]; !ok {
		return "", errors.New("请求方法不支持")
	}
	return method, nil
}

// normalizeAPIPath 规范化 API 路径，确保以斜杠开头并去除尾部斜杠。
func normalizeAPIPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("请求路径不能为空")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path, nil
}

// defaultAPIStatus 返回 API 状态默认值。
func defaultAPIStatus(status *int) int {
	if status == nil {
		return 1
	}
	return *status
}

// defaultAPISwitch 返回 API 开关类字段的默认值。
func defaultAPISwitch(value *int) int {
	if value == nil {
		return 1
	}
	return *value
}

// ensureAPIAvailable 校验同方法同路径的 API 是否可用。
func ensureAPIAvailable(apiID uint, method, path string) error {
	var count int64
	query := global.DB.Model(&model.API{}).Where("method = ? AND path = ?", method, path)
	if apiID > 0 {
		query = query.Where("id != ?", apiID)
	}
	query.Count(&count)
	if count > 0 {
		return errors.New("API已存在")
	}
	return nil
}

// toAPIInfoList 将 API 模型列表转换为响应 DTO 列表。
func toAPIInfoList(apis []model.API) []dto.APIInfo {
	list := make([]dto.APIInfo, len(apis))
	for i, item := range apis {
		list[i] = *toAPIInfo(item)
	}
	return list
}

// toAPIInfo 将 API 模型转换为响应 DTO。
func toAPIInfo(item model.API) *dto.APIInfo {
	return &dto.APIInfo{
		ID:             item.ID,
		Name:           item.Name,
		Method:         item.Method,
		Path:           item.Path,
		Group:          item.Group,
		PermissionCode: item.PermissionCode,
		Remark:         item.Remark,
		Sort:           item.Sort,
		Status:         item.Status,
		NeedAuth:       item.NeedAuth,
		NeedAudit:      item.NeedAudit,
	}
}

// shouldSyncAPIRoute 判断路由是否需要同步到 API 元数据表。
func shouldSyncAPIRoute(path string) bool {
	if path == "/ping" {
		return false
	}
	// 检验字符串是否为指定的/api/开头
	return strings.HasPrefix(path, "/api/")
}

// inferAPIGroup 根据路由路径推断 API 分组。
func inferAPIGroup(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "api"
	}

	segment := parts[0]
	if segment == "admin" && len(parts) > 1 {
		segment = parts[1]
	}

	switch segment {
	case "login", "register", "captcha":
		return "auth"
	case "user", "users":
		return "user"
	case "roles":
		return "role"
	case "permissions", "permission-groups", "permission-codes":
		return "permission"
	case "menus":
		return "menu"
	case "files", "files-browse":
		return "file"
	case "audit-logs", "login-logs", "operation-logs", "permission-logs", "data-access-logs":
		return "audit"
	case "dicts", "dict-types", "dict-items":
		return "dict"
	case "organizations":
		return "organization"
	case "apis", "api-groups", "api-methods":
		return "api"
	default:
		return segment
	}
}

// inferAPINeedAuth 根据路由路径推断 API 是否需要登录鉴权。
func inferAPINeedAuth(path string) int {
	switch path {
	case "/api/login", "/api/register", "/api/captcha":
		return 0
	default:
		if strings.HasPrefix(path, "/api/dicts/") ||
			strings.HasPrefix(path, "/api/avatars/") {
			return 0
		}
		return 1
	}
}

// generateAPIPermissionCode 根据 HTTP 方法和路径生成权限码。
func generateAPIPermissionCode(method, path string) string {
	code := strings.TrimPrefix(path, "/api/")
	code = strings.ReplaceAll(code, ":", "")
	code = strings.ReplaceAll(code, "/", ".")
	code = strings.Trim(code, ".")
	return strings.ToLower(code + "." + method)
}
