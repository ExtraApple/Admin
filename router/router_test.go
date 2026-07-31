package router

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"admin/global"
	"admin/model"
	"admin/service"
	"admin/utils"
)

// TestInitRouterRegistersAPIRoutes 验证路由初始化会注册 API 管理和文档路由。
func TestInitRouterRegistersAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := InitRouter(service.JWTConfig{Secret: "test"}, Options{
		APIDocs: APIDocsOptions{Enabled: true},
	})
	if r == nil {
		t.Fatal("router should not be nil")
	}

	expected := map[string]struct{}{
		"GET /docs":                            {},
		"GET /docs/openapi.json":               {},
		"GET /api/admin/apis/:id":              {},
		"GET /api/admin/api-groups":            {},
		"GET /api/admin/api-methods":           {},
		"POST /api/admin/apis/sync":            {},
		"POST /api/admin/apis/:id/menu-button": {},
		"GET /api/admin/files/:id/download":    {},
		"GET /api/admin/files/:id/preview":     {},
		"POST /api/admin/files/:id/revalidate": {},
		"GET /api/avatars/:user_id":            {},
		"GET /api/avatars/default":             {},
		"DELETE /api/user/avatar":              {},
	}
	registered := map[string]struct{}{}
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	for route := range expected {
		if _, ok := registered[route]; !ok {
			t.Fatalf("route %s should be registered", route)
		}
	}

	for _, route := range []string{
		"POST /api/files",
		"POST /api/user/files",
		"POST /api/user/uploads",
	} {
		if _, ok := registered[route]; ok {
			t.Fatalf("ordinary users must not have managed-file upload route %s", route)
		}
	}
}

// TestAdminFileAccessRoutesEnforceJWTAndDynamicPermissions 验证新增文件接口
// 必须依次通过 JWT 和 API 权限码校验，普通用户不能越权调用。
func TestAdminFileAccessRoutesEnforceJWTAndDynamicPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, ordinaryToken := setupAdminFileRouteTest(t)
	routes := []struct {
		method         string
		path           string
		registeredPath string
		permissionCode string
	}{
		{
			method:         http.MethodPost,
			path:           "/api/admin/files",
			registeredPath: "/api/admin/files",
			permissionCode: "admin.files.post",
		},
		{
			method:         http.MethodGet,
			path:           "/api/admin/files/not-a-number/download",
			registeredPath: "/api/admin/files/:id/download",
			permissionCode: "admin.files.id.download.get",
		},
		{
			method:         http.MethodGet,
			path:           "/api/admin/files/not-a-number/preview",
			registeredPath: "/api/admin/files/:id/preview",
			permissionCode: "admin.files.id.preview.get",
		},
		{
			method:         http.MethodPost,
			path:           "/api/admin/files/not-a-number/revalidate",
			registeredPath: "/api/admin/files/:id/revalidate",
			permissionCode: "admin.files.id.revalidate.post",
		},
	}

	registered := make(map[string]struct{})
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	requestCount := 0
	for _, route := range routes {
		t.Run(route.permissionCode, func(t *testing.T) {
			routeKey := route.method + " " + route.registeredPath
			if _, ok := registered[routeKey]; !ok {
				t.Fatalf("router and API metadata should share %s", routeKey)
			}

			requestCount++
			assertRouterStatus(t, r, route.method, route.path, "", http.StatusUnauthorized)

			requestCount++
			assertRouterStatus(
				t,
				r,
				route.method,
				route.path,
				ordinaryToken,
				http.StatusForbidden,
			)

			allowedToken := generateRouterTestToken(
				t,
				[]string{route.permissionCode},
			)
			requestCount++
			assertRouterStatus(
				t,
				r,
				route.method,
				route.path,
				allowedToken,
				http.StatusBadRequest,
			)
		})
	}

	waitForAuditLogCount(t, requestCount)
}

// TestOpenAPIDocumentEndpoint 验证 OpenAPI JSON 文档接口可正常返回核心结构。
func TestOpenAPIDocumentEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := InitRouter(service.JWTConfig{Secret: "test"}, Options{
		APIDocs: APIDocsOptions{Enabled: true},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi document should be valid json: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Fatalf("unexpected openapi version: %v", doc["openapi"])
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("openapi document should contain paths")
	}
	pathItem, ok := paths["/api/admin/apis/{id}"]
	if !ok {
		t.Fatal("openapi document should contain api detail route")
	}
	menuButtonPathItem, ok := paths["/api/admin/apis/{id}/menu-button"]
	if !ok {
		t.Fatal("openapi document should contain api menu-button route")
	}
	_ = pathItem
	operation := getOpenAPIOperation(t, menuButtonPathItem, "post")
	properties := getRequestBodyProperties(t, operation)
	for _, name := range []string{"parent_id", "name", "sort"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("menu-button request schema should contain %s", name)
		}
	}
	if _, ok := paths["/docs"]; ok {
		t.Fatal("openapi document should not expose docs route")
	}

	fileUploadOperation := getOpenAPIOperation(
		t,
		paths["/api/admin/files"],
		"post",
	)
	fileUploadDescription, _ := fileUploadOperation["description"].(string)
	for _, fragment := range []string{
		"PDF、UTF-8 TXT、UTF-8 CSV",
		"当前不支持 Office 文档",
		"只能包含一个名为 file 的文件 part",
	} {
		if !strings.Contains(fileUploadDescription, fragment) {
			t.Fatalf(
				"managed file upload description should contain %q, got %q",
				fragment,
				fileUploadDescription,
			)
		}
	}
	if strings.Contains(fileUploadDescription, "JPEG、PNG、WebP、PDF") {
		t.Fatalf(
			"managed file upload description still advertises images: %q",
			fileUploadDescription,
		)
	}
	fileUploadDataProperties := getJSONSuccessDataProperties(
		t,
		fileUploadOperation,
	)
	if _, ok := fileUploadDataProperties["content_sha256"]; !ok {
		t.Fatal("managed file upload response data should contain content_sha256")
	}
	assertOpenAPIResponseCodes(
		t,
		fileUploadOperation,
		"400",
		"413",
		"415",
		"422",
		"500",
		"503",
	)

	avatarUploadOperation := getOpenAPIOperation(
		t,
		paths["/api/user/avatar"],
		"post",
	)
	avatarDescription, _ := avatarUploadOperation["description"].(string)
	for _, fragment := range []string{"JPEG、PNG、WebP", "1,024×1,024", "JPEG 或 PNG"} {
		if !strings.Contains(avatarDescription, fragment) {
			t.Fatalf(
				"avatar upload description should contain %q, got %q",
				fragment,
				avatarDescription,
			)
		}
	}

	fileDetailOperation := getOpenAPIOperation(
		t,
		paths["/api/admin/files/{id}"],
		"get",
	)
	fileDetailDescription, _ := fileDetailOperation["description"].(string)
	for _, fragment := range []string{
		"validated、legacy_unverified、validation_error、blocked",
		"download_url",
	} {
		if !strings.Contains(fileDetailDescription, fragment) {
			t.Fatalf(
				"file detail description should contain %q, got %q",
				fragment,
				fileDetailDescription,
			)
		}
	}
	if strings.Contains(fileDetailDescription, "preview_url") {
		t.Fatalf(
			"file detail description must not promise preview_url: %q",
			fileDetailDescription,
		)
	}
	fileDetailDataProperties := getJSONSuccessDataProperties(
		t,
		fileDetailOperation,
	)
	if _, ok := fileDetailDataProperties["preview_url"]; ok {
		t.Fatal("file detail response data must not contain preview_url")
	}
	fileSchema, ok := fileDetailDataProperties["file"].(map[string]any)
	if !ok {
		t.Fatal("file detail response data should contain file schema")
	}
	fileProperties, ok := fileSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("file detail file schema should contain properties")
	}
	if _, ok := fileProperties["content_sha256"]; !ok {
		t.Fatal("file detail metadata should contain content_sha256")
	}

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("openapi document should contain components")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("openapi components should contain schemas")
	}
	errorSchema, ok := schemas["ErrorResponse"].(map[string]any)
	if !ok {
		t.Fatal("openapi schemas should contain ErrorResponse")
	}
	errorProperties, ok := errorSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("ErrorResponse should contain properties")
	}
	if _, ok := errorProperties["error_code"]; !ok {
		t.Fatal("ErrorResponse should document stable error_code")
	}

	updateSelfOperation := getOpenAPIOperation(
		t,
		paths["/api/user/info"],
		"put",
	)
	updateSelfProperties := getRequestBodyProperties(t, updateSelfOperation)
	if _, ok := updateSelfProperties["avatar"]; ok {
		t.Fatal("PUT /api/user/info schema must not expose writable avatar")
	}

	revalidateOperation := getOpenAPIOperation(
		t,
		paths["/api/admin/files/{id}/revalidate"],
		"post",
	)
	revalidateDataProperties := getJSONSuccessDataProperties(t, revalidateOperation)
	for _, name := range []string{
		"validation_status",
		"detected_content_type",
		"content_sha256",
		"validation_policy_version",
		"validated_at",
	} {
		if _, ok := revalidateDataProperties[name]; !ok {
			t.Fatalf("revalidate response data should contain %s", name)
		}
	}

	restoreAvatarOperation := getOpenAPIOperation(
		t,
		paths["/api/user/avatar"],
		"delete",
	)
	restoreAvatarDataProperties := getJSONSuccessDataProperties(
		t,
		restoreAvatarOperation,
	)
	if _, ok := restoreAvatarDataProperties["avatar"]; !ok {
		t.Fatal("restore-default response data should contain avatar")
	}
	if _, ok := restoreAvatarDataProperties["avatar_content_sha256"]; ok {
		t.Fatal("restore-default response must not expose avatar_content_sha256")
	}
	avatarUploadDataProperties := getJSONSuccessDataProperties(
		t,
		avatarUploadOperation,
	)
	if _, ok := avatarUploadDataProperties["avatar_content_sha256"]; ok {
		t.Fatal("avatar upload response must not expose avatar_content_sha256")
	}

	downloadOperation := getOpenAPIOperation(
		t,
		paths["/api/admin/files/{id}/download"],
		"get",
	)
	assertOpenAPIBinaryResponse(t, downloadOperation, "application/octet-stream")

	previewOperation := getOpenAPIOperation(
		t,
		paths["/api/admin/files/{id}/preview"],
		"get",
	)
	previewDescription, _ := previewOperation["description"].(string)
	if !strings.Contains(previewDescription, "始终返回 HTTP 409") {
		t.Fatalf(
			"preview description should document stable rejection, got %q",
			previewDescription,
		)
	}
	previewResponses, ok := previewOperation["responses"].(map[string]any)
	if !ok {
		t.Fatal("preview operation should contain responses")
	}
	if _, ok := previewResponses["200"]; ok {
		t.Fatal("preview operation must not document a successful binary response")
	}
	if _, ok := previewResponses["409"]; !ok {
		t.Fatal("preview operation should document HTTP 409")
	}

	avatarContentTypes := map[string][]string{
		"/api/avatars/{user_id}": {"image/jpeg", "image/png"},
		"/api/avatars/default":   {"image/png"},
	}
	for path, contentTypes := range avatarContentTypes {
		avatarOperation := getOpenAPIOperation(t, paths[path], "get")
		for _, contentType := range contentTypes {
			assertOpenAPIBinaryResponse(t, avatarOperation, contentType)
		}
		if _, ok := avatarOperation["security"]; ok {
			t.Fatalf("%s should be documented as anonymous", path)
		}
	}
}

// TestAPIDocsCanBeDisabled 验证 API 文档路由默认关闭。
func TestAPIDocsCanBeDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := InitRouter(service.JWTConfig{Secret: "test"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected docs route disabled by default, got %d", w.Code)
	}
}

func setupAdminFileRouteTest(t *testing.T) (*gin.Engine, string) {
	t.Helper()

	previousDB := global.DB
	previousRedis := global.Redis

	dbPath := filepath.Join(t.TempDir(), "router-permissions.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open router test database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
		&model.API{},
		&model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate router test database: %v", err)
	}

	const accessVersion = 1
	user := model.User{
		Username: "router-user",
		Password: "not-used",
		Email:    "router-user@example.com",
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create router test user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: accessVersion,
	}).Error; err != nil {
		t.Fatalf("create router test user access version: %v", err)
	}

	apis := []model.API{
		{
			Name:           "上传文件",
			Method:         http.MethodPost,
			Path:           "/api/admin/files",
			PermissionCode: "admin.files.post",
			Status:         1,
			NeedAuth:       1,
		},
		{
			Name:           "下载文件",
			Method:         http.MethodGet,
			Path:           "/api/admin/files/:id/download",
			PermissionCode: "admin.files.id.download.get",
			Status:         1,
			NeedAuth:       1,
		},
		{
			Name:           "预览文件",
			Method:         http.MethodGet,
			Path:           "/api/admin/files/:id/preview",
			PermissionCode: "admin.files.id.preview.get",
			Status:         1,
			NeedAuth:       1,
		},
		{
			Name:           "重新验证文件",
			Method:         http.MethodPost,
			Path:           "/api/admin/files/:id/revalidate",
			PermissionCode: "admin.files.id.revalidate.post",
			Status:         1,
			NeedAuth:       1,
		},
	}
	if err := db.Create(&apis).Error; err != nil {
		t.Fatalf("create router test API metadata: %v", err)
	}

	global.DB = db
	redisClient := redis.NewClient(&redis.Options{Addr: "router-test.invalid:6379"})
	redisClient.AddHook(routerRedisHook{})
	global.Redis = redisClient

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get router test sql database: %v", err)
	}
	t.Cleanup(func() {
		global.DB = previousDB
		global.Redis = previousRedis
		_ = redisClient.Close()
		_ = sqlDB.Close()
	})

	return InitRouter(service.JWTConfig{Secret: "router-test-secret"}),
		generateRouterTestToken(t, nil)
}

type routerRedisHook struct{}

func (routerRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (routerRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "exists") {
			exists, ok := cmd.(*redis.IntCmd)
			if !ok {
				return next(ctx, cmd)
			}
			exists.SetVal(0)
			return nil
		}
		return next(ctx, cmd)
	}
}

func (routerRedisHook) ProcessPipelineHook(
	next redis.ProcessPipelineHook,
) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		return next(ctx, cmds)
	}
}

func generateRouterTestToken(t *testing.T, permissions []string) string {
	t.Helper()

	token, _, err := utils.GenerateToken(
		1,
		1,
		[]string{"user"},
		permissions,
		"router-test-secret",
		15,
		60,
	)
	if err != nil {
		t.Fatalf("generate router test token: %v", err)
	}
	return token
}

func assertRouterStatus(
	t *testing.T,
	r http.Handler,
	method, path, token string,
	want int,
) {
	t.Helper()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	if w.Code != want {
		t.Fatalf(
			"%s %s status = %d, want %d; body=%s",
			method,
			path,
			w.Code,
			want,
			w.Body.String(),
		)
	}
}

func waitForAuditLogCount(t *testing.T, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := global.DB.Model(&model.AuditLog{}).Count(&count).Error; err == nil &&
			count >= int64(want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	var count int64
	_ = global.DB.Model(&model.AuditLog{}).Count(&count).Error
	t.Fatalf("audit log count = %d, want at least %d", count, want)
}

// getOpenAPIOperation 从 OpenAPI path item 中取出指定方法的 operation。
func getOpenAPIOperation(t *testing.T, pathItem any, method string) map[string]any {
	t.Helper()

	item, ok := pathItem.(map[string]any)
	if !ok {
		t.Fatal("path item should be object")
	}
	operation, ok := item[method].(map[string]any)
	if !ok {
		t.Fatalf("%s operation should exist", method)
	}
	return operation
}

// getRequestBodyProperties 从 OpenAPI operation 中取出请求体属性定义。
func getRequestBodyProperties(t *testing.T, operation map[string]any) map[string]any {
	t.Helper()

	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		t.Fatal("operation should contain requestBody")
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		t.Fatal("requestBody should contain content")
	}
	jsonContent, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatal("requestBody should contain application/json")
	}
	schema, ok := jsonContent["schema"].(map[string]any)
	if !ok {
		t.Fatal("application/json should contain schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema should contain properties")
	}
	return properties
}

func assertOpenAPIResponseCodes(
	t *testing.T,
	operation map[string]any,
	codes ...string,
) {
	t.Helper()

	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatal("operation should contain responses")
	}
	for _, code := range codes {
		if _, ok := responses[code]; !ok {
			t.Fatalf("operation should document HTTP %s", code)
		}
	}
}

func getJSONSuccessDataProperties(
	t *testing.T,
	operation map[string]any,
) map[string]any {
	t.Helper()

	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatal("operation should contain responses")
	}
	success, ok := responses["200"].(map[string]any)
	if !ok {
		t.Fatal("operation should contain HTTP 200 response")
	}
	content, ok := success["content"].(map[string]any)
	if !ok {
		t.Fatal("HTTP 200 response should contain content")
	}
	jsonContent, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatal("HTTP 200 response should contain application/json")
	}
	schema, ok := jsonContent["schema"].(map[string]any)
	if !ok {
		t.Fatal("application/json response should contain schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("JSON response schema should contain properties")
	}
	data, ok := properties["data"].(map[string]any)
	if !ok {
		t.Fatal("JSON response schema should contain data")
	}
	dataProperties, ok := data["properties"].(map[string]any)
	if !ok {
		t.Fatal("JSON response data should contain properties")
	}
	return dataProperties
}

func assertOpenAPIBinaryResponse(
	t *testing.T,
	operation map[string]any,
	contentType string,
) {
	t.Helper()

	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatal("operation should contain responses")
	}
	success, ok := responses["200"].(map[string]any)
	if !ok {
		t.Fatal("operation should contain HTTP 200 response")
	}
	content, ok := success["content"].(map[string]any)
	if !ok {
		t.Fatal("HTTP 200 response should contain content")
	}
	media, ok := content[contentType].(map[string]any)
	if !ok {
		t.Fatalf("HTTP 200 response should document %s", contentType)
	}
	schema, ok := media["schema"].(map[string]any)
	if !ok || schema["type"] != "string" || schema["format"] != "binary" {
		t.Fatalf("%s response should use string/binary schema", contentType)
	}
}
