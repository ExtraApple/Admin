package apidoc

import (
	"context"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type Config struct {
	Title       string
	Version     string
	Description string
}

type Metadata struct {
	Method         string
	Path           string
	Name           string
	Group          string
	Remark         string
	Status         int
	NeedAuth       int
	NeedAudit      int
	PermissionCode string
}

type MetadataSource interface {
	Snapshot(context.Context) ([]Metadata, error)
}

type Service struct {
	catalog  *routecatalog.Catalog
	metadata MetadataSource
	config   Config
}

func New(catalog *routecatalog.Catalog, metadata MetadataSource, config Config) *Service {
	return &Service{catalog: catalog, metadata: metadata, config: config}
}

func (service *Service) Document(ctx context.Context, serverURL string) (map[string]any, error) {
	metadata, err := service.metadataSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	config := service.config
	if strings.TrimSpace(config.Title) == "" {
		config.Title = "Admin API"
	}
	if strings.TrimSpace(config.Version) == "" {
		config.Version = "1.0.0"
	}
	if strings.TrimSpace(config.Description) == "" {
		config.Description = "Admin backend OpenAPI document generated from Route Catalog and API Metadata."
	}
	if strings.TrimSpace(serverURL) == "" {
		serverURL = "http://localhost:8080"
	}

	paths := make(map[string]any)
	tags := make(map[string]struct{})
	if service.catalog != nil {
		for _, descriptor := range service.catalog.Snapshot() {
			if !exposePath(descriptor.Path) {
				continue
			}
			key := strings.ToUpper(strings.TrimSpace(descriptor.Method)) + " " + strings.TrimSpace(descriptor.Path)
			entry, found := metadata[key]
			openAPIPath, params := openAPIPath(descriptor.Path)
			tag := strings.TrimSpace(entry.Group)
			if tag == "" {
				tag = strings.TrimSpace(descriptor.Group)
			}
			if tag == "" {
				tag = "api"
			}
			tags[tag] = struct{}{}
			pathItem, _ := paths[openAPIPath].(map[string]any)
			if pathItem == nil {
				pathItem = map[string]any{}
				paths[openAPIPath] = pathItem
			}
			pathItem[strings.ToLower(descriptor.Method)] = buildOperation(descriptor, entry, found, params)
		}
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]any{"title": config.Title, "version": config.Version, "description": config.Description},
		"servers": []map[string]string{{"url": serverURL}},
		"tags":    sortedTags(tags),
		"paths":   paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{"BearerAuth": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}},
			"schemas":         map[string]any{"ErrorResponse": errorSchema()},
		},
	}, nil
}

func (service *Service) metadataSnapshot(ctx context.Context) (map[string]Metadata, error) {
	result := make(map[string]Metadata)
	if service.metadata == nil {
		return result, nil
	}
	entries, err := service.metadata.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		method, path := strings.ToUpper(strings.TrimSpace(entry.Method)), strings.TrimSpace(entry.Path)
		if method == "" || path == "" {
			continue
		}
		result[method+" "+path] = entry
	}
	return result, nil
}

func (service *Service) Index(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerHTML))
}

func (service *Service) OpenAPI(c *gin.Context) {
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		host = "localhost:8080"
	}
	document, err := service.Document(c.Request.Context(), scheme+"://"+host)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, document)
}

func exposePath(path string) bool { return path == "/ping" || strings.HasPrefix(path, "/api/") }

func buildOperation(descriptor routecatalog.Descriptor, entry Metadata, metadataFound bool, params []string) map[string]any {
	summary := strings.TrimSpace(entry.Name)
	if summary == "" {
		summary = descriptor.OpenAPI.Summary
	}
	if summary == "" {
		summary = descriptor.Name
	}
	operation := map[string]any{"tags": []string{firstNonEmpty(entry.Group, descriptor.Group, "api")}, "summary": summary, "operationId": operationID(descriptor.Method, descriptor.Path), "responses": buildResponses(descriptor.OpenAPI.Responses)}
	if description := strings.TrimSpace(descriptor.OpenAPI.Description); description != "" {
		operation["description"] = description
	}
	if remark := strings.TrimSpace(entry.Remark); remark != "" {
		if existing, ok := operation["description"].(string); ok && existing != "" {
			operation["description"] = existing + "\n\n" + remark
		} else {
			operation["description"] = remark
		}
	}
	if metadataFound && entry.Status != 1 {
		operation["deprecated"] = true
	}
	if len(params) > 0 {
		operation["parameters"] = pathParameters(params)
	}
	if request := buildRequestBody(descriptor.OpenAPI.Request); request != nil {
		operation["requestBody"] = request
	}
	if descriptor.Access == routecatalog.Authenticated || descriptor.Access == routecatalog.PermissionControlled {
		operation["security"] = []map[string][]string{{"BearerAuth": {}}}
	}
	return operation
}

func buildRequestBody(request routecatalog.RequestBody) map[string]any {
	switch request.Kind {
	case routecatalog.JSONBody:
		content := map[string]any{"application/json": map[string]any{"schema": schemaFor(request.Schema)}}
		return map[string]any{"required": request.Required, "content": content}
	case routecatalog.MultipartBody:
		field := request.FileField
		if field == "" {
			field = "file"
		}
		fileProperty := map[string]any{"type": "string", "format": "binary"}
		if strings.TrimSpace(request.FileDescription) != "" {
			fileProperty["description"] = request.FileDescription
		}
		properties := map[string]any{field: fileProperty}
		schema := map[string]any{"type": "object", "required": []string{field}, "properties": properties}
		if request.Schema != nil {
			schema = schemaFor(request.Schema)
		}
		return map[string]any{"required": request.Required, "content": map[string]any{"multipart/form-data": map[string]any{"schema": schema}}}
	default:
		return nil
	}
}

func buildResponses(responses map[int]routecatalog.Response) map[string]any {
	result := make(map[string]any, len(responses))
	codes := make([]int, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	for _, code := range codes {
		response := responses[code]
		item := map[string]any{"description": firstNonEmpty(response.Description, http.StatusText(code), "response")}
		if response.Kind != routecatalog.NoBody {
			contentTypes := append([]string(nil), response.ContentTypes...)
			if len(contentTypes) == 0 {
				contentTypes = []string{"application/json"}
			}
			content := make(map[string]any, len(contentTypes))
			for _, contentType := range contentTypes {
				content[contentType] = map[string]any{"schema": schemaFor(response.Schema)}
			}
			item["content"] = content
		}
		result[strconv.Itoa(code)] = item
	}
	return result
}

func schemaFor(value reflect.Type) map[string]any {
	return schemaForStack(value, make(map[reflect.Type]bool))
}

func schemaForStack(value reflect.Type, stack map[reflect.Type]bool) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if value.Kind() == reflect.Pointer {
		schema := schemaForStack(value.Elem(), stack)
		schema["nullable"] = true
		return schema
	}
	if value == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	switch value.Kind() {
	case reflect.Struct:
		if stack[value] {
			return map[string]any{"type": "object"}
		}
		stack[value] = true
		schema := objectSchema(value, stack)
		delete(stack, value)
		return schema
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaForStack(value.Elem(), stack)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaForStack(value.Elem(), stack)}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer", "format": "int64", "minimum": 0}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	default:
		return map[string]any{"type": "string"}
	}
}

func objectSchema(value reflect.Type, stack map[reflect.Type]bool) map[string]any {
	properties := map[string]any{}
	required := []string{}
	for index := range value.NumField() {
		field := value.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, omit := jsonName(field)
		if omit || name == "" {
			continue
		}
		properties[name] = schemaForStack(field.Type, stack)
		if strings.Contains(field.Tag.Get("binding"), "required") {
			required = append(required, name)
		}
	}
	result := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

func jsonName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	if tag == "" {
		return field.Name, false
	}
	name := strings.Split(tag, ",")[0]
	return name, false
}
func openAPIPath(path string) (string, []string) {
	parts := strings.Split(path, "/")
	params := []string{}
	for index, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			name := strings.TrimLeft(part, ":*")
			if name != "" {
				parts[index] = "{" + name + "}"
				params = append(params, name)
			}
		}
	}
	return strings.Join(parts, "/"), params
}
func pathParameters(params []string) []map[string]any {
	result := make([]map[string]any, len(params))
	for index, name := range params {
		result[index] = map[string]any{"name": name, "in": "path", "required": true, "schema": map[string]any{"type": "string"}}
	}
	return result
}
func operationID(method, path string) string {
	value := strings.ToLower(method + "_" + strings.Trim(path, "/"))
	var builder strings.Builder
	underscore := false
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
			underscore = false
		} else if !underscore {
			builder.WriteByte('_')
			underscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}
func sortedTags(tags map[string]struct{}) []map[string]any {
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]map[string]any, len(names))
	for index, name := range names {
		result[index] = map[string]any{"name": name}
	}
	return result
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func errorSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"code": map[string]any{"type": "integer"}, "msg": map[string]any{"type": "string"}, "error_code": map[string]any{"type": "string"}}}
}

const swaggerHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Admin API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; background: #f8fafc; } #swagger-ui .topbar { display: none; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>window.ui = SwaggerUIBundle({url: "/docs/openapi.json", dom_id: "#swagger-ui", deepLinking: true, persistAuthorization: true});</script>
</body>
</html>`
