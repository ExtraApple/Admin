package dto

import "time"

// ========== 文件请求参数 ==========

type UpdateFileReq struct {
	Name string `json:"name" binding:"max=255"`
}

// ========== 文件响应 ==========

type FileInfo struct {
	ID                      uint   `json:"id"`
	Name                    string `json:"name"`
	ContentType             string `json:"content_type"`
	DetectedContentType     string `json:"detected_content_type"`
	Size                    int64  `json:"size"`
	UploaderID              uint   `json:"uploader_id"`
	ValidationStatus        string `json:"validation_status"`
	ValidationPolicyVersion string `json:"validation_policy_version"`
	ValidationErrorCode     string `json:"validation_error_code"`
	ValidatedAt             string `json:"validated_at"`
	CreatedAt               string `json:"created_at"`
}

type FileListResp struct {
	List  []FileInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

type FileDetailResp struct {
	File        *FileInfo `json:"file"`
	DownloadURL string    `json:"download_url,omitempty"`
	PreviewURL  string    `json:"preview_url,omitempty"`
}

// FileObjectInfo is storage metadata returned by the administrator browse
// endpoint. It does not claim that an object is a managed or validated file.
type FileObjectInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	LastModified time.Time `json:"last_modified"`
}
