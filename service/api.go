package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
)

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
	return toAPIInfo(api), nil
}

func DeleteAPI(apiID uint) error {
	var api model.API
	if err := global.DB.First(&api, apiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("API不存在")
		}
		return errors.New("查询API失败")
	}
	return global.DB.Unscoped().Delete(&api).Error
}

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

		var count int64
		global.DB.Model(&model.API{}).Where("method = ? AND path = ?", method, path).Count(&count)
		if count > 0 {
			continue
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
		created = append(created, *toAPIInfo(api))
	}

	return created, nil
}

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

		code := strings.TrimSpace(api.PermissionCode)
		if code == "" {
			code = generateAPIPermissionCode(api.Method, api.Path)
			if err := global.DB.Model(&api).Update("permission_code", code).Error; err != nil {
				return created, updatedAPI, errors.New("同步API权限码失败")
			}
			updatedAPI++
		}

		var count int64
		global.DB.Model(&model.Permission{}).Where("code = ?", code).Count(&count)
		if count > 0 {
			continue
		}

		permission := model.Permission{
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
