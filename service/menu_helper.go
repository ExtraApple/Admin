package service

import (
	"admin/model"
	"admin/request"
)

func buildMenuTree(menus []model.Menu, parentID uint) []request.MenuDetail {
	tree := []request.MenuDetail{}
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

func toMenuDetail(menu model.Menu) *request.MenuDetail {
	return &request.MenuDetail{
		ID:        menu.ID,
		ParentID:  menu.ParentID,
		Name:      menu.Name,
		Path:      menu.Path,
		Component: menu.Component,
		Icon:      menu.Icon,
		Sort:      menu.Sort,
		Type:      menu.Type,
		Status:    menu.Status,
	}
}

func defaultMenuType(t int) int {
	if t == 0 {
		return 1
	}
	return t
}

func defaultMenuStatus(status int) int {
	if status == 0 {
		return 1
	}
	return status
}
