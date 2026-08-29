package application_test

import (
	"context"
	"errors"
	"testing"

	"admin/internal/identity/application"
)

type registrationVerificationIssuerFake struct {
	token           string
	issueCalls      int
	invalidateCalls int
}

func (fake *registrationVerificationIssuerFake) Issue(context.Context, uint, string) (string, error) {
	fake.issueCalls++
	return fake.token, nil
}
func (fake *registrationVerificationIssuerFake) Invalidate(context.Context, uint, string) error {
	fake.invalidateCalls++
	return nil
}

type verificationEmailSenderFake struct {
	recipient string
	token     string
	err       error
}

func (fake *verificationEmailSenderFake) SendVerification(_ context.Context, recipient, token string) error {
	fake.recipient = recipient
	fake.token = token
	return fake.err
}

func TestRegisterSynchronouslySendsVerificationAndKeepsAccountOnDeliveryFailure(t *testing.T) {
	repository := &recordingUserRepository{}
	issuer := &registrationVerificationIssuerFake{token: "verification-token"}
	sender := &verificationEmailSenderFake{err: errors.New("smtp refused password=secret")}
	service := application.NewService(repository, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, blacklistFake{}, authorizationFake{}, &tokenFake{})
	service.ConfigureEmailVerification(issuer, sender)

	user, err := service.Register(context.Background(), application.RegisterRequest{Username: "new-user", Password: "ValidPass123!", Email: "new@example.com", Nickname: "New", CaptchaID: "id", CaptchaCode: "123456"})
	if err == nil {
		t.Fatal("Register() succeeded after synchronous SMTP delivery failure")
	}
	if code, _ := application.CodeOf(err); code != application.CodeEmailVerificationDeliveryFailed {
		t.Fatalf("error code = %q, want %q", code, application.CodeEmailVerificationDeliveryFailed)
	}
	if user.ID != 99 || repository.created.Email != "new@example.com" {
		t.Fatalf("registered account = %#v, want persisted account retained", user)
	}
	if issuer.issueCalls != 1 || issuer.invalidateCalls != 1 || sender.recipient != "new@example.com" || sender.token != "verification-token" {
		t.Fatalf("verification delivery calls = issue:%d invalidate:%d recipient:%q token:%q", issuer.issueCalls, issuer.invalidateCalls, sender.recipient, sender.token)
	}
}

func TestRegisterReturnsSuccessAfterVerificationDelivery(t *testing.T) {
	repository := &recordingUserRepository{}
	issuer := &registrationVerificationIssuerFake{token: "verification-token"}
	sender := &verificationEmailSenderFake{}
	service := application.NewService(repository, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, blacklistFake{}, authorizationFake{}, &tokenFake{})
	service.ConfigureEmailVerification(issuer, sender)

	user, err := service.Register(context.Background(), application.RegisterRequest{Username: "new-user", Password: "ValidPass123!", Email: "new@example.com", Nickname: "New", CaptchaID: "id", CaptchaCode: "123456"})
	if err != nil || user.ID != 99 || sender.recipient != "new@example.com" {
		t.Fatalf("Register() = user:%#v error:%v", user, err)
	}
}

var _ application.EmailVerificationIssuer = (*registrationVerificationIssuerFake)(nil)
var _ application.VerificationEmailSender = (*verificationEmailSenderFake)(nil)
