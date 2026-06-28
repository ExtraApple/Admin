package model

import "time"

type AuditLog struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"index;comment:操作用户ID"`
	Username  string    `gorm:"type:varchar(100);comment:用户名"`
	Method    string    `gorm:"type:varchar(10);index:idx_method_path;comment:请求方法"`
	Path      string    `gorm:"type:varchar(255);index;index:idx_method_path;comment:请求路径"`
	Query     string    `gorm:"type:text;comment:URL查询参数"`
	Body      string    `gorm:"type:text;comment:脱敏后的请求体"`
	Status    int       `gorm:"type:int;comment:HTTP状态码"`
	Duration  int64     `gorm:"comment:耗时毫秒"`
	ClientIP  string    `gorm:"type:varchar(50);comment:客户端IP"`
	UserAgent string    `gorm:"type:varchar(255);comment:User-Agent"`
	Category  string    `gorm:"type:varchar(50);index;comment:日志分类"`
	CreatedAt time.Time `gorm:"index;comment:请求时间"`
}

type AuditLogArchive struct {
	ID         uint      `gorm:"primaryKey"`
	UserID     uint      `gorm:"index;comment:操作用户ID"`
	Username   string    `gorm:"type:varchar(100);comment:用户名"`
	Method     string    `gorm:"type:varchar(10);index:idx_archive_method_path;comment:请求方法"`
	Path       string    `gorm:"type:varchar(255);index;index:idx_archive_method_path;comment:请求路径"`
	Query      string    `gorm:"type:text;comment:URL查询参数"`
	Body       string    `gorm:"type:text;comment:脱敏后的请求体"`
	Status     int       `gorm:"type:int;comment:HTTP状态码"`
	Duration   int64     `gorm:"comment:耗时毫秒"`
	ClientIP   string    `gorm:"type:varchar(50);comment:客户端IP"`
	UserAgent  string    `gorm:"type:varchar(255);comment:User-Agent"`
	Category   string    `gorm:"type:varchar(50);index;comment:日志分类"`
	CreatedAt  time.Time `gorm:"index;comment:请求时间"`
	ArchivedAt time.Time `gorm:"index;comment:归档时间"`
}
