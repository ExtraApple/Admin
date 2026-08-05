package routecatalog

import (
	"fmt"
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

type Response struct {
	Description  string
	Kind         BodyKind
	Schema       reflect.Type
	ContentTypes []string
}

type Operation struct {
	Summary     string
	Description string
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

type Catalog struct {
	descriptors []Descriptor
}

func New(descriptors []Descriptor) (*Catalog, error) {
	normalized := make([]Descriptor, len(descriptors))
	seen := make(map[string]int, len(descriptors))
	for index, descriptor := range descriptors {
		descriptor.Method = strings.ToUpper(strings.TrimSpace(descriptor.Method))
		descriptor.Path = normalizePath(descriptor.Path)
		if err := validateDescriptor(descriptor); err != nil {
			return nil, fmt.Errorf("descriptor %d: %w", index, err)
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
		switch response.Kind {
		case NoBody:
		case JSONBody:
			if response.Schema == nil {
				return fmt.Errorf("response schema is required for status %d", status)
			}
		case BinaryBody:
			if len(response.ContentTypes) == 0 {
				return fmt.Errorf("response content types are required for status %d", status)
			}
		default:
			return fmt.Errorf("response kind %d is unknown", response.Kind)
		}
	}
	return nil
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
		responses[status] = response
	}
	descriptor.OpenAPI.Responses = responses
	return descriptor
}
