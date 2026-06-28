package service

import (
	"errors"

	"admin/dto"
	"admin/global"
	"admin/model"
)

// 默认页面
func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	return page, pageSize
}

// 默认字典状态
func defaultDictStatus(status *int) int {
	if status == nil {
		return 1
	}
	return *status
}

// 字典类型是否存在
func ensureDictTypeExists(typeCode string) error {
	var count int64
	global.DB.Model(&model.DictType{}).Where("code = ?", typeCode).Count(&count)
	if count == 0 {
		return errors.New("字典类型不存在")
	}
	return nil
}

// 字典值是否存在
func ensureDictItemValueAvailable(itemID uint, typeCode, value string) error {
	var count int64
	query := global.DB.Model(&model.DictItem{}).Where("type_code = ? AND value = ?", typeCode, value)
	if itemID > 0 {
		query = query.Where("id != ?", itemID)
	}
	query.Count(&count)
	if count > 0 {
		return errors.New("同一字典类型下字典值已存在")
	}
	return nil
}

// 列出字典类型
func toDictTypeInfoList(types []model.DictType) []dto.DictTypeInfo {
	list := make([]dto.DictTypeInfo, len(types))
	for i, item := range types {
		list[i] = *toDictTypeInfo(item)
	}
	return list
}

// 字典类型信息
func toDictTypeInfo(item model.DictType) *dto.DictTypeInfo {
	return &dto.DictTypeInfo{
		ID:     item.ID,
		Name:   item.Name,
		Code:   item.Code,
		Remark: item.Remark,
		Sort:   item.Sort,
		Status: item.Status,
	}
}

// 出字典组
func toDictItemInfoList(items []model.DictItem) []dto.DictItemInfo {
	list := make([]dto.DictItemInfo, len(items))
	for i, item := range items {
		list[i] = *toDictItemInfo(item)
	}
	return list
}

// 列出字典组信息
func toDictItemInfo(item model.DictItem) *dto.DictItemInfo {
	return &dto.DictItemInfo{
		ID:       item.ID,
		TypeCode: item.TypeCode,
		Label:    item.Label,
		Value:    item.Value,
		Remark:   item.Remark,
		Sort:     item.Sort,
		Status:   item.Status,
	}
}
