package handler

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"

	"admin/service"
	"admin/service/uploadsecurity"
)

const (
	managedFileMultipartOverheadBytes = int64(1 * 1024 * 1024)
	avatarMultipartOverheadBytes      = int64(256 * 1024)
	uploadMultipartMemoryBytes        = int64(1 * 1024 * 1024)
)

func applyUploadBodyLimit(c *gin.Context, maxFileBytes, overheadBytes int64) {
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxFileBytes+overheadBytes,
	)
}

func uploadFormError(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return uploadsecurity.NewError(uploadsecurity.CodeUploadBodyTooLarge, err)
	}
	return uploadsecurity.NewError(uploadsecurity.CodeUploadBodyInvalid, err)
}

func parseSingleUpload(
	c *gin.Context,
	maxFileBytes, overheadBytes int64,
) (*multipart.FileHeader, func(), error) {
	applyUploadBodyLimit(c, maxFileBytes, overheadBytes)

	if err := c.Request.ParseMultipartForm(uploadMultipartMemoryBytes); err != nil {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
		return nil, func() {}, uploadFormError(err)
	}

	form := c.Request.MultipartForm
	cleanup := func() {
		if form != nil {
			_ = form.RemoveAll()
		}
	}

	totalFileParts := 0
	for _, fileHeaders := range form.File {
		totalFileParts += len(fileHeaders)
	}
	if totalFileParts == 0 {
		return nil, cleanup, uploadsecurity.NewError(
			uploadsecurity.CodeUploadFileMissing,
			nil,
		)
	}
	fileHeaders := form.File["file"]
	if totalFileParts != 1 || len(fileHeaders) != 1 {
		return nil, cleanup, uploadsecurity.NewError(
			uploadsecurity.CodeUploadMultipleFiles,
			nil,
		)
	}
	if fileHeaders[0].Size == 0 {
		return nil, cleanup, uploadsecurity.NewError(
			uploadsecurity.CodeFileEmpty,
			nil,
		)
	}
	if fileHeaders[0].Size > maxFileBytes {
		return nil, cleanup, uploadsecurity.NewError(
			uploadsecurity.CodeFileTooLarge,
			nil,
		)
	}
	return fileHeaders[0], cleanup, nil
}

func writeUploadError(c *gin.Context, err error) {
	httpError := uploadsecurity.ToHTTPError(err)
	c.JSON(httpError.Status, gin.H{
		"code":       httpError.Status,
		"error_code": httpError.Code,
		"msg":        httpError.Message,
	})
}

func setRejectedUploadAuditMetadata(
	c *gin.Context,
	purpose uploadsecurity.Purpose,
	file *multipart.FileHeader,
	err error,
) {
	code, ok := uploadsecurity.CodeOf(err)
	if !ok {
		code = uploadsecurity.CodeInternalError
	}

	metadata := service.UploadAuditMetadata{
		Purpose:          string(purpose),
		ValidationResult: service.UploadValidationRejected,
		ReasonCode:       string(code),
	}
	if file != nil {
		metadata.FileName = uploadsecurity.SanitizeAuditFileName(
			file.Filename,
			purpose,
		)
		metadata.FileSize = file.Size
		metadata.DeclaredMIME = file.Header.Get("Content-Type")
	}
	c.Set(service.UploadAuditMetadataContextKey, metadata)
}
