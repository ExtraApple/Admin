package service

import (
	"admin/dto"
	"admin/model"
	"strings"
)

// buildMenuTree 将扁平菜单列表按 parent_id 递归组装为树。
func buildMenuTree(menus []model.Menu, parentID uint) []dto.MenuDetail {
	tree := []dto.MenuDetail{}
	for _, menu := range menus {
		if menu.ParentID != parentID {
			continue
		}

		node := *toMenuDetail(menu)
		node.Children = buildMenuTree(menus, menu.ID)
		tree = append(tree, node)
	}
	return tree
}

// toMenuDetail 将菜单模型转换为接口返回结构。
func toMenuDetail(menu model.Menu) *dto.MenuDetail {
	return &dto.MenuDetail{
		ID:             menu.ID,
		ParentID:       menu.ParentID,
		Name:           menu.Name,
		Path:           menuPathValue(menu.Path),
		Component:      menu.Component,
		Icon:           menu.Icon,
		PermissionCode: menu.PermissionCode,
		Sort:           menu.Sort,
		Type:           menu.Type,
		Status:         menu.Status,
	}
}

// normalizeMenuPath 清理菜单路径，空路径返回 nil。
func normalizeMenuPath(path string) *string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &path
}

// menuPathValue 将可空菜单路径转换为字符串。
func menuPathValue(path *string) string {
	if path == nil {
		return ""
	}
	return *path
}

// filterMenusByPermissions 根据权限码过滤菜单并保留可见节点的父级链路。
func filterMenusByPermissions(menus []model.Menu, permissions []string) []model.Menu {
	if len(menus) == 0 {
		return []model.Menu{}
	}
	if containsString(permissions, "*") || containsString(permissions, "admin") {
		return menus
	}

	permissionSet := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		permissionSet[permission] = struct{}{}
	}

	visible := make(map[uint]struct{}, len(menus))
	menuByID := make(map[uint]model.Menu, len(menus))
	for _, menu := range menus {
		menuByID[menu.ID] = menu
		if menu.PermissionCode == "" {
			visible[menu.ID] = struct{}{}
			continue
		}
		if _, ok := permissionSet[menu.PermissionCode]; ok {
			visible[menu.ID] = struct{}{}
		}
	}

	for menuID := range visible {
		parentID := menuByID[menuID].ParentID
		for parentID != 0 {
			parent, ok := menuByID[parentID]
			if !ok {
				break
			}
			visible[parent.ID] = struct{}{}
			parentID = parent.ParentID
		}
	}

	filtered := make([]model.Menu, 0, len(visible))
	for _, menu := range menus {
		if _, ok := visible[menu.ID]; ok {
			filtered = append(filtered, menu)
		}
	}
	return filtered
}

// containsString 判断字符串切片中是否包含目标值。
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// hasRoleCode 判断角色列表中是否包含指定角色编码。
func hasRoleCode(roles []model.Role, code string) bool {
	for _, role := range roles {
		if role.Code == code {
			return true
		}
	}
	return false
}

// defaultMenuType 在未指定菜单类型时返回目录类型。
func defaultMenuType(t int) int {
	if t == 0 {
		return 1
	}
	return t
}

// defaultMenuStatus 在未指定状态时默认启用菜单。
func defaultMenuStatus(status int) int {
	if status == 0 {
		return 1
	}
	return status
}
