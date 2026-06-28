package dto

// ========== 权限请求参数 ==========

type CreatePermissionReq struct {
	Name  string `json:"name"  binding:"required,min=1,max=100"`
	Code  string `json:"code"  binding:"required,min=1,max=100"`
	Group string `json:"group" binding:"max=50"`
	Sort  int    `json:"sort"`
}

type UpdatePermissionReq struct {
	Name  string `json:"name"  binding:"max=100"`
	Group string `json:"group" binding:"max=50"`
	Sort  *int   `json:"sort"`
}

type AssignPermsToRoleReq struct {
	PermissionIDs []uint `json:"permission_ids" binding:"required"`
}

// ========== 权限响应 ==========

type PermissionInfo struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Code  string `json:"code"`
	Group string `json:"group"`
	Sort  int    `json:"sort"`
}

type PermissionListResp struct {
	List  []PermissionInfo `json:"list"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

// ========== 权限分组请求参数 ==========

type CreatePermGroupReq struct {
	Name string `json:"name" binding:"required,min=1,max=50"`
	Sort int    `json:"sort"`
}

type UpdatePermGroupReq struct {
	Name string `json:"name" binding:"max=50"`
	Sort *int   `json:"sort"`
}

// ========== 权限分组响应 ==========

type PermGroupInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

type PermGroupListResp struct {
	List  []PermGroupInfo `json:"list"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
}
