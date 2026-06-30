package dto

type CreateAPIReq struct {
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Method         string `json:"method" binding:"required,min=2,max=10"`
	Path           string `json:"path" binding:"required,min=1,max=255"`
	Group          string `json:"group" binding:"max=50"`
	PermissionCode string `json:"permission_code" binding:"max=100"`
	Remark         string `json:"remark" binding:"max=255"`
	Sort           int    `json:"sort"`
	Status         *int   `json:"status" binding:"omitempty,oneof=0 1"`
	NeedAuth       *int   `json:"need_auth" binding:"omitempty,oneof=0 1"`
	NeedAudit      *int   `json:"need_audit" binding:"omitempty,oneof=0 1"`
}

type UpdateAPIReq struct {
	Name           *string `json:"name" binding:"omitempty,min=1,max=100"`
	Method         *string `json:"method" binding:"omitempty,min=2,max=10"`
	Path           *string `json:"path" binding:"omitempty,min=1,max=255"`
	Group          *string `json:"group" binding:"omitempty,max=50"`
	PermissionCode *string `json:"permission_code" binding:"omitempty,max=100"`
	Remark         *string `json:"remark" binding:"omitempty,max=255"`
	Sort           *int    `json:"sort"`
	Status         *int    `json:"status" binding:"omitempty,oneof=0 1"`
	NeedAuth       *int    `json:"need_auth" binding:"omitempty,oneof=0 1"`
	NeedAudit      *int    `json:"need_audit" binding:"omitempty,oneof=0 1"`
}

type APIInfo struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	Group          string `json:"group"`
	PermissionCode string `json:"permission_code"`
	Remark         string `json:"remark"`
	Sort           int    `json:"sort"`
	Status         int    `json:"status"`
	NeedAuth       int    `json:"need_auth"`
	NeedAudit      int    `json:"need_audit"`
}

type APIListResp struct {
	List  []APIInfo `json:"list"`
	Total int64     `json:"total"`
	Page  int       `json:"page"`
	Size  int       `json:"size"`
}

type SyncAPIItem struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
