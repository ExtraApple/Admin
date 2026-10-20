package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/organization"
)

type hierarchyTransactionFacts struct {
	Units       []organization.Unit
	Memberships []organization.Membership
	Roles       []authgorm.UserRole
	Versions    []authgorm.UserAccessVersion
}

func hierarchyFacts(t *testing.T, fixture *organizationJWTFixture) hierarchyTransactionFacts {
	t.Helper()
	var facts hierarchyTransactionFacts
	for _, query := range []struct {
		order string
		rows  any
	}{
		{"id", &facts.Units}, {"user_id, organization_id", &facts.Memberships},
		{"user_id, role_id", &facts.Roles}, {"user_id", &facts.Versions},
	} {
		if err := fixture.db.Order(query.order).Find(query.rows).Error; err != nil {
			t.Fatal(err)
		}
	}
	return facts
}

type hierarchyTreeNode struct {
	ID         uint                `json:"id"`
	Manageable bool                `json:"manageable"`
	Children   []hierarchyTreeNode `json:"children"`
}

func hierarchyManagedIDs(t *testing.T, fixture *organizationJWTFixture, token string) []uint {
	t.Helper()
	response := fixture.request(http.MethodGet, "/api/admin/organizations/tree", token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("tree HTTP %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []hierarchyTreeNode `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var ids []uint
	var visit func([]hierarchyTreeNode)
	visit = func(nodes []hierarchyTreeNode) {
		for _, node := range nodes {
			if node.Manageable {
				ids = append(ids, node.ID)
			}
			visit(node.Children)
		}
	}
	visit(payload.Data)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func TestHierarchyMoveHTTPCommitsFiniteDependenciesOrRollsBackEveryOwner(t *testing.T) {
	for _, failSecondVersion := range []bool{false, true} {
		t.Run(fmt.Sprintf("second_version_failure_%t", failSecondVersion), func(t *testing.T) {
			fixture := newOrganizationJWTFixture(t)
			createUnit := func(code string, parentID uint) organization.Unit {
				unit := organization.Unit{Name: code, Code: code, ParentID: parentID, Status: 1}
				if err := fixture.db.Create(&unit).Error; err != nil {
					t.Fatal(err)
				}
				return unit
			}
			common := createUnit("hier-common", 0)
			oldRoot := createUnit("hier-old", common.ID)
			newRoot := createUnit("hier-new", common.ID)
			moving := createUnit("hier-moving", oldRoot.ID)
			createUnit("hier-descendant", moving.ID)
			child := authgorm.Role{Name: "Hierarchy children", Code: "hier-children", Status: 1, DataScope: "org_and_children"}
			all := authgorm.Role{Name: "Hierarchy all", Code: "hier-all", Status: 1, DataScope: "all"}
			fixed := authgorm.Role{Name: "Hierarchy fixed", Code: "hier-fixed", Status: 1, DataScope: "org"}
			for _, role := range []*authgorm.Role{&child, &all, &fixed} {
				if err := fixture.db.Create(role).Error; err != nil {
					t.Fatal(err)
				}
			}
			oldUser := fixture.createUser("hier-old-user")
			newUser := fixture.createUser("hier-new-user")
			allUser := fixture.createUser("hier-all-child-user")
			bothUser := fixture.createUser("hier-both-roots-user")
			commonUser := fixture.createUser("hier-common-user")
			movedUser := fixture.createUser("hier-moved-user")
			fixedUser := fixture.createUser("hier-fixed-user")
			memberships := []organization.Membership{
				{UserID: oldUser.ID, OrganizationID: oldRoot.ID}, {UserID: newUser.ID, OrganizationID: newRoot.ID},
				{UserID: allUser.ID, OrganizationID: oldRoot.ID}, {UserID: bothUser.ID, OrganizationID: oldRoot.ID},
				{UserID: bothUser.ID, OrganizationID: newRoot.ID}, {UserID: commonUser.ID, OrganizationID: common.ID},
				{UserID: movedUser.ID, OrganizationID: moving.ID}, {UserID: fixedUser.ID, OrganizationID: oldRoot.ID},
			}
			for index := range memberships {
				memberships[index].CreatedAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			if err := fixture.db.Create(&memberships).Error; err != nil {
				t.Fatal(err)
			}
			var roleRows []authgorm.UserRole
			for _, userID := range []uint{oldUser.ID, newUser.ID, allUser.ID, bothUser.ID, commonUser.ID, movedUser.ID} {
				roleRows = append(roleRows, authgorm.UserRole{UserID: userID, RoleID: child.ID})
			}
			roleRows = append(roleRows, authgorm.UserRole{UserID: allUser.ID, RoleID: all.ID}, authgorm.UserRole{UserID: fixedUser.ID, RoleID: fixed.ID})
			if err := fixture.db.Create(&roleRows).Error; err != nil {
				t.Fatal(err)
			}
			oldBothToken := fixture.token(bothUser)
			beforeCoverage := hierarchyManagedIDs(t, fixture, oldBothToken)
			before := hierarchyFacts(t, fixture)
			if failSecondVersion {
				trigger := fmt.Sprintf("CREATE TRIGGER hierarchy_second_version BEFORE UPDATE ON user_access_versions WHEN NEW.user_id = %d BEGIN SELECT RAISE(ABORT, 'hierarchy second version failure'); END", newUser.ID)
				if err := fixture.db.Exec(trigger).Error; err != nil {
					t.Fatal(err)
				}
			}
			response := fixture.request(http.MethodPut, fmt.Sprintf("/api/admin/organizations/%d", moving.ID), fixture.adminToken, map[string]any{"parent_id": newRoot.ID})
			after := hierarchyFacts(t, fixture)
			if failSecondVersion {
				if response.Code != http.StatusInternalServerError {
					t.Fatalf("move HTTP %d: %s", response.Code, response.Body.String())
				}
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("failed second version left changed hierarchy/member/role/version facts: before=%#v after=%#v", before, after)
				}
				if got := hierarchyManagedIDs(t, fixture, oldBothToken); !reflect.DeepEqual(got, beforeCoverage) {
					t.Fatalf("rollback coverage %v, want %v", got, beforeCoverage)
				}
				return
			}
			if response.Code != http.StatusOK {
				t.Fatalf("move HTTP %d: %s", response.Code, response.Body.String())
			}
			var stored organization.Unit
			if err := fixture.db.First(&stored, moving.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.ParentID != newRoot.ID {
				t.Fatalf("stored parent %d, want %d", stored.ParentID, newRoot.ID)
			}
			if !reflect.DeepEqual(after.Memberships, before.Memberships) || !reflect.DeepEqual(after.Roles, before.Roles) {
				t.Fatal("move changed membership/role rows or retained join times")
			}
			dependents := map[uint]bool{oldUser.ID: true, newUser.ID: true, allUser.ID: true, bothUser.ID: true}
			for _, version := range after.Versions {
				want := 1
				if dependents[version.UserID] {
					want = 2
				}
				if version.Version != want {
					t.Fatalf("user %d version %d, want %d", version.UserID, version.Version, want)
				}
			}
			if got := hierarchyManagedIDs(t, fixture, fixture.token(bothUser)); !reflect.DeepEqual(got, beforeCoverage) {
				t.Fatalf("multi-root final coverage %v, want unchanged %v", got, beforeCoverage)
			}
			oldTokenResponse := fixture.request(http.MethodGet, "/api/admin/organizations/tree", oldBothToken, nil)
			if oldTokenResponse.Code != http.StatusUnauthorized {
				t.Fatalf("pre-move dependent token HTTP %d, want401", oldTokenResponse.Code)
			}
		})
	}
}
