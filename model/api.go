package model

import "gorm.io/gorm"

// API 保存系统接口的元数据，用于后台管理、权限绑定和后续动态鉴权。
type API struct {
	gorm.Model
	Name           string `gorm:"type:varchar(100);not null;comment:API名称"`
	Method         string `gorm:"type:varchar(10);not null;uniqueIndex:idx_api_method_path;comment:请求方法"`
	Path           string `gorm:"type:varchar(255);not null;uniqueIndex:idx_api_method_path;comment:请求路径"`
	Group          string `gorm:"column:api_group;type:varchar(50);index;comment:API分组"`
	PermissionCode string `gorm:"type:varchar(100);index;comment:绑定权限码"`
	Remark         string `gorm:"type:varchar(255);comment:备注"`
	Sort           int    `gorm:"type:int;default:0;comment:排序"`
	Status         int    `gorm:"type:tinyint;default:1;comment:状态 1启用 0禁用"`
	NeedAuth       int    `gorm:"type:tinyint;default:1;comment:是否需要认证 1是 0否"`
	NeedAudit      int    `gorm:"type:tinyint;default:1;comment:是否记录审计日志 1是 0否"`
}

func (API) TableName() string {
	return "apis"
}
