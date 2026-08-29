package application_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type organizationAudienceReaderFake struct{}

func (organizationAudienceReaderFake) Memberships(context.Context, uint) ([]application.OrganizationMembership, error) {
	return []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}, {OrganizationID: 20, JoinedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)}}, nil
}

func (organizationAudienceReaderFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	return []uint{10, 11}, nil
}

func (organizationAudienceReaderFake) MemberUserIDs(context.Context, uint) ([]uint, error) {
	return []uint{1, 2}, nil
}

func (organizationAudienceReaderFake) RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error) {
	return []uint{2}, nil
}

var _ application.OrganizationAudienceReader = organizationAudienceReaderFake{}

func TestOrganizationAudienceContractCarriesMembershipJoinedAt(t *testing.T) {
	reader := organizationAudienceReaderFake{}
	memberships, err := reader.Memberships(context.Background(), 1)
	wantMemberships := []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}, {OrganizationID: 20, JoinedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)}}
	if err != nil || !reflect.DeepEqual(memberships, wantMemberships) {
		t.Fatalf("Memberships() = %#v, %v", memberships, err)
	}
	descendants, err := reader.DescendantOrganizationIDs(context.Background(), []uint{10})
	if err != nil || !reflect.DeepEqual(descendants, []uint{10, 11}) {
		t.Fatalf("DescendantOrganizationIDs() = %v, %v", descendants, err)
	}
	users, err := reader.RoleMemberUserIDs(context.Background(), 10, 7)
	if err != nil || !reflect.DeepEqual(users, []uint{2}) {
		t.Fatalf("RoleMemberUserIDs() = %v, %v", users, err)
	}
}
