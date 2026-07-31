package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"admin/global"
	"admin/model"
	"admin/service"
	"admin/utils"
)

const (
	authUserContractSecret      = "auth-user-contract-secret"
	authUserContractCaptchaID   = "auth-user-contract-captcha"
	authUserContractCaptchaCode = "123456"
)

// TestAuthAndAdminUserHTTPContractBaseline 固化登录与管理员用户 HTTP 契约。
func TestAuthAndAdminUserHTTPContractBaseline(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, admin := setupAuthUserContractRouter(t)
	registered := make(map[string]struct{})
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	expectedAdminRoutes := []string{
		"GET /api/admin/users",
		"PUT /api/admin/users/:id",
		"DELETE /api/admin/users/:id",
		"PUT /api/admin/users/:id/status",
		"PUT /api/admin/users/:id/kick",
	}
	for _, route := range expectedAdminRoutes {
		if _, ok := registered[route]; !ok {
			t.Fatalf("administrator user route %s should be registered", route)
		}
	}
	loginBody := map[string]any{
		"username":     admin.Username,
		"password":     "ValidPass123!",
		"captcha_id":   authUserContractCaptchaID,
		"captcha_code": authUserContractCaptchaCode,
	}
	loginResponse := performAuthUserContractRequest(
		t,
		r,
		http.MethodPost,
		"/api/login",
		loginBody,
		"",
		http.StatusOK,
	)
	assertAuthUserContractCode(t, loginResponse, http.StatusOK)
	if loginResponse["msg"] != "登录成功" {
		t.Fatalf("login msg = %v, want 登录成功", loginResponse["msg"])
	}

	loginData := authUserContractObject(t, loginResponse["data"], "login data")
	accessToken := authUserContractString(t, loginData["access_token"], "access_token")
	refreshToken := authUserContractString(t, loginData["refresh_token"], "refresh_token")
	userData := authUserContractObject(t, loginData["user"], "login user")
	if userData["username"] != admin.Username {
		t.Fatalf("login user username = %v, want %s", userData["username"], admin.Username)
	}

	for name, token := range map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	} {
		claims, err := utils.ParseToken(token, authUserContractSecret)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if claims.UserID != admin.ID || claims.TokenVersion != admin.TokenVersion {
			t.Fatalf(
				"%s claims user/version = %d/%d, want %d/%d",
				name,
				claims.UserID,
				claims.TokenVersion,
				admin.ID,
				admin.TokenVersion,
			)
		}
	}
	waitForAuditLogCount(t, 1)

	adminCases := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "list",
			method:     http.MethodGet,
			path:       "/api/admin/users?page=1&size=10",
			wantStatus: http.StatusOK,
		},
		{
			name:       "update",
			method:     http.MethodPut,
			path:       "/api/admin/users/not-a-number",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "参数错误",
		},
		{
			name:       "delete",
			method:     http.MethodDelete,
			path:       "/api/admin/users/not-a-number",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "参数错误",
		},
		{
			name:       "status",
			method:     http.MethodPut,
			path:       "/api/admin/users/not-a-number/status",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "参数错误",
		},
		{
			name:       "kick",
			method:     http.MethodPut,
			path:       "/api/admin/users/not-a-number/kick",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "参数错误",
		},
	}
	for i, tc := range adminCases {
		t.Run(tc.name, func(t *testing.T) {
			response := performAuthUserContractRequest(
				t,
				r,
				tc.method,
				tc.path,
				tc.body,
				accessToken,
				tc.wantStatus,
			)
			assertAuthUserContractCode(t, response, tc.wantStatus)
			if tc.wantMsg != "" && response["msg"] != tc.wantMsg {
				t.Fatalf("%s msg = %v, want %s", tc.name, response["msg"], tc.wantMsg)
			}
			if tc.name == "list" {
				data := authUserContractObject(t, response["data"], "user list data")
				for _, field := range []string{"list", "total", "page", "size"} {
					if _, ok := data[field]; !ok {
						t.Fatalf("user list data should contain %s", field)
					}
				}
			}
			waitForAuditLogCount(t, i+2)
		})
	}

}

func TestRefreshTokenHTTPContractReturnsNewTokenPair(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, admin := setupAuthUserContractRouter(t)
	registered := make(map[string]struct{})
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	if _, ok := registered["POST /api/refresh"]; !ok {
		t.Fatal("POST /api/refresh should be registered")
	}

	loginResponse := performAuthUserContractRequest(
		t,
		r,
		http.MethodPost,
		"/api/login",
		map[string]any{
			"username":     admin.Username,
			"password":     "ValidPass123!",
			"captcha_id":   authUserContractCaptchaID,
			"captcha_code": authUserContractCaptchaCode,
		},
		"",
		http.StatusOK,
	)
	loginData := authUserContractObject(t, loginResponse["data"], "login data")
	refreshToken := authUserContractString(
		t,
		loginData["refresh_token"],
		"login refresh_token",
	)
	waitForAuditLogCount(t, 1)

	refreshResponse := performAuthUserContractRequest(
		t,
		r,
		http.MethodPost,
		"/api/refresh",
		map[string]any{"refresh_token": refreshToken},
		"",
		http.StatusOK,
	)
	waitForAuditLogCount(t, 2)
	assertAuthUserContractCode(t, refreshResponse, http.StatusOK)
	if refreshResponse["msg"] != "刷新成功" {
		t.Fatalf("refresh msg = %v, want 刷新成功", refreshResponse["msg"])
	}
	refreshData := authUserContractObject(
		t,
		refreshResponse["data"],
		"refresh data",
	)
	for name, token := range map[string]string{
		"access_token": authUserContractString(
			t,
			refreshData["access_token"],
			"refreshed access_token",
		),
		"refresh_token": authUserContractString(
			t,
			refreshData["refresh_token"],
			"refreshed refresh_token",
		),
	} {
		claims, err := utils.ParseToken(token, authUserContractSecret)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if claims.UserID != admin.ID || claims.TokenVersion != admin.TokenVersion {
			t.Fatalf(
				"%s claims user/version = %d/%d, want %d/%d",
				name,
				claims.UserID,
				claims.TokenVersion,
				admin.ID,
				admin.TokenVersion,
			)
		}
	}
}

func TestRefreshTokenHTTPContractRejectsAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, admin := setupAuthUserContractRouter(t)
	accessToken, _, err := utils.GenerateToken(
		admin.ID,
		admin.TokenVersion,
		[]string{"admin"},
		nil,
		authUserContractSecret,
		15,
		60,
	)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	requestBody, err := json.Marshal(map[string]any{
		"refresh_token": accessToken,
	})
	if err != nil {
		t.Fatalf("marshal access-token refresh request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/refresh", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	waitForAuditLogCount(t, 1)

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode access-token refresh response: %v", err)
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/refresh status = %d, want 401", w.Code)
	}
	assertAuthUserContractCode(t, response, http.StatusUnauthorized)
	if response["msg"] != service.ErrRefreshTokenInvalid.Error() {
		t.Fatalf(
			"access-token refresh msg = %v, want %s",
			response["msg"],
			service.ErrRefreshTokenInvalid,
		)
	}
	if _, ok := response["data"]; ok {
		t.Fatalf("access-token refresh must not return token data: %#v", response)
	}
}

func TestJWTAuthRejectsRefreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, admin := setupAuthUserContractRouter(t)
	_, refreshToken, err := utils.GenerateToken(
		admin.ID,
		admin.TokenVersion,
		[]string{"admin"},
		nil,
		authUserContractSecret,
		15,
		60,
	)
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	waitForAuditLogCount(t, 1)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh-token protected request status = %d, want 401", w.Code)
	}
}

func TestRefreshTokenHTTPContractReturnsStableUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, _ := setupAuthUserContractRouter(t)
	for index, tc := range []struct {
		name string
		body any
	}{
		{
			name: "invalid token",
			body: map[string]any{"refresh_token": "not-a-jwt"},
		},
		{
			name: "missing token",
			body: map[string]any{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := performAuthUserContractRequest(
				t,
				r,
				http.MethodPost,
				"/api/refresh",
				tc.body,
				"",
				http.StatusUnauthorized,
			)
			assertAuthUserContractCode(t, response, http.StatusUnauthorized)
			if response["msg"] != service.ErrRefreshTokenInvalid.Error() {
				t.Fatalf(
					"refresh failure msg = %v, want %s",
					response["msg"],
					service.ErrRefreshTokenInvalid,
				)
			}
			if _, ok := response["data"]; ok {
				t.Fatalf("refresh failure response should not contain token data: %#v", response)
			}
			waitForAuditLogCount(t, index+1)
		})
	}
}

func TestJWTAuthReadsAuthorizationVersionAfterUserStatusCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, admin := setupAuthUserContractRouter(t)
	accessToken, _, err := utils.GenerateToken(
		admin.ID,
		admin.TokenVersion,
		[]string{"admin"},
		nil,
		authUserContractSecret,
		15,
		60,
	)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	response := performAuthUserContractRequest(
		t,
		r,
		http.MethodGet,
		"/api/user/info",
		nil,
		accessToken,
		http.StatusOK,
	)
	assertAuthUserContractCode(t, response, http.StatusOK)
	waitForAuditLogCount(t, 1)

	if err := global.DB.Model(&exitAccessVersionTestUser{}).
		Where("id = ?", admin.ID).
		UpdateColumn("status", 0).Error; err != nil {
		t.Fatalf("disable user before authorization-order check: %v", err)
	}
	if err := global.DB.Migrator().DropTable(&model.UserAccessVersion{}); err != nil {
		t.Fatalf("drop access-version table for authorization-order check: %v", err)
	}
	response = performAuthUserContractRequest(
		t,
		r,
		http.MethodGet,
		"/api/user/info",
		nil,
		accessToken,
		http.StatusUnauthorized,
	)
	assertAuthUserContractCode(t, response, http.StatusUnauthorized)
	if response["msg"] != "账号已被禁用" {
		t.Fatalf(
			"disabled user response msg = %v, want status rejection before authorization storage read",
			response["msg"],
		)
	}
	waitForAuditLogCount(t, 2)
}

func TestJWTAuthRejectsMissingAuthorizationVersionWithoutLegacyFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, admin := setupAuthUserContractRouter(t)
	accessToken, _, err := utils.GenerateToken(
		admin.ID,
		admin.TokenVersion,
		[]string{"admin"},
		nil,
		authUserContractSecret,
		15,
		60,
	)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	if err := global.DB.Delete(
		&model.UserAccessVersion{},
		"user_id = ?",
		admin.ID,
	).Error; err != nil {
		t.Fatalf("delete authorization access version: %v", err)
	}

	response := performAuthUserContractRequest(
		t,
		r,
		http.MethodGet,
		"/api/user/info",
		nil,
		accessToken,
		http.StatusUnauthorized,
	)
	assertAuthUserContractCode(t, response, http.StatusUnauthorized)
	if response["msg"] != service.ErrAccessVersionNotFound.Error() {
		t.Fatalf(
			"missing authorization version msg = %v, want %s",
			response["msg"],
			service.ErrAccessVersionNotFound,
		)
	}
	waitForAuditLogCount(t, 1)
}

func TestAccessAndRefreshTokensRejectInvalidAuthorizationState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name          string
		mutate        func(t *testing.T, user exitAccessVersionTestUser)
		wantAccessMsg string
	}{
		{
			name: "authorization version mismatch ignores matching legacy field",
			mutate: func(t *testing.T, user exitAccessVersionTestUser) {
				t.Helper()
				if err := global.DB.Model(&model.UserAccessVersion{}).
					Where("user_id = ?", user.ID).
					UpdateColumn("version", user.TokenVersion+1).Error; err != nil {
					t.Fatalf("increment authorization version: %v", err)
				}
			},
			wantAccessMsg: "Token已失效，请重新登录",
		},
		{
			name: "disabled user",
			mutate: func(t *testing.T, user exitAccessVersionTestUser) {
				t.Helper()
				if err := global.DB.Model(&exitAccessVersionTestUser{}).
					Where("id = ?", user.ID).
					UpdateColumn("status", 0).Error; err != nil {
					t.Fatalf("disable user: %v", err)
				}
			},
			wantAccessMsg: "账号已被禁用",
		},
		{
			name: "soft deleted user",
			mutate: func(t *testing.T, user exitAccessVersionTestUser) {
				t.Helper()
				if err := global.DB.Delete(&exitAccessVersionTestUser{}, user.ID).Error; err != nil {
					t.Fatalf("soft delete user: %v", err)
				}
			},
			wantAccessMsg: "用户不存在",
		},
		{
			name: "missing authorization version ignores matching legacy field",
			mutate: func(t *testing.T, user exitAccessVersionTestUser) {
				t.Helper()
				if err := global.DB.Delete(
					&model.UserAccessVersion{},
					"user_id = ?",
					user.ID,
				).Error; err != nil {
					t.Fatalf("delete authorization version: %v", err)
				}
			},
			wantAccessMsg: service.ErrAccessVersionNotFound.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, user := setupAuthUserContractRouter(t)
			accessToken, refreshToken, err := utils.GenerateToken(
				user.ID,
				user.TokenVersion,
				[]string{"admin"},
				nil,
				authUserContractSecret,
				15,
				60,
			)
			if err != nil {
				t.Fatalf("generate token pair: %v", err)
			}

			tc.mutate(t, user)

			accessResponse := performAuthUserContractRequest(
				t,
				r,
				http.MethodGet,
				"/api/user/info",
				nil,
				accessToken,
				http.StatusUnauthorized,
			)
			assertAuthUserContractCode(t, accessResponse, http.StatusUnauthorized)
			if accessResponse["msg"] != tc.wantAccessMsg {
				t.Fatalf(
					"access rejection msg = %v, want %s",
					accessResponse["msg"],
					tc.wantAccessMsg,
				)
			}
			waitForAuditLogCount(t, 1)

			refreshResponse := performAuthUserContractRequest(
				t,
				r,
				http.MethodPost,
				"/api/refresh",
				map[string]any{"refresh_token": refreshToken},
				"",
				http.StatusUnauthorized,
			)
			assertAuthUserContractCode(t, refreshResponse, http.StatusUnauthorized)
			if refreshResponse["msg"] != service.ErrRefreshTokenInvalid.Error() {
				t.Fatalf(
					"refresh rejection msg = %v, want %s",
					refreshResponse["msg"],
					service.ErrRefreshTokenInvalid,
				)
			}
			if _, ok := refreshResponse["data"]; ok {
				t.Fatalf(
					"refresh rejection must not return token data: %#v",
					refreshResponse,
				)
			}
			waitForAuditLogCount(t, 2)
		})
	}
}

func setupAuthUserContractRouter(t *testing.T) (*gin.Engine, exitAccessVersionTestUser) {
	t.Helper()

	previousDB := global.DB
	previousRedis := global.Redis

	dbPath := filepath.Join(t.TempDir(), "auth-user-contract.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open auth user contract database: %v", err)
	}
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
		&model.Role{},
		&model.UserRole{},
		&model.Permission{},
		&model.RolePermission{},
		&model.API{},
		&model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate auth user contract database: %v", err)
	}
	if db.Migrator().HasColumn(&model.User{}, "token_version") {
		t.Fatal("exit-version HTTP contract database still has users.token_version")
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash auth user contract password: %v", err)
	}
	admin := exitAccessVersionTestUser{
		Username:     "contract-admin",
		Password:     string(passwordHash),
		Email:        "contract-admin@example.com",
		Role:         "admin",
		Status:       1,
		TokenVersion: 7,
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create auth user contract administrator: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  admin.ID,
		Version: admin.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create auth user contract access version: %v", err)
	}
	adminRole := model.Role{
		Name:      "contract administrator",
		Code:      "admin",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&adminRole).Error; err != nil {
		t.Fatalf("create auth user contract role: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: admin.ID,
		RoleID: adminRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign auth user contract role: %v", err)
	}

	apis := []model.API{
		authUserContractAPI("用户列表", http.MethodGet, "/api/admin/users"),
		authUserContractAPI("修改用户", http.MethodPut, "/api/admin/users/:id"),
		authUserContractAPI("删除用户", http.MethodDelete, "/api/admin/users/:id"),
		authUserContractAPI("切换用户状态", http.MethodPut, "/api/admin/users/:id/status"),
		authUserContractAPI("强制用户下线", http.MethodPut, "/api/admin/users/:id/kick"),
	}
	if err := db.Create(&apis).Error; err != nil {
		t.Fatalf("create auth user contract API metadata: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: "auth-user-contract.invalid:6379",
	})
	redisClient.AddHook(authUserContractRedisHook{})
	global.DB = db
	global.Redis = redisClient

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get auth user contract sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		global.DB = previousDB
		global.Redis = previousRedis
		_ = redisClient.Close()
		_ = sqlDB.Close()
	})

	return InitRouter(service.JWTConfig{
		Secret:            authUserContractSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	}), admin
}

func authUserContractAPI(name, method, path string) model.API {
	return model.API{
		Name:           name,
		Method:         method,
		Path:           path,
		PermissionCode: "contract." + strings.ToLower(method),
		Status:         1,
		NeedAuth:       1,
	}
}

func performAuthUserContractRequest(
	t *testing.T,
	r http.Handler,
	method, path string,
	body any,
	token string,
	wantStatus int,
) map[string]any {
	t.Helper()

	var requestBody []byte
	if body != nil {
		var err error
		requestBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s request: %v", method, path, err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(requestBody))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf(
			"%s %s status = %d, want %d; body=%s",
			method,
			path,
			w.Code,
			wantStatus,
			w.Body.String(),
		)
	}

	if w.Body.Len() == 0 {
		return map[string]any{}
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("%s %s response should be JSON: %v", method, path, err)
	}
	return response
}

func assertAuthUserContractCode(
	t *testing.T,
	response map[string]any,
	want int,
) {
	t.Helper()

	code, ok := response["code"].(float64)
	if !ok || int(code) != want {
		t.Fatalf("response code = %v, want %d", response["code"], want)
	}
}

func authUserContractObject(t *testing.T, value any, name string) map[string]any {
	t.Helper()

	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %T, want JSON object", name, value)
	}
	return object
}

func authUserContractString(t *testing.T, value any, name string) string {
	t.Helper()

	text, ok := value.(string)
	if !ok || text == "" {
		t.Fatalf("%s = %v, want non-empty string", name, value)
	}
	return text
}

type authUserContractRedisHook struct{}

func (authUserContractRedisHook) DialHook(
	next redis.DialHook,
) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (authUserContractRedisHook) ProcessHook(
	next redis.ProcessHook,
) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		switch strings.ToLower(cmd.Name()) {
		case "get":
			getCmd, ok := cmd.(*redis.StringCmd)
			if !ok {
				return fmt.Errorf("unexpected GET command type %T", cmd)
			}
			key := fmt.Sprint(cmd.Args()[1])
			if key == "captcha:"+authUserContractCaptchaID {
				getCmd.SetVal(authUserContractCaptchaCode)
				return nil
			}
			getCmd.SetErr(redis.Nil)
			return redis.Nil
		case "exists":
			existsCmd, ok := cmd.(*redis.IntCmd)
			if !ok {
				return fmt.Errorf("unexpected EXISTS command type %T", cmd)
			}
			existsCmd.SetVal(0)
			return nil
		case "del":
			delCmd, ok := cmd.(*redis.IntCmd)
			if !ok {
				return fmt.Errorf("unexpected DEL command type %T", cmd)
			}
			delCmd.SetVal(1)
			return nil
		default:
			return fmt.Errorf("unexpected Redis command %q", cmd.Name())
		}
	}
}

func (authUserContractRedisHook) ProcessPipelineHook(
	next redis.ProcessPipelineHook,
) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		return next(ctx, cmds)
	}
}
