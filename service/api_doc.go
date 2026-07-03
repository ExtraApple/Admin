package service

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"admin/dto"
	"admin/global"
	"admin/model"
)

func BuildOpenAPIDocument(routes []dto.OpenAPIRoute, cfg dto.OpenAPIDocConfig) map[string]any {
	applyOpenAPIDefaults(&cfg)

	metadata := loadAPIDocMetadata()
	paths := map[string]any{}
	tagSet := map[string]struct{}{}

	for _, route := range routes {
		method := strings.ToUpper(strings.TrimSpace(route.Method))
		path := strings.TrimSpace(route.Path)
		if !shouldExposeOpenAPIRoute(path) {
			continue
		}

		openAPIPath, params := ginPathToOpenAPIPath(path)
		api := metadata[method+" "+path]
		tag := resolveOpenAPITag(path, api)
		tagSet[tag] = struct{}{}

		pathItem, _ := paths[openAPIPath].(map[string]any)
		if pathItem == nil {
			pathItem = map[string]any{}
			paths[openAPIPath] = pathItem
		}

		pathItem[strings.ToLower(method)] = buildOpenAPIOperation(method, path, tag, params, api)
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       cfg.Title,
			"version":     cfg.Version,
			"description": cfg.Description,
		},
		"servers": []map[string]string{
			{"url": cfg.ServerURL},
		},
		"tags":       buildOpenAPITags(tagSet),
		"paths":      paths,
		"components": buildOpenAPIComponents(),
	}
}

func applyOpenAPIDefaults(cfg *dto.OpenAPIDocConfig) {
	if strings.TrimSpace(cfg.Title) == "" {
		cfg.Title = "Admin API"
	}
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = "1.0.0"
	}
	if strings.TrimSpace(cfg.Description) == "" {
		cfg.Description = "Admin backend OpenAPI document generated from Gin routes and API metadata."
	}
	if strings.TrimSpace(cfg.ServerURL) == "" {
		cfg.ServerURL = "http://localhost:8080"
	}
}

func loadAPIDocMetadata() map[string]model.API {
	result := map[string]model.API{}
	if global.DB == nil {
		return result
	}

	var apis []model.API
	if err := global.DB.Find(&apis).Error; err != nil {
		return result
	}
	for _, api := range apis {
		method := strings.ToUpper(strings.TrimSpace(api.Method))
		path := strings.TrimSpace(api.Path)
		if method == "" || path == "" {
			continue
		}
		result[method+" "+path] = api
	}
	return result
}

func shouldExposeOpenAPIRoute(path string) bool {
	if path == "/ping" {
		return true
	}
	if strings.HasPrefix(path, "/docs") {
		return false
	}
	return strings.HasPrefix(path, "/api/")
}

func ginPathToOpenAPIPath(path string) (string, []string) {
	parts := strings.Split(path, "/")
	params := []string{}
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			name := strings.TrimLeft(part, ":*")
			if name == "" {
				continue
			}
			parts[i] = "{" + name + "}"
			params = append(params, name)
		}
	}
	return strings.Join(parts, "/"), params
}

func resolveOpenAPITag(path string, api model.API) string {
	if strings.TrimSpace(api.Group) != "" {
		return api.Group
	}
	if path == "/ping" {
		return "system"
	}
	return inferAPIGroup(path)
}

func buildOpenAPIOperation(method, path, tag string, params []string, api model.API) map[string]any {
	operation := map[string]any{
		"tags":        []string{tag},
		"summary":     resolveOpenAPISummary(method, path, api),
		"operationId": buildOpenAPIOperationID(method, path),
		"responses":   buildOpenAPIResponses(),
	}

	description := strings.TrimSpace(api.Remark)
	if description != "" {
		operation["description"] = description
	}
	if api.ID > 0 && api.Status != 1 {
		operation["deprecated"] = true
	}
	if len(params) > 0 {
		operation["parameters"] = buildOpenAPIPathParams(params)
	}
	if requestBody := buildOpenAPIRequestBody(method, path); requestBody != nil {
		operation["requestBody"] = requestBody
	}
	if shouldAttachOpenAPISecurity(path, api) {
		operation["security"] = []map[string][]string{
			{"BearerAuth": []string{}},
		}
	}

	return operation
}

func resolveOpenAPISummary(method, path string, api model.API) string {
	if strings.TrimSpace(api.Name) != "" {
		return api.Name
	}
	return method + " " + path
}

func buildOpenAPIOperationID(method, path string) string {
	value := strings.ToLower(method + "_" + strings.Trim(path, "/"))
	var b strings.Builder
	previousUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			previousUnderscore = false
			continue
		}
		if !previousUnderscore {
			b.WriteByte('_')
			previousUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func buildOpenAPIPathParams(params []string) []map[string]any {
	result := make([]map[string]any, 0, len(params))
	for _, param := range params {
		result = append(result, map[string]any{
			"name":        param,
			"in":          "path",
			"required":    true,
			"description": "Path parameter: " + param,
			"schema": map[string]any{
				"type": "string",
			},
		})
	}
	return result
}

func buildOpenAPIRequestBody(method, path string) map[string]any {
	method = strings.ToUpper(method)
	if _, ok := openAPIFileUploadRoutes[method+" "+path]; ok {
		return map[string]any{
			"required": true,
			"content": map[string]any{
				"multipart/form-data": map[string]any{
					"schema": map[string]any{
						"type":     "object",
						"required": []string{"file"},
						"properties": map[string]any{
							"file": map[string]any{
								"type":   "string",
								"format": "binary",
							},
						},
					},
				},
			},
		}
	}

	reqType, ok := openAPIRequestSchemas[method+" "+path]
	if !ok {
		return nil
	}

	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": buildSchemaFromType(reqType),
			},
		},
	}
}

var openAPIFileUploadRoutes = map[string]struct{}{
	"POST /api/user/avatar": {},
	"POST /api/admin/files": {},
}

var openAPIRequestSchemas = map[string]reflect.Type{
	"POST /api/register":                      reflect.TypeOf(dto.RegisterReq{}),
	"POST /api/login":                         reflect.TypeOf(dto.LoginReq{}),
	"PUT /api/user/info":                      reflect.TypeOf(dto.UpdateSelfReq{}),
	"PUT /api/user/password":                  reflect.TypeOf(dto.ChangePasswordReq{}),
	"PUT /api/admin/users/:id":                reflect.TypeOf(dto.AdminUpdateUserReq{}),
	"POST /api/admin/roles":                   reflect.TypeOf(dto.CreateRoleReq{}),
	"PUT /api/admin/roles/:id":                reflect.TypeOf(dto.UpdateRoleReq{}),
	"POST /api/admin/roles/:id/users":         reflect.TypeOf(dto.AssignUsersToRoleReq{}),
	"POST /api/admin/roles/:id/data-scope":    reflect.TypeOf(dto.AssignRoleDataScopeReq{}),
	"POST /api/admin/permissions":             reflect.TypeOf(dto.CreatePermissionReq{}),
	"PUT /api/admin/permissions/:id":          reflect.TypeOf(dto.UpdatePermissionReq{}),
	"POST /api/admin/roles/:id/permissions":   reflect.TypeOf(dto.AssignPermsToRoleReq{}),
	"POST /api/admin/permission-groups":       reflect.TypeOf(dto.CreatePermGroupReq{}),
	"PUT /api/admin/permission-groups/:id":    reflect.TypeOf(dto.UpdatePermGroupReq{}),
	"PUT /api/admin/files/:id":                reflect.TypeOf(dto.UpdateFileReq{}),
	"POST /api/admin/menus":                   reflect.TypeOf(dto.CreateMenuReq{}),
	"PUT /api/admin/menus/:id":                reflect.TypeOf(dto.UpdateMenuReq{}),
	"POST /api/admin/menus/sync":              reflect.TypeOf(dto.SyncMenusReq{}),
	"POST /api/admin/menus/:id/apis":          reflect.TypeOf(dto.AssignAPIsToMenuReq{}),
	"POST /api/admin/roles/:id/menus":         reflect.TypeOf(dto.AssignMenusToRoleReq{}),
	"POST /api/admin/dict-types":              reflect.TypeOf(dto.CreateDictTypeReq{}),
	"PUT /api/admin/dict-types/:id":           reflect.TypeOf(dto.UpdateDictTypeReq{}),
	"POST /api/admin/dict-items":              reflect.TypeOf(dto.CreateDictItemReq{}),
	"PUT /api/admin/dict-items/:id":           reflect.TypeOf(dto.UpdateDictItemReq{}),
	"POST /api/admin/organizations":           reflect.TypeOf(dto.CreateOrganizationReq{}),
	"PUT /api/admin/organizations/:id":        reflect.TypeOf(dto.UpdateOrganizationReq{}),
	"POST /api/admin/organizations/:id/users": reflect.TypeOf(dto.AssignUsersToOrganizationReq{}),
	"POST /api/admin/apis":                    reflect.TypeOf(dto.CreateAPIReq{}),
	"PUT /api/admin/apis/:id":                 reflect.TypeOf(dto.UpdateAPIReq{}),
	"POST /api/admin/apis/:id/menu-button":    reflect.TypeOf(dto.GenerateMenuButtonFromAPIReq{}),
}

func buildSchemaFromType(t reflect.Type) map[string]any {
	nullable := false
	if t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}

	var schema map[string]any
	switch t.Kind() {
	case reflect.Struct:
		schema = buildObjectSchema(t)
	case reflect.Slice, reflect.Array:
		schema = map[string]any{
			"type":  "array",
			"items": buildSchemaFromType(t.Elem()),
		}
	default:
		schema = buildPrimitiveSchema(t)
	}
	if nullable {
		schema["nullable"] = true
	}
	return schema
}

func buildObjectSchema(t reflect.Type) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}

	required := []string{}
	properties := schema["properties"].(map[string]any)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}

		name, ok := jsonFieldName(field)
		if !ok {
			continue
		}

		fieldSchema := buildSchemaFromType(field.Type)
		applyBindingToSchema(fieldSchema, field.Tag.Get("binding"))
		properties[name] = fieldSchema

		if isRequiredBinding(field.Tag.Get("binding")) {
			required = append(required, name)
		}
	}

	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		name = lowerFirst(field.Name)
	}
	return name, true
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func buildPrimitiveSchema(t reflect.Type) map[string]any {
	nullable := false
	if t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}

	schema := map[string]any{}
	switch t.Kind() {
	case reflect.Bool:
		schema["type"] = "boolean"
		schema["example"] = true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		schema["type"] = "integer"
		schema["format"] = "int32"
		schema["example"] = 1
	case reflect.Int64:
		schema["type"] = "integer"
		schema["format"] = "int64"
		schema["example"] = 1
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		schema["type"] = "integer"
		schema["format"] = "uint32"
		schema["example"] = 1
	case reflect.Uint64:
		schema["type"] = "integer"
		schema["format"] = "uint64"
		schema["example"] = 1
	case reflect.Float32:
		schema["type"] = "number"
		schema["format"] = "float"
		schema["example"] = 1.0
	case reflect.Float64:
		schema["type"] = "number"
		schema["format"] = "double"
		schema["example"] = 1.0
	case reflect.String:
		schema["type"] = "string"
		schema["example"] = "string"
	default:
		schema["type"] = "object"
	}

	if nullable {
		schema["nullable"] = true
	}
	return schema
}

func applyBindingToSchema(schema map[string]any, binding string) {
	if binding == "" {
		return
	}

	for _, rule := range strings.Split(binding, ",") {
		rule = strings.TrimSpace(rule)
		if strings.HasPrefix(rule, "min=") {
			if value, err := strconv.Atoi(strings.TrimPrefix(rule, "min=")); err == nil {
				applyMinToSchema(schema, value)
			}
			continue
		}
		if strings.HasPrefix(rule, "max=") {
			if value, err := strconv.Atoi(strings.TrimPrefix(rule, "max=")); err == nil {
				applyMaxToSchema(schema, value)
			}
			continue
		}
		if strings.HasPrefix(rule, "len=") {
			if value, err := strconv.Atoi(strings.TrimPrefix(rule, "len=")); err == nil {
				schema["minLength"] = value
				schema["maxLength"] = value
			}
			continue
		}
		if strings.HasPrefix(rule, "oneof=") {
			values := strings.Fields(strings.TrimPrefix(rule, "oneof="))
			if len(values) > 0 {
				schema["enum"] = normalizeEnumValues(values, schema["type"])
			}
			continue
		}
		if rule == "email" {
			schema["format"] = "email"
			schema["example"] = "user@example.com"
		}
	}
}

func applyMinToSchema(schema map[string]any, value int) {
	switch schema["type"] {
	case "string":
		schema["minLength"] = value
	case "array":
		schema["minItems"] = value
	default:
		schema["minimum"] = value
	}
}

func applyMaxToSchema(schema map[string]any, value int) {
	switch schema["type"] {
	case "string":
		schema["maxLength"] = value
	case "array":
		schema["maxItems"] = value
	default:
		schema["maximum"] = value
	}
}

func normalizeEnumValues(values []string, schemaType any) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		if schemaType == "integer" {
			if parsed, err := strconv.Atoi(value); err == nil {
				result = append(result, parsed)
				continue
			}
		}
		result = append(result, value)
	}
	return result
}

func isRequiredBinding(binding string) bool {
	for _, rule := range strings.Split(binding, ",") {
		if strings.TrimSpace(rule) == "required" {
			return true
		}
	}
	return false
}

func shouldAttachOpenAPISecurity(path string, api model.API) bool {
	if path == "/ping" {
		return false
	}
	if api.ID > 0 {
		return api.NeedAuth == 1
	}
	return inferAPINeedAuth(path) == 1
}

func buildOpenAPIResponses() map[string]any {
	return map[string]any{
		"200": map[string]any{
			"description": "success",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/CommonResponse",
					},
				},
			},
		},
		"400": map[string]any{
			"description": "bad request",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/ErrorResponse",
					},
				},
			},
		},
		"401": map[string]any{
			"description": "unauthorized",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/ErrorResponse",
					},
				},
			},
		},
		"403": map[string]any{
			"description": "forbidden",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/ErrorResponse",
					},
				},
			},
		},
	}
}

func buildOpenAPITags(tagSet map[string]struct{}) []map[string]string {
	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	result := make([]map[string]string, 0, len(tags))
	for _, tag := range tags {
		result = append(result, map[string]string{"name": tag})
	}
	return result
}

func buildOpenAPIComponents() map[string]any {
	return map[string]any{
		"securitySchemes": map[string]any{
			"BearerAuth": map[string]any{
				"type":         "http",
				"scheme":       "bearer",
				"bearerFormat": "JWT",
			},
		},
		"schemas": map[string]any{
			"CommonResponse": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{"type": "integer", "example": 200},
					"msg":  map[string]any{"type": "string", "example": "success"},
					"data": map[string]any{"nullable": true},
				},
			},
			"ErrorResponse": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{"type": "integer", "example": 400},
					"msg":  map[string]any{"type": "string", "example": "参数错误"},
				},
			},
		},
	}
}
