package model

import "gorm.io/gorm"

// File 文件管理表 — 存储文件元数据，实际文件在 MinIO
type File struct {
	gorm.Model
	Name        string `gorm:"type:varchar(255);comment:原始文件名"`
	Bucket      string `gorm:"type:varchar(100);not null;comment:MinIO bucket"`
	ObjectName  string `gorm:"type:varchar(500);not null;comment:MinIO 对象名"`
	ContentType string `gorm:"type:varchar(100);comment:MIME 类型"`
	Size        int64  `gorm:"comment:文件大小(字节)"`
	UploaderID  uint   `gorm:"index;comment:上传者ID"`
}
