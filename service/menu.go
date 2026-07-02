package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
)

func GetMenuTree() ([]dto.MenuDetail, error) {
	var menus []model.Menu
	if err := global.DB.Order("sort asc, id asc").Find(&menus).Error; err != nil {
		return nil, errors.New("查询菜单失败")
	}
	return buildMenuTree(menus, 0), nil
}

func CreateMenu(req dto.CreateMenuReq) (*dto.MenuDetail, error) {
	path := normalizeMenuPath(req.Path)
	if path != nil {
		var exist int64
		global.DB.Model(&model.Menu{}).Where("path = ?", *path).Count(&exist)
		if exist > 0 {
			return nil, errors.New("菜单路径已存在")
		}
	}

	menu := model.Menu{
		ParentID:       req.ParentID,
		Name:           req.Name,
		Path:           path,
		Component:      req.Component,
		Icon:           req.Icon,
		PermissionCode: strings.TrimSpace(req.PermissionCode),
		Sort:           req.Sort,
		Type:           defaultMenuType(req.Type),
		Status:         defaultMenuStatus(req.Status),
	}
	if err := global.DB.Create(&menu).Error; err != nil {
		return nil, errors.New("创建菜单失败: " + err.Error())
	}
	bumpAllUsersTokenVersion()
	return toMenuDetail(menu), nil
}

func UpdateMenu(menuID uint, req dto.UpdateMenuReq) (*dto.MenuDetail, error) {
	var menu model.Menu
	if err := global.DB.First(&menu, menuID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("菜单不存在")
		}
		return nil, errors.New("查询菜单失败")
	}

	updates := map[string]any{}
	if req.ParentID != nil {
		if *req.ParentID == menuID {
			return nil, errors.New("父级菜单不能是自己")
		}
		updates["parent_id"] = *req.ParentID
	}
	if req.Name != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	if req.Path != "" {
		path := normalizeMenuPath(req.Path)
		var exist int64
		global.DB.Model(&model.Menu{}).Where("id != ? AND path = ?", menuID, *path).Count(&exist)
		if exist > 0 {
			return nil, errors.New("菜单路径已存在")
		}
		updates["path"] = path
	}
	if req.Component != "" {
		updates["component"] = req.Component
	}
	if req.Icon != "" {
		updates["icon"] = req.Icon
	}
	if req.PermissionCode != nil {
		updates["permission_code"] = strings.TrimSpace(*req.PermissionCode)
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}

	if err := global.DB.Model(&menu).Updates(updates).Error; err != nil {
		return nil, errors.New("修改菜单失败")
	}
	bumpAllUsersTokenVersion()
	global.DB.First(&menu, menuID)
	return toMenuDetail(menu), nil
}

func DeleteMenu(menuID uint) error {
	var menu model.Menu
	if err := global.DB.First(&menu, menuID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("菜单不存在")
		}
		return errors.New("查询菜单失败")
	}

	var childCount int64
	global.DB.Model(&model.Menu{}).Where("parent_id = ?", menuID).Count(&childCount)
	if childCount > 0 {
		return errors.New("存在子菜单，不能直接删除")
	}

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("menu_id = ?", menuID).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		if err := tx.Where("menu_id = ?", menuID).Delete(&model.MenuAPI{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&menu).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	bumpAllUsersTokenVersion()
	return nil
}

func AssignMenusToRole(roleID uint, menuIDs []uint) error {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return errors.New("角色不存在")
	}

	global.DB.Where("role_id = ?", roleID).Delete(&model.RoleMenu{})

	records := make([]model.RoleMenu, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		records = append(records, model.RoleMenu{RoleID: roleID, MenuID: menuID})
	}
	if len(records) == 0 {
		bumpUsersTokenVersionByRole(roleID)
		return nil
	}
	if err := global.DB.Create(&records).Error; err != nil {
		return errors.New("分配菜单失败: " + err.Error())
	}
	bumpUsersTokenVersionByRole(roleID)
	return nil
}

func GetRoleMenus(roleID uint) ([]dto.MenuDetail, error) {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return nil, errors.New("角色不存在")
	}

	var rms []model.RoleMenu
	global.DB.Where("role_id = ?", roleID).Find(&rms)
	if len(rms) == 0 {
		return []dto.MenuDetail{}, nil
	}

	menuIDs := make([]uint, len(rms))
	for i, rm := range rms {
		menuIDs[i] = rm.MenuID
	}

	var menus []model.Menu
	global.DB.Where("id IN ? AND status = 1", menuIDs).Order("sort asc, id asc").Find(&menus)
	return buildMenuTree(menus, 0), nil
}

func GetUserMenus(userID uint) ([]dto.MenuDetail, error) {
	var userRoles []model.UserRole
	global.DB.Where("user_id = ?", userID).Find(&userRoles)
	if len(userRoles) == 0 {
		return []dto.MenuDetail{}, nil
	}

	roleIDs := make([]uint, len(userRoles))
	for i, ur := range userRoles {
		roleIDs[i] = ur.RoleID
	}

	var roles []model.Role
	global.DB.Where("id IN ? AND status = 1", roleIDs).Find(&roles)
	if hasRoleCode(roles, "admin") {
		var menus []model.Menu
		global.DB.Where("status = 1").Order("sort asc, id asc").Find(&menus)
		return buildMenuTree(menus, 0), nil
	}

	var roleMenus []model.RoleMenu
	global.DB.Where("role_id IN ?", roleIDs).Find(&roleMenus)
	if len(roleMenus) == 0 {
		return []dto.MenuDetail{}, nil
	}

	seen := map[uint]struct{}{}
	menuIDs := make([]uint, 0, len(roleMenus))
	for _, rm := range roleMenus {
		if _, ok := seen[rm.MenuID]; ok {
			continue
		}
		seen[rm.MenuID] = struct{}{}
		menuIDs = append(menuIDs, rm.MenuID)
	}

	var menus []model.Menu
	global.DB.Where("id IN ? AND status = 1", menuIDs).Order("sort asc, id asc").Find(&menus)
	return buildMenuTree(filterMenusByPermissions(menus, GetUserPermissions(userID)), 0), nil
}

func SyncMenus(routes []dto.SyncMenuItem) (int, error) {
	created := 0
	for _, route := range routes {
		path := normalizeMenuPath(route.Path)
		if path == nil {
			continue
		}

		var exist int64
		global.DB.Model(&model.Menu{}).Where("path = ?", *path).Count(&exist)
		if exist > 0 {
			continue
		}

		parentID := uint(0)
		if route.ParentPath != "" {
			var parent model.Menu
			if err := global.DB.Where("path = ?", route.ParentPath).First(&parent).Error; err == nil {
				parentID = parent.ID
			}
		}

		menu := model.Menu{
			ParentID:       parentID,
			Name:           route.Name,
			Path:           path,
			Component:      route.Component,
			Icon:           route.Icon,
			PermissionCode: strings.TrimSpace(route.PermissionCode),
			Sort:           route.Sort,
			Type:           defaultMenuType(route.Type),
			Status:         1,
		}
		if err := global.DB.Create(&menu).Error; err != nil {
			return created, errors.New("同步菜单失败: " + err.Error())
		}
		created++
	}
	if created > 0 {
		bumpAllUsersTokenVersion()
	}
	return created, nil
}
