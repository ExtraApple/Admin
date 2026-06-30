package dto

type CreateOrganizationReq struct {
	ParentID uint   `json:"parent_id"`
	Name     string `json:"name" binding:"required,min=1,max=100"`
	Code     string `json:"code" binding:"required,min=1,max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     int    `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type UpdateOrganizationReq struct {
	ParentID *uint  `json:"parent_id"`
	Name     string `json:"name" binding:"max=100"`
	Code     string `json:"code" binding:"max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     *int   `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type OrganizationInfo struct {
	ID       uint   `json:"id"`
	ParentID uint   `json:"parent_id"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Remark   string `json:"remark"`
	Sort     int    `json:"sort"`
	Status   int    `json:"status"`
}

type OrganizationTree struct {
	ID       uint               `json:"id"`
	ParentID uint               `json:"parent_id"`
	Name     string             `json:"name"`
	Code     string             `json:"code"`
	Remark   string             `json:"remark"`
	Sort     int                `json:"sort"`
	Status   int                `json:"status"`
	Children []OrganizationTree `json:"children,omitempty"`
}

type OrganizationListResp struct {
	List  []OrganizationInfo `json:"list"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
}

type AssignUsersToOrganizationReq struct {
	UserIDs []uint `json:"user_ids" binding:"required"`
}
