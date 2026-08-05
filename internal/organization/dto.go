package organization

type CreateUnitRequest struct {
	ParentID uint   `json:"parent_id"`
	Name     string `json:"name" binding:"required,min=1,max=100"`
	Code     string `json:"code" binding:"required,min=1,max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     int    `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type UpdateUnitRequest struct {
	ParentID *uint  `json:"parent_id"`
	Name     string `json:"name" binding:"max=100"`
	Code     string `json:"code" binding:"max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     *int   `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type AssignUsersRequest struct {
	UserIDs []uint `json:"user_ids" binding:"required"`
}

type UnitInfo struct {
	ID       uint   `json:"id"`
	ParentID uint   `json:"parent_id"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Remark   string `json:"remark"`
	Sort     int    `json:"sort"`
	Status   int    `json:"status"`
}

type TreeNode struct {
	ID       uint       `json:"id"`
	ParentID uint       `json:"parent_id"`
	Name     string     `json:"name"`
	Code     string     `json:"code"`
	Remark   string     `json:"remark"`
	Sort     int        `json:"sort"`
	Status   int        `json:"status"`
	Children []TreeNode `json:"children,omitempty"`
}

type ListResponse struct {
	List  []UnitInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

type MemberInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Status   int    `json:"status"`
}
