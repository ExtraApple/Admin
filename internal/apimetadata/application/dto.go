package application

import "context"

type CreateInput struct {
	Name           string
	Method         string
	Path           string
	Group          string
	PermissionCode string
	Remark         string
	Sort           int
	Status         *int
	NeedAuth       *int
	NeedAudit      *int
}

type UpdateInput struct {
	Name           *string
	Method         *string
	Path           *string
	Group          *string
	PermissionCode *string
	Remark         *string
	Sort           *int
	Status         *int
	NeedAuth       *int
	NeedAudit      *int
}

type MethodOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Button struct {
	ID             uint   `json:"id"`
	ParentID       uint   `json:"parent_id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Component      string `json:"component"`
	Icon           string `json:"icon"`
	PermissionCode string `json:"permission_code"`
	Sort           int    `json:"sort"`
	Type           int    `json:"type"`
	Status         int    `json:"status"`
}

type RouteFact struct {
	Method                string
	Path                  string
	Name                  string
	Group                 string
	Authenticated         bool
	PermissionControlled  bool
	DefaultPermissionCode string
	NeedAudit             bool
}

type RouteSource interface {
	Routes(context.Context) ([]RouteFact, error)
}
