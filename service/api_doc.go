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

// BuildOpenAPIDocument 根据 Gin 路由和 API 元数据生成 OpenAPI 文档。
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

// applyOpenAPIDefaults 为 OpenAPI 文档配置填充默认值。
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

// loadAPIDocMetadata 加载 API 元数据并按方法和路径建立索引。
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

// shouldExposeOpenAPIRoute 判断路由是否应出现在 OpenAPI 文档中。
func shouldExposeOpenAPIRoute(path string) bool {
	if path == "/ping" {
		return true
	}
	if strings.HasPrefix(path, "/docs") {
		return false
	}
	return strings.HasPrefix(path, "/api/")
}

// ginPathToOpenAPIPath 将 Gin 路由参数格式转换为 OpenAPI 路径格式。
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

// resolveOpenAPITag 根据 API 元数据或路径推断 OpenAPI 标签。
func resolveOpenAPITag(path string, api model.API) string {
	if strings.TrimSpace(api.Group) != "" {
		return api.Group
	}
	if path == "/ping" {
		return "system"
	}
	return inferAPIGroup(path)
}

// buildOpenAPIOperation 构建单个 OpenAPI operation 描述。
func buildOpenAPIOperation(method, path, tag string, params []string, api model.API) map[string]any {
	operation := map[string]any{
		"tags":        []string{tag},
		"summary":     resolveOpenAPISummary(method, path, api),
		"operationId": buildOpenAPIOperationID(method, path),
		"responses":   buildOpenAPIResponses(method, path),
	}

	description := resolveOpenAPIDescription(method, path, api)
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

// resolveOpenAPISummary 解析 OpenAPI operation 的摘要文案。
func resolveOpenAPISummary(method, path string, api model.API) string {
	if strings.TrimSpace(api.Name) != "" {
		return api.Name
	}
	return method + " " + path
}

// resolveOpenAPIDescription 合并路由级安全约束和 API 元数据备注。
func resolveOpenAPIDescription(method, path string, api model.API) string {
	parts := make([]string, 0, 2)
	if description := openAPIRouteDescriptions[method+" "+path]; description != "" {
		parts = append(parts, description)
	}
	if remark := strings.TrimSpace(api.Remark); remark != "" {
		parts = append(parts, remark)
	}
	return strings.Join(parts, "\n\n")
}

var openAPIRouteDescriptions = map[string]string{
	"POST /api/admin/files":                "管理员业务文件上传只能包含一个名为 file 的文件 part。V1 允许 JPEG、PNG、WebP、PDF、DOCX、XLSX、PPTX、UTF-8 TXT、UTF-8 CSV；扩展名、声明 MIME、检测 MIME 和专用验证结果必须一致，文件通过验证后才会写入对象存储。",
	"GET /api/admin/files/:id":             "返回文件验证状态 validated、legacy_unverified、validation_error、blocked 及可信元数据。download_url 仅在状态允许下载时返回；preview_url 仅为 validated JPEG、PNG、WebP 返回；不会暴露 MinIO 直连地址。",
	"GET /api/admin/files/:id/download":    "使用 JWT、动态 API 权限、有效期和 HMAC 签名下载文件。validated 文件使用规范 MIME；legacy_unverified 和 validation_error 强制作为 application/octet-stream 附件；blocked 拒绝访问。",
	"GET /api/admin/files/:id/preview":     "使用 JWT、动态 API 权限、有效期和 HMAC 签名内联预览，仅允许 validated JPEG、PNG、WebP，其他格式或状态返回稳定冲突错误。",
	"POST /api/admin/files/:id/revalidate": "仅允许重新验证 legacy_unverified 或 validation_error 文件；通过后更新为 validated，明确策略拒绝更新为 blocked，临时基础设施错误更新为 validation_error。",
	"POST /api/user/avatar":                "头像上传只能包含一个名为 file 的文件 part，仅接受可完整解码的静态 JPEG、PNG、WebP。系统移除非像素元数据，按比例缩放至最大 1,024×1,024，并根据透明通道重新编码为 JPEG 或 PNG。",
	"DELETE /api/user/avatar":              "清除当前可信头像标识并恢复 /api/avatars/default；旧系统头像对象在数据库更新成功后清理，清理失败不会回滚恢复结果。",
	"GET /api/avatars/:user_id":            "匿名返回服务端标准化的 JPEG/PNG 可信头像；用户、状态或存储对象不可用时统一返回内置默认 PNG，不暴露 object key 或历史头像 URL。",
	"GET /api/avatars/default":             "匿名返回应用内置默认 PNG。",
	"PUT /api/user/info":                   "只允许修改昵称和邮箱；请求中出现 avatar 字段会被拒绝，头像必须通过专用上传或恢复默认接口修改。",
}

// buildOpenAPIOperationID 根据方法和路径生成稳定的 operationId。
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

// buildOpenAPIPathParams 构建 OpenAPI 路径参数定义。
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

// buildOpenAPIRequestBody 根据路由元数据构建请求体 schema。
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

// buildSchemaFromType 将 Go 类型转换为 OpenAPI schema。
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

// buildObjectSchema 将结构体类型转换为 OpenAPI object schema。
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

// jsonFieldName 从结构体字段标签中解析 JSON 字段名。
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

// lowerFirst 将字符串首字母转换为小写。
func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// buildPrimitiveSchema 将基础 Go 类型转换为 OpenAPI schema。
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

// applyBindingToSchema 将 Gin binding 规则映射到 OpenAPI schema 约束。
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

// applyMinToSchema 根据 schema 类型写入最小值或最小长度约束。
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

// applyMaxToSchema 根据 schema 类型写入最大值或最大长度约束。
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

// normalizeEnumValues 根据 schema 类型转换枚举值。
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

// isRequiredBinding 判断 binding 标签是否包含 required 约束。
func isRequiredBinding(binding string) bool {
	for _, rule := range strings.Split(binding, ",") {
		if strings.TrimSpace(rule) == "required" {
			return true
		}
	}
	return false
}

// shouldAttachOpenAPISecurity 判断 OpenAPI operation 是否需要挂载 BearerAuth。
func shouldAttachOpenAPISecurity(path string, api model.API) bool {
	if path == "/ping" {
		return false
	}
	if api.ID > 0 {
		return api.NeedAuth == 1
	}
	return inferAPINeedAuth(path) == 1
}

// buildOpenAPIResponses 构建通用响应并补充文件安全相关状态。
func buildOpenAPIResponses(method, path string) map[string]any {
	responses := map[string]any{
		"200": buildOpenAPISuccessResponse(method, path),
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

	for _, code := range openAPIAdditionalResponseCodes[method+" "+path] {
		responses[code] = openAPIErrorResponse(code)
	}
	return responses
}

func buildOpenAPISuccessResponse(method, path string) map[string]any {
	route := method + " " + path
	if contentTypes, ok := openAPIBinaryResponseTypes[route]; ok {
		content := make(map[string]any, len(contentTypes))
		for _, contentType := range contentTypes {
			content[contentType] = map[string]any{
				"schema": map[string]any{
					"type":   "string",
					"format": "binary",
				},
			}
		}
		return map[string]any{
			"description": "binary content",
			"headers": map[string]any{
				"Content-Type": map[string]any{
					"schema": map[string]any{"type": "string"},
				},
				"Content-Disposition": map[string]any{
					"schema": map[string]any{"type": "string"},
				},
				"X-Content-Type-Options": map[string]any{
					"schema":  map[string]any{"type": "string"},
					"example": "nosniff",
				},
				"Cache-Control": map[string]any{
					"schema": map[string]any{"type": "string"},
				},
			},
			"content": content,
		}
	}

	if responseType, ok := openAPIJSONResponseSchemas[route]; ok {
		return map[string]any{
			"description": "success",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"code": map[string]any{
								"type":    "integer",
								"example": 200,
							},
							"msg": map[string]any{
								"type":    "string",
								"example": "success",
							},
							"data": buildSchemaFromType(responseType),
						},
					},
				},
			},
		}
	}

	return map[string]any{
		"description": "success",
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"$ref": "#/components/schemas/CommonResponse",
				},
			},
		},
	}
}

var openAPIJSONResponseSchemas = map[string]reflect.Type{
	"POST /api/admin/files":                reflect.TypeOf(dto.FileInfo{}),
	"GET /api/admin/files/:id":             reflect.TypeOf(dto.FileDetailResp{}),
	"POST /api/admin/files/:id/revalidate": reflect.TypeOf(dto.FileInfo{}),
	"POST /api/user/avatar":                reflect.TypeOf(dto.UserInfo{}),
	"DELETE /api/user/avatar":              reflect.TypeOf(dto.UserInfo{}),
}

var openAPIBinaryResponseTypes = map[string][]string{
	"GET /api/admin/files/:id/download": {
		"application/octet-stream",
		"image/jpeg",
		"image/png",
		"image/webp",
		"application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"text/plain",
		"text/csv",
	},
	"GET /api/admin/files/:id/preview": {
		"image/jpeg",
		"image/png",
		"image/webp",
	},
	"GET /api/avatars/:user_id": {
		"image/jpeg",
		"image/png",
	},
	"GET /api/avatars/default": {
		"image/png",
	},
}

var openAPIAdditionalResponseCodes = map[string][]string{
	"POST /api/admin/files":                {"413", "415", "422", "500", "503"},
	"GET /api/admin/files/:id":             {"404", "409", "500", "503"},
	"GET /api/admin/files/:id/download":    {"404", "409", "500", "503"},
	"GET /api/admin/files/:id/preview":     {"404", "409", "500", "503"},
	"POST /api/admin/files/:id/revalidate": {"404", "409", "415", "422", "500", "503"},
	"POST /api/user/avatar":                {"413", "415", "422", "500", "503"},
	"DELETE /api/user/avatar":              {"500", "503"},
	"PUT /api/user/info":                   {"409", "500"},
}

func openAPIErrorResponse(code string) map[string]any {
	descriptions := map[string]string{
		"404": "not found",
		"409": "state conflict",
		"413": "payload too large",
		"415": "unsupported media type",
		"422": "invalid file content",
		"500": "internal error",
		"503": "service unavailable",
	}
	description := descriptions[code]
	if description == "" {
		description = "error"
	}
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"$ref": "#/components/schemas/ErrorResponse",
				},
			},
		},
	}
}

// buildOpenAPITags 将标签集合转换为稳定排序的 OpenAPI 标签列表。
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

// buildOpenAPIComponents 构建 OpenAPI components 定义。
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
					"code":       map[string]any{"type": "integer", "example": 400},
					"error_code": map[string]any{"type": "string", "example": "REQUEST_INVALID"},
					"msg":        map[string]any{"type": "string", "example": "请求参数不合法"},
				},
			},
		},
	}
}
