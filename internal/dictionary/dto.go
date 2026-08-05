package dictionary

type CreateTypeRequest struct {
	Name   string `json:"name" binding:"required,min=1,max=100"`
	Code   string `json:"code" binding:"required,min=1,max=100"`
	Remark string `json:"remark" binding:"max=255"`
	Sort   int    `json:"sort"`
	Status *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type UpdateTypeRequest struct {
	Name   string `json:"name" binding:"max=100"`
	Code   string `json:"code" binding:"max=100"`
	Remark string `json:"remark" binding:"max=255"`
	Sort   *int   `json:"sort"`
	Status *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type CreateItemRequest struct {
	TypeCode string `json:"type_code" binding:"required,min=1,max=100"`
	Label    string `json:"label" binding:"required,min=1,max=100"`
	Value    string `json:"value" binding:"required,min=1,max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     int    `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type UpdateItemRequest struct {
	TypeCode string `json:"type_code" binding:"max=100"`
	Label    string `json:"label" binding:"max=100"`
	Value    string `json:"value" binding:"max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     *int   `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type TypeInfo struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Code   string `json:"code"`
	Remark string `json:"remark"`
	Sort   int    `json:"sort"`
	Status int    `json:"status"`
}

type TypeListResponse struct {
	List  []TypeInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

type ItemInfo struct {
	ID       uint   `json:"id"`
	TypeCode string `json:"type_code"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Remark   string `json:"remark"`
	Sort     int    `json:"sort"`
	Status   int    `json:"status"`
}

type ItemListResponse struct {
	List  []ItemInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}
