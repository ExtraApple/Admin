package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"admin/internal/identity/application"
	"admin/internal/identity/domain"
)

type emailCredentialRepositoryFake struct {
	issuedCount   int
	credential    domain.EmailVerificationCredential
	replaceCall   int
	deletedBefore time.Time
	confirmUserID uint
	confirmHash   string
	confirmAt     time.Time
}

func (fake *emailCredentialRepositoryFake) CountIssuedSince(context.Context, uint, time.Time) (int, error) {
	return fake.issuedCount, nil
}
func (fake *emailCredentialRepositoryFake) ReplaceActive(_ context.Context, credential domain.EmailVerificationCredential) error {
	fake.replaceCall++
	fake.credential = credential
	return nil
}

func (fake *emailCredentialRepositoryFake) DeleteTerminalBefore(_ context.Context, before time.Time) error {
	fake.deletedBefore = before
	return nil
}

func (fake *emailCredentialRepositoryFake) InvalidateActive(_ context.Context, _ uint, _ string, _ time.Time) error {
	fake.replaceCall++
	return nil
}

func (fake *emailCredentialRepositoryFake) Confirm(_ context.Context, userID uint, tokenHash string, at time.Time) (application.EmailVerificationConfirmation, error) {
	fake.confirmUserID = userID
	fake.confirmHash = tokenHash
	fake.confirmAt = at
	return application.EmailVerificationConfirmation{}, nil
}

type emailVerificationTransactionFake struct{}

func (emailVerificationTransactionFake) Run(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func TestEmailVerificationIssueUsesSingleUseHashed32ByteCredential(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	repository := &emailCredentialRepositoryFake{}
	random := bytesReader(make([]byte, 32))
	service := application.NewEmailVerificationService(repository, emailVerificationTransactionFake{}, application.WithEmailVerificationClock(func() time.Time { return now }), application.WithEmailVerificationRandom(random))

	token, err := service.Issue(context.Background(), 42, "new@example.com")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(rawToken) != 32 {
		t.Fatalf("token length = %d, error = %v; want 32 random bytes", len(rawToken), err)
	}
	wantHash := sha256.Sum256(rawToken)
	if repository.replaceCall != 1 || repository.credential.UserID != 42 || repository.credential.Email != "new@example.com" || repository.credential.TokenHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("stored credential = %#v", repository.credential)
	}
	if !repository.credential.CreatedAt.Equal(now) || !repository.credential.ExpiresAt.Equal(now.Add(15*time.Minute)) || repository.credential.UsedAt != nil {
		t.Fatalf("credential lifetime = %#v", repository.credential)
	}
}

func TestEmailVerificationIssueSharesThreePerHourQuotaAndDoesNotMutateWhenExhausted(t *testing.T) {
	repository := &emailCredentialRepositoryFake{issuedCount: 3}
	service := application.NewEmailVerificationService(repository, emailVerificationTransactionFake{}, application.WithEmailVerificationRandom(errorReader{}))

	_, err := service.Issue(context.Background(), 42, "new@example.com")
	if err == nil {
		t.Fatal("Issue() succeeded after three issues in the rolling hour")
	}
	if code, _ := application.CodeOf(err); code != application.CodeEmailVerificationRateLimited {
		t.Fatalf("error code = %q, want %q", code, application.CodeEmailVerificationRateLimited)
	}
	details, ok := application.DetailsOf(err)
	if !ok || details.RetryAfterSeconds < 1 {
		t.Fatalf("error details = %#v, want Retry-After", details)
	}
	if repository.replaceCall != 0 {
		t.Fatal("quota exhaustion mutated existing credential state")
	}
}

func TestEmailVerificationIssueCountsFailedDeliveryCredentialAndReplacesPriorCredential(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	repository := &emailCredentialRepositoryFake{issuedCount: 2}
	service := application.NewEmailVerificationService(repository, emailVerificationTransactionFake{}, application.WithEmailVerificationClock(func() time.Time { return now }), application.WithEmailVerificationRandom(bytesReader(make([]byte, 32))))

	if _, err := service.Issue(context.Background(), 7, "a@example.com"); err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if repository.replaceCall != 1 {
		t.Fatal("Issue() did not replace the previous active credential")
	}
	if _, err := service.Issue(context.Background(), 7, "a@example.com"); err != nil {
		t.Fatalf("second Issue() error = %v", err)
	}
	if repository.replaceCall != 2 {
		t.Fatal("failed-delivery issue was not retained in quota accounting contract")
	}
}

type bytesReader []byte

func (reader bytesReader) Read(buffer []byte) (int, error) {
	if len(reader) == 0 {
		return 0, io.EOF
	}
	n := copy(buffer, reader)
	return n, nil
}

func TestEmailVerificationCleanupKeepsTerminalCredentialsForTwentyFourHours(t *testing.T) {
	now := time.Date(2026, time.January, 3, 3, 4, 5, 0, time.UTC)
	repository := &emailCredentialRepositoryFake{}
	service := application.NewEmailVerificationService(repository, emailVerificationTransactionFake{}, application.WithEmailVerificationClock(func() time.Time { return now }))

	if err := service.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	wantBefore := now.Add(-24 * time.Hour)

	if !repository.deletedBefore.Equal(wantBefore) {
		t.Fatalf("cleanup cutoff = %s, want %s", repository.deletedBefore, wantBefore)
	}
}

func TestEmailVerificationConfirmPassesHashedTokenToAtomicPromotion(t *testing.T) {
	repository := &emailCredentialRepositoryFake{}
	service := application.NewEmailVerificationService(repository, emailVerificationTransactionFake{})
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	if err := service.Confirm(context.Background(), 42, token); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if repository.confirmUserID != 42 || repository.confirmHash == "" || repository.confirmAt.IsZero() {
		t.Fatalf("confirmation call = user:%d hash:%q at:%s", repository.confirmUserID, repository.confirmHash, repository.confirmAt)
	}
}

func TestEmailVerificationResendUsesPendingEmailAndRejectsAlreadyVerifiedWithoutPending(t *testing.T) {
	repository := &emailCredentialRepositoryFake{}
	verifiedUser := userRepositoryFake{user: domain.User{ID: 42, Email: "old@example.com", PendingEmail: "new@example.com", EmailVerifiedAt: timePointer(), Status: 1}}
	service := application.NewEmailVerificationService(repository, emailVerificationTransactionFake{}, application.WithEmailVerificationUserRepository(verifiedUser), application.WithEmailVerificationRandom(bytesReader(make([]byte, 32))))
	sender := &verificationEmailSenderFake{}
	service.ConfigureSender(sender)
	if err := service.Resend(context.Background(), verifiedUser.user.ID); err != nil {
		t.Fatalf("Resend() error = %v", err)
	}
	if sender.recipient != "new@example.com" {
		t.Fatalf("resend recipient = %q, want pending email", sender.recipient)
	}

	verifiedUser.user.PendingEmail = ""
	service = application.NewEmailVerificationService(repository, emailVerificationTransactionFake{}, application.WithEmailVerificationUserRepository(verifiedUser))
	service.ConfigureSender(&verificationEmailSenderFake{})
	if err := service.Resend(context.Background(), verifiedUser.user.ID); err == nil {
		t.Fatal("verified resend succeeded without pending email")
	} else if code, _ := application.CodeOf(err); code != application.CodeConflict {
		t.Fatalf("verified resend error = %v, want conflict", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("random source unavailable") }

var _ io.Reader = bytesReader{}
