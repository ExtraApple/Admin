package dto

// ========== 角色请求参数 ==========

type CreateRoleReq struct {
	Name        string `json:"name"        binding:"required,min=1,max=50"`
	Code        string `json:"code"        binding:"required,min=1,max=50"`
	Description string `json:"description" binding:"max=255"`
	Sort        int    `json:"sort"`
	Status      int    `json:"status"`
	DataScope   string `json:"data_scope"`
}

type UpdateRoleReq struct {
	Name        string `json:"name"        binding:"max=50"`
	Code        string `json:"code"        binding:"max=50"`
	Description string `json:"description" binding:"max=255"`
	Sort        *int   `json:"sort"`
	Status      *int   `json:"status"`
	DataScope   string `json:"data_scope"`
}

type AssignUsersToRoleReq struct {
	UserIDs []uint `json:"user_ids" binding:"required"`
}

type AssignRoleDataScopeReq struct {
	DataScope       string `json:"data_scope" binding:"required"`
	OrganizationIDs []uint `json:"organization_ids"`
}

// ========== 角色响应 ==========

type RoleInfo struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Sort        int    `json:"sort"`
	Status      int    `json:"status"`
	DataScope   string `json:"data_scope"`
}

type RoleListResp struct {
	List  []RoleInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

type RoleDataScopeInfo struct {
	RoleID          uint   `json:"role_id"`
	DataScope       string `json:"data_scope"`
	OrganizationIDs []uint `json:"organization_ids"`
}
