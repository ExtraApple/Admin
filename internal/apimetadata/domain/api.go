package domain

import "time"

type API struct {
	ID             uint
	Name           string
	Method         string
	Path           string
	Group          string
	PermissionCode string
	Remark         string
	Sort           int
	Status         int
	NeedAuth       int
	NeedAudit      int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Policy struct {
	Status         int
	NeedAuth       int
	NeedAudit      int
	PermissionCode string
}

type GroupOption struct {
	Group string `json:"group"`
	Count int64  `json:"count"`
}
