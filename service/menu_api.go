package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
)

// AssignAPIsToMenu 为菜单绑定 API，并同步菜单、API 和权限码。
func AssignAPIsToMenu(menuID uint, req dto.AssignAPIsToMenuReq) error {
	apiIDs := uniqueUintIDs(req.APIIDs)
	if len(apiIDs) == 0 {
		return errors.New("请选择要绑定的API")
	}

	var menu model.Menu
	if err := global.DB.First(&menu, menuID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("菜单不存在")
		}
		return errors.New("查询菜单失败")
	}

	apis, err := getEnabledAuthAPIs(apiIDs)
	if err != nil {
		return err
	}

	permissionCode, err := resolveMenuAPIPermissionCode(req.PermissionCode, apis)
	if err != nil {
		return err
	}

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := setAPIsPermissionCode(tx, apis, permissionCode); err != nil {
			return err
		}
		if err := ensurePermissionByCode(tx, permissionCode, menu.Name, inferAPIGroupFromList(apis), menu.Sort); err != nil {
			return err
		}
		if err := tx.Model(&model.Menu{}).Where("id = ?", menuID).Update("permission_code", permissionCode).Error; err != nil {
			return errors.New("同步菜单权限码失败")
		}
		if err := tx.Where("menu_id = ?", menuID).Delete(&model.MenuAPI{}).Error; err != nil {
			return errors.New("清理菜单API关联失败")
		}

		records := make([]model.MenuAPI, 0, len(apiIDs))
		for _, apiID := range apiIDs {
			records = append(records, model.MenuAPI{MenuID: menuID, APIID: apiID})
		}
		if err := tx.Create(&records).Error; err != nil {
			return errors.New("绑定菜单API失败: " + err.Error())
		}
		return nil
	}); err != nil {
		return err
	}

	bumpAllUsersTokenVersion()
	return nil
}

// GetMenuAPIs 查询菜单已绑定的 API 列表。
func GetMenuAPIs(menuID uint) ([]dto.APIInfo, error) {
	var menu model.Menu
	if err := global.DB.First(&menu, menuID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("菜单不存在")
		}
		return nil, errors.New("查询菜单失败")
	}

	var apiIDs []uint
	if err := global.DB.Model(&model.MenuAPI{}).Where("menu_id = ?", menuID).Pluck("api_id", &apiIDs).Error; err != nil {
		return nil, errors.New("查询菜单API关联失败")
	}
	if len(apiIDs) == 0 {
		return []dto.APIInfo{}, nil
	}

	var apis []model.API
	if err := global.DB.Where("id IN ?", apiIDs).Order("api_group asc, sort asc, id asc").Find(&apis).Error; err != nil {
		return nil, errors.New("查询API列表失败")
	}
	return toAPIInfoList(apis), nil
}

// GenerateMenuButtonFromAPI 根据 API 生成按钮菜单并建立绑定关系。
func GenerateMenuButtonFromAPI(apiID uint, req dto.GenerateMenuButtonFromAPIReq) (*dto.MenuDetail, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("按钮名称不能为空")
	}

	var parent model.Menu
	if err := global.DB.First(&parent, req.ParentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("父级菜单不存在")
		}
		return nil, errors.New("查询父级菜单失败")
	}

	var api model.API
	if err := global.DB.First(&api, apiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("API不存在")
		}
		return nil, errors.New("查询API失败")
	}
	if err := validateLinkableAPI(api); err != nil {
		return nil, err
	}

	permissionCode := strings.TrimSpace(api.PermissionCode)
	if permissionCode == "" {
		permissionCode = generateAPIPermissionCode(api.Method, api.Path)
	}

	var menu model.Menu
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := setAPIsPermissionCode(tx, []model.API{api}, permissionCode); err != nil {
			return err
		}
		if err := ensurePermissionByCode(tx, permissionCode, name, api.Group, req.Sort); err != nil {
			return err
		}

		menu = model.Menu{
			ParentID:       req.ParentID,
			Name:           name,
			Path:           nil,
			Component:      "",
			Icon:           "",
			PermissionCode: permissionCode,
			Sort:           req.Sort,
			Type:           3,
			Status:         1,
		}
		if err := tx.Create(&menu).Error; err != nil {
			return errors.New("生成按钮菜单失败: " + err.Error())
		}
		if err := tx.Create(&model.MenuAPI{MenuID: menu.ID, APIID: api.ID}).Error; err != nil {
			return errors.New("绑定按钮菜单API失败: " + err.Error())
		}
		return nil
	}); err != nil {
		return nil, err
	}

	bumpAllUsersTokenVersion()
	return toMenuDetail(menu), nil
}

// syncLinkedMenusByAPI 在 API 权限码变化后同步所有关联菜单。
func syncLinkedMenusByAPI(api model.API) error {
	var menuIDs []uint
	if err := global.DB.Model(&model.MenuAPI{}).Where("api_id = ?", api.ID).Pluck("menu_id", &menuIDs).Error; err != nil {
		return errors.New("查询菜单API关联失败")
	}
	menuIDs = uniqueUintIDs(menuIDs)
	if len(menuIDs) == 0 {
		return nil
	}

	permissionCode := strings.TrimSpace(api.PermissionCode)
	if permissionCode == "" {
		if api.NeedAuth != 1 {
			return nil
		}
		permissionCode = generateAPIPermissionCode(api.Method, api.Path)
	}

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Menu{}).Where("id IN ?", menuIDs).Update("permission_code", permissionCode).Error; err != nil {
			return errors.New("同步菜单权限码失败")
		}

		var linkedAPIIDs []uint
		if err := tx.Model(&model.MenuAPI{}).Where("menu_id IN ?", menuIDs).Pluck("api_id", &linkedAPIIDs).Error; err != nil {
			return errors.New("查询关联API失败")
		}
		linkedAPIIDs = uniqueUintIDs(linkedAPIIDs)
		if len(linkedAPIIDs) > 0 {
			if err := tx.Model(&model.API{}).Where("id IN ?", linkedAPIIDs).Update("permission_code", permissionCode).Error; err != nil {
				return errors.New("同步API权限码失败")
			}
		}
		if err := ensurePermissionByCode(tx, permissionCode, api.Name, api.Group, api.Sort); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	bumpAllUsersTokenVersion()
	return nil
}

// getEnabledAuthAPIs 查询可绑定菜单权限的启用鉴权 API。
func getEnabledAuthAPIs(apiIDs []uint) ([]model.API, error) {
	var apis []model.API
	if err := global.DB.Where("id IN ?", apiIDs).Find(&apis).Error; err != nil {
		return nil, errors.New("查询API列表失败")
	}
	if len(apis) != len(apiIDs) {
		return nil, errors.New("存在不存在的API")
	}
	for _, api := range apis {
		if err := validateLinkableAPI(api); err != nil {
			return nil, err
		}
	}
	return apis, nil
}

// validateLinkableAPI 校验 API 是否允许绑定到菜单权限。
func validateLinkableAPI(api model.API) error {
	if api.Status != 1 {
		return errors.New("只能绑定启用状态的API")
	}
	if api.NeedAuth != 1 {
		return errors.New("公开API不需要绑定菜单权限")
	}
	return nil
}

// resolveMenuAPIPermissionCode 解析菜单绑定 API 时应使用的权限码。
func resolveMenuAPIPermissionCode(manual string, apis []model.API) (string, error) {
	manual = strings.TrimSpace(manual)
	if manual != "" {
		return manual, nil
	}

	permissionSet := map[string]struct{}{}
	permissionCode := ""
	for _, api := range apis {
		code := strings.TrimSpace(api.PermissionCode)
		if code == "" {
			code = generateAPIPermissionCode(api.Method, api.Path)
		}
		permissionSet[code] = struct{}{}
		permissionCode = code
	}
	if len(permissionSet) == 0 {
		return "", errors.New("未找到可用API权限码")
	}
	if len(permissionSet) > 1 {
		return "", errors.New("多个API权限码不一致，请手动指定permission_code")
	}
	return permissionCode, nil
}

// setAPIsPermissionCode 在事务内同步 API 的权限码。
func setAPIsPermissionCode(tx *gorm.DB, apis []model.API, permissionCode string) error {
	apiIDs := make([]uint, 0, len(apis))
	for _, api := range apis {
		if strings.TrimSpace(api.PermissionCode) == permissionCode {
			continue
		}
		apiIDs = append(apiIDs, api.ID)
	}
	apiIDs = uniqueUintIDs(apiIDs)
	if len(apiIDs) == 0 {
		return nil
	}
	if err := tx.Model(&model.API{}).Where("id IN ?", apiIDs).Update("permission_code", permissionCode).Error; err != nil {
		return errors.New("同步API权限码失败")
	}
	return nil
}

// ensurePermissionByCode 在事务内确保指定权限码已存在。
func ensurePermissionByCode(tx *gorm.DB, code, name, group string, sort int) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("权限码不能为空")
	}

	var count int64
	if err := tx.Model(&model.Permission{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return errors.New("查询权限码失败")
	}
	if count > 0 {
		return nil
	}

	permission := model.Permission{
		Name:  strings.TrimSpace(name),
		Code:  code,
		Group: strings.TrimSpace(group),
		Sort:  sort,
	}
	if permission.Name == "" {
		permission.Name = code
	}
	if permission.Group == "" {
		permission.Group = "api"
	}
	if err := tx.Create(&permission).Error; err != nil {
		return errors.New("创建权限码失败: " + err.Error())
	}
	return nil
}

// inferAPIGroupFromList 从 API 列表中推断权限分组。
func inferAPIGroupFromList(apis []model.API) string {
	for _, api := range apis {
		group := strings.TrimSpace(api.Group)
		if group != "" {
			return group
		}
	}
	return "api"
}
