package organization_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/organization"
	"admin/testsupport/testutil"
)

func TestReplaceMembershipsPreservesExistingJoinedAtAndCreatesNewMembershipTime(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(organization.Models()...); err != nil {
		t.Fatalf("migrate organization models: %v", err)
	}
	joinedAt := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	if err := db.Create(&organization.Membership{UserID: 7, OrganizationID: 10, CreatedAt: joinedAt}).Error; err != nil {
		t.Fatalf("create existing membership: %v", err)
	}
	repository := organization.NewGORMRepository(db)
	if err := repository.ReplaceMemberships(context.Background(), 10, []uint{7, 8}); err != nil {
		t.Fatalf("ReplaceMemberships() = %v", err)
	}
	memberships, err := repository.MemberOrganizationMemberships(context.Background(), 7)
	if err != nil || len(memberships) != 1 || memberships[0].OrganizationID != 10 || !memberships[0].JoinedAt.Equal(joinedAt) {
		t.Fatalf("MemberOrganizationMemberships() = %#v, %v", memberships, err)
	}
	var added organization.Membership
	if err := db.First(&added, "user_id = ? AND organization_id = ?", 8, 10).Error; err != nil {
		t.Fatalf("load added membership: %v", err)
	}
	if added.CreatedAt.IsZero() || !added.CreatedAt.After(joinedAt) {
		t.Fatalf("added membership CreatedAt = %s, want after %s", added.CreatedAt, joinedAt)
	}
}
