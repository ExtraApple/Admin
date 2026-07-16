package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
)

// GetAPIs 分页查询 API 元数据，支持关键字、分组、方法和状态筛选。
func GetAPIs(page, pageSize int, keyword, group, method string, status, needAuth, needAudit *int) ([]dto.APIInfo, int64, error) {
	page, pageSize = normalizePage(page, pageSize)

	var apis []model.API
	var total int64
	query := global.DB.Model(&model.API{})

	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR path LIKE ? OR permission_code LIKE ?", like, like, like)
	}
	if group != "" {
		query = query.Where("api_group = ?", strings.TrimSpace(group))
	}
	if method != "" {
		normalizedMethod, err := normalizeAPIMethod(method)
		if err != nil {
			return nil, 0, err
		}
		query = query.Where("method = ?", normalizedMethod)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if needAuth != nil {
		query = query.Where("need_auth = ?", *needAuth)
	}
	if needAudit != nil {
		query = query.Where("need_audit = ?", *needAudit)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errors.New("查询API列表失败")
	}
	if err := query.Order("api_group asc, sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&apis).Error; err != nil {
		return nil, 0, errors.New("查询API列表失败")
	}
	return toAPIInfoList(apis), total, nil
}

// GetAPI 查询单个 API 元数据详情。
func GetAPI(apiID uint) (*dto.APIInfo, error) {
	var api model.API
	if err := global.DB.First(&api, apiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("API不存在")
		}
		return nil, errors.New("查询API失败")
	}
	return toAPIInfo(api), nil
}

// GetAPIGroupOptions 统计并返回 API 分组选项。
func GetAPIGroupOptions() ([]dto.APIGroupOption, error) {
	type groupRow struct {
		Group string `gorm:"column:api_group"`
		Count int64  `gorm:"column:count"`
	}

	var rows []groupRow
	if err := global.DB.Model(&model.API{}).
		Select("api_group, COUNT(*) AS count").
		Group("api_group").
		Order("api_group asc").
		Scan(&rows).Error; err != nil {
		return nil, errors.New("查询API分组失败")
	}

	list := make([]dto.APIGroupOption, 0, len(rows))
	for _, row := range rows {
		group := strings.TrimSpace(row.Group)
		if group == "" {
			group = "api"
		}
		list = append(list, dto.APIGroupOption{
			Group: group,
			Count: row.Count,
		})
	}
	return list, nil
}

// GetAPIMethodOptions 返回系统支持的 HTTP 方法选项。
func GetAPIMethodOptions() []dto.APIMethodOption {
	list := make([]dto.APIMethodOption, 0, len(supportedAPIMethodList))
	for _, method := range supportedAPIMethodList {
		list = append(list, dto.APIMethodOption{
			Label: method,
			Value: method,
		})
	}
	return list
}

// CreateAPI 创建 API 元数据记录。
func CreateAPI(req dto.CreateAPIReq) (*dto.APIInfo, error) {
	method, err := normalizeAPIMethod(req.Method)
	if err != nil {
		return nil, err
	}
	path, err := normalizeAPIPath(req.Path)
	if err != nil {
		return nil, err
	}
	if err := ensureAPIAvailable(0, method, path); err != nil {
		return nil, err
	}

	api := model.API{
		Name:           strings.TrimSpace(req.Name),
		Method:         method,
		Path:           path,
		Group:          strings.TrimSpace(req.Group),
		PermissionCode: strings.TrimSpace(req.PermissionCode),
		Remark:         strings.TrimSpace(req.Remark),
		Sort:           req.Sort,
		Status:         defaultAPIStatus(req.Status),
		NeedAuth:       defaultAPISwitch(req.NeedAuth),
		NeedAudit:      defaultAPISwitch(req.NeedAudit),
	}
	if api.Name == "" {
		return nil, errors.New("API名称不能为空")
	}
	if api.Group == "" {
		api.Group = inferAPIGroup(path)
	}

	if err := global.DB.Create(&api).Error; err != nil {
		return nil, errors.New("创建API失败: " + err.Error())
	}
	return toAPIInfo(api), nil
}

// UpdateAPI 修改 API 元数据，并在权限码变化时同步关联菜单。
func UpdateAPI(apiID uint, req dto.UpdateAPIReq) (*dto.APIInfo, error) {
	var api model.API
	if err := global.DB.First(&api, apiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("API不存在")
		}
		return nil, errors.New("查询API失败")
	}

	targetMethod := api.Method
	targetPath := api.Path
	if req.Method != nil {
		method, err := normalizeAPIMethod(*req.Method)
		if err != nil {
			return nil, err
		}
		targetMethod = method
	}
	if req.Path != nil {
		path, err := normalizeAPIPath(*req.Path)
		if err != nil {
			return nil, err
		}
		targetPath = path
	}
	if targetMethod != api.Method || targetPath != api.Path {
		if err := ensureAPIAvailable(apiID, targetMethod, targetPath); err != nil {
			return nil, err
		}
	}

	updates := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, errors.New("API名称不能为空")
		}
		updates["name"] = name
	}
	if req.Method != nil {
		updates["method"] = targetMethod
	}
	if req.Path != nil {
		updates["path"] = targetPath
	}
	if req.Group != nil {
		updates["api_group"] = strings.TrimSpace(*req.Group)
	}
	if req.PermissionCode != nil {
		updates["permission_code"] = strings.TrimSpace(*req.PermissionCode)
	}
	if req.Remark != nil {
		updates["remark"] = strings.TrimSpace(*req.Remark)
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.NeedAuth != nil {
		updates["need_auth"] = *req.NeedAuth
	}
	if req.NeedAudit != nil {
		updates["need_audit"] = *req.NeedAudit
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}

	if err := global.DB.Model(&api).Updates(updates).Error; err != nil {
		return nil, errors.New("修改API失败")
	}
	global.DB.First(&api, apiID)
	if req.PermissionCode != nil {
		if err := syncLinkedMenusByAPI(api); err != nil {
			return nil, err
		}
	}
	return toAPIInfo(api), nil
}

// DeleteAPI 删除 API 元数据并清理菜单关联。
func DeleteAPI(apiID uint) error {
	var api model.API
	if err := global.DB.First(&api, apiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("API不存在")
		}
		return errors.New("查询API失败")
	}

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("api_id = ?", apiID).Delete(&model.MenuAPI{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&api).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// SyncAPIs 根据路由列表创建缺失的 API 元数据。
func SyncAPIs(routes []dto.SyncAPIItem) ([]dto.APIInfo, error) {
	created := []dto.APIInfo{}

	for _, route := range routes {
		method, err := normalizeAPIMethod(route.Method)
		if err != nil {
			continue
		}
		path, err := normalizeAPIPath(route.Path)
		if err != nil || !shouldSyncAPIRoute(path) {
			continue
		}

		var existing model.API
		err = global.DB.Unscoped().Where("method = ? AND path = ?", method, path).First(&existing).Error
		if err == nil {
			if existing.DeletedAt.Valid {
				if err := global.DB.Unscoped().Model(&existing).Updates(map[string]any{
					"deleted_at": nil,
					"status":     1,
				}).Error; err != nil {
					return created, errors.New("恢复API失败: " + err.Error())
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, errors.New("查询API失败: " + err.Error())
		}

		needAuth := inferAPINeedAuth(path)
		permissionCode := ""
		if needAuth == 1 {
			permissionCode = generateAPIPermissionCode(method, path)
		}

		api := model.API{
			Name:           method + " " + path,
			Method:         method,
			Path:           path,
			Group:          inferAPIGroup(path),
			PermissionCode: permissionCode,
			Status:         1,
			NeedAuth:       needAuth,
			NeedAudit:      1,
		}
		if err := global.DB.Create(&api).Error; err != nil {
			return created, errors.New("同步API失败: " + err.Error())
		}
		if needAuth == 0 {
			if err := global.DB.Model(&api).
				UpdateColumn("need_auth", 0).Error; err != nil {
				return created, errors.New("同步API认证配置失败: " + err.Error())
			}
			api.NeedAuth = 0
		}
		created = append(created, *toAPIInfo(api))
	}

	return created, nil
}

// SyncAPIPermissions 为需要鉴权的 API 同步权限码和权限记录。
func SyncAPIPermissions() ([]string, int, error) {
	var apis []model.API
	if err := global.DB.Find(&apis).Error; err != nil {
		return nil, 0, errors.New("查询API列表失败")
	}

	created := []string{}
	updatedAPI := 0
	for _, api := range apis {
		if api.NeedAuth == 0 {
			continue
		}
		// 移除字符串前后空白字符
		code := strings.TrimSpace(api.PermissionCode)
		if code == "" {
			code = generateAPIPermissionCode(api.Method, api.Path)
			if err := global.DB.Model(&api).Update("permission_code", code).Error; err != nil {
				return created, updatedAPI, errors.New("同步API权限码失败")
			}
			updatedAPI++
		}

		var permission model.Permission
		err := global.DB.Unscoped().Where("code = ?", code).First(&permission).Error
		if err == nil {
			if permission.DeletedAt.Valid {
				if err := global.DB.Unscoped().Model(&permission).Updates(map[string]any{
					"deleted_at": nil,
				}).Error; err != nil {
					return created, updatedAPI, errors.New("恢复API权限失败")
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, updatedAPI, errors.New("查询API权限失败")
		}

		permission = model.Permission{
			Name:  api.Name,
			Code:  code,
			Group: api.Group,
			Sort:  api.Sort,
		}
		if err := global.DB.Create(&permission).Error; err != nil {
			return created, updatedAPI, errors.New("创建API权限失败: " + err.Error())
		}
		created = append(created, code)
	}

	return created, updatedAPI, nil
}
