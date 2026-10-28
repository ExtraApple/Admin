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
	Deleted        bool
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
	ParentID            uint
	Name                string
	Path                string
	Component           string
	Icon                string
	PermissionCode      string
	Sort                int
	Type                int
	Status              *int
	ExpectedFingerprint string
}

type UpdateInput struct {
	ParentID            *uint
	Name                *string
	Path                *string
	Component           *string
	Icon                *string
	PermissionCode      *string
	Sort                *int
	Type                *int
	Status              *int
	ExpectedFingerprint string
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
	Deleted        bool
}

// APIPermissionSyncTarget preserves the API sync's original business order and
// resolved code while Navigation prepares the whole request's lock targets.
type APIPermissionSyncTarget struct {
	API  APIRecord
	Code string
}

type RoleMenuAssignment struct {
	RoleID uint
	MenuID uint
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
