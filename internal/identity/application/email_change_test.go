package application_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	identitymodule "admin/internal/identity"
	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

func TestUpdateSelfCreatesPendingEmailAfterPasswordReauthentication(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7, Password: "old-hash", Email: "old@example.com", EmailVerifiedAt: timePointer()}}}
	issuer := &registrationVerificationIssuerFake{token: "verification-token"}
	sender := &verificationEmailSenderFake{}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, &accessManagementFake{})
	service.ConfigureEmailVerification(issuer, sender)

	user, err := service.UpdateSelf(context.Background(), 7, application.UpdateSelfRequest{Email: "new@example.com", CurrentPassword: "OldPass123!"})
	if err != nil {
		t.Fatalf("UpdateSelf() error = %v", err)
	}
	if user.Email != "old@example.com" || repository.changes.Email != nil || repository.changes.PendingEmail == nil || *repository.changes.PendingEmail != "new@example.com" || repository.changes.ClearEmailVerifiedAt {
		t.Fatalf("verified email change = user:%#v changes:%#v", user, repository.changes)
	}
	if sender.recipient != "new@example.com" || issuer.issueCalls != 1 {
		t.Fatalf("verification delivery = recipient:%q issue_calls:%d", sender.recipient, issuer.issueCalls)
	}
}

func TestUpdateSelfLocksUserBeforeEmailStateTransition(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7, Password: "old-hash", Email: "old@example.com", EmailVerifiedAt: timePointer()}}}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, &accessManagementFake{})
	service.ConfigureEmailVerification(&registrationVerificationIssuerFake{token: "verification-token"}, &verificationEmailSenderFake{})
	if _, err := service.UpdateSelf(context.Background(), 7, application.UpdateSelfRequest{Email: "new@example.com", CurrentPassword: "OldPass123!"}); err != nil {
		t.Fatalf("UpdateSelf() error = %v", err)
	}
	if repository.lockCalls != 1 {
		t.Fatalf("user lock calls = %d, want 1", repository.lockCalls)
	}
}

func TestEmailConfirmationAndReplacementSerializeOnUserRow(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitymodule.Models()...); err != nil {
		t.Fatalf("migrate identity models: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	user := domain.User{Username: "cross-concurrent", Password: "old-hash", Email: "old@example.com", PendingEmail: "candidate@example.com", EmailVerifiedAt: &verifiedAt, Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	rawToken := bytes.Repeat([]byte{0x42}, 32)
	digest := sha256.Sum256(rawToken)
	if err := db.Create(&identitymodule.EmailVerificationCredential{UserID: user.ID, Email: user.PendingEmail, TokenHash: hex.EncodeToString(digest[:]), ExpiresAt: verifiedAt.Add(15 * time.Minute)}).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	tx := platformdatabase.NewTransactionRunner(db)
	verification := application.NewEmailVerificationService(repository, tx, application.WithEmailVerificationClock(func() time.Time { return verifiedAt }))
	users := application.NewUserService(repository, passwordFake{}, tx, &accessManagementFake{})
	users.ConfigureEmailVerification(verification, &verificationEmailSenderFake{})
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	confirmResults := make(chan error, 1)
	updateResults := make(chan error, 1)
	go func() { confirmResults <- verification.Confirm(context.Background(), user.ID, token) }()
	go func() {
		_, err := users.UpdateSelf(context.Background(), user.ID, application.UpdateSelfRequest{Email: "replacement@example.com", CurrentPassword: "OldPass123!"})
		updateResults <- err
	}()
	confirmErr := <-confirmResults
	updateErr := <-updateResults
	if updateErr != nil {
		t.Fatalf("concurrent email replacement error = %v", updateErr)
	}
	if confirmErr != nil {
		if code, _ := application.CodeOf(confirmErr); code != application.CodeEmailVerificationInvalid {
			t.Fatalf("concurrent confirmation error = %v", confirmErr)
		}
	}
	loaded, err := repository.FindByID(context.Background(), user.ID)
	if err != nil || loaded.Email != "candidate@example.com" || loaded.PendingEmail != "replacement@example.com" || loaded.EmailVerifiedAt == nil {
		t.Fatalf("cross-concurrent final user = %#v error=%v", loaded, err)
	}
}

func TestUpdateSelfAtomicallyReplacesUnverifiedCurrentEmail(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7, Password: "old-hash", Email: "old@example.com"}}}
	issuer := &registrationVerificationIssuerFake{token: "verification-token"}
	sender := &verificationEmailSenderFake{}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, &accessManagementFake{})
	service.ConfigureEmailVerification(issuer, sender)

	if _, err := service.UpdateSelf(context.Background(), 7, application.UpdateSelfRequest{Email: "new@example.com", CurrentPassword: "OldPass123!"}); err != nil {
		t.Fatalf("UpdateSelf() error = %v", err)
	}
	if repository.changes.Email == nil || *repository.changes.Email != "new@example.com" || repository.changes.PendingEmail == nil || *repository.changes.PendingEmail != "" || !repository.changes.ClearEmailVerifiedAt {
		t.Fatalf("unverified email replacement changes = %#v", repository.changes)
	}
}

func TestUpdateSelfRejectsEmailChangeWithInvalidCurrentPassword(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7, Password: "old-hash", Email: "old@example.com", EmailVerifiedAt: timePointer()}}}
	service := application.NewUserService(repository, rejectingPasswordFake{}, &transactionFake{}, &accessManagementFake{})

	_, err := service.UpdateSelf(context.Background(), 7, application.UpdateSelfRequest{Email: "new@example.com", CurrentPassword: "wrong"})
	if err == nil {
		t.Fatal("UpdateSelf() accepted an invalid current password")
	}
	if code, _ := application.CodeOf(err); code != application.CodeValidationInvalid {
		t.Fatalf("error code = %q, want validation error", code)
	}
	details, _ := application.DetailsOf(err)
	if len(details.Fields) != 1 || details.Fields[0].Field != "current_password" {
		t.Fatalf("validation fields = %#v", details.Fields)
	}
	if repository.changes.Email != nil {
		t.Fatal("invalid current password changed the email")
	}
}

func timePointer() *time.Time {
	value := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	return &value
}

func TestUpdateByAdminRejectsEmailField(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 42, Email: "target@example.com", Status: 1}}}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, &accessManagementFake{scope: domain.UserScope{UserIDs: []uint{42}}})

	_, err := service.UpdateByAdmin(context.Background(), 7, 42, application.AdminUpdateUserRequest{Email: "new@example.com"})
	if err == nil {
		t.Fatal("UpdateByAdmin() accepted an email field")
	}
	details, _ := application.DetailsOf(err)
	if len(details.Fields) != 1 || details.Fields[0].Field != "email" {
		t.Fatalf("validation fields = %#v", details.Fields)
	}
}

func TestUpdateSelfKeepsEmailStateWhenVerificationDeliveryFails(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7, Password: "old-hash", Email: "old@example.com", EmailVerifiedAt: timePointer()}}}
	issuer := &registrationVerificationIssuerFake{token: "verification-token"}
	sender := &verificationEmailSenderFake{err: fmt.Errorf("smtp password=secret rejected")}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, &accessManagementFake{})
	service.ConfigureEmailVerification(issuer, sender)

	_, err := service.UpdateSelf(context.Background(), 7, application.UpdateSelfRequest{Email: "new@example.com", CurrentPassword: "OldPass123!"})
	if err == nil {
		t.Fatal("UpdateSelf() succeeded after SMTP failure")
	}
	if code, _ := application.CodeOf(err); code != application.CodeEmailVerificationDeliveryFailed {
		t.Fatalf("error code = %q, want delivery failure", code)
	}
	if repository.user.Email != "old@example.com" || repository.user.PendingEmail != "new@example.com" || repository.user.EmailVerifiedAt == nil || issuer.invalidateCalls != 1 {
		t.Fatalf("state after delivery failure = user:%#v invalidate_calls:%d", repository.user, issuer.invalidateCalls)
	}
}

func TestUpdateSelfMapsConcurrentEmailUniquenessFailureToConflict(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7, Password: "old-hash", Email: "old@example.com"}}, emailExistsValues: []bool{false, true}, updateErr: fmt.Errorf("unique constraint")}
	issuer := &registrationVerificationIssuerFake{token: "verification-token"}
	sender := &verificationEmailSenderFake{}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, &accessManagementFake{})
	service.ConfigureEmailVerification(issuer, sender)

	_, err := service.UpdateSelf(context.Background(), 7, application.UpdateSelfRequest{Email: "new@example.com", CurrentPassword: "OldPass123!"})
	if err == nil {
		t.Fatal("UpdateSelf() succeeded after uniqueness failure")
	}
	if code, _ := application.CodeOf(err); code != application.CodeConflict {
		t.Fatalf("error code = %q, want conflict", code)
	}
	if issuer.invalidateCalls != 1 {
		t.Fatalf("invalidated credentials = %d, want 1", issuer.invalidateCalls)
	}
}
