package httpresponse

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// Envelope is the only JSON shape used by business success and error responses.
type Envelope[T any] struct {
	Code      int    `json:"code"`
	ErrorCode string `json:"error_code"`
	Msg       string `json:"msg"`
	Data      T      `json:"data"`
}

// NoDataValue makes the absence of a business payload explicit as JSON null.
func NoDataValue() any { return nil }

type FieldError struct {
	Field     string `json:"field"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

type ValidationErrorData struct {
	Fields []FieldError `json:"fields"`
}

type FieldErrorDefinition struct {
	Field   string
	Code    string
	Message string
}

type ErrorDefinition struct {
	Owner      string
	Code       string
	Status     int
	Message    string
	DataSchema reflect.Type
	Fields     []FieldErrorDefinition
}

type ErrorContext struct {
	Definition ErrorDefinition
	Cause      error
}

const errorContextKey = "httpresponse.error_context"

// UploadAuditMetadataKey is the shared Gin context key carrying upload audit
// metadata from the writing module to the audit module. It lives here so both
// sides reference one literal instead of keeping independent copies.
const UploadAuditMetadataKey = "upload_audit_metadata"

var errorCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

var internalErrorDefinition = ErrorDefinition{
	Owner:   "http",
	Code:    "HTTP_INTERNAL_ERROR",
	Status:  500,
	Message: "internal server error",
}

func InternalErrorDefinition() ErrorDefinition {
	return internalErrorDefinition
}

func (definition ErrorDefinition) Validate() error {
	if strings.TrimSpace(definition.Owner) == "" {
		return errors.New("error definition owner is required")
	}
	if !errorCodePattern.MatchString(definition.Code) {
		return fmt.Errorf("error definition code %q is invalid", definition.Code)
	}
	if definition.Status < 400 || definition.Status > 599 {
		return fmt.Errorf("error definition status %d is invalid", definition.Status)
	}
	if !safeEnglish(definition.Message) {
		return errors.New("error definition message must be safe English")
	}
	seen := make(map[string]struct{}, len(definition.Fields))
	for _, field := range definition.Fields {
		if strings.TrimSpace(field.Field) == "" {
			return errors.New("field error field is required")
		}
		if !errorCodePattern.MatchString(field.Code) {
			return fmt.Errorf("field error code %q is invalid", field.Code)
		}
		if !safeEnglish(field.Message) {
			return errors.New("field error message must be safe English")
		}
		key := field.Field + "\x00" + field.Code
		if _, exists := seen[key]; exists {
			return fmt.Errorf("field error %q is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (definition ErrorDefinition) ValidateData(data any) error {
	if data == nil {
		return nil
	}
	if definition.DataSchema == nil {
		return errors.New("error data is not allowed")
	}
	valueType := reflect.TypeOf(data)
	if !valueType.AssignableTo(definition.DataSchema) {
		return fmt.Errorf("error data type %s is not assignable to %s", valueType, definition.DataSchema)
	}
	if validation, ok := data.(ValidationErrorData); ok {
		allowed := make(map[string]struct{}, len(definition.Fields))
		for _, field := range definition.Fields {
			allowed[field.Field+"\x00"+field.Code] = struct{}{}
		}
		for _, field := range validation.Fields {
			if _, exists := allowed[field.Field+"\x00"+field.ErrorCode]; !exists {
				return fmt.Errorf("field error %q is not declared", field.Field+"\x00"+field.ErrorCode)
			}
			if !safeEnglish(field.Message) {
				return fmt.Errorf("field error %q message is not safe English", field.ErrorCode)
			}
		}
	}
	return nil
}

func WriteSuccess(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope[any]{Code: status, ErrorCode: "", Msg: "success", Data: normalizeData(data)})
}

func WriteError(c *gin.Context, definition ErrorDefinition, cause error, data any) {
	if err := definition.Validate(); err != nil {
		definition = internalErrorDefinition
		if cause == nil {
			cause = err
		}
	}
	if definition.ValidateData(data) != nil {
		data = nil
	} else {
		data = normalizeErrorData(data)
	}
	SetErrorContext(c, definition, cause)
	c.JSON(definition.Status, Envelope[any]{Code: definition.Status, ErrorCode: definition.Code, Msg: definition.Message, Data: data})
}

func normalizeErrorData(data any) any {
	validation, ok := data.(ValidationErrorData)
	if !ok {
		return data
	}
	validation.Fields = append([]FieldError(nil), validation.Fields...)
	sort.SliceStable(validation.Fields, func(left, right int) bool {
		if validation.Fields[left].Field == validation.Fields[right].Field {
			return validation.Fields[left].ErrorCode < validation.Fields[right].ErrorCode
		}
		return validation.Fields[left].Field < validation.Fields[right].Field
	})
	return validation
}

func SetErrorContext(c *gin.Context, definition ErrorDefinition, cause error) {
	c.Set(errorContextKey, ErrorContext{Definition: definition, Cause: cause})
}

func ErrorContextOf(c *gin.Context) (ErrorContext, bool) {
	value, exists := c.Get(errorContextKey)
	if !exists {
		return ErrorContext{}, false
	}
	context, ok := value.(ErrorContext)
	return context, ok
}

func normalizeData(data any) any {
	if data == nil {
		return nil
	}
	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Slice && value.IsNil() {
		return reflect.MakeSlice(value.Type(), 0, 0).Interface()
	}
	return data
}

func safeEnglish(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}

func RequestInvalidDefinition() ErrorDefinition {
	return ErrorDefinition{Owner: "http", Code: "HTTP_REQUEST_INVALID", Status: 400, Message: "request is invalid"}
}

func NotFoundDefinition() ErrorDefinition {
	return ErrorDefinition{Owner: "http", Code: "HTTP_NOT_FOUND", Status: 404, Message: "resource not found"}
}

func MethodNotAllowedDefinition() ErrorDefinition {
	return ErrorDefinition{Owner: "http", Code: "HTTP_METHOD_NOT_ALLOWED", Status: 405, Message: "method not allowed"}
}
