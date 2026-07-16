package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	FileValidationStatusLegacyUnverified = "legacy_unverified"
	FileValidationStatusValidated        = "validated"
	FileValidationStatusBlocked          = "blocked"
	FileValidationStatusValidationError  = "validation_error"
	FileUploadPolicyVersion              = "file-upload-v1"
)

// File 文件管理表，存储文件元数据；实际文件位于 MinIO。
type File struct {
	gorm.Model
	Name                    string     `gorm:"type:varchar(255);comment:展示文件名"`
	Bucket                  string     `gorm:"type:varchar(100);not null;comment:MinIO bucket"`
	ObjectName              string     `gorm:"type:varchar(500);not null;comment:MinIO 对象名"`
	ContentType             string     `gorm:"type:varchar(100);comment:规范 MIME 类型"`
	DetectedContentType     string     `gorm:"type:varchar(100);comment:检测 MIME 类型"`
	Size                    int64      `gorm:"comment:文件大小（字节）"`
	UploaderID              uint       `gorm:"index;comment:上传者 ID"`
	ValidationStatus        string     `gorm:"type:varchar(32);not null;default:legacy_unverified;index;comment:文件验证状态"`
	ValidationPolicyVersion string     `gorm:"type:varchar(64);comment:验证策略版本"`
	ValidationErrorCode     string     `gorm:"type:varchar(64);comment:稳定验证失败原因"`
	ValidatedAt             *time.Time `gorm:"comment:验证完成时间"`
}
