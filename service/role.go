package service

import (
	"errors"

	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
)

// GetAllRoles 获取角色列表（分页）
func GetAllRoles(page, pageSize int) ([]dto.RoleInfo, int64, error) {
	var roles []model.Role
	var total int64

	global.DB.Model(&model.Role{}).Count(&total)
	if err := global.DB.Order("sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&roles).Error; err != nil {
		return nil, 0, errors.New("查询角色列表失败")
	}

	list := make([]dto.RoleInfo, len(roles))
	for i, r := range roles {
		list[i] = dto.RoleInfo{
			ID:          r.ID,
			Name:        r.Name,
			Code:        r.Code,
			Description: r.Description,
			Sort:        r.Sort,
			Status:      r.Status,
			DataScope:   r.DataScope,
		}
	}
	return list, total, nil
}

// CreateRole 创建角色
func CreateRole(req dto.CreateRoleReq) (*dto.RoleInfo, error) {
	dataScope, err := normalizeDataScope(req.DataScope)
	if err != nil {
		return nil, err
	}
	if dataScope == model.DataScopeCustom {
		return nil, errors.New("自定义数据范围请创建角色后通过数据权限接口配置")
	}

	var exist int64
	global.DB.Model(&model.Role{}).Where("name = ? OR code = ?", req.Name, req.Code).Count(&exist)
	if exist > 0 {
		return nil, errors.New("角色名称或编码已存在")
	}

	role := model.Role{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Sort:        req.Sort,
		Status:      1,
		DataScope:   dataScope,
	}
	if req.Status == 0 || req.Status == 1 {
		role.Status = req.Status
	}
	if err := global.DB.Create(&role).Error; err != nil {
		return nil, errors.New("创建角色失败: " + err.Error())
	}

	return &dto.RoleInfo{
		ID: role.ID, Name: role.Name, Code: role.Code,
		Description: role.Description, Sort: role.Sort, Status: role.Status, DataScope: role.DataScope,
	}, nil
}

// UpdateRole 修改角色
func UpdateRole(roleID uint, req dto.UpdateRoleReq) (*dto.RoleInfo, error) {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("角色不存在")
		}
		return nil, errors.New("查询角色失败")
	}
	if role.Code == "admin" {
		return nil, errors.New("不能修改超级管理员角色")
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Code != "" {
		updates["code"] = req.Code
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.DataScope != "" {
		dataScope, err := normalizeDataScope(req.DataScope)
		if err != nil {
			return nil, err
		}
		if dataScope == model.DataScopeCustom {
			return nil, errors.New("自定义数据范围请通过数据权限接口配置")
		}
		updates["data_scope"] = dataScope
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}

	// 检查名称/编码是否冲突
	var exist int64
	if req.Name != "" {
		global.DB.Model(&model.Role{}).Where("id != ? AND name = ?", roleID, req.Name).Count(&exist)
		if exist > 0 {
			return nil, errors.New("角色名称已被占用")
		}
	}
	if req.Code != "" {
		exist = 0 // 重置
		global.DB.Model(&model.Role{}).Where("id != ? AND code = ?", roleID, req.Code).Count(&exist)
		if exist > 0 {
			return nil, errors.New("角色编码已被占用")
		}
	}

	if err := global.DB.Model(&role).Updates(updates).Error; err != nil {
		return nil, errors.New("修改角色失败")
	}
	bumpUsersTokenVersionByRole(roleID)

	global.DB.First(&role, roleID)
	return &dto.RoleInfo{
		ID: role.ID, Name: role.Name, Code: role.Code,
		Description: role.Description, Sort: role.Sort, Status: role.Status, DataScope: role.DataScope,
	}, nil
}

// DeleteRole 删除角色
func DeleteRole(roleID uint) error {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("角色不存在")
		}
		return errors.New("查询角色失败")
	}
	if role.Code == "admin" {
		return errors.New("不能删除超级管理员角色")
	}

	var oldUserIDs []uint
	global.DB.Model(&model.UserRole{}).Where("role_id = ?", roleID).Pluck("user_id", &oldUserIDs)
	// 删除关联
	global.DB.Where("role_id = ?", roleID).Delete(&model.UserRole{})
	global.DB.Where("role_id = ?", roleID).Delete(&model.RoleDataScope{})
	// 硬删除角色
	if err := global.DB.Unscoped().Delete(&role).Error; err != nil {
		return err
	}
	bumpUserTokenVersion(oldUserIDs...)
	return nil
}

// AssignUsersToRole 将用户分配到角色
func AssignUsersToRole(roleID uint, userIDs []uint) error {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return errors.New("角色不存在")
	}

	var oldUserIDs []uint
	global.DB.Model(&model.UserRole{}).Where("role_id = ?", roleID).Pluck("user_id", &oldUserIDs)

	// 先清除该角色所有旧关联
	global.DB.Where("role_id = ?", roleID).Delete(&model.UserRole{})

	// 批量插入新关联
	var records []model.UserRole
	for _, uid := range userIDs {
		records = append(records, model.UserRole{UserID: uid, RoleID: roleID})
	}
	if len(records) > 0 {
		if err := global.DB.Create(&records).Error; err != nil {
			return errors.New("分配用户失败: " + err.Error())
		}
	}
	affectedUserIDs := append(oldUserIDs, userIDs...)
	bumpUserTokenVersion(affectedUserIDs...)
	return nil
}

// GetRoleUsers 获取角色下的所有用户
func GetRoleUsers(roleID uint) ([]dto.UserInfo, error) {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return nil, errors.New("角色不存在")
	}

	var userRoles []model.UserRole
	global.DB.Where("role_id = ?", roleID).Find(&userRoles)

	userIDs := make([]uint, len(userRoles))
	for i, ur := range userRoles {
		userIDs[i] = ur.UserID
	}
	if len(userIDs) == 0 {
		return []dto.UserInfo{}, nil
	}

	var users []model.User
	global.DB.Where("id IN ?", userIDs).Find(&users)

	list := make([]dto.UserInfo, len(users))
	for i, u := range users {
		list[i] = dto.UserInfo{
			ID: u.ID, Username: u.Username, Nickname: u.Nickname,
			Avatar: u.Avatar, Email: u.Email, Role: u.Role, Status: u.Status,
		}
	}
	return list, nil
}

// AssignRoleDataScope 配置角色数据范围。
func AssignRoleDataScope(roleID uint, req dto.AssignRoleDataScopeReq) error {
	dataScope, err := normalizeDataScope(req.DataScope)
	if err != nil {
		return err
	}

	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return errors.New("角色不存在")
	}
	if role.Code == "admin" {
		return errors.New("超级管理员角色固定拥有全部数据权限")
	}

	organizationIDs := uniqueUintIDs(req.OrganizationIDs)
	if dataScope == model.DataScopeCustom {
		if len(organizationIDs) == 0 {
			return errors.New("自定义数据范围必须选择组织")
		}
		if err := ensureOrganizationsExist(organizationIDs); err != nil {
			return err
		}
	}

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&role).Update("data_scope", dataScope).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RoleDataScope{}).Error; err != nil {
			return err
		}
		if dataScope != model.DataScopeCustom {
			return nil
		}

		records := make([]model.RoleDataScope, 0, len(organizationIDs))
		for _, orgID := range organizationIDs {
			records = append(records, model.RoleDataScope{
				RoleID:         roleID,
				OrganizationID: orgID,
			})
		}
		return tx.Create(&records).Error
	}); err != nil {
		return errors.New("配置角色数据权限失败: " + err.Error())
	}

	bumpUsersTokenVersionByRole(roleID)
	return nil
}

// GetRoleDataScope 查询角色数据范围。
func GetRoleDataScope(roleID uint) (*dto.RoleDataScopeInfo, error) {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return nil, errors.New("角色不存在")
	}

	var organizationIDs []uint
	if err := global.DB.Model(&model.RoleDataScope{}).
		Where("role_id = ?", roleID).
		Order("organization_id asc").
		Pluck("organization_id", &organizationIDs).Error; err != nil {
		return nil, errors.New("查询角色数据权限失败")
	}

	dataScope := role.DataScope
	if dataScope == "" {
		dataScope = model.DataScopeAll
	}
	return &dto.RoleDataScopeInfo{
		RoleID:          role.ID,
		DataScope:       dataScope,
		OrganizationIDs: organizationIDs,
	}, nil
}
