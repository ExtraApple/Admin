package routecatalog

import (
	"admin/internal/platform/httpresponse"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
)

type AccessLevel uint8

const (
	Public AccessLevel = iota + 1
	Authenticated
	PermissionControlled
)

type BodyKind uint8

const (
	NoBody BodyKind = iota + 1
	JSONBody
	MultipartBody
	BinaryBody
)

type RequestBody struct {
	Kind            BodyKind
	Schema          reflect.Type
	Required        bool
	FileField       string
	FileDescription string
}

// Header describes a string-valued HTTP header. Value, when set, is the exact
// permitted value rather than an example containing request credentials.
type Header struct {
	Name        string
	Description string
	Required    bool
	Value       string
}

// CookieSecurity describes the browser session credential, including the
// production attributes used when setting it. Required is operation-specific;
// optional credentials allow login replacement and idempotent logout.
type CookieSecurity struct {
	Name        string
	Description string
	Path        string
	SameSite    string
	Required    bool
	HTTPOnly    bool
	Secure      bool
	HostOnly    bool
	Session     bool
}

type Response struct {
	Description  string
	Kind         BodyKind
	Schema       reflect.Type
	DataSchema   reflect.Type
	ContentTypes []string
	Errors       []httpresponse.ErrorDefinition
	Headers      []Header
}

type Operation struct {
	Summary     string
	Description string
	Protocol    string
	QuerySchema reflect.Type
	Cookie      *CookieSecurity
	Headers     []Header
	Request     RequestBody
	Responses   map[int]Response
}

type Descriptor struct {
	Method                string
	Path                  string
	Access                AccessLevel
	Handler               gin.HandlerFunc
	Name                  string
	Group                 string
	DefaultPermissionCode string
	DefaultAuditCategory  string
	OpenAPI               Operation
}

func JSONResponse(description string, dataSchema reflect.Type) Response {
	return Response{Description: description, Kind: JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{}), DataSchema: dataSchema}
}

func ErrorResponse(description string, definitions ...httpresponse.ErrorDefinition) Response {
	return Response{Description: description, Kind: JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{}), Errors: append([]httpresponse.ErrorDefinition(nil), definitions...)}
}

func DataSchemaOf(response any) reflect.Type {
	value := reflect.TypeOf(response)
	if value == nil {
		return nil
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return value
	}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if strings.Split(field.Tag.Get("json"), ",")[0] == "data" {
			return field.Type
		}
	}
	return nil

}

type Catalog struct {
	descriptors []Descriptor
}

func New(descriptors []Descriptor) (*Catalog, error) {
	normalized := make([]Descriptor, len(descriptors))
	seen := make(map[string]int, len(descriptors))
	definitions := make(map[string]httpresponse.ErrorDefinition)
	for index, descriptor := range descriptors {
		descriptor.Method = strings.ToUpper(strings.TrimSpace(descriptor.Method))
		descriptor.Path = normalizePath(descriptor.Path)
		if err := validateDescriptor(descriptor); err != nil {
			return nil, fmt.Errorf("descriptor %d: %w", index, err)
		}
		for _, response := range descriptor.OpenAPI.Responses {
			for _, definition := range response.Errors {
				if existing, ok := definitions[definition.Code]; ok && !sameErrorDefinition(existing, definition) {
					return nil, fmt.Errorf("error code %q ownership or definition conflict", definition.Code)
				}
				definitions[definition.Code] = definition
			}
		}
		key := descriptor.Method + " " + descriptor.Path
		if previous, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate route %s in descriptors %d and %d", key, previous, index)
		}
		seen[key] = index
		normalized[index] = cloneDescriptor(descriptor)
	}
	return &Catalog{descriptors: normalized}, nil
}

func (catalog *Catalog) Snapshot() []Descriptor {
	result := make([]Descriptor, len(catalog.descriptors))
	for index, descriptor := range catalog.descriptors {
		result[index] = cloneDescriptor(descriptor)
	}
	return result
}

func normalizePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func validateDescriptor(descriptor Descriptor) error {
	supportedMethods := map[string]struct{}{
		"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {}, "OPTIONS": {}, "HEAD": {},
	}
	if _, ok := supportedMethods[descriptor.Method]; !ok {
		return fmt.Errorf("method %q is not supported", descriptor.Method)
	}
	if descriptor.Path == "" {
		return fmt.Errorf("path is required")
	}
	if descriptor.Handler == nil {
		return fmt.Errorf("handler is required")
	}
	if descriptor.Access != Public && descriptor.Access != Authenticated && descriptor.Access != PermissionControlled {
		return fmt.Errorf("access level %d is unknown", descriptor.Access)
	}
	if descriptor.Access != PermissionControlled && strings.TrimSpace(descriptor.DefaultPermissionCode) != "" {
		return fmt.Errorf("permission code is only valid for PermissionControlled routes")
	}
	if strings.TrimSpace(descriptor.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(descriptor.Group) == "" {
		return fmt.Errorf("group is required")
	}
	if strings.TrimSpace(descriptor.DefaultAuditCategory) == "" {
		return fmt.Errorf("audit category is required")
	}
	if strings.TrimSpace(descriptor.OpenAPI.Summary) == "" {
		return fmt.Errorf("OpenAPI summary is required")
	}
	if err := validateRequest(descriptor.OpenAPI.Request); err != nil {
		return err
	}
	if err := validateHeaders(descriptor.OpenAPI.Headers, true); err != nil {
		return fmt.Errorf("request headers: %w", err)
	}
	if err := validateCookie(descriptor); err != nil {
		return err
	}
	if err := validateProtocol(descriptor); err != nil {
		return err
	}
	if len(descriptor.OpenAPI.Responses) == 0 {
		return fmt.Errorf("OpenAPI responses are required")
	}

	for status, response := range descriptor.OpenAPI.Responses {
		if status < 100 || status > 599 {
			return fmt.Errorf("response status %d is invalid", status)
		}
		if strings.TrimSpace(response.Description) == "" {
			return fmt.Errorf("response %d description is required", status)
		}
		if err := validateHeaders(response.Headers, false); err != nil {
			return fmt.Errorf("response %d headers: %w", status, err)
		}
		switch response.Kind {
		case NoBody:
			if len(response.Errors) > 0 {
				return fmt.Errorf("response %d errors require a JSON body", status)
			}
		case JSONBody:
			if response.Schema == nil {
				return fmt.Errorf("response schema is required for status %d", status)
			}
			if status >= 400 {
				if !isEnvelopeSchema(response.Schema) {
					return fmt.Errorf("response %d error schema must be a four-field envelope", status)
				}
				if len(response.Errors) == 0 {
					return fmt.Errorf("response %d error definitions are required", status)
				}
				for _, definition := range response.Errors {
					if err := validateResponseError(status, definition); err != nil {
						return err
					}
				}
			} else if len(response.Errors) > 0 {
				return fmt.Errorf("response %d errors are only valid for failure responses", status)
			}
		case BinaryBody:
			if len(response.ContentTypes) == 0 {
				return fmt.Errorf("response content types are required for status %d", status)
			}
			if len(response.Errors) > 0 {
				return fmt.Errorf("response %d errors require a JSON body", status)
			}
		default:
			return fmt.Errorf("response kind %d is unknown", response.Kind)
		}
	}
	return nil
}

func validateProtocol(descriptor Descriptor) error {
	switch strings.ToLower(strings.TrimSpace(descriptor.OpenAPI.Protocol)) {
	case "":
		return nil
	case "websocket":
		if descriptor.Method != http.MethodGet {
			return fmt.Errorf("WebSocket protocol requires GET method")
		}
		if response, ok := descriptor.OpenAPI.Responses[http.StatusSwitchingProtocols]; !ok || response.Kind != NoBody {
			return fmt.Errorf("WebSocket protocol requires a no-body 101 response")
		}
		return nil
	default:
		return fmt.Errorf("protocol %q is unknown", descriptor.OpenAPI.Protocol)
	}
}

func validateRequest(request RequestBody) error {
	switch request.Kind {
	case NoBody:
		return nil
	case JSONBody:
		if request.Schema == nil {
			return fmt.Errorf("request schema is required")
		}
		return nil
	case MultipartBody:
		if strings.TrimSpace(request.FileField) == "" {
			return fmt.Errorf("file field is required for multipart request")
		}
		if strings.TrimSpace(request.FileDescription) == "" {
			return fmt.Errorf("file description is required for multipart request")
		}
		return nil
	default:
		return fmt.Errorf("request kind %d is unknown", request.Kind)
	}
}

func cloneDescriptor(descriptor Descriptor) Descriptor {
	responses := make(map[int]Response, len(descriptor.OpenAPI.Responses))
	for status, response := range descriptor.OpenAPI.Responses {
		response.ContentTypes = append([]string(nil), response.ContentTypes...)
		response.Headers = append([]Header(nil), response.Headers...)
		response.Errors = cloneErrorDefinitions(response.Errors)
		responses[status] = response
	}
	descriptor.OpenAPI.Responses = responses
	descriptor.OpenAPI.Headers = append([]Header(nil), descriptor.OpenAPI.Headers...)
	if descriptor.OpenAPI.Cookie != nil {
		cookie := *descriptor.OpenAPI.Cookie
		descriptor.OpenAPI.Cookie = &cookie
	}
	return descriptor
}

func cloneErrorDefinitions(definitions []httpresponse.ErrorDefinition) []httpresponse.ErrorDefinition {
	if definitions == nil {
		return nil
	}
	result := make([]httpresponse.ErrorDefinition, len(definitions))
	for index := range definitions {
		result[index] = definitions[index]
		result[index].Fields = append([]httpresponse.FieldErrorDefinition(nil), definitions[index].Fields...)
	}
	return result
}

func validateResponseError(status int, definition httpresponse.ErrorDefinition) error {
	if err := definition.Validate(); err != nil {
		return fmt.Errorf("error definition %q: %w", definition.Code, err)
	}
	if definition.Status != status {
		return fmt.Errorf("error definition %q status %d does not match response status %d", definition.Code, definition.Status, status)
	}
	return nil
}

func isEnvelopeSchema(value reflect.Type) bool {
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct || value.NumField() != 4 {
		return false
	}
	fields := make(map[string]struct{}, value.NumField())
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			return false
		}
		fields[name] = struct{}{}
	}
	for _, name := range []string{"code", "error_code", "msg", "data"} {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func sameErrorDefinition(left, right httpresponse.ErrorDefinition) bool {
	if left.Owner != right.Owner || left.Code != right.Code || left.Status != right.Status || left.Message != right.Message || left.DataSchema != right.DataSchema || len(left.Fields) != len(right.Fields) {
		return false
	}
	for index := range left.Fields {
		if left.Fields[index] != right.Fields[index] {
			return false
		}
	}
	return true
}

func validHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			continue
		}
		return false
	}
	return true
}

func validateHeaders(headers []Header, request bool) error {
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		name := strings.ToLower(header.Name)
		if !validHTTPToken(header.Name) {
			return fmt.Errorf("header name is invalid")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("header %q is duplicated", header.Name)
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(header.Description) == "" {
			return fmt.Errorf("header %q description is required", header.Name)
		}
		if strings.ContainsAny(header.Value, "\r\n") {
			return fmt.Errorf("header %q value is invalid", header.Name)
		}
		if request && (name == "authorization" || name == "cookie" || name == "content-type") {
			return fmt.Errorf("request header %q must use the security or request body contract", header.Name)
		}
		if !request && name == "set-cookie" && header.Value != "" {
			return fmt.Errorf("Set-Cookie must describe attributes without a credential value")
		}
	}
	return nil
}

func findHeader(headers []Header, name string) (Header, bool) {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return header, true
		}
	}
	return Header{}, false
}

func validateCookie(descriptor Descriptor) error {
	operation := descriptor.OpenAPI
	cookie := operation.Cookie
	if cookie == nil {
		for _, response := range operation.Responses {
			if _, found := findHeader(response.Headers, "Set-Cookie"); found {
				return fmt.Errorf("Set-Cookie requires a Cookie security description")
			}
		}
		return nil
	}
	if !validHTTPToken(cookie.Name) || cookie.Name == "BearerAuth" {
		return fmt.Errorf("Cookie security name is invalid")
	}
	if strings.TrimSpace(cookie.Description) == "" {
		return fmt.Errorf("Cookie security description is required")
	}
	if cookie.Path != "/api" || cookie.SameSite != "Strict" || !cookie.HTTPOnly || !cookie.Secure || !cookie.HostOnly || !cookie.Session {
		return fmt.Errorf("Cookie security requires Path=/api, SameSite=Strict, HttpOnly, HTTPS Secure, Host-only and session lifetime")
	}
	if descriptor.Method != http.MethodPost || operation.Request.Kind != JSONBody || !operation.Request.Required {
		return fmt.Errorf("Cookie control requires POST and a required JSON request")
	}
	origin, hasOrigin := findHeader(operation.Headers, "Origin")
	admin, hasAdmin := findHeader(operation.Headers, "X-Admin-Request")
	if !hasOrigin || !origin.Required || !hasAdmin || !admin.Required || admin.Value != "1" {
		return fmt.Errorf("Cookie control requires Origin and X-Admin-Request: 1 headers")
	}
	for status, response := range operation.Responses {
		cache, found := findHeader(response.Headers, "Cache-Control")
		if !found || !cache.Required || cache.Value != "no-store" {
			return fmt.Errorf("Cookie control response %d requires Cache-Control: no-store", status)
		}
		if status >= 200 && status < 300 {
			setCookie, found := findHeader(response.Headers, "Set-Cookie")
			if !found || !setCookie.Required {
				return fmt.Errorf("Cookie control response %d requires Set-Cookie", status)
			}
		}
	}
	return nil
}
