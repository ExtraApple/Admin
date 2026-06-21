package service

import (
	"errors"

	"gorm.io/gorm"

	"admin/global"
	"admin/model"
	"admin/request"
)

func GetAllPermissions(page, pageSize int) ([]request.PermissionInfo, int64, error) {
	var perms []model.Permission
	var total int64

	global.DB.Model(&model.Permission{}).Count(&total)
	if err := global.DB.Order("sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&perms).Error; err != nil {
		return nil, 0, errors.New("查询权限列表失败")
	}

	list := make([]request.PermissionInfo, len(perms))
	for i, p := range perms {
		list[i] = request.PermissionInfo{
			ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
		}
	}
	return list, total, nil
}

func CreatePermission(req request.CreatePermissionReq) (*request.PermissionInfo, error) {
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
	return &request.PermissionInfo{
		ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
	}, nil
}

func UpdatePermission(permID uint, req request.UpdatePermissionReq) (*request.PermissionInfo, error) {
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
	return &request.PermissionInfo{
		ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
	}, nil
}

func DeletePermission(permID uint) error {
	var p model.Permission
	if err := global.DB.First(&p, permID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("权限不存在")
		}
		return errors.New("查询权限失败")
	}
	global.DB.Where("permission_id = ?", permID).Delete(&model.RolePermission{})
	return global.DB.Unscoped().Delete(&p).Error
}

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
	return nil
}

func GetRolePermissions(roleID uint) ([]request.PermissionInfo, error) {
	var role model.Role
	if err := global.DB.First(&role, roleID).Error; err != nil {
		return nil, errors.New("角色不存在")
	}

	var rps []model.RolePermission
	global.DB.Where("role_id = ?", roleID).Find(&rps)
	if len(rps) == 0 {
		return []request.PermissionInfo{}, nil
	}

	permIDs := make([]uint, len(rps))
	for i, rp := range rps {
		permIDs[i] = rp.PermissionID
	}
	var perms []model.Permission
	global.DB.Where("id IN ?", permIDs).Order("sort asc").Find(&perms)

	list := make([]request.PermissionInfo, len(perms))
	for i, p := range perms {
		list[i] = request.PermissionInfo{
			ID: p.ID, Name: p.Name, Code: p.Code, Group: p.Group, Sort: p.Sort,
		}
	}
	return list, nil
}

// GetUserPermissions 查询用户所有生效的权限码（用于注入 JWT）
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

// GetPermissionCodes 获取所有权限码列表（供前端使用）
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

// SyncPermissions 扫描路由并同步权限（根据 router.Routes() 自动创建）
func SyncPermissions(routes []map[string]string) ([]string, error) {
	var created []string

	for _, r := range routes {
		method := r["method"]
		path := r["path"]
		// 跳过公开路由和验证码
		if path == "/ping" || path == "/api/captcha" || path == "/api/register" || path == "/api/login" {
			continue
		}
		// 只同步 /api/ 开头的路由
		if len(path) < 5 || path[:5] != "/api/" {
			continue
		}

		// 生成权限码：将路径和方法转换为 code
		// 例：GET /api/admin/users → api.admin.users.get
		code := path[5:] // 去掉 /api/
		// 替换路径参数 :id 为 id
		if idx := contains(code, ":"); idx >= 0 {
			code = code[:idx] + code[idx+1:]
		}
		// 替换 / 为 .
		code = replaceAll(code, "/", ".")
		// 添加方法后缀
		code = code + "." + toLower(method)

		// 检查是否已存在
		var exist int64
		global.DB.Model(&model.Permission{}).Where("code = ?", code).Count(&exist)
		if exist > 0 {
			continue
		}

		// 自动分组：取第一层路径
		group := ""
		if idx := contains(code, "."); idx >= 0 {
			group = code[:idx]
		}

		// 创建权限
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

// ========== 权限分组 CRUD ==========

func GetAllPermGroups(page, pageSize int) ([]request.PermGroupInfo, int64, error) {
	var groups []model.PermissionGroup
	var total int64

	global.DB.Model(&model.PermissionGroup{}).Count(&total)
	if err := global.DB.Order("sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&groups).Error; err != nil {
		return nil, 0, errors.New("查询权限分组列表失败")
	}

	list := make([]request.PermGroupInfo, len(groups))
	for i, g := range groups {
		list[i] = request.PermGroupInfo{ID: g.ID, Name: g.Name, Sort: g.Sort}
	}
	return list, total, nil
}

func CreatePermGroup(req request.CreatePermGroupReq) (*request.PermGroupInfo, error) {
	var exist int64
	global.DB.Model(&model.PermissionGroup{}).Where("name = ?", req.Name).Count(&exist)
	if exist > 0 {
		return nil, errors.New("分组名称已存在")
	}

	g := model.PermissionGroup{Name: req.Name, Sort: req.Sort}
	if err := global.DB.Create(&g).Error; err != nil {
		return nil, errors.New("创建权限分组失败: " + err.Error())
	}
	return &request.PermGroupInfo{ID: g.ID, Name: g.Name, Sort: g.Sort}, nil
}

func UpdatePermGroup(groupID uint, req request.UpdatePermGroupReq) (*request.PermGroupInfo, error) {
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
	return &request.PermGroupInfo{ID: g.ID, Name: g.Name, Sort: g.Sort}, nil
}

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

// ========== 辅助函数 ==========

func contains(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

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
