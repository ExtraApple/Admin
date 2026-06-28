package dto

// ========== 文件请求参数 ==========

type UpdateFileReq struct {
	Name string `json:"name" binding:"max=255"`
}

// ========== 文件响应 ==========

type FileInfo struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Bucket      string `json:"bucket"`
	ObjectName  string `json:"object_name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	UploaderID  uint   `json:"uploader_id"`
	CreatedAt   string `json:"created_at"`
}

type FileListResp struct {
	List  []FileInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}
