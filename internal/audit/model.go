package audit

import (
	"encoding/json"
	"time"
)

// AuditLog is the provider-neutral hot audit record. Its fields intentionally
// contain only request metadata that is safe to persist after redaction.
type AuditLog struct {
	ID        uint            `gorm:"primaryKey"`
	UserID    uint            `gorm:"index;comment:操作用户 ID"`
	Username  string          `gorm:"type:varchar(100);comment:用户名"`
	Method    string          `gorm:"type:varchar(10);index:idx_method_path;comment:请求方法"`
	Path      string          `gorm:"type:varchar(255);index;index:idx_method_path;comment:请求路径"`
	Query     string          `gorm:"type:text;comment:URL 查询参数"`
	Body      string          `gorm:"type:text;comment:脱敏后的请求体"`
	Metadata  json.RawMessage `gorm:"type:json;comment:结构化审计元数据"`
	Status    int             `gorm:"type:int;comment:HTTP 状态码"`
	Duration  int64           `gorm:"comment:耗时毫秒"`
	ClientIP  string          `gorm:"type:varchar(50);comment:客户端 IP"`
	UserAgent string          `gorm:"type:varchar(255);comment:User-Agent"`
	Category  string          `gorm:"type:varchar(50);index;comment:日志分类"`
	CreatedAt time.Time       `gorm:"index;comment:请求时间"`
}

func (AuditLog) TableName() string { return "audit_logs" }

// AuditLogArchive is the cold-storage copy of an AuditLog.
type AuditLogArchive struct {
	ID         uint            `gorm:"primaryKey"`
	UserID     uint            `gorm:"index;comment:操作用户 ID"`
	Username   string          `gorm:"type:varchar(100);comment:用户名"`
	Method     string          `gorm:"type:varchar(10);index:idx_archive_method_path;comment:请求方法"`
	Path       string          `gorm:"type:varchar(255);index;index:idx_archive_method_path;comment:请求路径"`
	Query      string          `gorm:"type:text;comment:URL 查询参数"`
	Body       string          `gorm:"type:text;comment:脱敏后的请求体"`
	Metadata   json.RawMessage `gorm:"type:json;comment:结构化审计元数据"`
	Status     int             `gorm:"type:int;comment:HTTP 状态码"`
	Duration   int64           `gorm:"comment:耗时毫秒"`
	ClientIP   string          `gorm:"type:varchar(50);comment:客户端 IP"`
	UserAgent  string          `gorm:"type:varchar(255);comment:User-Agent"`
	Category   string          `gorm:"type:varchar(50);index;comment:日志分类"`
	CreatedAt  time.Time       `gorm:"index;comment:请求时间"`
	ArchivedAt time.Time       `gorm:"index;comment:归档时间"`
}

func (AuditLogArchive) TableName() string { return "audit_log_archives" }

// Models returns the persistence models owned by Audit. The values are plain
// records so the application layer has no dependency on a database provider.
func Models() []any { return []any{AuditLog{}, AuditLogArchive{}} }
