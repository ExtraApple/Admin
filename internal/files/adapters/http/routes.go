package httpadapter

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"

	"github.com/gin-gonic/gin"

	"admin/internal/files/application"
	"admin/internal/routecatalog"
	"admin/internal/uploadsecurity"
)

const (
	multipartOverheadBytes int64 = 1024 * 1024
	multipartMemoryBytes   int64 = 1024 * 1024
)

type handler struct {
	service        *application.Service
	maxUploadBytes int64
}
type fileEnvelope struct {
	Code int                  `json:"code"`
	Msg  string               `json:"msg,omitempty"`
	Data application.FileInfo `json:"data"`
}
type detailEnvelope struct {
	Code int                            `json:"code"`
	Data application.FileDetailResponse `json:"data"`
}
type listEnvelope struct {
	Code int                          `json:"code"`
	Data application.FileListResponse `json:"data"`
}
type browseEnvelope struct {
	Code int                          `json:"code"`
	Data []application.FileObjectInfo `json:"data"`
}

func Routes(service *application.Service, maxUploadBytes int64) []routecatalog.Descriptor {
	h := &handler{service: service, maxUploadBytes: maxUploadBytes}
	return []routecatalog.Descriptor{
		fileRoute(http.MethodPost, "/api/admin/files", "Upload File", "admin.files.post", h.upload, nil, fileEnvelope{}, "管理员普通文件上传只能包含一个名为 file 的文件 part。V1 只允许 PDF、UTF-8 TXT、UTF-8 CSV，当前不支持 Office 文档，也不支持图片；扩展名、声明 MIME、检测 MIME 和专用验证结果必须一致，文件通过验证后才会写入对象存储。响应元数据包含服务端计算的 content_sha256。", true),
		fileRoute(http.MethodGet, "/api/admin/files", "List Files", "admin.files.get", h.list, nil, listEnvelope{}, "按创建时间倒序分页返回文件元数据。", false),
		fileRoute(http.MethodGet, "/api/admin/files/:id", "Get File", "admin.files.id.get", h.detail, nil, detailEnvelope{}, "返回文件验证状态 validated、legacy_unverified、validation_error、blocked、content_sha256 及其他可信元数据。download_url 仅在状态允许下载时返回；不会暴露 MinIO 直连地址。", false),
		fileRoute(http.MethodPut, "/api/admin/files/:id", "Update File", "admin.files.id.put", h.update, application.UpdateFileRequest{}, fileEnvelope{}, "修改安全展示文件名并保留可信规范扩展名。", false),
		fileRoute(http.MethodDelete, "/api/admin/files/:id", "Delete File", "admin.files.id.delete", h.delete, nil, nil, "删除对象存储内容和文件记录。", false),
		binaryRoute("/api/admin/files/:id/download", "Download File", "admin.files.id.download.get", h.download, "使用 JWT、动态 API 权限、有效期和 HMAC 签名下载文件。validated 文件使用规范 MIME；legacy_unverified 和 validation_error 强制作为 application/octet-stream 附件；blocked 拒绝访问。"),
		previewRoute(h.preview),
		fileRoute(http.MethodPost, "/api/admin/files/:id/revalidate", "Revalidate File", "admin.files.id.revalidate.post", h.revalidate, nil, fileEnvelope{}, "仅允许重新验证 legacy_unverified 或 validation_error 文件；通过后更新为 validated，明确策略拒绝更新为 blocked，临时基础设施错误更新为 validation_error。", false),
		fileRoute(http.MethodGet, "/api/admin/files-browse", "Browse Files", "admin.files-browse.get", h.browse, nil, browseEnvelope{}, "浏览 files bucket 的对象元数据，不赋予托管文件验证状态。", false),
	}
}
func fileRoute(method, path, name, permission string, fn gin.HandlerFunc, request, response any, description string, multipart bool) routecatalog.Descriptor {
	body := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if multipart {
		body = routecatalog.RequestBody{Kind: routecatalog.MultipartBody, Required: true, FileField: "file", FileDescription: "Validated PDF, UTF-8 TXT or UTF-8 CSV"}
	} else if request != nil {
		body = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(request), Required: true}
	}
	responses := map[int]routecatalog.Response{
		http.StatusOK:                    routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(response)),
		http.StatusBadRequest:            fileErrorResponseFor(http.StatusBadRequest, uploadsecurity.CodeRequestInvalid, uploadsecurity.CodeUploadBodyInvalid, uploadsecurity.CodeUploadFileMissing, uploadsecurity.CodeUploadMultipleFiles, uploadsecurity.CodeFileEmpty, uploadsecurity.CodeFileNameInvalid),
		http.StatusRequestEntityTooLarge: fileErrorResponseFor(http.StatusRequestEntityTooLarge, uploadsecurity.CodeUploadBodyTooLarge, uploadsecurity.CodeFileTooLarge),
		http.StatusUnsupportedMediaType:  fileErrorResponseFor(http.StatusUnsupportedMediaType, uploadsecurity.CodeFileTypeNotAllowed, uploadsecurity.CodeFileTypeMismatch),
		http.StatusUnprocessableEntity:   fileErrorResponseFor(http.StatusUnprocessableEntity, uploadsecurity.CodeFileEncodingInvalid, uploadsecurity.CodeFileContentInvalid, uploadsecurity.CodeImageDimensionLimit, uploadsecurity.CodeImageDecodeInvalid),
		http.StatusNotFound:              fileErrorResponseFor(http.StatusNotFound, uploadsecurity.CodeFileNotFound, uploadsecurity.CodeStorageObjectNotFound),
		http.StatusConflict:              fileErrorResponseFor(http.StatusConflict, uploadsecurity.CodeFileStateBlocked, uploadsecurity.CodeFileStateConflict),
		http.StatusServiceUnavailable:    fileErrorResponseFor(http.StatusServiceUnavailable, uploadsecurity.CodeStorageUnavailable),
		http.StatusInternalServerError:   fileErrorResponseFor(http.StatusInternalServerError, uploadsecurity.CodePersistenceFailed, uploadsecurity.CodeInternalError),
	}
	return routecatalog.Descriptor{Method: method, Path: path, Access: routecatalog.PermissionControlled, Handler: fn, Name: name, Group: "file", DefaultPermissionCode: permission, DefaultAuditCategory: "file", OpenAPI: routecatalog.Operation{Summary: name, Description: description, Request: body, Responses: responses}}
}
func binaryRoute(path, name, permission string, fn gin.HandlerFunc, description string) routecatalog.Descriptor {
	d := fileRoute(http.MethodGet, path, name, permission, fn, nil, nil, description, false)
	d.OpenAPI.Responses = map[int]routecatalog.Response{
		http.StatusOK:                  routecatalog.Response{Description: "file attachment", Kind: routecatalog.BinaryBody, ContentTypes: []string{"application/octet-stream", "application/pdf", "text/plain", "text/csv"}},
		http.StatusBadRequest:          fileErrorResponseFor(http.StatusBadRequest, uploadsecurity.CodeRequestInvalid),
		http.StatusForbidden:           fileErrorResponseFor(http.StatusForbidden, uploadsecurity.CodeFileAccessInvalid),
		http.StatusNotFound:            fileErrorResponseFor(http.StatusNotFound, uploadsecurity.CodeFileNotFound, uploadsecurity.CodeStorageObjectNotFound),
		http.StatusConflict:            fileErrorResponseFor(http.StatusConflict, uploadsecurity.CodeFileStateBlocked, uploadsecurity.CodeFileStateConflict),
		http.StatusServiceUnavailable:  fileErrorResponseFor(http.StatusServiceUnavailable, uploadsecurity.CodeStorageUnavailable),
		http.StatusInternalServerError: fileErrorResponseFor(http.StatusInternalServerError, uploadsecurity.CodeInternalError),
	}
	return d
}
func previewRoute(fn gin.HandlerFunc) routecatalog.Descriptor {
	d := fileRoute(http.MethodGet, "/api/admin/files/:id/preview", "Preview File", "admin.files.id.preview.get", fn, nil, nil, "该兼容路由不提供管理员普通文件预览；当前策略始终返回 HTTP 409 和稳定文件状态冲突错误，不读取或返回对象内容。", false)
	d.OpenAPI.Responses = map[int]routecatalog.Response{
		http.StatusConflict: fileErrorResponseFor(http.StatusConflict, uploadsecurity.CodeFileStateConflict),
	}
	return d
}

func (h *handler) upload(c *gin.Context) {
	file, cleanup, err := parseUpload(c, h.maxUploadBytes)
	defer cleanup()
	if err != nil {
		setRejectedAudit(c, nil, err)
		writeError(c, err)
		return
	}
	reader, err := file.Open()
	if err != nil {
		classified := uploadsecurity.NewError(uploadsecurity.CodeUploadBodyInvalid, err)
		setRejectedAudit(c, file, classified)
		writeError(c, classified)
		return
	}
	defer reader.Close()
	if h.service == nil {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil))
		return
	}
	info, err := h.service.Upload(c.Request.Context(), application.UploadInput{UploaderID: c.GetUint("userID"), FileName: file.Filename, ContentType: file.Header.Get("Content-Type"), Size: file.Size, MaxBytes: h.maxUploadBytes, Reader: reader})
	if err != nil {
		setRejectedAudit(c, file, err)
		writeError(c, err)
		return
	}
	c.Set(application.UploadAuditMetadataContextKey, application.AuditMetadata{Purpose: string(uploadsecurity.PurposeManagedFile), FileName: info.Name, FileSize: info.Size, DeclaredMIME: file.Header.Get("Content-Type"), DetectedMIME: info.DetectedContentType, ValidationResult: application.UploadValidationAccepted, PolicyVersion: info.ValidationPolicyVersion})
	fileSuccess(c, info)
}
func (h *handler) list(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeRequestInvalid, err))
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("size", "10"))
	if err != nil || size < 1 {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeRequestInvalid, err))
		return
	}
	list, total, err := h.service.List(c.Request.Context(), c.GetUint("userID"), page, size, c.Query("prefix"))
	if err != nil {
		writeError(c, err)
		return
	}
	fileSuccess(c, application.FileListResponse{List: list, Total: total, Page: page, Size: size})
}
func (h *handler) detail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), c.GetUint("userID"), id)
	if err != nil {
		writeError(c, err)
		return
	}
	fileSuccess(c, result)
}
func (h *handler) update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req application.UpdateFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeRequestInvalid, err))
		return
	}
	result, err := h.service.Update(c.Request.Context(), c.GetUint("userID"), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	fileSuccess(c, result)
}
func (h *handler) delete(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), c.GetUint("userID"), id); err != nil {
		writeError(c, err)
		return
	}
	fileSuccess(c, nil)
}
func (h *handler) download(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	expires, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil || expires < 1 || c.Query("signature") == "" {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeRequestInvalid, err))
		return
	}
	h.content(c, application.FileAccessInput{UserID: c.GetUint("userID"), FileID: id, ExpiresAt: expires, Signature: c.Query("signature"), Mode: application.ModeDownload})
}
func (h *handler) preview(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	expires, _ := strconv.ParseInt(c.Query("expires"), 10, 64)
	h.content(c, application.FileAccessInput{UserID: c.GetUint("userID"), FileID: id, ExpiresAt: expires, Signature: c.Query("signature"), Mode: application.ModePreview})
}
func (h *handler) content(c *gin.Context, input application.FileAccessInput) {
	if h.service == nil {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil))
		return
	}
	content, err := h.service.Open(c.Request.Context(), input)
	if err != nil {
		writeError(c, err)
		return
	}
	if content == nil || content.Reader == nil {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil))
		return
	}
	defer content.Reader.Close()
	c.Header("Content-Type", content.ContentType)
	c.Header("Content-Disposition", mime.FormatMediaType(string(content.Disposition), map[string]string{"filename": content.FileName}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, content.Reader); err != nil {
		_ = c.Error(err)
	}
}
func (h *handler) revalidate(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.Revalidate(c.Request.Context(), c.GetUint("userID"), id, h.maxUploadBytes)
	if err != nil {
		writeError(c, err)
		return
	}
	fileSuccess(c, result)
}
func (h *handler) browse(c *gin.Context) {
	result, err := h.service.Browse(c.Request.Context(), c.GetUint("userID"), c.Query("prefix"))
	if err != nil {
		writeError(c, err)
		return
	}
	fileSuccess(c, result)
}
func pathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		writeError(c, uploadsecurity.NewError(uploadsecurity.CodeRequestInvalid, err))
		return 0, false
	}
	return uint(value), true
}
func writeError(c *gin.Context, err error) {
	writeFileError(c, err)
}
func parseUpload(c *gin.Context, max int64) (*multipart.FileHeader, func(), error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max+multipartOverheadBytes)
	if err := c.Request.ParseMultipartForm(multipartMemoryBytes); err != nil {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return nil, func() {}, uploadsecurity.NewError(uploadsecurity.CodeUploadBodyTooLarge, err)
		}
		return nil, func() {}, uploadsecurity.NewError(uploadsecurity.CodeUploadBodyInvalid, err)
	}
	form := c.Request.MultipartForm
	cleanup := func() {
		if form != nil {
			_ = form.RemoveAll()
		}
	}
	count := 0
	for _, headers := range form.File {
		count += len(headers)
	}
	if count == 0 {
		return nil, cleanup, uploadsecurity.NewError(uploadsecurity.CodeUploadFileMissing, nil)
	}
	headers := form.File["file"]
	if count != 1 || len(headers) != 1 {
		return nil, cleanup, uploadsecurity.NewError(uploadsecurity.CodeUploadMultipleFiles, nil)
	}
	if headers[0].Size == 0 {
		return nil, cleanup, uploadsecurity.NewError(uploadsecurity.CodeFileEmpty, nil)
	}
	if headers[0].Size > max {
		return nil, cleanup, uploadsecurity.NewError(uploadsecurity.CodeFileTooLarge, nil)
	}
	return headers[0], cleanup, nil
}
func setRejectedAudit(c *gin.Context, file *multipart.FileHeader, err error) {
	code, ok := uploadsecurity.CodeOf(err)
	if !ok {
		code = uploadsecurity.CodeInternalError
	}
	metadata := application.AuditMetadata{Purpose: string(uploadsecurity.PurposeManagedFile), ValidationResult: application.UploadValidationRejected, ReasonCode: string(code)}
	if file != nil {
		metadata.FileName = uploadsecurity.SanitizeAuditFileName(file.Filename, uploadsecurity.PurposeManagedFile)
		metadata.FileSize = file.Size
		metadata.DeclaredMIME = file.Header.Get("Content-Type")
	}
	c.Set(application.UploadAuditMetadataContextKey, metadata)
}
