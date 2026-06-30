package service

import (
	"errors"

	"admin/dto"
	"admin/global"
	"admin/model"
)

// ensureOrganizationParentExists 校验父组织是否存在，parentID 为 0 表示根节点。
func ensureOrganizationParentExists(parentID uint) error {
	if parentID == 0 {
		return nil
	}

	var count int64
	global.DB.Model(&model.Organization{}).Where("id = ?", parentID).Count(&count)
	if count == 0 {
		return errors.New("父组织不存在")
	}
	return nil
}

// ensureOrganizationExists 校验组织是否存在。
func ensureOrganizationExists(orgID uint) error {
	var count int64
	global.DB.Model(&model.Organization{}).Where("id = ?", orgID).Count(&count)
	if count == 0 {
		return errors.New("组织不存在")
	}
	return nil
}

// ensureOrganizationCodeAvailable 校验组织编码在其他组织中未被占用。
func ensureOrganizationCodeAvailable(orgID uint, code string) error {
	var count int64
	query := global.DB.Model(&model.Organization{}).Where("code = ?", code)
	if orgID > 0 {
		query = query.Where("id != ?", orgID)
	}
	query.Count(&count)
	if count > 0 {
		return errors.New("组织编码已存在")
	}
	return nil
}

// ensureUsersExist 校验传入用户 ID 是否全部存在。
func ensureUsersExist(userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}

	var count int64
	global.DB.Model(&model.User{}).Where("id IN ?", userIDs).Count(&count)
	if count != int64(len(userIDs)) {
		return errors.New("存在无效用户")
	}
	return nil
}

func uniqueUintIDs(ids []uint) []uint {
	seen := map[uint]struct{}{}
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// validateOrganizationParent 校验父级组织合法性，避免把节点挂到自己或子孙节点下。
func validateOrganizationParent(orgID, parentID uint) error {
	if parentID == 0 {
		return nil
	}
	if parentID == orgID {
		return errors.New("不能将组织挂载到自身下面")
	}
	if err := ensureOrganizationParentExists(parentID); err != nil {
		return err
	}
	if isOrganizationDescendant(orgID, parentID) {
		return errors.New("不能将组织挂载到自身子组织下面")
	}
	return nil
}

// isOrganizationDescendant 判断 parentID 是否位于 orgID 的子孙链路中。
func isOrganizationDescendant(orgID, parentID uint) bool {
	currentID := parentID
	for currentID != 0 {
		if currentID == orgID {
			return true
		}

		var organization model.Organization
		if err := global.DB.Select("parent_id").First(&organization, currentID).Error; err != nil {
			return false
		}
		currentID = organization.ParentID
	}
	return false
}

// buildOrganizationTree 将扁平组织列表按 parent_id 递归组装为树。
func buildOrganizationTree(organizations []model.Organization, parentID uint) []dto.OrganizationTree {
	tree := []dto.OrganizationTree{}
	for _, organization := range organizations {
		if organization.ParentID != parentID {
			continue
		}

		node := *toOrganizationTree(organization)
		node.Children = buildOrganizationTree(organizations, organization.ID)
		tree = append(tree, node)
	}
	return tree
}

// toOrganizationInfoList 将组织模型列表转换为响应 DTO 列表。
func toOrganizationInfoList(organizations []model.Organization) []dto.OrganizationInfo {
	list := make([]dto.OrganizationInfo, len(organizations))
	for i, item := range organizations {
		list[i] = *toOrganizationInfo(item)
	}
	return list
}

// toOrganizationInfo 将组织模型转换为列表/详情响应结构。
func toOrganizationInfo(item model.Organization) *dto.OrganizationInfo {
	return &dto.OrganizationInfo{
		ID:       item.ID,
		ParentID: item.ParentID,
		Name:     item.Name,
		Code:     item.Code,
		Remark:   item.Remark,
		Sort:     item.Sort,
		Status:   item.Status,
	}
}

// toOrganizationTree 将组织模型转换为树节点响应结构。
func toOrganizationTree(item model.Organization) *dto.OrganizationTree {
	return &dto.OrganizationTree{
		ID:       item.ID,
		ParentID: item.ParentID,
		Name:     item.Name,
		Code:     item.Code,
		Remark:   item.Remark,
		Sort:     item.Sort,
		Status:   item.Status,
	}
}

func toUserInfoList(users []model.User) []dto.UserInfo {
	list := make([]dto.UserInfo, len(users))
	for i, user := range users {
		list[i] = dto.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
			Email:    user.Email,
			Role:     user.Role,
			Status:   user.Status,
		}
	}
	return list
}
