package uploadsecurity

import "net/http"

// HTTPError is the safe client-facing representation of an upload failure.
type HTTPError struct {
	Status  int
	Code    Code
	Message string
}

var httpErrors = map[Code]HTTPError{
	CodeRequestInvalid:         {http.StatusBadRequest, CodeRequestInvalid, "请求参数不正确"},
	CodeAvatarFieldNotWritable: {http.StatusBadRequest, CodeAvatarFieldNotWritable, "头像只能通过专用接口修改"},
	CodeUploadBodyInvalid:      {http.StatusBadRequest, CodeUploadBodyInvalid, "上传请求格式不正确"},
	CodeUploadBodyTooLarge:     {http.StatusRequestEntityTooLarge, CodeUploadBodyTooLarge, "上传请求超过大小限制"},
	CodeUploadFileMissing:      {http.StatusBadRequest, CodeUploadFileMissing, "请选择文件"},
	CodeUploadMultipleFiles:    {http.StatusBadRequest, CodeUploadMultipleFiles, "每次只能上传一个文件"},
	CodeFileEmpty:              {http.StatusBadRequest, CodeFileEmpty, "文件不能为空"},
	CodeFileTooLarge:           {http.StatusRequestEntityTooLarge, CodeFileTooLarge, "文件超过大小限制"},
	CodeFileNameInvalid:        {http.StatusBadRequest, CodeFileNameInvalid, "文件名不合法"},
	CodeFileTypeNotAllowed:     {http.StatusUnsupportedMediaType, CodeFileTypeNotAllowed, "文件类型不受支持"},
	CodeFileTypeMismatch:       {http.StatusUnsupportedMediaType, CodeFileTypeMismatch, "文件类型不一致"},
	CodeFileEncodingInvalid:    {http.StatusUnprocessableEntity, CodeFileEncodingInvalid, "文件编码无效"},
	CodeFileContentInvalid:     {http.StatusUnprocessableEntity, CodeFileContentInvalid, "文件内容无效"},
	CodeImageDimensionLimit:    {http.StatusUnprocessableEntity, CodeImageDimensionLimit, "图片尺寸超过限制"},
	CodeImageDecodeInvalid:     {http.StatusUnprocessableEntity, CodeImageDecodeInvalid, "图片无法完整解码"},
	CodeFileNotFound:           {http.StatusNotFound, CodeFileNotFound, "文件不存在"},
	CodeFileAccessInvalid:      {http.StatusForbidden, CodeFileAccessInvalid, "文件访问链接无效或已过期"},
	CodeFileStateBlocked:       {http.StatusConflict, CodeFileStateBlocked, "文件已被安全策略封锁"},
	CodeFileStateConflict:      {http.StatusConflict, CodeFileStateConflict, "当前文件状态不允许此操作"},
	CodeStorageObjectNotFound:  {http.StatusNotFound, CodeStorageObjectNotFound, "文件对象不存在"},
	CodeStorageUnavailable:     {http.StatusServiceUnavailable, CodeStorageUnavailable, "文件存储服务暂时不可用"},
	CodePersistenceFailed:      {http.StatusInternalServerError, CodePersistenceFailed, "数据保存失败"},
}

var internalHTTPError = HTTPError{
	Status:  http.StatusInternalServerError,
	Code:    CodeInternalError,
	Message: "服务器内部错误",
}

// ToHTTPError maps classified failures to stable HTTP status, code and message
// values. Internal causes are deliberately excluded.
func ToHTTPError(err error) HTTPError {
	code, ok := CodeOf(err)
	if !ok {
		return internalHTTPError
	}
	httpError, ok := httpErrors[code]
	if !ok {
		return internalHTTPError
	}
	return httpError
}
