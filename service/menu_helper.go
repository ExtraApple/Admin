package service

import (
	"admin/dto"
	"admin/model"
)

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

func toMenuDetail(menu model.Menu) *dto.MenuDetail {
	return &dto.MenuDetail{
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
