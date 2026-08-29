package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"time"

	"admin/internal/identity/domain"
)

const (
	emailVerificationLifetime = 15 * time.Minute
	emailVerificationQuota    = 3
	emailVerificationWindow   = time.Hour
)

type EmailVerificationOption func(*EmailVerificationService)

type EmailVerificationService struct {
	credentials  EmailVerificationRepository
	transactions TransactionRunner
	users        UserRepository
	sender       VerificationEmailSender
	now          func() time.Time
	random       io.Reader
}

func NewEmailVerificationService(credentials EmailVerificationRepository, transactions TransactionRunner, options ...EmailVerificationOption) *EmailVerificationService {
	service := &EmailVerificationService{
		credentials:  credentials,
		transactions: transactions,
		now:          time.Now,
		random:       rand.Reader,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func WithEmailVerificationClock(clock func() time.Time) EmailVerificationOption {
	return func(service *EmailVerificationService) {
		if clock != nil {
			service.now = clock
		}
	}
}

func WithEmailVerificationRandom(random io.Reader) EmailVerificationOption {
	return func(service *EmailVerificationService) {
		if random != nil {
			service.random = random
		}
	}
}

func WithEmailVerificationUserRepository(users UserRepository) EmailVerificationOption {
	return func(service *EmailVerificationService) { service.users = users }
}

func (service *EmailVerificationService) ConfigureSender(sender VerificationEmailSender) {
	service.sender = sender
}

// Issue creates a new one-time credential. Created credentials remain in the
// repository after delivery failures so failed attempts consume the quota.
func (service *EmailVerificationService) Issue(ctx context.Context, userID uint, email string) (string, error) {
	now := service.now().UTC()
	issued, err := service.credentials.CountIssuedSince(ctx, userID, now.Add(-emailVerificationWindow))
	if err != nil {
		return "", NewError(CodeInternalError, err)
	}
	if issued >= emailVerificationQuota {
		return "", &Error{Code: CodeEmailVerificationRateLimited, Details: ErrorDetails{RetryAfterSeconds: int(emailVerificationWindow / time.Second)}}
	}

	rawToken := make([]byte, 32)
	if _, err := io.ReadFull(service.random, rawToken); err != nil {
		return "", NewError(CodeInternalError, err)
	}
	tokenHash := sha256.Sum256(rawToken)
	credential := domain.EmailVerificationCredential{
		UserID:    userID,
		Email:     email,
		TokenHash: hex.EncodeToString(tokenHash[:]),
		CreatedAt: now,
		ExpiresAt: now.Add(emailVerificationLifetime),
	}
	operation := func(tx context.Context) error {
		return service.credentials.ReplaceActive(tx, credential)
	}
	if service.transactions != nil {
		if err := service.transactions.Run(ctx, operation); err != nil {
			return "", NewError(CodeInternalError, err)
		}
	} else if err := operation(ctx); err != nil {

		return "", NewError(CodeInternalError, err)
	}
	return base64.RawURLEncoding.EncodeToString(rawToken), nil
}

func (service *EmailVerificationService) Cleanup(ctx context.Context) error {
	before := service.now().UTC().Add(-24 * time.Hour)
	if err := service.credentials.DeleteTerminalBefore(ctx, before); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *EmailVerificationService) Invalidate(ctx context.Context, userID uint, email string) error {
	if err := service.credentials.InvalidateActive(ctx, userID, email, service.now().UTC()); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *EmailVerificationService) Confirm(ctx context.Context, userID uint, token string) error {
	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(rawToken) != 32 {
		return ErrEmailVerificationInvalid
	}
	digest := sha256.Sum256(rawToken)
	now := service.now().UTC()
	var outcome EmailVerificationConfirmation
	operation := func(tx context.Context) error {
		var operationErr error
		outcome, operationErr = service.credentials.Confirm(tx, userID, hex.EncodeToString(digest[:]), now)
		return operationErr
	}
	if service.transactions != nil {
		err = service.transactions.Run(ctx, operation)
	} else {
		err = operation(ctx)
	}
	if err == nil {
		if outcome.Conflict {
			return NewError(CodeConflict, nil)
		}
		return nil
	}
	if code, ok := CodeOf(err); ok && code == CodeEmailVerificationInvalid {
		return err
	}
	return NewError(CodeInternalError, err)
}

func (service *EmailVerificationService) Send(ctx context.Context, userID uint, email string) error {
	if service.sender == nil {
		return NewError(CodeInternalError, nil)
	}
	token, err := service.Issue(ctx, userID, email)
	if err != nil {
		return err
	}
	if err := service.sender.SendVerification(ctx, email, token); err != nil {
		_ = service.Invalidate(ctx, userID, email)
		return NewError(CodeEmailVerificationDeliveryFailed, err)
	}
	return nil
}

func (service *EmailVerificationService) Resend(ctx context.Context, userID uint) error {
	if service.users == nil {
		return NewError(CodeInternalError, nil)
	}
	user, err := service.users.FindByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}
	if !user.Enabled() {
		return NewError(CodeAccountDisabled, nil)
	}
	if user.EmailVerifiedAt != nil && user.PendingEmail == "" {
		return NewError(CodeConflict, nil)
	}
	target := user.PendingEmail
	if target == "" {
		target = user.Email
	}
	return service.Send(ctx, userID, target)
}
