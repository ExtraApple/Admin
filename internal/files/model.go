package files

import (
	"time"

	"gorm.io/gorm"
)

const (
	FileBucket                           = "files"
	FileValidationStatusLegacyUnverified = "legacy_unverified"
	FileValidationStatusValidated        = "validated"
	FileValidationStatusBlocked          = "blocked"
	FileValidationStatusValidationError  = "validation_error"
	FileUploadPolicyVersion              = "file-upload-v1"
)

// File is the Files-owned persistence record. The explicit table name keeps
// the migration compatible with the legacy files table.
type File struct {
	gorm.Model
	Name                    string     `gorm:"type:varchar(255);comment:展示文件名"`
	Bucket                  string     `gorm:"type:varchar(100);not null;comment:MinIO bucket"`
	ObjectName              string     `gorm:"type:varchar(500);not null;comment:MinIO 对象名"`
	ContentType             string     `gorm:"type:varchar(100);comment:规范 MIME 类型"`
	DetectedContentType     string     `gorm:"type:varchar(100);comment:检测 MIME 类型"`
	ContentSHA256           string     `gorm:"type:varchar(64);comment:服务端计算的内容 SHA-256 摘要"`
	Size                    int64      `gorm:"comment:文件大小（字节）"`
	UploaderID              uint       `gorm:"index;comment:上传者 ID"`
	Purpose                 string     `gorm:"type:varchar(32);not null;default:managed_file;index;comment:文件用途"`
	LogicalMessageID        string     `gorm:"type:char(36);index;comment:绑定的逻辑消息 ID"`
	BindingExpiresAt        *time.Time `gorm:"index;comment:消息图片绑定截止时间"`
	ValidationStatus        string     `gorm:"type:varchar(32);not null;default:legacy_unverified;index;comment:文件验证状态"`
	ValidationPolicyVersion string     `gorm:"type:varchar(64);comment:验证策略版本"`
	ValidationErrorCode     string     `gorm:"type:varchar(64);comment:稳定验证失败原因"`
	ValidatedAt             *time.Time `gorm:"comment:验证完成时间"`
}

func (File) TableName() string { return "files" }

// MessageImageCleanupJob retains the immutable object location until cleanup
// succeeds. It is physically deleted together with its File Record.
type MessageImageCleanupJob struct {
	ID            uint       `gorm:"primaryKey;index:idx_message_image_cleanup_due,priority:3"`
	FileID        uint       `gorm:"not null;uniqueIndex:ux_message_image_cleanup_file"`
	Bucket        string     `gorm:"type:varchar(100);not null"`
	ObjectName    string     `gorm:"type:varchar(500);not null"`
	ObjectKeyHash string     `gorm:"type:char(64);not null;uniqueIndex:ux_message_image_cleanup_path"`
	Status        string     `gorm:"type:varchar(16);not null;default:pending;index:idx_message_image_cleanup_due,priority:1"`
	RetryCount    uint       `gorm:"not null;default:0"`
	NextRetryAt   *time.Time `gorm:"index:idx_message_image_cleanup_due,priority:2"`
	LastErrorCode string     `gorm:"type:varchar(100)"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (MessageImageCleanupJob) TableName() string { return "message_image_cleanup_jobs" }

func Models() []any { return []any{File{}, MessageImageCleanupJob{}} }
