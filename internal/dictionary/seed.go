package dictionary

import (
	"errors"

	authdomain "admin/internal/authorization/domain"

	"gorm.io/gorm"
)

type seedDictType struct {
	Name   string
	Code   string
	Remark string
	Sort   int
	Status int
	Items  []seedDictItem
}

type seedDictItem struct {
	Label  string
	Value  string
	Remark string
	Sort   int
	Status int
}

// Seed restores Dictionary-owned types and items.
func Seed(tx *gorm.DB) (types, items int, err error) {
	for _, item := range defaultDictTypes() {
		var dictType Type
		err := tx.Unscoped().Where("code = ?", item.Code).First(&dictType).Error
		if err == nil {
			if dictType.DeletedAt.Valid {
				if err := tx.Unscoped().Model(&dictType).Updates(map[string]any{"deleted_at": nil, "status": 1}).Error; err != nil {
					return types, items, err
				}
			}
		} else {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return types, items, err
			}
			dictType = Type{Name: item.Name, Code: item.Code, Remark: item.Remark, Sort: item.Sort, Status: item.Status}
			if err := tx.Create(&dictType).Error; err != nil {
				return types, items, err
			}
			types++
		}
		for _, dictItem := range item.Items {
			var existingItem Item
			err := tx.Unscoped().Where("type_code = ? AND value = ?", item.Code, dictItem.Value).First(&existingItem).Error
			if err == nil {
				if existingItem.DeletedAt.Valid {
					if err := tx.Unscoped().Model(&existingItem).Updates(map[string]any{"deleted_at": nil, "status": 1}).Error; err != nil {
						return types, items, err
					}
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return types, items, err
			}
			if err := tx.Create(&Item{TypeCode: item.Code, Label: dictItem.Label, Value: dictItem.Value, Remark: dictItem.Remark, Sort: dictItem.Sort, Status: dictItem.Status}).Error; err != nil {
				return types, items, err
			}
			items++
		}
	}
	return types, items, nil
}

func defaultDictTypes() []seedDictType {
	return []seedDictType{
		{Name: "用户状态", Code: "user_status", Remark: "用户启用状态", Sort: 10, Status: 1, Items: []seedDictItem{{Label: "禁用", Value: "0", Sort: 1, Status: 1}, {Label: "启用", Value: "1", Sort: 2, Status: 1}}},
		{Name: "角色状态", Code: "role_status", Remark: "角色启用状态", Sort: 20, Status: 1, Items: []seedDictItem{{Label: "禁用", Value: "0", Sort: 1, Status: 1}, {Label: "启用", Value: "1", Sort: 2, Status: 1}}},
		{Name: "菜单类型", Code: "menu_type", Remark: "后台菜单节点类型", Sort: 30, Status: 1, Items: []seedDictItem{{Label: "目录", Value: "1", Sort: 1, Status: 1}, {Label: "菜单", Value: "2", Sort: 2, Status: 1}, {Label: "按钮", Value: "3", Sort: 3, Status: 1}}},
		{Name: "HTTP 方法", Code: "api_method", Remark: "API 请求方法", Sort: 40, Status: 1, Items: []seedDictItem{{Label: "GET", Value: "GET", Sort: 1, Status: 1}, {Label: "POST", Value: "POST", Sort: 2, Status: 1}, {Label: "PUT", Value: "PUT", Sort: 3, Status: 1}, {Label: "PATCH", Value: "PATCH", Sort: 4, Status: 1}, {Label: "DELETE", Value: "DELETE", Sort: 5, Status: 1}}},
		{Name: "数据范围", Code: "data_scope", Remark: "角色数据权限范围", Sort: 50, Status: 1, Items: []seedDictItem{{Label: "全部数据", Value: string(authdomain.DataScopeAll), Sort: 1, Status: 1}, {Label: "仅本人数据", Value: string(authdomain.DataScopeSelf), Sort: 2, Status: 1}, {Label: "本组织数据", Value: string(authdomain.DataScopeOrg), Sort: 3, Status: 1}, {Label: "本组织及下级组织", Value: string(authdomain.DataScopeOrgAndChildren), Sort: 4, Status: 1}, {Label: "自定义组织", Value: string(authdomain.DataScopeCustom), Sort: 5, Status: 1}}},
	}
}
