package gormadapter_test

import (
	"context"
	"testing"

	"admin/testsupport/testutil"
	authgorm "admin/internal/authorization/adapters/gorm"
)

func TestAccessVersionsEnsureAndIncrementUsesAuthorizationOwnedTable(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(authgorm.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate access versions: %v", err)
	}
	versions := authgorm.NewAccessVersions(db)
	ctx := context.Background()

	current, err := versions.EnsureAndIncrement(ctx, 42)
	if err != nil || current != 2 {
		t.Fatalf("first EnsureAndIncrement = %d, %v; want 2", current, err)
	}
	current, err = versions.EnsureAndIncrement(ctx, 42)
	if err != nil || current != 3 {
		t.Fatalf("second EnsureAndIncrement = %d, %v; want 3", current, err)
	}
	if current, err := versions.Current(ctx, 42); err != nil || current != 3 {
		t.Fatalf("Current = %d, %v; want 3", current, err)
	}
}

func TestAccessVersionsEnsureCreatesWithoutInvalidatingExistingTokens(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(authgorm.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate access versions: %v", err)
	}
	versions := authgorm.NewAccessVersions(db)
	ctx := context.Background()
	current, err := versions.Ensure(ctx, 42)
	if err != nil || current != 1 {
		t.Fatalf("first Ensure = %d, %v; want 1", current, err)
	}
	current, err = versions.Ensure(ctx, 42)
	if err != nil || current != 1 {
		t.Fatalf("second Ensure = %d, %v; want unchanged 1", current, err)
	}
}
