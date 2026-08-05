package dictionary

import "gorm.io/gorm"

type Type struct {
	gorm.Model
	Name   string `gorm:"type:varchar(100);not null;comment:字典名称"`
	Code   string `gorm:"type:varchar(100);not null;uniqueIndex;comment:字典编码"`
	Remark string `gorm:"type:varchar(255);comment:备注"`
	Sort   int    `gorm:"type:int;default:0;comment:排序"`
	Status int    `gorm:"type:tinyint;not null;comment:状态 1启用 0禁用"`
}

func (Type) TableName() string { return "dict_types" }

type Item struct {
	gorm.Model
	TypeCode string `gorm:"type:varchar(100);not null;index;uniqueIndex:idx_dict_item_type_value;comment:字典类型编码"`
	Label    string `gorm:"type:varchar(100);not null;comment:显示文本"`
	Value    string `gorm:"type:varchar(100);not null;uniqueIndex:idx_dict_item_type_value;comment:字典值"`
	Remark   string `gorm:"type:varchar(255);comment:备注"`
	Sort     int    `gorm:"type:int;default:0;comment:排序"`
	Status   int    `gorm:"type:tinyint;not null;comment:状态 1启用 0禁用"`
}

func (Item) TableName() string { return "dict_items" }

func Models() []any { return []any{Type{}, Item{}} }
