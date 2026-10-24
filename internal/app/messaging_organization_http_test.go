package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	authdomain "admin/internal/authorization/domain"
	"admin/internal/identity"
	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/organization"
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

func TestMessagingAppHTTPResolvesSystemWideOrganizationScope(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}

	application, err := app.New(context.Background(), testAppConfig(), app.Resources{
		DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop(),
	}, app.Options{
		Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
		Middleware: app.HTTPMiddleware{
			Authenticated: func(c *gin.Context) {
				c.Set("userID", uint(1))
				c.Next()
			},
			PermissionControlled: func(c *gin.Context) { c.Next() },
		},
	})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	seedMessagingSystemWideFixture(t, db)
	handler := application.Handler()

	requestJSON := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	listCategories := requestJSON(http.MethodGet, "/api/admin/message-categories?page=1&size=20", "")
	if listCategories.Code != http.StatusOK {
		t.Fatalf("system-wide category list status=%d body=%s", listCategories.Code, listCategories.Body.String())
	}

	broadcast := requestJSON(http.MethodPost, "/api/admin/messages/broadcast", `{"category_code":"notice","title":"All users","markdown":"Broadcast body","all_users":true}`)
	if broadcast.Code != http.StatusOK {
		t.Fatalf("system-wide broadcast status=%d body=%s", broadcast.Code, broadcast.Body.String())
	}
	var broadcastPayload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(broadcast.Body.Bytes(), &broadcastPayload); err != nil || len(broadcastPayload.Data) != 2 {
		t.Fatalf("system-wide broadcast payload=%s err=%v", broadcast.Body.String(), err)
	}

	announcement := requestJSON(http.MethodPost, "/api/admin/announcements", `{"category_code":"notice","title":"System notice","markdown":"Announcement body","all_users":true}`)
	if announcement.Code != http.StatusOK {
		t.Fatalf("system-wide announcement status=%d body=%s", announcement.Code, announcement.Body.String())
	}
	var announcementPayload struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(announcement.Body.Bytes(), &announcementPayload); err != nil || len(announcementPayload.Data) != 2 || announcementPayload.Data[0].ID == 0 {
		t.Fatalf("system-wide announcement payload=%s err=%v", announcement.Body.String(), err)
	}

	published := requestJSON(http.MethodPost, "/api/admin/announcements/"+strconv.FormatUint(uint64(announcementPayload.Data[0].ID), 10)+"/publish", "")
	if published.Code != http.StatusOK {
		t.Fatalf("system-wide announcement publish status=%d body=%s", published.Code, published.Body.String())
	}
}

func seedMessagingSystemWideFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	user := identity.User{Model: gorm.Model{ID: 1}, Username: "messaging-admin", Password: "unused", Email: "messaging-admin@test.local", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create messaging admin: %v", err)
	}
	role := authgorm.Role{Name: "Messaging all", Code: "messaging-all", Status: 1, DataScope: string(authdomain.DataScopeAll)}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create messaging role: %v", err)
	}
	if err := db.Create(&authgorm.UserRole{UserID: user.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("assign messaging role: %v", err)
	}
	for _, code := range []string{
		"admin.messages.category.manage",
		"admin.messages.broadcast.manage",
		"admin.messages.announcement.manage",
	} {
		permission := authgorm.Permission{Name: code, Code: code, Group: "messaging"}
		if err := db.Create(&permission).Error; err != nil {
			t.Fatalf("create permission %q: %v", code, err)
		}
		if err := db.Create(&authgorm.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
			t.Fatalf("assign permission %q: %v", code, err)
		}
	}
	organizations := []organization.Unit{
		{Name: "Headquarters", Code: "hq", Status: 1},
		{Name: "Branch", Code: "branch", Status: 1},
	}
	if err := db.Create(&organizations).Error; err != nil {
		t.Fatalf("create organizations: %v", err)
	}
	categories := []messaginggorm.MessageCategory{
		{OrganizationID: organizations[0].ID, Code: "notice", Name: "Notice", Enabled: true},
		{OrganizationID: organizations[1].ID, Code: "notice", Name: "Notice", Enabled: true},
	}
	if err := db.Create(&categories).Error; err != nil {
		t.Fatalf("create message categories: %v", err)
	}
}
