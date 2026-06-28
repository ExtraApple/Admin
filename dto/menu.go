package dto

type CreateMenuReq struct {
	ParentID  uint   `json:"parent_id"`
	Name      string `json:"name" binding:"required,min=1,max=100"`
	Path      string `json:"path" binding:"max=255"`
	Component string `json:"component" binding:"max=255"`
	Icon      string `json:"icon" binding:"max=100"`
	Sort      int    `json:"sort"`
	Type      int    `json:"type"`
	Status    int    `json:"status"`
}

type UpdateMenuReq struct {
	ParentID  *uint  `json:"parent_id"`
	Name      string `json:"name" binding:"max=100"`
	Path      string `json:"path" binding:"max=255"`
	Component string `json:"component" binding:"max=255"`
	Icon      string `json:"icon" binding:"max=100"`
	Sort      *int   `json:"sort"`
	Type      *int   `json:"type"`
	Status    *int   `json:"status"`
}

type AssignMenusToRoleReq struct {
	MenuIDs []uint `json:"menu_ids" binding:"required"`
}

type SyncMenuItem struct {
	Name       string `json:"name" binding:"required,min=1,max=100"`
	Path       string `json:"path" binding:"required,max=255"`
	Component  string `json:"component" binding:"max=255"`
	Icon       string `json:"icon" binding:"max=100"`
	ParentPath string `json:"parent_path" binding:"max=255"`
	Sort       int    `json:"sort"`
	Type       int    `json:"type"`
}

type SyncMenusReq struct {
	Routes []SyncMenuItem `json:"routes" binding:"required"`
}

type MenuDetail struct {
	ID        uint         `json:"id"`
	ParentID  uint         `json:"parent_id"`
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	Component string       `json:"component"`
	Icon      string       `json:"icon"`
	Sort      int          `json:"sort"`
	Type      int          `json:"type"`
	Status    int          `json:"status"`
	Children  []MenuDetail `json:"children,omitempty"`
}
