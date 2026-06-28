package dto

type AuditLogListReq struct {
	Page      int    `form:"page"`
	Size      int    `form:"size"`
	UserID    uint   `form:"user_id"`
	Method    string `form:"method"`
	Path      string `form:"path"`
	Status    int    `form:"status"`
	Category  string `form:"category"`
	StartTime string `form:"start_time"`
	EndTime   string `form:"end_time"`
}

type AuditLogInfo struct {
	ID        uint   `json:"id"`
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Query     string `json:"query"`
	Body      string `json:"body"`
	Status    int    `json:"status"`
	Duration  int64  `json:"duration"`
	ClientIP  string `json:"client_ip"`
	UserAgent string `json:"user_agent"`
	Category  string `json:"category"`
	CreatedAt string `json:"created_at"`
}

type AuditLogListResp struct {
	List  []AuditLogInfo `json:"list"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}
