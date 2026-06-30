package service

import (
	"errors"

	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
)

// GetAllPermissions 分页查询权限列表，并按排序和 ID 稳定排序。
func GetAllPermissions(page, pageSize int) ([]dto.PermissionInfo, int64, error) {
	var perms []model.Permission
	var total int64

	global.DB.Model(&model.Permission{}).Count(&total)
	if err := global.DB.Order("sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&perms).Error; err != nil {
		return nil, 0, errors.New("查询权限列表失败")
	}

	list := make([]dto.PermissionInfo, len(perms))
	for i, p := range perms {
		list[i] = dto.PermissionInfo{
			ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
		}
	}
	return list, total, nil
}

// CreatePermission 创建权限记录，并保证权限码唯一。
func CreatePermission(req dto.CreatePermissionReq) (*dto.PermissionInfo, error) {
	var exist int64
	global.DB.Model(&model.Permission{}).Where("code = ?", req.Code).Count(&exist)
	if exist > 0 {
		return nil, errors.New("权限码已存在")
	}

	p := model.Permission{
		Name: req.Name, Code: req.Code, Group: req.Group, Sort: req.Sort,
	}
	if err := global.DB.Create(&p).Error; err != nil {
		return nil, errors.New("创建权限失败: " + err.Error())
	}
	return &dto.PermissionInfo{
		ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
	}, nil
}

// UpdatePermission 修改权限展示信息，权限码本身不允许在此处变更。
func UpdatePermission(permID uint, req dto.UpdatePermissionReq) (*dto.PermissionInfo, error) {
	var p model.Permission
	if err := global.DB.First(&p, permID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("权限不存在")
		}
		return nil, errors.New("查询权限失败")
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Group != "" {
		updates["group"] = req.Group
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}

	if err := global.DB.Model(&p).Updates(updates).Error; err != nil {
		return nil, errors.New("修改权限失败")
	}

	global.DB.First(&p, permID)
	return &dto.PermissionInfo{
		ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
	}, nil
}

// DeletePermission 删除权限记录，并清理角色权限关联。
func DeletePermission(permID uint) error {
	var p model.Permission
	if err := global.DB.First(&p, permID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("权限不存在")
		}
		return errors.New("查询权限失败")
	}
	var roleIDs []uint
	global.DB.Model(&model.RolePermission{}).Where("permission_id = ?", permID).Pluck("role_id", &roleIDs)
	global.DB.Where("permission_id = ?", permID).Delete(&model.RolePermission{})
	if err := global.DB.Unscoped().Delete(&p).Error; err != nil {
		return err
	}
	bumpUsersTokenVersionByRoles(roleIDs)
	return nil
}

// AssignPermissionsToRole 为角色全量替换权限集合。
func AssignPermissionsToRole(roleID uint, permIDs []uint) error {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return errors.New("角色不存在")
	}
	global.DB.Where("role_id = ?", roleID).Delete(&model.RolePermission{})

	var records []model.RolePermission
	for _, pid := range permIDs {
		records = append(records, model.RolePermission{RoleID: roleID, PermissionID: pid})
	}
	if len(records) > 0 {
		if err := global.DB.Create(&records).Error; err != nil {
			return errors.New("分配权限失败: " + err.Error())
		}
	}
	bumpUsersTokenVersionByRole(roleID)
	return nil
}

// GetRolePermissions 查询角色已分配的权限列表。
func GetRolePermissions(roleID uint) ([]dto.PermissionInfo, error) {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return nil, errors.New("角色不存在")
	}

	var rps []model.RolePermission
	global.DB.Where("role_id = ?", roleID).Find(&rps)
	if len(rps) == 0 {
		return []dto.PermissionInfo{}, nil
	}

	permIDs := make([]uint, len(rps))
	for i, rp := range rps {
		permIDs[i] = rp.PermissionID
	}
	var perms []model.Permission
	global.DB.Where("id IN ?", permIDs).Order("sort asc").Find(&perms)

	list := make([]dto.PermissionInfo, len(perms))
	for i, p := range perms {
		list[i] = dto.PermissionInfo{
			ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
		}
	}
	return list, nil
}

// GetUserPermissions 汇总用户通过角色获得的所有权限码，用于写入 JWT。
func GetUserPermissions(userID uint) []string {
	var permIDs []uint
	global.DB.Model(&model.RolePermission{}).
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id").
		Where("user_roles.user_id = ?", userID).
		Pluck("role_permissions.permission_id", &permIDs)

	if len(permIDs) == 0 {
		return nil
	}
	var codes []string
	global.DB.Model(&model.Permission{}).
		Where("id IN ?", permIDs).
		Pluck("code", &codes)
	return codes
}

// GetPermissionCodes 返回全部权限码，供前端做按钮级权限控制。
func GetPermissionCodes() []string {
	var codes []string
	global.DB.Model(&model.Permission{}).
		Order("`group` asc, sort asc").
		Pluck("code", &codes)
	if codes == nil {
		return []string{}
	}
	return codes
}

// SyncPermissions 根据 Gin 路由表生成缺失的权限码。
func SyncPermissions(routes []map[string]string) ([]string, error) {
	var created []string

	for _, r := range routes {
		method := r["method"]
		path := r["path"]
		if path == "/ping" || path == "/api/captcha" || path == "/api/register" || path == "/api/login" {
			continue
		}
		if len(path) < 5 || path[:5] != "/api/" {
			continue
		}

		code := path[5:]
		if idx := contains(code, ":"); idx >= 0 {
			code = code[:idx] + code[idx+1:]
		}
		code = replaceAll(code, "/", ".")
		code = code + "." + toLower(method)

		var exist int64
		global.DB.Model(&model.Permission{}).Where("code = ?", code).Count(&exist)
		if exist > 0 {
			continue
		}

		group := ""
		if idx := contains(code, "."); idx >= 0 {
			group = code[:idx]
		}

		p := model.Permission{
			Name:  method + " " + path,
			Code:  code,
			Group: group,
			Sort:  0,
		}
		if err := global.DB.Create(&p).Error; err == nil {
			created = append(created, code)
		}
	}

	return created, nil
}

// GetAllPermGroups 分页查询权限分组列表。
func GetAllPermGroups(page, pageSize int) ([]dto.PermGroupInfo, int64, error) {
	var groups []model.PermissionGroup
	var total int64

	global.DB.Model(&model.PermissionGroup{}).Count(&total)
	if err := global.DB.Order("sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&groups).Error; err != nil {
		return nil, 0, errors.New("查询权限分组列表失败")
	}

	list := make([]dto.PermGroupInfo, len(groups))
	for i, g := range groups {
		list[i] = dto.PermGroupInfo{ID: g.ID, Name: g.Name, Sort: g.Sort}
	}
	return list, total, nil
}

// CreatePermGroup 创建权限分组，并校验名称唯一。
func CreatePermGroup(req dto.CreatePermGroupReq) (*dto.PermGroupInfo, error) {
	var exist int64
	global.DB.Model(&model.PermissionGroup{}).Where("name = ?", req.Name).Count(&exist)
	if exist > 0 {
		return nil, errors.New("分组名称已存在")
	}

	g := model.PermissionGroup{Name: req.Name, Sort: req.Sort}
	if err := global.DB.Create(&g).Error; err != nil {
		return nil, errors.New("创建权限分组失败: " + err.Error())
	}
	return &dto.PermGroupInfo{ID: g.ID, Name: g.Name, Sort: g.Sort}, nil
}

// UpdatePermGroup 修改权限分组名称或排序。
func UpdatePermGroup(groupID uint, req dto.UpdatePermGroupReq) (*dto.PermGroupInfo, error) {
	var g model.PermissionGroup
	if err := global.DB.First(&g, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("分组不存在")
		}
		return nil, errors.New("查询分组失败")
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}

	if err := global.DB.Model(&g).Updates(updates).Error; err != nil {
		return nil, errors.New("修改分组失败")
	}

	global.DB.First(&g, groupID)
	return &dto.PermGroupInfo{ID: g.ID, Name: g.Name, Sort: g.Sort}, nil
}

// DeletePermGroup 硬删除权限分组记录。
func DeletePermGroup(groupID uint) error {
	var g model.PermissionGroup
	if err := global.DB.First(&g, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("分组不存在")
		}
		return errors.New("查询分组失败")
	}
	return global.DB.Unscoped().Delete(&g).Error
}

// contains 返回子串第一次出现的位置，找不到时返回 -1。
func contains(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// replaceAll 使用简单字符串扫描替换所有匹配片段。
func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); {
		if i <= len(s)-len(old) && s[i:i+len(old)] == old {
			result += new
			i += len(old)
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}

// toLower 将 ASCII 大写字母转换为小写，用于生成权限码。
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			result[i] = s[i] + 32
		} else {
			result[i] = s[i]
		}
	}
	return string(result)
}
