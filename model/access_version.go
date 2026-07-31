package model

import "time"

// UserAccessVersion stores the authorization-owned version for one user.
type UserAccessVersion struct {
	UserID    uint      `gorm:"primaryKey;comment:用户ID"`
	Version   int       `gorm:"type:int;not null;default:1;check:version > 0;comment:授权版本"`
	CreatedAt time.Time `gorm:"comment:创建时间"`
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}
