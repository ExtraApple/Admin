package service

import (
	"errors"

	"admin/dto"
	"admin/global"
	"admin/model"
)

// checkParentOrgExists 校验父组织是否存在，parentID 为 0 表示根节点。
func checkParentOrgExists(parentID uint) error {
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

// checkOrgExists 校验组织是否存在。
func checkOrgExists(orgID uint) error {
	var count int64
	global.DB.Model(&model.Organization{}).Where("id = ?", orgID).Count(&count)
	if count == 0 {
		return errors.New("组织不存在")
	}
	return nil
}

// checkOrgCodeAvailable 校验组织编码在其他组织中未被占用。
func checkOrgCodeAvailable(orgID uint, code string) error {
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

// uniqueUintIDs 去除 uint ID 列表中的零值和重复值。
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
