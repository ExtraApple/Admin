package application_test

import "context"

type permissionReferenceLockerFake struct{}

// Isolated API fixtures contain no authorization resources to lock.
func (*permissionReferenceLockerFake) LockPermissions(context.Context, []string) error { return nil }
