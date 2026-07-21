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
	path := ""
	if menu.Path != nil {
		path = *menu.Path
	}

	return &dto.MenuDetail{
		ID:             menu.ID,
		ParentID:       menu.ParentID,
		Name:           menu.Name,
		Path:           path,
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

// defaultMenuType 在未指定菜单类型时返回目录类型。
func defaultMenuType(t int) int {
	if t == 0 {
		return 1
	}
	return t
}
