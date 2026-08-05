package audit

import "encoding/json"

// AuditLogListRequest contains the legacy query parameters and pagination
// defaults used by the admin audit endpoints.
type AuditLogListRequest struct {
	Page      int    `form:"page" json:"page"`
	Size      int    `form:"size" json:"size"`
	UserID    uint   `form:"user_id" json:"user_id"`
	Method    string `form:"method" json:"method"`
	Path      string `form:"path" json:"path"`
	Status    int    `form:"status" json:"status"`
	Category  string `form:"category" json:"category"`
	StartTime string `form:"start_time" json:"start_time"`
	EndTime   string `form:"end_time" json:"end_time"`
}

// AuditLogInfo is the stable response shape of the legacy list endpoint.
type AuditLogInfo struct {
	ID        uint            `json:"id"`
	UserID    uint            `json:"user_id"`
	Username  string          `json:"username"`
	Method    string          `json:"method"`
	Path      string          `json:"path"`
	Query     string          `json:"query"`
	Body      string          `json:"body"`
	Metadata  json.RawMessage `json:"metadata"`
	Status    int             `json:"status"`
	Duration  int64           `json:"duration"`
	ClientIP  string          `json:"client_ip"`
	UserAgent string          `json:"user_agent"`
	Category  string          `json:"category"`
	CreatedAt string          `json:"created_at"`
}

type AuditLogListResponse struct {
	List  []AuditLogInfo `json:"list"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}
