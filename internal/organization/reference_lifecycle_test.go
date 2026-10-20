package organization_test

import (
	"context"
	"testing"

	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
)

func TestRemovedDeletedOrdinaryOrganizationMemberRetainsInvalidationHistory(t *testing.T) {
	for _, deleteUnit := range []bool{false, true} {
		name := "replace members"
		if deleteUnit {
			name = "delete organization"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			if err := fixture.db.Create(&organization.Membership{UserID: 30, OrganizationID: fixture.leaf.ID}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.versions.Ensure(ctx, 30); err != nil {
				t.Fatal(err)
			}
			if err := fixture.db.Exec("UPDATE user_access_versions SET version = 7 WHERE user_id = 30").Error; err != nil {
				t.Fatal(err)
			}
			policy := organizationMemberPolicy{30: {Exists: true, Deleted: true, Visible: true}}
			service := organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), policy, noCustomOrganizationReferences{}, noHierarchyDependencies{})
			var err error
			if deleteUnit {
				err = service.DeleteUnit(ctx, 7, fixture.leaf.ID)
			} else {
				err = service.SetUsers(ctx, 7, fixture.leaf.ID, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			memberships, err := fixture.repository.MemberOrganizationMemberships(ctx, 30)
			if err != nil || len(memberships) != 0 {
				t.Fatalf("cleared memberships = %#v, %v", memberships, err)
			}
			version, err := fixture.versions.Current(ctx, 30)
			if err != nil || version != 8 {
				t.Fatalf("retained invalidation history = %d, %v", version, err)
			}
			_, err = fixture.repository.FindUnitByID(ctx, fixture.leaf.ID)
			if deleteUnit && err != organization.ErrNotFound {
				t.Fatalf("deleted unit lookup = %v", err)
			}
			if !deleteUnit && err != nil {
				t.Fatalf("membership replacement removed unit: %v", err)
			}
		})
	}
}
