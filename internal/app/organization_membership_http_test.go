package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	authdomain "admin/internal/authorization/domain"
	"admin/internal/identity"
	identityjwt "admin/internal/identity/adapters/jwt"
	identityapplication "admin/internal/identity/application"
	identitydomain "admin/internal/identity/domain"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type organizationJWTFixture struct {
	t           *testing.T
	db          *gorm.DB
	application *app.Application
	tokens      *identityjwt.Service
	admin       identity.User
	adminToken  string
}

func TestOrganizationHTTPOverwritesMembersAndInvalidatesOldTokens(t *testing.T) {
	fixture := newOrganizationJWTFixture(t)
	first := fixture.createUser("organization-member-one")
	second := fixture.createUser("organization-member-two")
	firstOldToken := fixture.token(first)

	unit := fixture.post("/api/admin/organizations", map[string]any{"name": "Members", "code": "members"})
	unitID := uint(unit["id"].(float64))
	assigned := fixture.request(http.MethodPost, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, map[string]any{
		"user_ids": []uint{first.ID, second.ID, second.ID},
	})
	if assigned.Code != http.StatusOK {
		t.Fatalf("assign Organization Members status = %d body=%s", assigned.Code, assigned.Body.String())
	}
	assertOrganizationMemberIDs(t, fixture.request(http.MethodGet, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, nil), []uint{first.ID, second.ID})

	invalidated := fixture.request(http.MethodGet, "/api/admin/organizations", firstOldToken, nil)
	if invalidated.Code != http.StatusUnauthorized {
		t.Fatalf("old member token status = %d, want 401; body=%s", invalidated.Code, invalidated.Body.String())
	}

	secondCurrentToken := fixture.token(second)
	replaced := fixture.request(http.MethodPost, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, map[string]any{
		"user_ids": []uint{first.ID},
	})
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace Organization Members status = %d body=%s", replaced.Code, replaced.Body.String())
	}
	assertOrganizationMemberIDs(t, fixture.request(http.MethodGet, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, nil), []uint{first.ID})
	if response := fixture.request(http.MethodGet, "/api/admin/organizations", secondCurrentToken, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("removed member old token status = %d, want 401", response.Code)
	}

	invalidAssignment := fixture.request(http.MethodPost, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, map[string]any{
		"user_ids": []uint{999999},
	})
	if invalidAssignment.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid Organization Member status = %d, want 422", invalidAssignment.Code)
	}
	assertOrganizationMemberIDs(t, fixture.request(http.MethodGet, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, nil), []uint{first.ID})

	firstCurrentToken := fixture.token(first)
	deleted := fixture.request(http.MethodDelete, "/api/admin/organizations/"+itoa(unitID), fixture.adminToken, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete Organization with Members status = %d body=%s", deleted.Code, deleted.Body.String())
	}
	if response := fixture.request(http.MethodGet, "/api/admin/organizations", firstCurrentToken, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("deleted Organization Member old token status = %d, want 401", response.Code)
	}
}

func TestOrganizationUsersUseIdentityDirectoryAvatars(t *testing.T) {
	fixture := newOrganizationJWTFixture(t)
	trusted := fixture.createUser("organization-trusted")
	untrusted := fixture.createUser("organization-untrusted")
	invalidMetadata := fixture.createUser("organization-invalid-metadata")
	trustedObject := "avatars/" + itoa(trusted.ID) + "/00000000-0000-4000-8000-000000000008.png"
	if err := fixture.db.Model(&trusted).Updates(map[string]any{
		"avatar": "https://legacy.example/trusted.png", "avatar_object_name": trustedObject,
		"avatar_content_type": "image/png", "avatar_validation_status": identitydomain.AvatarValidationStatusValidated,
	}).Error; err != nil {
		t.Fatalf("trust organization avatar: %v", err)
	}
	if err := fixture.db.Model(&untrusted).Update("avatar", "https://legacy.example/untrusted.png").Error; err != nil {
		t.Fatalf("set legacy organization avatar: %v", err)
	}
	invalidObject := "avatars/" + itoa(invalidMetadata.ID) + "/00000000-0000-4000-8000-000000000011.png"
	if err := fixture.db.Model(&invalidMetadata).Updates(map[string]any{
		"avatar": "https://legacy.example/invalid.png", "avatar_object_name": invalidObject,
		"avatar_content_type": "image/jpeg", "avatar_validation_status": identitydomain.AvatarValidationStatusValidated,
	}).Error; err != nil {
		t.Fatalf("set invalid organization avatar metadata: %v", err)
	}
	unit := fixture.post("/api/admin/organizations", map[string]any{"name": "Directory Members", "code": "directory-members"})
	unitID := uint(unit["id"].(float64))
	empty := fixture.post("/api/admin/organizations", map[string]any{"name": "Empty Directory Members", "code": "empty-directory-members"})
	emptyID := uint(empty["id"].(float64))
	assigned := fixture.request(http.MethodPost, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, map[string]any{"user_ids": []uint{trusted.ID, untrusted.ID, invalidMetadata.ID}})
	if assigned.Code != http.StatusOK {
		t.Fatalf("assign directory members status=%d body=%s", assigned.Code, assigned.Body.String())
	}

	response := fixture.request(http.MethodGet, "/api/admin/organizations/"+itoa(unitID)+"/users", fixture.adminToken, nil)
	var payload struct {
		Data []struct {
			ID     uint   `json:"id"`
			Avatar string `json:"avatar"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode organization users: %v", err)
	}
	if response.Code != http.StatusOK || len(payload.Data) != 3 || payload.Data[0].ID != trusted.ID || payload.Data[0].Avatar != "/api/avatars/"+itoa(trusted.ID) || payload.Data[1].ID != untrusted.ID || payload.Data[1].Avatar != "/api/avatars/default" || payload.Data[2].ID != invalidMetadata.ID || payload.Data[2].Avatar != "/api/avatars/default" {
		t.Fatalf("organization users status=%d data=%#v body=%s", response.Code, payload.Data, response.Body.String())
	}

	emptyResponse := fixture.request(http.MethodGet, "/api/admin/organizations/"+itoa(emptyID)+"/users", fixture.adminToken, nil)
	var emptyPayload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(emptyResponse.Body.Bytes(), &emptyPayload); err != nil {
		t.Fatalf("decode empty organization users: %v", err)
	}
	if emptyResponse.Code != http.StatusOK || emptyPayload.Data == nil || len(emptyPayload.Data) != 0 {
		t.Fatalf("empty organization users status=%d data=%#v body=%s", emptyResponse.Code, emptyPayload.Data, emptyResponse.Body.String())
	}
}

func TestOrganizationHTTPAppliesScopeAndPreservesVisibleAncestors(t *testing.T) {
	fixture := newOrganizationJWTFixture(t)
	root := fixture.post("/api/admin/organizations", map[string]any{"name": "Root", "code": "scope-root", "sort": 20})
	rootID := uint(root["id"].(float64))
	child := fixture.post("/api/admin/organizations", map[string]any{"parent_id": rootID, "name": "Child", "code": "scope-child", "sort": 10})
	childID := uint(child["id"].(float64))
	fixture.post("/api/admin/organizations", map[string]any{"name": "Other", "code": "scope-other", "sort": 5})

	scoped := fixture.createUser("organization-scoped-user")
	role := authgorm.Role{Name: "Organization Scope", Code: "organization-scope", Status: 1, DataScope: string(authdomain.DataScopeOrg)}
	if err := fixture.db.Create(&role).Error; err != nil {
		t.Fatalf("create scoped role: %v", err)
	}
	if err := fixture.db.Create(&authgorm.UserRole{UserID: scoped.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("assign scoped role: %v", err)
	}
	assigned := fixture.request(http.MethodPost, "/api/admin/organizations/"+itoa(childID)+"/users", fixture.adminToken, map[string]any{"user_ids": []uint{scoped.ID}})
	if assigned.Code != http.StatusOK {
		t.Fatalf("assign scoped Organization Member status = %d body=%s", assigned.Code, assigned.Body.String())
	}
	scopedToken := fixture.token(scoped)

	listResponse := fixture.request(http.MethodGet, "/api/admin/organizations", scopedToken, nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("scoped Organization list status = %d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listPayload struct {
		Data struct {
			List []struct {
				ID uint `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode scoped Organization list: %v", err)
	}
	if len(listPayload.Data.List) != 1 || listPayload.Data.List[0].ID != childID {
		t.Fatalf("scoped Organization list = %#v, want child only", listPayload.Data.List)
	}

	treeResponse := fixture.request(http.MethodGet, "/api/admin/organizations/tree", scopedToken, nil)
	if treeResponse.Code != http.StatusOK {
		t.Fatalf("scoped Organization tree status = %d body=%s", treeResponse.Code, treeResponse.Body.String())
	}
	var treePayload struct {
		Data []struct {
			ID       uint `json:"id"`
			Children []struct {
				ID uint `json:"id"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(treeResponse.Body.Bytes(), &treePayload); err != nil {
		t.Fatalf("decode scoped Organization tree: %v", err)
	}
	if len(treePayload.Data) != 1 || treePayload.Data[0].ID != rootID || len(treePayload.Data[0].Children) != 1 || treePayload.Data[0].Children[0].ID != childID {
		t.Fatalf("scoped Organization tree = %#v, want root ancestor with child", treePayload.Data)
	}
}

func newOrganizationJWTFixture(t *testing.T) *organizationJWTFixture {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App database: %v", err)
	}
	admin := identity.User{Username: "organization-jwt-admin", Password: "unused", Email: "organization-jwt-admin@test.local", Status: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create Organization JWT admin: %v", err)
	}
	role := authgorm.Role{Name: "Organization JWT Admin", Code: "admin", Status: 1, DataScope: string(authdomain.DataScopeAll)}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create Organization JWT admin role: %v", err)
	}
	if err := db.Create(&authgorm.UserRole{UserID: admin.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("assign Organization JWT admin role: %v", err)
	}
	if err := db.Create(&authgorm.UserAccessVersion{UserID: admin.ID, Version: 1}).Error; err != nil {
		t.Fatalf("create Organization JWT admin access version: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	conf := testAppConfig()
	conf.Jwt.Expire = 30
	conf.Jwt.RefreshExpire = 60
	tokens := identityjwt.NewService(identityjwt.Config{Secret: conf.Jwt.Secret, AccessExpireMins: conf.Jwt.Expire, RefreshExpireMins: conf.Jwt.RefreshExpire})
	authenticate := func(c *gin.Context) {
		token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		claims, parseErr := tokens.Parse(c.Request.Context(), token)
		var accessVersion authgorm.UserAccessVersion
		versionErr := db.First(&accessVersion, "user_id = ?", claims.UserID).Error
		if parseErr != nil || claims.Purpose != identityapplication.TokenPurposeAccess || versionErr != nil || accessVersion.Version != claims.TokenVersion {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("userID", claims.UserID)
		c.Next()
	}
	application, err := app.New(context.Background(), conf, app.Resources{
		DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop(),
	}, app.Options{
		Middleware: app.HTTPMiddleware{Authenticated: authenticate, PermissionControlled: func(c *gin.Context) { c.Next() }},
		Seed:       func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
	})
	if err != nil {
		t.Fatalf("assemble Organization JWT App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	fixture := &organizationJWTFixture{t: t, db: db, application: application, tokens: tokens, admin: admin}
	fixture.adminToken = fixture.token(admin)
	return fixture
}

func (fixture *organizationJWTFixture) createUser(username string) identity.User {
	fixture.t.Helper()
	user := identity.User{Username: username, Password: "unused", Email: username + "@test.local", Status: 1}
	if err := fixture.db.Create(&user).Error; err != nil {
		fixture.t.Fatalf("create Organization Member %q: %v", username, err)
	}
	if err := fixture.db.Create(&authgorm.UserAccessVersion{UserID: user.ID, Version: 1}).Error; err != nil {
		fixture.t.Fatalf("create Organization Member access version %q: %v", username, err)
	}
	return user
}

func (fixture *organizationJWTFixture) token(user identity.User) string {
	fixture.t.Helper()
	var accessVersion authgorm.UserAccessVersion
	if err := fixture.db.First(&accessVersion, "user_id = ?", user.ID).Error; err != nil {
		fixture.t.Fatalf("load Organization user access version: %v", err)
	}
	pair, err := fixture.tokens.Issue(context.Background(), identityapplication.TokenIssue{UserID: user.ID, TokenVersion: accessVersion.Version})
	if err != nil {
		fixture.t.Fatalf("generate Organization access token: %v", err)
	}
	return pair.AccessToken
}

func (fixture *organizationJWTFixture) post(path string, body map[string]any) map[string]any {
	fixture.t.Helper()
	response := fixture.request(http.MethodPost, path, fixture.adminToken, body)
	if response.Code != http.StatusOK {
		fixture.t.Fatalf("POST %s status = %d body=%s", path, response.Code, response.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		fixture.t.Fatalf("decode POST %s: %v", path, err)
	}
	return payload.Data
}

func (fixture *organizationJWTFixture) request(method, path, token string, body map[string]any) *httptest.ResponseRecorder {
	fixture.t.Helper()
	if token == "" {
		return requestOrganization(fixture.t, fixture.application.Handler(), method, path, body)
	}
	request := httptest.NewRequest(method, path, nil)
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			fixture.t.Fatalf("encode authenticated %s %s: %v", method, path, err)
		}
		request = httptest.NewRequest(method, path, bytes.NewReader(encoded))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	fixture.application.Handler().ServeHTTP(response, request)
	return response
}

func assertOrganizationMemberIDs(t *testing.T, response *httptest.ResponseRecorder, want []uint) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("Organization Members status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode Organization Members: %v", err)
	}
	if len(payload.Data) != len(want) {
		t.Fatalf("Organization Member IDs = %#v, want %v", payload.Data, want)
	}
	for index, id := range want {
		if payload.Data[index].ID != id {
			t.Fatalf("Organization Member IDs = %#v, want %v", payload.Data, want)
		}
	}
}
