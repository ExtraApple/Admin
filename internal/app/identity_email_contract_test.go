package app

import (
	"context"
	"testing"
	"time"

	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/identity/domain"
	messagingapplication "admin/internal/messaging/application"
	"admin/testsupport/testutil"
)

func TestMessagingIdentityReaderReturnsOnlyVerifiedEmail(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	verified := domain.User{Username: "verified", Password: "hash", Email: "verified@example.com", Status: 1}
	unverified := domain.User{Username: "unverified", Password: "hash", Email: "unverified@example.com", Status: 1}
	if err := repository.Create(context.Background(), &verified); err != nil {
		t.Fatalf("create verified user: %v", err)
	}
	if err := repository.Create(context.Background(), &unverified); err != nil {
		t.Fatalf("create unverified user: %v", err)
	}
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := db.Model(&identitygorm.UserModel{}).Where("id = ?", verified.ID).Update("email_verified_at", verifiedAt).Error; err != nil {
		t.Fatalf("verify user: %v", err)
	}

	reader := messagingIdentityReader{users: repository}
	email, eligible, err := reader.LookupVerifiedEmail(context.Background(), verified.ID)
	if err != nil || !eligible || email.Address != "verified@example.com" {
		t.Fatalf("verified email = %#v eligible=%v error=%v", email, eligible, err)
	}
	_, eligible, err = reader.LookupVerifiedEmail(context.Background(), unverified.ID)
	if err != nil || eligible {
		t.Fatalf("unverified email eligible=%v error=%v", eligible, err)
	}
}

var _ messagingapplication.IdentityReader = messagingIdentityReader{}
