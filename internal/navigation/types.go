package navigation

type Menu struct {
	ID             uint
	ParentID       uint
	Name           string
	Path           *string
	Component      string
	Icon           string
	PermissionCode string
	Sort           int
	Type           int
	Status         int
}

type MenuDetail struct {
	ID             uint         `json:"id"`
	ParentID       uint         `json:"parent_id"`
	Name           string       `json:"name"`
	Path           string       `json:"path"`
	Component      string       `json:"component"`
	Icon           string       `json:"icon"`
	PermissionCode string       `json:"permission_code"`
	Sort           int          `json:"sort"`
	Type           int          `json:"type"`
	Status         int          `json:"status"`
	Children       []MenuDetail `json:"children,omitempty"`
}

type CreateInput struct {
	ParentID       uint
	Name           string
	Path           string
	Component      string
	Icon           string
	PermissionCode string
	Sort           int
	Type           int
	Status         int
}

type UpdateInput struct {
	ParentID       *uint
	Name           string
	Path           string
	Component      string
	Icon           string
	PermissionCode *string
	Sort           *int
	Type           *int
	Status         *int
}

type SyncItem struct {
	Name           string
	Path           string
	Component      string
	Icon           string
	PermissionCode string
	ParentPath     string
	Sort           int
	Type           int
}

type APIRecord struct {
	ID             uint
	Name           string
	Method         string
	Path           string
	Group          string
	PermissionCode string
	Sort           int
	Status         int
	NeedAuth       int
	NeedAudit      int
	Remark         string
}

type PermissionSeed struct {
	Code  string
	Name  string
	Group string
	Sort  int
}

type PermissionRef struct {
	ID   uint
	Code string
}

type UserAccess struct {
	RoleIDs     []uint
	IsAdmin     bool
	Permissions []string
}
