package dto

type CreateDictTypeReq struct {
	Name   string `json:"name" binding:"required,min=1,max=100"`
	Code   string `json:"code" binding:"required,min=1,max=100"`
	Remark string `json:"remark" binding:"max=255"`
	Sort   int    `json:"sort"`
	Status *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type UpdateDictTypeReq struct {
	Name   string `json:"name" binding:"max=100"`
	Code   string `json:"code" binding:"max=100"`
	Remark string `json:"remark" binding:"max=255"`
	Sort   *int   `json:"sort"`
	Status *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type DictTypeInfo struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Code   string `json:"code"`
	Remark string `json:"remark"`
	Sort   int    `json:"sort"`
	Status int    `json:"status"`
}

type DictTypeListResp struct {
	List  []DictTypeInfo `json:"list"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

type CreateDictItemReq struct {
	TypeCode string `json:"type_code" binding:"required,min=1,max=100"`
	Label    string `json:"label" binding:"required,min=1,max=100"`
	Value    string `json:"value" binding:"required,min=1,max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     int    `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type UpdateDictItemReq struct {
	TypeCode string `json:"type_code" binding:"max=100"`
	Label    string `json:"label" binding:"max=100"`
	Value    string `json:"value" binding:"max=100"`
	Remark   string `json:"remark" binding:"max=255"`
	Sort     *int   `json:"sort"`
	Status   *int   `json:"status" binding:"omitempty,oneof=0 1"`
}

type DictItemInfo struct {
	ID       uint   `json:"id"`
	TypeCode string `json:"type_code"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Remark   string `json:"remark"`
	Sort     int    `json:"sort"`
	Status   int    `json:"status"`
}

type DictItemListResp struct {
	List  []DictItemInfo `json:"list"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}
