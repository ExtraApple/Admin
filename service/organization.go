package service

import (
	"errors"

	"admin/dto"
	"admin/global"
	"admin/model"

	"gorm.io/gorm"
)

// GetOrganizations 分页查询组织列表，支持关键字和状态筛选。
func GetOrganizations(operatorID uint, page, pageSize int, keyword string, status *int) ([]dto.OrganizationInfo, int64, error) {
	page, pageSize = normalizePage(page, pageSize)

	var organizations []model.Organization
	var total int64
	query := global.DB.Model(&model.Organization{})
	visibleOrgIDs, hasAllData, err := GetVisibleOrganizationIDs(operatorID)
	if err != nil {
		return nil, 0, err
	}
	if !hasAllData {
		if len(visibleOrgIDs) == 0 {
			return []dto.OrganizationInfo{}, 0, nil
		}
		query = query.Where("id IN ?", visibleOrgIDs)
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errors.New("查询组织列表失败")
	}
	if err := query.Order("sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&organizations).Error; err != nil {
		return nil, 0, errors.New("查询组织列表失败")
	}

	list := make([]dto.OrganizationInfo, len(organizations))
	for i, item := range organizations {
		list[i] = *toOrganizationInfo(item)
	}
	return list, total, nil
}

// GetOrganizationTree 查询可见组织并组装为树形结构。
func GetOrganizationTree(operatorID uint) ([]dto.OrganizationTree, error) {
	var organizations []model.Organization
	query := global.DB.Order("sort asc, id asc")
	visibleOrgIDs, hasAllData, err := GetVisibleOrganizationIDs(operatorID)
	if err != nil {
		return nil, err
	}
	if !hasAllData {
		if len(visibleOrgIDs) == 0 {
			return []dto.OrganizationTree{}, nil
		}
		query = query.Where("id IN ?", includeAncestorOrgIDs(visibleOrgIDs))
	}
	if err := query.Find(&organizations).Error; err != nil {
		return nil, errors.New("查询组织树失败")
	}
	return buildOrganizationTree(organizations, 0), nil
}

// CreateOrganization 创建组织节点，并校验父级存在和编码唯一性。
func CreateOrganization(operatorID uint, req dto.CreateOrganizationReq) (*dto.OrganizationInfo, error) {
	if req.ParentID != 0 {
		if err := validateOrgVisibility(operatorID, req.ParentID); err != nil {
			return nil, err
		}
	} else if _, hasAllData, err := GetVisibleOrganizationIDs(operatorID); err != nil {
		return nil, err
	} else if !hasAllData {
		return nil, errors.New("无权创建根组织")
	}

	if err := checkParentOrgExists(req.ParentID); err != nil {
		return nil, err
	}
	if err := checkOrgCodeAvailable(0, req.Code); err != nil {
		return nil, err
	}

	organization := model.Organization{
		ParentID: req.ParentID,
		Name:     req.Name,
		Code:     req.Code,
		Remark:   req.Remark,
		Sort:     req.Sort,
		Status:   defaultDictStatus(req.Status),
	}
	if err := global.DB.Create(&organization).Error; err != nil {
		return nil, errors.New("创建组织失败: " + err.Error())
	}
	return toOrganizationInfo(organization), nil
}

// UpdateOrganization 修改组织节点，并防止形成循环父子关系。
func UpdateOrganization(operatorID, orgID uint, req dto.UpdateOrganizationReq) (*dto.OrganizationInfo, error) {
	var organization model.Organization
	if err := global.DB.First(&organization, orgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("组织不存在")
		}
		return nil, errors.New("查询组织失败")
	}
	if err := validateOrgVisibility(operatorID, orgID); err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if req.ParentID != nil {
		if *req.ParentID != 0 {
			if err := validateOrgVisibility(operatorID, *req.ParentID); err != nil {
				return nil, err
			}
		}
		if err := validateOrgParent(orgID, *req.ParentID); err != nil {
			return nil, err
		}
		updates["parent_id"] = *req.ParentID
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Code != "" {
		if err := checkOrgCodeAvailable(orgID, req.Code); err != nil {
			return nil, err
		}
		updates["code"] = req.Code
	}
	if req.Remark != "" {
		updates["remark"] = req.Remark
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}

	if err := global.DB.Model(&organization).Updates(updates).Error; err != nil {
		return nil, errors.New("修改组织失败")
	}

	global.DB.First(&organization, orgID)
	return toOrganizationInfo(organization), nil
}

// DeleteOrganization 删除没有子组织的组织节点。
func DeleteOrganization(operatorID, orgID uint) error {
	var organization model.Organization
	if err := global.DB.First(&organization, orgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("组织不存在")
		}
		return errors.New("查询组织失败")
	}
	if err := validateOrgVisibility(operatorID, orgID); err != nil {
		return err
	}

	var childCount int64
	global.DB.Model(&model.Organization{}).Where("parent_id = ?", orgID).Count(&childCount)
	if childCount > 0 {
		return errors.New("该组织存在子组织，不能删除")
	}

	return global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ?", orgID).Delete(&model.UserOrganization{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&organization).Error
	})
}

// SetOrganizationUsers 覆盖指定组织的成员绑定关系。
func SetOrganizationUsers(operatorID, orgID uint, userIDs []uint) error {
	if err := checkOrgExists(orgID); err != nil {
		return err
	}
	if err := validateOrgVisibility(operatorID, orgID); err != nil {
		return err
	}

	userIDs = uniqueUintIDs(userIDs)
	if len(userIDs) > 0 {
		var count int64
		global.DB.Model(&model.User{}).Where("id IN ?", userIDs).Count(&count)
		if count != int64(len(userIDs)) {
			return errors.New("存在无效用户")
		}
	}

	var oldUserIDs []uint
	global.DB.Model(&model.UserOrganization{}).Where("organization_id = ?", orgID).Pluck("user_id", &oldUserIDs)

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ?", orgID).Delete(&model.UserOrganization{}).Error; err != nil {
			return err
		}

		records := make([]model.UserOrganization, 0, len(userIDs))
		for _, userID := range userIDs {
			records = append(records, model.UserOrganization{
				UserID:         userID,
				OrganizationID: orgID,
			})
		}
		if len(records) == 0 {
			return nil
		}
		return tx.Create(&records).Error
	}); err != nil {
		return err
	}

	affectedUserIDs := append(oldUserIDs, userIDs...)
	revokeTokensForUsers(affectedUserIDs...)
	return nil
}

// GetOrganizationUsers 查询指定组织下的成员列表。
func GetOrganizationUsers(operatorID, orgID uint) ([]dto.UserInfo, error) {
	if err := checkOrgExists(orgID); err != nil {
		return nil, err
	}
	if err := validateOrgVisibility(operatorID, orgID); err != nil {
		return nil, err
	}

	var userOrganizations []model.UserOrganization
	if err := global.DB.Where("organization_id = ?", orgID).Find(&userOrganizations).Error; err != nil {
		return nil, errors.New("查询组织成员失败")
	}

	userIDs := make([]uint, len(userOrganizations))
	for i, item := range userOrganizations {
		userIDs[i] = item.UserID
	}
	if len(userIDs) == 0 {
		return []dto.UserInfo{}, nil
	}

	var users []model.User
	if err := global.DB.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, errors.New("查询组织成员失败")
	}

	list := make([]dto.UserInfo, len(users))
	for i, user := range users {
		list[i] = UserInfoFromModel(user)
	}
	return list, nil
}

// validateOrgParent 校验父级组织合法性，避免把节点挂到自己或子孙节点下。
func validateOrgParent(orgID, parentID uint) error {
	if parentID == 0 {
		return nil
	}
	if parentID == orgID {
		return errors.New("不能将组织挂载到自身下面")
	}
	if err := checkParentOrgExists(parentID); err != nil {
		return err
	}

	currentID := parentID
	for currentID != 0 {
		if currentID == orgID {
			return errors.New("不能将组织挂载到自身子组织下面")
		}

		var organization model.Organization
		if err := global.DB.Select("parent_id").First(&organization, currentID).Error; err != nil {
			return nil
		}
		currentID = organization.ParentID
	}
	return nil
}

// buildOrganizationTree 将扁平组织列表按 parent_id 递归组装为树。
func buildOrganizationTree(organizations []model.Organization, parentID uint) []dto.OrganizationTree {
	tree := []dto.OrganizationTree{}
	for _, organization := range organizations {
		if organization.ParentID != parentID {
			continue
		}

		node := dto.OrganizationTree{
			ID:       organization.ID,
			ParentID: organization.ParentID,
			Name:     organization.Name,
			Code:     organization.Code,
			Remark:   organization.Remark,
			Sort:     organization.Sort,
			Status:   organization.Status,
		}
		node.Children = buildOrganizationTree(organizations, organization.ID)
		tree = append(tree, node)
	}
	return tree
}
