package httpadapter

import (
	"net/http"

	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"admin/internal/uploadsecurity"
	"github.com/gin-gonic/gin"
)

func fileErrorDefinition(code uploadsecurity.Code, cause error) httpresponse.ErrorDefinition {
	mapped := uploadsecurity.ToHTTPError(uploadsecurity.NewError(code, cause))
	return httpresponse.ErrorDefinition{Owner: "files", Code: string(code), Status: mapped.Status, Message: fileEnglishMessage(code)}
}

func fileEnglishMessage(code uploadsecurity.Code) string {
	switch code {
	case uploadsecurity.CodeRequestInvalid:
		return "request is invalid"
	case uploadsecurity.CodeAvatarFieldNotWritable:
		return "avatar must be changed through the avatar endpoint"
	case uploadsecurity.CodeUploadBodyInvalid:
		return "upload body is invalid"
	case uploadsecurity.CodeUploadBodyTooLarge, uploadsecurity.CodeFileTooLarge:
		return "upload exceeds the size limit"
	case uploadsecurity.CodeUploadFileMissing:
		return "upload file is required"
	case uploadsecurity.CodeUploadMultipleFiles:
		return "only one upload file is allowed"
	case uploadsecurity.CodeFileEmpty:
		return "file is empty"
	case uploadsecurity.CodeFileNameInvalid:
		return "file name is invalid"
	case uploadsecurity.CodeFileTypeNotAllowed:
		return "file type is not allowed"
	case uploadsecurity.CodeFileTypeMismatch:
		return "file type does not match its content"
	case uploadsecurity.CodeFileEncodingInvalid:
		return "file encoding is invalid"
	case uploadsecurity.CodeFileContentInvalid:
		return "file content is invalid"
	case uploadsecurity.CodeImageDimensionLimit:
		return "image dimensions exceed the limit"
	case uploadsecurity.CodeImageDecodeInvalid:
		return "image cannot be decoded"
	case uploadsecurity.CodeFileNotFound:
		return "file was not found"
	case uploadsecurity.CodeFileAccessInvalid:
		return "file access is invalid or expired"
	case uploadsecurity.CodeFileStateBlocked:
		return "file is blocked by security policy"
	case uploadsecurity.CodeFileStateConflict:
		return "file state does not allow this operation"
	case uploadsecurity.CodeStorageObjectNotFound:
		return "storage object was not found"
	case uploadsecurity.CodeStorageUnavailable:
		return "file storage is unavailable"
	case uploadsecurity.CodePersistenceFailed:
		return "file persistence failed"
	default:
		return "file operation failed"
	}
}

func fileErrorResponseFor(status int, codes ...uploadsecurity.Code) routecatalog.Response {
	definitions := make([]httpresponse.ErrorDefinition, 0, len(codes))
	for _, code := range codes {
		definition := fileErrorDefinition(code, nil)
		if definition.Status == status {
			definitions = append(definitions, definition)
		}
	}
	return routecatalog.ErrorResponse("file operation failed", definitions...)
}

func fileErrorResponse(code uploadsecurity.Code) routecatalog.Response {
	definition := fileErrorDefinition(code, nil)
	return routecatalog.ErrorResponse(definition.Message, definition)
}

func writeFileError(c *gin.Context, err error) {
	code, ok := uploadsecurity.CodeOf(err)
	if !ok {
		code = uploadsecurity.CodeInternalError
	}
	definition := fileErrorDefinition(code, err)
	httpresponse.WriteError(c, definition, err, nil)
}

func fileSuccess(c *gin.Context, data any) { httpresponse.WriteSuccess(c, http.StatusOK, data) }
