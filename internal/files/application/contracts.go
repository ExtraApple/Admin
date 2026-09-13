package application

import (
	"context"
	"errors"
	"io"
	"time"

	"admin/internal/files/domain"
	"admin/internal/uploadsecurity"
)

var (
	ErrFileNotFound  = errors.New("file not found")
	ErrStateConflict = errors.New("file state conflict")
)

type ObjectStorage interface {
	Put(context.Context, ObjectInput) error
	Open(context.Context, string, string) (io.ReadCloser, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, ListOptions) ([]Object, error)
	Move(context.Context, string, string, string) error
}
type ObjectInput struct {
	Bucket, Name string
	Reader       io.Reader
	Size         int64
	ContentType  string
}
type Object struct {
	Name, ContentType string
	Size              int64
	LastModified      time.Time
}
type ListOptions struct {
	Prefix    string
	Recursive bool
}

// UploadValidator is deliberately expressed in terms of the shared security
// module so policy, staging, re-encoding and SHA-256 behavior stay identical.
type UploadValidator interface {
	Validate(context.Context, uploadsecurity.Input) (uploadsecurity.Result, error)
}
type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }

type Signer interface {
	Sign(Claims) (string, error)
	Verify(Claims, string, time.Time) error
}
type Claims struct {
	UserID, FileID   uint
	Mode             AccessMode
	ExpiresAt        int64
	ValidationStatus string
}
type AccessMode string

const (
	ModeDownload AccessMode = "download"
	ModePreview  AccessMode = "preview"
)

type Repository interface {
	Create(context.Context, *domain.File) error
	FindByID(context.Context, uint) (domain.File, error)
	List(context.Context, int, int, string) ([]domain.File, int64, error)
	UpdateName(context.Context, uint, string) error
	Delete(context.Context, uint) error
	UpdateValidation(context.Context, uint, ValidationUpdate) error
	FindRotationCandidates(context.Context, time.Time, string, int) ([]domain.File, error)
	UpdateBucket(context.Context, uint, string) error
}
type MessageImageRepository interface {
	CreateMessageImage(context.Context, *domain.File) error
	FindMessageImage(context.Context, uint) (domain.File, error)
	BindMessageImages(context.Context, MessageImageBindRequest) error
	DeleteExpiredMessageImages(context.Context, time.Time, int) ([]domain.File, error)
}

type MessageImageBindRequest struct {
	ActorID          uint
	MessageLogicalID string
	ImageIDs         []uint
	Now              time.Time
}

type MessageImageUploadInput struct {
	UploaderID            uint
	FileName, ContentType string
	Size                  int64
	Reader                io.Reader
}

type TemporaryMessageImage struct {
	ID        uint
	ExpiresAt time.Time
}

type MessageImageOpenRequest struct {
	ID               uint
	MessageLogicalID string
}

type MessageImageContent struct {
	Reader      io.ReadCloser
	ContentType string
	Size        int64
}

type ValidationUpdate struct {
	ContentType, DetectedContentType, ContentSHA256, Status, PolicyVersion, ErrorCode string
	ValidatedAt                                                                       *time.Time
}
type ObjectNameGenerator interface {
	ManagedFileName(uploadsecurity.CanonicalType, string) (string, error)
}

type AuditMetadata struct {
	Purpose          string `json:"purpose"`
	FileName         string `json:"file_name,omitempty"`
	FileSize         int64  `json:"file_size,omitempty"`
	DeclaredMIME     string `json:"declared_mime,omitempty"`
	DetectedMIME     string `json:"detected_mime,omitempty"`
	ValidationResult string `json:"validation_result"`
	ReasonCode       string `json:"reason_code,omitempty"`
	PolicyVersion    string `json:"policy_version,omitempty"`
}

const (
	UploadValidationAccepted = "accepted"
	UploadValidationRejected = "rejected"
)

type UploadInput struct {
	UploaderID            uint
	FileName, ContentType string
	Size, MaxBytes        int64
	Reader                io.Reader
}
type UpdateFileRequest struct {
	Name string `json:"name" binding:"max=255"`
}
type FileInfo struct {
	ID                      uint   `json:"id"`
	Name                    string `json:"name"`
	ContentType             string `json:"content_type"`
	DetectedContentType     string `json:"detected_content_type"`
	ContentSHA256           string `json:"content_sha256"`
	Size                    int64  `json:"size"`
	UploaderID              uint   `json:"uploader_id"`
	ValidationStatus        string `json:"validation_status"`
	ValidationPolicyVersion string `json:"validation_policy_version"`
	ValidationErrorCode     string `json:"validation_error_code"`
	ValidatedAt             string `json:"validated_at"`
	CreatedAt               string `json:"created_at"`
}
type FileListResponse struct {
	List  []FileInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}
type FileDetailResponse struct {
	File        *FileInfo `json:"file"`
	DownloadURL string    `json:"download_url,omitempty"`
}
type FileObjectInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	LastModified time.Time `json:"last_modified"`
}
type AccessDecision struct {
	FileName, Bucket, ObjectName, ContentType, ValidationStatus string
	Disposition                                                 FileDisposition
}
type FileDisposition string

const (
	DispositionAttachment FileDisposition = "attachment"
	DispositionInline     FileDisposition = "inline"
)

type Content struct {
	FileName, ContentType string
	Disposition           FileDisposition
	Reader                io.ReadCloser
}
type FileAccessInput struct {
	UserID, FileID uint
	ExpiresAt      int64
	Signature      string
	Mode           AccessMode
}
type RotationConfig struct {
	Enabled               bool
	HotBucket, ColdBucket string
	Days, BatchSize       int
}
type RotationResult struct {
	Examined, Moved int
	Failures        []error
}

type Dependencies struct {
	// wiring: required —— 缺失时全部文件读写返回 file_internal_error（repository() 前置校验，无兜底）
	Repository Repository
	// wiring: required —— 缺失时对象存储读写失败，上传与浏览返回 file_storage_unavailable
	Storage ObjectStorage
	// wiring: required —— 缺失时上传与重新验证返回 file_internal_error，校验策略整体失效
	Validator UploadValidator
	// wiring: required —— 缺失时消息图片上传、绑定、读取与过期清理全部不可用
	MessageImages MessageImageRepository
	// wiring: required —— 缺失时消息图片上传返回 file_internal_error，图片安全策略失效
	MessageImageValidator UploadValidator
	// wiring: optional —— 缺失时 constructor 兜底 directTransactionRunner{}，事务退化为直通执行
	Transactions TransactionRunner
	// wiring: required —— 缺失时下载链接不再签发，下载与预览签名校验返回 file_internal_error
	Signer Signer
	// wiring: optional —— 缺失时 constructor 兜底 ClockFunc(time.Now)
	Clock Clock
	// wiring: optional —— 缺失时 constructor 兜底 UUIDObjectNames{}
	ObjectNames ObjectNameGenerator
	// wiring: optional —— 缺失或为 0 时 constructor 兜底 300 秒
	DownloadURLExpireSeconds int
}
