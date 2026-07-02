package service

import (
	"errors"

	"admin/global"
	"admin/model"
)

func normalizeDataScope(scope string) (string, error) {
	if scope == "" {
		return model.DataScopeAll, nil
	}

	switch scope {
	case model.DataScopeAll,
		model.DataScopeSelf,
		model.DataScopeOrg,
		model.DataScopeOrgAndChildren,
		model.DataScopeCustom:
		return scope, nil
	default:
		return "", errors.New("数据范围不合法")
	}
}

func ensureOrganizationsExist(orgIDs []uint) error {
	orgIDs = uniqueUintIDs(orgIDs)
	if len(orgIDs) == 0 {
		return nil
	}

	var count int64
	if err := global.DB.Model(&model.Organization{}).Where("id IN ?", orgIDs).Count(&count).Error; err != nil {
		return errors.New("查询组织失败")
	}
	if count != int64(len(orgIDs)) {
		return errors.New("存在无效组织")
	}
	return nil
}

func getOperatorDataScope(operatorID uint) (bool, []uint, error) {
	if operatorID == 0 {
		return false, nil, errors.New("用户身份无效")
	}

	var roles []model.Role
	if err := global.DB.Model(&model.Role{}).
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ? AND roles.status = ?", operatorID, 1).
		Find(&roles).Error; err != nil {
		return false, nil, errors.New("查询用户角色失败")
	}

	if len(roles) == 0 {
		return false, []uint{}, nil
	}

	var orgIDs []uint
	for _, role := range roles {
		dataScope := role.DataScope
		if dataScope == "" {
			dataScope = model.DataScopeAll
		}

		if role.Code == "admin" || dataScope == model.DataScopeAll {
			return true, nil, nil
		}

		switch dataScope {
		case model.DataScopeSelf:
			continue
		case model.DataScopeOrg:
			orgIDs = append(orgIDs, getUserOrganizationIDs(operatorID)...)
		case model.DataScopeOrgAndChildren:
			userOrgIDs := getUserOrganizationIDs(operatorID)
			orgIDs = append(orgIDs, expandOrganizationIDsWithChildren(userOrgIDs)...)
		case model.DataScopeCustom:
			orgIDs = append(orgIDs, getRoleCustomOrganizationIDs(role.ID)...)
		}
	}

	return false, uniqueUintIDs(orgIDs), nil
}

func ensureUserVisibleToOperator(operatorID, targetID uint) error {
	visibleUserIDs, hasAllData, err := GetVisibleUserIDs(operatorID)
	if err != nil {
		return err
	}
	if hasAllData || containsUint(visibleUserIDs, targetID) {
		return nil
	}
	return errors.New("无权操作数据范围外的用户")
}

func ensureOrganizationVisibleToOperator(operatorID, orgID uint) error {
	visibleOrgIDs, hasAllData, err := GetVisibleOrganizationIDs(operatorID)
	if err != nil {
		return err
	}
	if hasAllData || containsUint(visibleOrgIDs, orgID) {
		return nil
	}
	return errors.New("无权操作数据范围外的组织")
}

func GetVisibleUserIDs(operatorID uint) ([]uint, bool, error) {
	hasAllData, orgIDs, err := getOperatorDataScope(operatorID)
	if err != nil {
		return nil, false, err
	}
	if hasAllData {
		return nil, true, nil
	}

	userIDs := []uint{operatorID}
	if len(orgIDs) > 0 {
		var orgUserIDs []uint
		if err := global.DB.Model(&model.UserOrganization{}).
			Where("organization_id IN ?", orgIDs).
			Pluck("user_id", &orgUserIDs).Error; err != nil {
			return nil, false, errors.New("查询可见用户失败")
		}
		userIDs = append(userIDs, orgUserIDs...)
	}

	return uniqueUintIDs(userIDs), false, nil
}

func GetVisibleOrganizationIDs(operatorID uint) ([]uint, bool, error) {
	hasAllData, orgIDs, err := getOperatorDataScope(operatorID)
	if err != nil {
		return nil, false, err
	}
	if hasAllData {
		return nil, true, nil
	}
	return uniqueUintIDs(orgIDs), false, nil
}

func getUserOrganizationIDs(userID uint) []uint {
	var orgIDs []uint
	global.DB.Model(&model.UserOrganization{}).
		Where("user_id = ?", userID).
		Pluck("organization_id", &orgIDs)
	return uniqueUintIDs(orgIDs)
}

func getRoleCustomOrganizationIDs(roleID uint) []uint {
	var orgIDs []uint
	global.DB.Model(&model.RoleDataScope{}).
		Where("role_id = ?", roleID).
		Pluck("organization_id", &orgIDs)
	return uniqueUintIDs(orgIDs)
}

func expandOrganizationIDsWithChildren(rootIDs []uint) []uint {
	rootIDs = uniqueUintIDs(rootIDs)
	if len(rootIDs) == 0 {
		return []uint{}
	}

	var organizations []model.Organization
	if err := global.DB.Select("id", "parent_id").Find(&organizations).Error; err != nil {
		return rootIDs
	}

	childrenByParent := map[uint][]uint{}
	for _, organization := range organizations {
		childrenByParent[organization.ParentID] = append(childrenByParent[organization.ParentID], organization.ID)
	}

	result := make([]uint, 0, len(rootIDs))
	queue := append([]uint{}, rootIDs...)
	seen := map[uint]struct{}{}
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if _, ok := seen[currentID]; ok {
			continue
		}
		seen[currentID] = struct{}{}
		result = append(result, currentID)
		queue = append(queue, childrenByParent[currentID]...)
	}
	return result
}

func expandOrganizationIDsWithAncestors(orgIDs []uint) []uint {
	orgIDs = uniqueUintIDs(orgIDs)
	if len(orgIDs) == 0 {
		return []uint{}
	}

	var organizations []model.Organization
	if err := global.DB.Select("id", "parent_id").Find(&organizations).Error; err != nil {
		return orgIDs
	}

	parentByID := map[uint]uint{}
	for _, organization := range organizations {
		parentByID[organization.ID] = organization.ParentID
	}

	result := append([]uint{}, orgIDs...)
	for _, orgID := range orgIDs {
		currentID := parentByID[orgID]
		for currentID != 0 {
			result = append(result, currentID)
			currentID = parentByID[currentID]
		}
	}
	return uniqueUintIDs(result)
}

func containsUint(ids []uint, target uint) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
