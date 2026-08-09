package app

import (
	"context"
	"reflect"
	"testing"

	identityapplication "admin/internal/identity/application"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/navigation"
)

type navigationUserDirectoryFake struct {
	userIDs []uint
	calls   int
}

func (fake *navigationUserDirectoryFake) ListUsersByIDs(context.Context, []uint) ([]identitydomain.DirectoryUser, error) {
	return []identitydomain.DirectoryUser{}, nil
}

func (fake *navigationUserDirectoryFake) ListUserIDs(context.Context) ([]uint, error) {
	fake.calls++
	return append([]uint(nil), fake.userIDs...), nil
}

func TestNavigationAllUserIDsUsesIdentityDirectory(t *testing.T) {
	directory := &navigationUserDirectoryFake{userIDs: []uint{3, 7}}
	var authorization navigation.Authorization = navigationAuthorizationCapability{users: directory}

	userIDs, err := authorization.AllUserIDs(context.Background())
	if err != nil {
		t.Fatalf("AllUserIDs() error = %v", err)
	}
	if !reflect.DeepEqual(userIDs, []uint{3, 7}) {
		t.Fatalf("AllUserIDs() = %#v, want [3 7]", userIDs)
	}
	if directory.calls != 1 {
		t.Fatalf("Identity Directory calls = %d, want 1", directory.calls)
	}
}

var _ identityapplication.UserDirectory = (*navigationUserDirectoryFake)(nil)
