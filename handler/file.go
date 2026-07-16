package handler

import (
	"context"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"admin/dto"
	"admin/service"
	"admin/service/fileaccess"
	"admin/service/uploadsecurity"
)

type FileDetailGetter interface {
	Get(ctx context.Context, userID, fileID uint) (*dto.FileDetailResp, error)
}

type FileUploader interface {
	Upload(ctx context.Context, input service.UploadFileInput) (*dto.FileInfo, error)
}

type FileContentOpener interface {
	Open(ctx context.Context, input service.FileAccessInput) (*service.FileContent, error)
}

type FileRevalidator interface {
	Revalidate(ctx context.Context, fileID uint) (*dto.FileInfo, error)
}

type FileHandler struct {
	Details        FileDetailGetter
	Uploads        FileUploader
	Contents       FileContentOpener
	Revalidations  FileRevalidator
	List           func(context.Context, int, int, string) ([]dto.FileInfo, int64, error)
	Update         func(context.Context, uint, dto.UpdateFileReq) (*dto.FileInfo, error)
	Delete         func(context.Context, uint) error
	Browse         func(context.Context, string) ([]dto.FileObjectInfo, error)
	MaxUploadBytes int64
}

// Upload 上传文件
func (h *FileHandler) Upload(c *gin.Context) {
	file, cleanup, err := parseSingleUpload(
		c,
		h.MaxUploadBytes,
		managedFileMultipartOverheadBytes,
	)
	defer cleanup()
	if err != nil {
		setRejectedUploadAuditMetadata(
			c,
			uploadsecurity.PurposeManagedFile,
			nil,
			err,
		)
		writeUploadError(c, err)
		return
	}

	f, err := file.Open()
	if err != nil {
		classifiedErr := uploadsecurity.NewError(
			uploadsecurity.CodeUploadBodyInvalid,
			err,
		)
		setRejectedUploadAuditMetadata(
			c,
			uploadsecurity.PurposeManagedFile,
			file,
			classifiedErr,
		)
		writeUploadError(c, classifiedErr)
		return
	}
	defer f.Close()

	if h.Uploads == nil {
		classifiedErr := uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		)
		setRejectedUploadAuditMetadata(
			c,
			uploadsecurity.PurposeManagedFile,
			file,
			classifiedErr,
		)
		writeUploadError(c, classifiedErr)
		return
	}

	info, err := h.Uploads.Upload(c.Request.Context(), service.UploadFileInput{
		UploaderID:  c.GetUint("userID"),
		FileName:    file.Filename,
		ContentType: file.Header.Get("Content-Type"),
		Size:        file.Size,
		MaxBytes:    h.MaxUploadBytes,
		Reader:      f,
	})
	if err != nil {
		setRejectedUploadAuditMetadata(
			c,
			uploadsecurity.PurposeManagedFile,
			file,
			err,
		)
		writeUploadError(c, err)
		return
	}
	c.Set(service.UploadAuditMetadataContextKey, service.UploadAuditMetadata{
		Purpose:          string(uploadsecurity.PurposeManagedFile),
		FileName:         info.Name,
		FileSize:         info.Size,
		DeclaredMIME:     file.Header.Get("Content-Type"),
		DetectedMIME:     info.DetectedContentType,
		ValidationResult: service.UploadValidationAccepted,
		PolicyVersion:    info.ValidationPolicyVersion,
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "上传成功", "data": info})
}

// Download 下载文件
func (h *FileHandler) Download(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	expiresAt, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil || expiresAt < 1 || c.Query("signature") == "" {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	if h.Contents == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}

	content, err := h.Contents.Open(c.Request.Context(), service.FileAccessInput{
		UserID:    c.GetUint("userID"),
		FileID:    uint(fileID),
		ExpiresAt: expiresAt,
		Signature: c.Query("signature"),
		Mode:      fileaccess.ModeDownload,
	})
	if err != nil {
		writeUploadError(c, err)
		return
	}
	if content == nil || content.Reader == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}
	defer content.Reader.Close()

	c.Header("Content-Type", content.ContentType)
	c.Header("Content-Disposition", mime.FormatMediaType(
		string(content.Disposition),
		map[string]string{"filename": content.FileName},
	))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, content.Reader); err != nil {
		_ = c.Error(err)
	}
}

// Preview 预览文件
func (h *FileHandler) Preview(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	expiresAt, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil || expiresAt < 1 || c.Query("signature") == "" {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	if h.Contents == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}

	content, err := h.Contents.Open(c.Request.Context(), service.FileAccessInput{
		UserID:    c.GetUint("userID"),
		FileID:    uint(fileID),
		ExpiresAt: expiresAt,
		Signature: c.Query("signature"),
		Mode:      fileaccess.ModePreview,
	})
	if err != nil {
		writeUploadError(c, err)
		return
	}
	if content == nil || content.Reader == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}
	defer content.Reader.Close()

	c.Header("Content-Type", content.ContentType)
	c.Header("Content-Disposition", mime.FormatMediaType(
		string(content.Disposition),
		map[string]string{"filename": content.FileName},
	))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, content.Reader); err != nil {
		_ = c.Error(err)
	}
}

// Revalidate 重新验证历史文件
func (h *FileHandler) Revalidate(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	if h.Revalidations == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}

	info, err := h.Revalidations.Revalidate(c.Request.Context(), uint(fileID))
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "重新验证完成",
		"data": info,
	})
}

// GetFile 获取文件详情
func (h *FileHandler) GetFile(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}

	if h.Details == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}

	info, err := h.Details.Get(c.Request.Context(), c.GetUint("userID"), uint(fileID))
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": info})
}

// ListFiles 文件列表
func (h *FileHandler) ListFiles(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("size", "10"))
	if err != nil || size < 1 {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	prefix := c.Query("prefix")

	listFiles := h.List
	if listFiles == nil {
		listFiles = func(
			_ context.Context,
			page, size int,
			prefix string,
		) ([]dto.FileInfo, int64, error) {
			return service.ListFiles(page, size, prefix)
		}
	}
	list, total, err := listFiles(c.Request.Context(), page, size, prefix)
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.FileListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// UpdateFile 修改文件信息
func (h *FileHandler) UpdateFile(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	var req dto.UpdateFileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}

	updateFile := h.Update
	if updateFile == nil {
		updateFile = func(
			_ context.Context,
			fileID uint,
			req dto.UpdateFileReq,
		) (*dto.FileInfo, error) {
			return service.UpdateFile(fileID, req)
		}
	}
	info, err := updateFile(c.Request.Context(), uint(fileID), req)
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": info})
}

// DeleteFile 删除文件
func (h *FileHandler) DeleteFile(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	deleteFile := h.Delete
	if deleteFile == nil {
		deleteFile = func(_ context.Context, fileID uint) error {
			return service.DeleteFile(fileID)
		}
	}
	if err := deleteFile(c.Request.Context(), uint(fileID)); err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// BrowseFiles 按目录浏览
func (h *FileHandler) BrowseFiles(c *gin.Context) {
	prefix := c.Query("prefix")
	browseFiles := h.Browse
	if browseFiles == nil {
		browseFiles = func(
			_ context.Context,
			prefix string,
		) ([]dto.FileObjectInfo, error) {
			return service.BrowseFiles(prefix)
		}
	}
	files, err := browseFiles(c.Request.Context(), prefix)
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": files})
}
