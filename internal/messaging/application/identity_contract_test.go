package application_test

import (
	"context"
	"testing"

	"admin/internal/messaging/application"
)

type identityReaderFake struct {
	user  application.IdentityUser
	email application.VerifiedEmail
	ok    bool
}

func (fake identityReaderFake) LookupUser(context.Context, uint) (application.IdentityUser, error) {
	return fake.user, nil
}

func (fake identityReaderFake) LookupVerifiedEmail(context.Context, uint) (application.VerifiedEmail, bool, error) {
	return fake.email, fake.ok, nil
}

var _ application.IdentityReader = identityReaderFake{}

func TestIdentityContractUsesMessagingOwnedSafeValues(t *testing.T) {
	reader := identityReaderFake{
		user:  application.IdentityUser{ID: 7, DisplayName: "Alice", Enabled: true},
		email: application.VerifiedEmail{Address: "alice@example.test"},
		ok:    true,
	}
	user, err := reader.LookupUser(context.Background(), 7)
	if err != nil || user.ID != 7 || user.DisplayName != "Alice" || !user.Enabled {
		t.Fatalf("LookupUser() = %#v, %v", user, err)
	}
	email, eligible, err := reader.LookupVerifiedEmail(context.Background(), 7)
	if err != nil || !eligible || email.Address != "alice@example.test" {
		t.Fatalf("LookupVerifiedEmail() = %#v, %v, %v", email, eligible, err)
	}
}
