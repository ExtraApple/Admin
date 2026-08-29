package application

import "context"

// IdentityReader is the Messaging-owned view of the Identity capabilities it
// consumes. It deliberately excludes Identity persistence, credentials and
// email-verification internals.
type IdentityReader interface {
	LookupUser(context.Context, uint) (IdentityUser, error)
	LookupVerifiedEmail(context.Context, uint) (VerifiedEmail, bool, error)
}

// IdentityUser is the minimum identity fact required to address or validate a
// Messaging participant.
type IdentityUser struct {
	ID          uint
	DisplayName string
	Enabled     bool
}

// VerifiedEmail is the only email address a notification adapter may receive.
// A false eligibility result from IdentityReader means no email may be used.
type VerifiedEmail struct {
	Address string
}
