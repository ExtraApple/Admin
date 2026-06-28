package service

import (
	"errors"

	"admin/dto"
	"admin/global"
	"admin/model"

	"gorm.io/gorm"
)

// 获取字典类型
func GetDictTypes(page, pageSize int, keyword string, status *int) ([]dto.DictTypeInfo, int64, error) {
	page, pageSize = normalizePage(page, pageSize)

	var types []model.DictType
	var total int64
	query := global.DB.Model(&model.DictType{})
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errors.New("查询字典类型失败")
	}
	if err := query.Order("sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&types).Error; err != nil {
		return nil, 0, errors.New("查询字典类型失败")
	}
	return toDictTypeInfoList(types), total, nil
}

// 创建字典类型
func CreateDictType(req dto.CreateDictTypeReq) (*dto.DictTypeInfo, error) {
	var exist int64
	global.DB.Model(&model.DictType{}).Where("code = ?", req.Code).Count(&exist)
	if exist > 0 {
		return nil, errors.New("字典编码已存在")
	}

	dictType := model.DictType{
		Name:   req.Name,
		Code:   req.Code,
		Remark: req.Remark,
		Sort:   req.Sort,
		Status: defaultDictStatus(req.Status),
	}
	if err := global.DB.Create(&dictType).Error; err != nil {
		return nil, errors.New("创建字典类型失败: " + err.Error())
	}
	return toDictTypeInfo(dictType), nil
}

// 更新字典类型
func UpdateDictType(typeID uint, req dto.UpdateDictTypeReq) (*dto.DictTypeInfo, error) {
	var dictType model.DictType
	if err := global.DB.First(&dictType, typeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("字典类型不存在")
		}
		return nil, errors.New("查询字典类型失败")
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Code != "" {
		var exist int64
		global.DB.Model(&model.DictType{}).Where("id != ? AND code = ?", typeID, req.Code).Count(&exist)
		if exist > 0 {
			return nil, errors.New("字典编码已存在")
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

	oldCode := dictType.Code
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&dictType).Updates(updates).Error; err != nil {
			return err
		}
		if req.Code != "" && req.Code != oldCode {
			if err := tx.Model(&model.DictItem{}).Where("type_code = ?", oldCode).Update("type_code", req.Code).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, errors.New("修改字典类型失败")
	}

	global.DB.First(&dictType, typeID)
	return toDictTypeInfo(dictType), nil
}

// 删除字典类型
func DeleteDictType(typeID uint) error {
	var dictType model.DictType
	if err := global.DB.First(&dictType, typeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("字典类型不存在")
		}
		return errors.New("查询字典类型失败")
	}

	return global.DB.Transaction(func(tx *gorm.DB) error {
		// 执行硬删除
		if err := tx.Unscoped().Where("type_code = ?", dictType.Code).Delete(&model.DictItem{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&dictType).Error
	})
}

// 获得字典组
func GetDictItems(page, pageSize int, typeCode, keyword string, status *int) ([]dto.DictItemInfo, int64, error) {
	page, pageSize = normalizePage(page, pageSize)

	var items []model.DictItem
	var total int64
	query := global.DB.Model(&model.DictItem{})
	if typeCode != "" {
		query = query.Where("type_code = ?", typeCode)
	}
	if keyword != "" {
		query = query.Where("label LIKE ? OR value LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errors.New("查询字典条目失败")
	}
	if err := query.Order("type_code asc, sort asc, id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error; err != nil {
		return nil, 0, errors.New("查询字典条目失败")
	}
	return toDictItemInfoList(items), total, nil
}

// 创建字典组
func CreateDictItem(req dto.CreateDictItemReq) (*dto.DictItemInfo, error) {
	if err := ensureDictTypeExists(req.TypeCode); err != nil {
		return nil, err
	}
	if err := ensureDictItemValueAvailable(0, req.TypeCode, req.Value); err != nil {
		return nil, err
	}

	item := model.DictItem{
		TypeCode: req.TypeCode,
		Label:    req.Label,
		Value:    req.Value,
		Remark:   req.Remark,
		Sort:     req.Sort,
		Status:   defaultDictStatus(req.Status),
	}
	if err := global.DB.Create(&item).Error; err != nil {
		return nil, errors.New("创建字典条目失败: " + err.Error())
	}
	return toDictItemInfo(item), nil
}

// 更新字典组
func UpdateDictItem(itemID uint, req dto.UpdateDictItemReq) (*dto.DictItemInfo, error) {
	var item model.DictItem
	if err := global.DB.First(&item, itemID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("字典条目不存在")
		}
		return nil, errors.New("查询字典条目失败")
	}

	targetTypeCode := item.TypeCode
	if req.TypeCode != "" {
		if err := ensureDictTypeExists(req.TypeCode); err != nil {
			return nil, err
		}
		targetTypeCode = req.TypeCode
	}

	targetValue := item.Value
	if req.Value != "" {
		targetValue = req.Value
	}
	if targetTypeCode != item.TypeCode || targetValue != item.Value {
		if err := ensureDictItemValueAvailable(itemID, targetTypeCode, targetValue); err != nil {
			return nil, err
		}
	}

	updates := map[string]any{}
	if req.TypeCode != "" {
		updates["type_code"] = req.TypeCode
	}
	if req.Label != "" {
		updates["label"] = req.Label
	}
	if req.Value != "" {
		updates["value"] = req.Value
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

	if err := global.DB.Model(&item).Updates(updates).Error; err != nil {
		return nil, errors.New("修改字典条目失败")
	}
	global.DB.First(&item, itemID)
	return toDictItemInfo(item), nil
}

// 删除字典组
func DeleteDictItem(itemID uint) error {
	var item model.DictItem
	if err := global.DB.First(&item, itemID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("字典条目不存在")
		}
		return errors.New("查询字典条目失败")
	}
	return global.DB.Unscoped().Delete(&item).Error
}

// 根据字典类型编码，获取启用状态的字典条目
func GetEnabledDictItemsByTypeCode(typeCode string) ([]dto.DictItemInfo, error) {
	var dictType model.DictType
	if err := global.DB.Where("code = ? AND status = 1", typeCode).First(&dictType).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []dto.DictItemInfo{}, nil
		}
		return nil, errors.New("查询字典类型失败")
	}

	var items []model.DictItem
	if err := global.DB.Where("type_code = ? AND status = 1", typeCode).
		Order("sort asc, id asc").
		Find(&items).Error; err != nil {
		return nil, errors.New("查询字典条目失败")
	}
	return toDictItemInfoList(items), nil
}
