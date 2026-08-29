package identity_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin/internal/identity"
	identityapplication "admin/internal/identity/application"
	identitydomain "admin/internal/identity/domain"
	"admin/testsupport/testutil"
)

func TestIdentityOwnsUsersTableAndAuthenticationDTOs(t *testing.T) {
	models := identity.Models()
	if len(models) != 2 {
		t.Fatalf("Identity models = %d, want user and email credential models", len(models))
	}
	if reflect.TypeOf(models[0]) != reflect.TypeOf(identity.User{}) || reflect.TypeOf(models[1]) != reflect.TypeOf(identity.EmailVerificationCredential{}) {
		t.Fatalf("Identity models = %T, %T", models[0], models[1])
	}

	request := identity.RegisterRequest{
		Username:    "alice",
		Password:    "ValidPass123!",
		Email:       "alice@example.com",
		Nickname:    "Alice",
		CaptchaID:   "captcha-id",
		CaptchaCode: "123456",
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal RegisterRequest: %v", err)
	}
	want := `{"username":"alice","password":"ValidPass123!","email":"alice@example.com","nickname":"Alice","captcha_id":"captcha-id","captcha_code":"123456"}`
	if string(encoded) != want {
		t.Fatalf("RegisterRequest JSON = %s, want %s", encoded, want)
	}

	var update identity.UpdateSelfRequest
	if err := json.Unmarshal([]byte(`{"nickname":"after","avatar":null}`), &update); err != nil {
		t.Fatalf("unmarshal UpdateSelfRequest: %v", err)
	}
	if !update.HasAvatarField() {
		t.Fatal("UpdateSelfRequest did not preserve explicit avatar field")
	}
}

func TestUserInfoFromDomainExposesOnlyTrustedAvatarRoutes(t *testing.T) {
	const trustedObject = "avatars/7/00000000-0000-4000-8000-000000000007.png"
	tests := []struct {
		name       string
		user       identitydomain.User
		wantAvatar string
	}{
		{
			name:       "trusted object",
			user:       identitydomain.User{ID: 7, Avatar: "https://legacy.example/trusted.png", AvatarObjectName: trustedObject, AvatarContentType: "image/png", AvatarValidationStatus: identitydomain.AvatarValidationStatusValidated},
			wantAvatar: "/api/avatars/7",
		},
		{
			name:       "legacy URL only",
			user:       identitydomain.User{ID: 7, Avatar: "https://legacy.example/avatar.png"},
			wantAvatar: "/api/avatars/default",
		},
		{
			name:       "object owned by another user",
			user:       identitydomain.User{ID: 7, AvatarObjectName: "avatars/8/00000000-0000-4000-8000-000000000007.png", AvatarContentType: "image/png", AvatarValidationStatus: identitydomain.AvatarValidationStatusValidated},
			wantAvatar: "/api/avatars/default",
		},
		{
			name:       "object not validated",
			user:       identitydomain.User{ID: 7, AvatarObjectName: trustedObject, AvatarContentType: "image/png"},
			wantAvatar: "/api/avatars/default",
		},
		{
			name:       "MIME does not match suffix",
			user:       identitydomain.User{ID: 7, AvatarObjectName: trustedObject, AvatarContentType: "image/jpeg", AvatarValidationStatus: identitydomain.AvatarValidationStatusValidated},
			wantAvatar: "/api/avatars/default",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := identity.UserInfoFromDomain(test.user)
			if got.Avatar != test.wantAvatar {
				t.Fatalf("UserInfoFromDomain() avatar = %q, want %q", got.Avatar, test.wantAvatar)
			}
		})
	}
}

type projectionDirectoryRepository struct {
	record identityapplication.DirectoryUserRecord
}

func (repository projectionDirectoryRepository) ListUsersByIDs(context.Context, []uint) ([]identityapplication.DirectoryUserRecord, error) {
	return []identityapplication.DirectoryUserRecord{repository.record}, nil
}

func (repository projectionDirectoryRepository) ListUserIDs(context.Context) ([]uint, error) {
	return []uint{repository.record.ID}, nil
}

func TestIdentityPublicUserProjectionStaysConsistentAcrossFormalPaths(t *testing.T) {
	const trustedObject = "avatars/7/00000000-0000-4000-8000-000000000007.png"
	tests := []struct {
		name          string
		user          identitydomain.User
		avatarTrusted bool
		wantAvatar    string
	}{
		{
			name:          "trusted object",
			user:          identitydomain.User{ID: 7, Username: "alice", Nickname: "Alice", Email: "alice@example.com", Role: "user", Status: 1, Avatar: "https://legacy.example/trusted.png", AvatarObjectName: trustedObject, AvatarContentType: "image/png", AvatarValidationStatus: identitydomain.AvatarValidationStatusValidated},
			avatarTrusted: true,
			wantAvatar:    "/api/avatars/7",
		},
		{
			name:       "legacy URL only",
			user:       identitydomain.User{ID: 7, Username: "alice", Nickname: "Alice", Email: "alice@example.com", Role: "user", Status: 1, Avatar: "https://legacy.example/avatar.png"},
			wantAvatar: "/api/avatars/default",
		},
		{
			name:       "object owned by another user",
			user:       identitydomain.User{ID: 7, Username: "alice", Nickname: "Alice", Email: "alice@example.com", Role: "user", Status: 1, AvatarObjectName: "avatars/8/00000000-0000-4000-8000-000000000007.png", AvatarContentType: "image/png", AvatarValidationStatus: identitydomain.AvatarValidationStatusValidated},
			wantAvatar: "/api/avatars/default",
		},
		{
			name:       "object not validated",
			user:       identitydomain.User{ID: 7, Username: "alice", Nickname: "Alice", Email: "alice@example.com", Role: "user", Status: 1, AvatarObjectName: trustedObject, AvatarContentType: "image/png"},
			wantAvatar: "/api/avatars/default",
		},
		{
			name:       "MIME does not match suffix",
			user:       identitydomain.User{ID: 7, Username: "alice", Nickname: "Alice", Email: "alice@example.com", Role: "user", Status: 1, AvatarObjectName: trustedObject, AvatarContentType: "image/jpeg", AvatarValidationStatus: identitydomain.AvatarValidationStatusValidated},
			wantAvatar: "/api/avatars/default",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := projectionDirectoryRepository{record: identityapplication.DirectoryUserRecord{
				ID: test.user.ID, Username: test.user.Username, Nickname: test.user.Nickname,
				Email: test.user.Email, Role: test.user.Role, Status: test.user.Status,
				AvatarTrusted: test.avatarTrusted,
			}}
			directoryUsers, err := identityapplication.NewDirectoryService(repository).ListUsersByIDs(context.Background(), []uint{test.user.ID})
			if err != nil || len(directoryUsers) != 1 {
				t.Fatalf("DirectoryService.ListUsersByIDs() users = %#v, error = %v", directoryUsers, err)
			}
			want := identity.UserInfo{ID: 7, Username: "alice", Nickname: "Alice", Avatar: test.wantAvatar, Email: "alice@example.com", Role: "user", Status: 1}
			dto := identity.UserInfoFromDomain(test.user)
			if !reflect.DeepEqual(dto, want) {
				t.Fatalf("Identity DTO projection = %#v, want %#v", dto, want)
			}
			directory := directoryUsers[0]
			directoryProjection := identity.UserInfo{ID: directory.ID, Username: directory.Username, Nickname: directory.Nickname, Avatar: directory.Avatar, Email: directory.Email, Role: directory.Role, Status: directory.Status}
			if !reflect.DeepEqual(directoryProjection, want) {
				t.Fatalf("UserDirectory projection = %#v, want %#v", directoryProjection, want)
			}
		})
	}
}

func TestIdentityModelsPersistEmailVerificationStateAndCredential(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identity.Models()...); err != nil {
		t.Fatalf("migrate identity models: %v", err)
	}
	if !db.Migrator().HasTable("users") || !db.Migrator().HasTable("email_verification_credentials") {
		t.Fatal("identity migration did not create email verification tables")
	}
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	user := identity.User{Username: "alice", Password: "hash", Email: "alice@example.com", PendingEmail: "new@example.com", EmailVerifiedAt: &verifiedAt, Role: "user", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	credential := identity.EmailVerificationCredential{UserID: user.ID, Email: "new@example.com", TokenHash: "hash-of-token", ExpiresAt: verifiedAt.Add(15 * time.Minute)}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	var loaded identity.User
	if err := db.First(&loaded, user.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if loaded.PendingEmail != "new@example.com" || loaded.EmailVerifiedAt == nil || !loaded.EmailVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("email state = %+v", loaded)
	}
	var loadedCredential identity.EmailVerificationCredential
	if err := db.First(&loadedCredential, credential.ID).Error; err != nil {
		t.Fatalf("load credential: %v", err)
	}
	if loadedCredential.UserID != user.ID || loadedCredential.Email != "new@example.com" || loadedCredential.TokenHash != "hash-of-token" || loadedCredential.UsedAt != nil {
		t.Fatalf("credential = %+v", loadedCredential)
	}
}

func TestUserInfoIncludesEmailVerificationStateWithoutCredentialFields(t *testing.T) {
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	data, err := json.Marshal(identity.UserInfoFromDomain(identitydomain.User{ID: 7, Email: "old@example.com", PendingEmail: "new@example.com", EmailVerifiedAt: &verifiedAt}))
	if err != nil {
		t.Fatalf("marshal user info: %v", err)
	}
	encoded := string(data)
	for _, field := range []string{`"email":"old@example.com"`, `"pending_email":"new@example.com"`, `"email_verified":true`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("user info missing %s: %s", field, encoded)
		}
	}
	for _, secret := range []string{"token", "expires_at", "smtp", "password"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("user info leaked %s: %s", secret, encoded)
		}
	}
}
