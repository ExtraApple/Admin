package application_test

import (
	"context"
	"reflect"
	"testing"

	"admin/internal/identity/application"
	"admin/internal/identity/domain"
)

type directoryRepositoryFake struct {
	records []application.DirectoryUserRecord
	userIDs []uint
	calls   [][]uint
}

func (fake *directoryRepositoryFake) ListUsersByIDs(_ context.Context, userIDs []uint) ([]application.DirectoryUserRecord, error) {
	fake.calls = append(fake.calls, append([]uint(nil), userIDs...))
	return append([]application.DirectoryUserRecord(nil), fake.records...), nil
}

func (fake *directoryRepositoryFake) ListUserIDs(context.Context) ([]uint, error) {
	return append([]uint(nil), fake.userIDs...), nil
}

func TestUserDirectoryMapsSafeValuesAndTrustedAvatar(t *testing.T) {
	repository := &directoryRepositoryFake{records: []application.DirectoryUserRecord{
		{ID: 7, Username: "alice", Nickname: "Alice", Email: "alice@example.com", Role: "user", Status: 1, AvatarTrusted: true},
		{ID: 3, Username: "bob", Nickname: "Bob", Email: "bob@example.com", Role: "user", Status: 0},
	}}
	service := application.NewDirectoryService(repository)

	users, err := service.ListUsersByIDs(context.Background(), []uint{7, 3})
	if err != nil {
		t.Fatalf("ListUsersByIDs() error = %v", err)
	}
	want := []domain.DirectoryUser{
		{ID: 7, Username: "alice", Nickname: "Alice", Avatar: "/api/avatars/7", Email: "alice@example.com", Role: "user", Status: 1},
		{ID: 3, Username: "bob", Nickname: "Bob", Avatar: "/api/avatars/default", Email: "bob@example.com", Role: "user", Status: 0},
	}
	if !reflect.DeepEqual(users, want) {
		t.Fatalf("directory users = %#v, want %#v", users, want)
	}
	if !reflect.DeepEqual(repository.calls, [][]uint{{7, 3}}) {
		t.Fatalf("repository calls = %#v, want one batch call", repository.calls)
	}
}

func TestUserDirectoryReturnsNonNilEmptyResults(t *testing.T) {
	repository := &directoryRepositoryFake{}
	service := application.NewDirectoryService(repository)

	users, err := service.ListUsersByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListUsersByIDs() error = %v", err)
	}
	if users == nil || len(users) != 0 {
		t.Fatalf("empty directory users = %#v, want non-nil empty", users)
	}
	if len(repository.calls) != 0 {
		t.Fatalf("repository calls = %#v, want no call for empty input", repository.calls)
	}
}

func TestUserDirectoryListsUserIDsThroughItsRepository(t *testing.T) {
	repository := &directoryRepositoryFake{userIDs: []uint{3, 7}}
	service := application.NewDirectoryService(repository)

	userIDs, err := service.ListUserIDs(context.Background())
	if err != nil {
		t.Fatalf("ListUserIDs() error = %v", err)
	}
	if !reflect.DeepEqual(userIDs, []uint{3, 7}) {
		t.Fatalf("user IDs = %#v, want [3 7]", userIDs)
	}
}
