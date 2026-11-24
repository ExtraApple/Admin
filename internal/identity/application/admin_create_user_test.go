package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

type countingPasswordHasher struct {
	calls int
	hash  string
	err   error
}

func (hasher *countingPasswordHasher) Compare(string, string) error { return nil }
func (hasher *countingPasswordHasher) Hash(string) (string, error) {
	hasher.calls++
	if hasher.err != nil {
		return "", hasher.err
	}
	if hasher.hash != "" {
		return hasher.hash, nil
	}
	return "bcrypt-hash", nil
}

type failingVerificationSender struct {
	called bool
}

func (sender *failingVerificationSender) SendVerification(context.Context, string, string) error {
	sender.called = true
	return errors.New("smtp should not be called by admin create")
}

type failingVerificationIssuer struct {
	called bool
}

func (issuer *failingVerificationIssuer) Issue(context.Context, uint, string) (string, error) {
	issuer.called = true
	return "", errors.New("issuer should not be called by admin create")
}
func (issuer *failingVerificationIssuer) Invalidate(context.Context, uint, string) error { return nil }

func TestUserServiceCreateByAdminPersistsIdentityAndInitialVersionWithoutEmailVerification(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&identitygorm.UserModel{}, &authgorm.UserAccessVersion{}); err != nil {
		t.Fatal(err)
	}
	repository := identitygorm.NewRepository(db)
	access := &lifecycleAccess{db: db}
	hasher := &countingPasswordHasher{hash: "$2a$10$fakehash"}
	service := application.NewUserService(repository, hasher, platformdatabase.NewTransactionRunner(db), access)
	issuer := &failingVerificationIssuer{}
	sender := &failingVerificationSender{}
	service.ConfigureEmailVerification(issuer, sender)

	ctx := context.Background()
	created, err := service.CreateByAdmin(ctx, 1, application.AdminCreateUserRequest{
		Username: "alice_ops",
		Password: "Password1!",
		Email:    "alice.ops@example.test",
		Nickname: "运维甲",
	})
	if err != nil {
		t.Fatalf("CreateByAdmin(default status) error = %v", err)
	}
	if created.ID == 0 || created.Username != "alice_ops" || created.Nickname != "运维甲" || created.Email != "alice.ops@example.test" || created.Status != 1 {
		t.Fatalf("created = %#v", created)
	}
	if created.Avatar != "/api/avatars/default" || created.AccessVersion != 1 || created.HasUnmanagedOrganizations || created.Roles == nil || len(created.Roles) != 0 || created.Organizations == nil || len(created.Organizations) != 0 {
		t.Fatalf("created projection = %#v", created)
	}
	if issuer.called || sender.called {
		t.Fatal("CreateByAdmin triggered email verification")
	}

	var stored identitygorm.UserModel
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Role != "user" || stored.Password != "$2a$10$fakehash" || stored.EmailVerifiedAt != nil || stored.PendingEmail != "" || stored.Status != 1 {
		t.Fatalf("stored default user = %#v", stored)
	}

	disabled := 0
	createdDisabled, err := service.CreateByAdmin(ctx, 1, application.AdminCreateUserRequest{
		Username: "bob_disabled",
		Password: "Password1!",
		Email:    "bob.disabled@example.test",
		Status:   &disabled,
	})
	if err != nil {
		t.Fatalf("CreateByAdmin(status=0) error = %v", err)
	}
	if createdDisabled.Status != 0 {
		t.Fatalf("createdDisabled.Status = %d, want 0", createdDisabled.Status)
	}
	var storedDisabled identitygorm.UserModel
	if err := db.First(&storedDisabled, createdDisabled.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedDisabled.Status != 0 {
		t.Fatalf("storedDisabled.Status in DB = %d, want 0", storedDisabled.Status)
	}
}

func TestUserServiceCreateByAdminRejectsNonAllScopeBeforeConflictAndHashing(t *testing.T) {
	repository := &managementRepositoryFake{identityConflict: true}
	hasher := &countingPasswordHasher{}
	access := &accessManagementFake{scope: domain.UserScope{All: false, UserIDs: []uint{7}}}
	service := application.NewUserService(repository, hasher, &transactionFake{}, access)

	_, err := service.CreateByAdmin(context.Background(), 7, application.AdminCreateUserRequest{
		Username: "existing",
		Password: "Password1!",
		Email:    "existing@example.test",
	})
	if code, ok := application.CodeOf(err); !ok || code != application.CodePermissionDenied {
		t.Fatalf("CreateByAdmin(non-all) err = %v (code %q), want permission denied", err, code)
	}
	if hasher.calls != 0 || repository.createCalls != 0 {
		t.Fatalf("hasher.calls=%d createCalls=%d, want 0", hasher.calls, repository.createCalls)
	}

	downgraded := domain.UserScope{All: false, UserIDs: []uint{7}}
	access = &accessManagementFake{
		scope:           domain.UserScope{All: true},
		managementScope: &downgraded,
	}
	service = application.NewUserService(repository, hasher, &transactionFake{}, access)
	_, err = service.CreateByAdmin(context.Background(), 7, application.AdminCreateUserRequest{
		Username: "existing",
		Password: "Password1!",
		Email:    "existing@example.test",
	})
	if code, ok := application.CodeOf(err); !ok || code != application.CodePermissionDenied {
		t.Fatalf("CreateByAdmin(concurrent scope downgrade) err = %v (code %q), want permission denied", err, code)
	}
	if repository.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0 after tx scope downgrade", repository.createCalls)
	}
}

func TestUserServiceCreateByAdminDetectsActivePendingSoftDeletedAndRaceConflicts(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&identitygorm.UserModel{}, &authgorm.UserAccessVersion{}); err != nil {
		t.Fatal(err)
	}
	repository := identitygorm.NewRepository(db)
	ctx := context.Background()

	active := domain.User{Username: "active_user", Password: "h", Email: "active@example.test", PendingEmail: "pending@example.test", Role: "user", Status: 1}
	deleted := domain.User{Username: "deleted_user", Password: "h", Email: "deleted@example.test", Role: "user", Status: 1}
	if err := repository.Create(ctx, &active); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, &deleted); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, deleted.ID); err != nil {
		t.Fatal(err)
	}

	service := application.NewUserService(repository, passwordFake{}, platformdatabase.NewTransactionRunner(db), &lifecycleAccess{db: db})
	cases := []struct {
		name     string
		username string
		email    string
	}{
		{"active username", "active_user", "fresh1@example.test"},
		{"active email", "fresh1", "active@example.test"},
		{"active pending email", "fresh2", "pending@example.test"},
		{"soft-deleted username", "deleted_user", "fresh3@example.test"},
		{"soft-deleted email", "fresh4", "deleted@example.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateByAdmin(ctx, 1, application.AdminCreateUserRequest{
				Username: tc.username,
				Password: "Password1!",
				Email:    tc.email,
			})
			if code, ok := application.CodeOf(err); !ok || code != application.CodeConflict {
				t.Fatalf("CreateByAdmin(%s) err = %v (code %q), want conflict", tc.name, err, code)
			}
		})
	}

	// Simulate concurrent insert unique constraint race.
	raceRepo := &managementRepositoryFake{
		identityConflictValues: []bool{false, true},
		createErr:              errors.New("UNIQUE constraint failed"),
	}
	raceService := application.NewUserService(raceRepo, passwordFake{}, &transactionFake{}, &accessManagementFake{})
	_, err := raceService.CreateByAdmin(ctx, 1, application.AdminCreateUserRequest{
		Username: "race_user",
		Password: "Password1!",
		Email:    "race@example.test",
	})
	if code, ok := application.CodeOf(err); !ok || code != application.CodeConflict {
		t.Fatalf("CreateByAdmin(unique race) err = %v (code %q), want conflict", err, code)
	}
}

func TestUserServiceCreateByAdminEnforcesSharedPasswordAndBlankValidation(t *testing.T) {
	repository := &managementRepositoryFake{}
	hasher := &countingPasswordHasher{}
	service := application.NewUserService(repository, hasher, &transactionFake{}, &accessManagementFake{})

	oversized := "Aa1!" + strings.Repeat("密", 23) // 4 + 69 = 73 UTF-8 bytes, 27 chars
	for _, password := range []string{"abcdef", oversized} {
		_, err := service.CreateByAdmin(context.Background(), 1, application.AdminCreateUserRequest{
			Username: "valid_user",
			Password: password,
			Email:    "valid@example.test",
		})
		code, ok := application.CodeOf(err)
		details, _ := application.DetailsOf(err)
		if !ok || code != application.CodeValidationInvalid || len(details.Fields) != 1 || details.Fields[0].Field != "password" || details.Fields[0].ErrorCode != "IDENTITY_PASSWORD_INVALID" {
			t.Fatalf("CreateByAdmin(password=%q) err=%v code=%q fields=%#v", password, err, code, details.Fields)
		}
	}
	if hasher.calls != 0 {
		t.Fatalf("hasher.calls = %d, want 0 on password validation failure", hasher.calls)
	}

	for _, req := range []application.AdminCreateUserRequest{
		{Username: "   ", Password: "Password1!", Email: "blank@example.test"},
		{Username: "valid_user", Password: "Password1!", Email: "blank@example.test", Nickname: "   "},
	} {
		_, err := service.CreateByAdmin(context.Background(), 1, req)
		if code, ok := application.CodeOf(err); !ok || code != application.CodeValidationInvalid {
			t.Fatalf("CreateByAdmin(blank input %#v) err=%v code=%q, want validation invalid", req, err, code)
		}
	}
}
