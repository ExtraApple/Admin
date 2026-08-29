package application_test

import (
	"context"
	"testing"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type categoryStoreFake struct {
	categories map[uint]domain.MessageCategory
	nextID     uint
	deleteOK   bool
}

func (store *categoryStoreFake) CreateCategory(_ context.Context, category domain.MessageCategory) (domain.MessageCategory, error) {
	store.nextID++
	category.ID = store.nextID
	store.categories[category.ID] = category
	return category, nil
}

func (store *categoryStoreFake) FindCategory(_ context.Context, id uint) (domain.MessageCategory, error) {
	category, ok := store.categories[id]
	if !ok {
		return domain.MessageCategory{}, application.ErrNotFound
	}
	return category, nil
}

func (store *categoryStoreFake) FindCategoryByCode(_ context.Context, organizationID uint, code string) (domain.MessageCategory, error) {
	for _, category := range store.categories {
		if category.OrganizationID == organizationID && category.Code == code {
			return category, nil
		}
	}
	return domain.MessageCategory{}, application.ErrNotFound
}

func (store *categoryStoreFake) ListCategories(_ context.Context, query application.CategoryListQuery) ([]domain.MessageCategory, int64, error) {
	result := make([]domain.MessageCategory, 0)
	for _, category := range store.categories {
		for _, organizationID := range query.OrganizationIDs {
			if category.OrganizationID == organizationID && (!query.EnabledOnly || category.Enabled) {
				result = append(result, category)
			}
		}
	}
	return result, int64(len(result)), nil
}

func (store *categoryStoreFake) UpdateCategory(_ context.Context, category domain.MessageCategory) error {
	if _, ok := store.categories[category.ID]; !ok {
		return application.ErrNotFound
	}
	store.categories[category.ID] = category
	return nil
}

func (store *categoryStoreFake) DeleteCategoryIfUnused(_ context.Context, id uint) (bool, error) {
	if _, ok := store.categories[id]; !ok {
		return false, application.ErrNotFound
	}
	if !store.deleteOK {
		return false, nil
	}
	delete(store.categories, id)
	return true, nil
}

type categoryAuthorizationFake struct {
	allowed bool
	scope   application.MessageOrganizationScope
}

func (reader categoryAuthorizationFake) HasPermission(context.Context, uint, string) (bool, error) {
	return reader.allowed, nil
}

func (reader categoryAuthorizationFake) OrganizationScope(context.Context, uint) (application.MessageOrganizationScope, error) {
	return reader.scope, nil
}

type adminOrganizationFake struct {
	descendants []uint
	all         []uint
}

func (organization adminOrganizationFake) Memberships(context.Context, uint) ([]application.OrganizationMembership, error) {
	return nil, nil
}

func (organization adminOrganizationFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	return organization.descendants, nil
}

func (organization adminOrganizationFake) MemberUserIDs(context.Context, uint) ([]uint, error) {
	return nil, nil
}

func (organization adminOrganizationFake) RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error) {
	return nil, nil
}

func (organization adminOrganizationFake) AllOrganizationIDs(context.Context) ([]uint, error) {
	return organization.all, nil
}

func TestCategoryServiceManagesOnlyPermittedOrganizationScope(t *testing.T) {
	store := &categoryStoreFake{categories: make(map[uint]domain.MessageCategory), deleteOK: true}
	service := application.NewService(application.Dependencies{
		Categories:    store,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10, 11}},
	})

	category, err := service.CreateCategory(context.Background(), application.CategoryCreateRequest{ActorID: 7, OrganizationID: 11, Code: "notice", Name: "Notice", Sort: 1, Enabled: true})
	if err != nil || category.ID == 0 || category.OrganizationID != 11 {
		t.Fatalf("CreateCategory() = %#v, %v", category, err)
	}
	if _, err := service.CreateCategory(context.Background(), application.CategoryCreateRequest{ActorID: 7, OrganizationID: 12, Code: "notice", Name: "Notice", Enabled: true}); err != application.ErrOrganizationNotManaged {
		t.Fatalf("out-of-scope CreateCategory() error = %v, want ErrOrganizationNotManaged", err)
	}

	disabled := false
	updated, err := service.UpdateCategory(context.Background(), application.CategoryUpdateRequest{ActorID: 7, CategoryID: category.ID, Enabled: &disabled})
	if err != nil || updated.Enabled {
		t.Fatalf("UpdateCategory() = %#v, %v", updated, err)
	}
	if err := service.DeleteCategory(context.Background(), application.CategoryDeleteRequest{ActorID: 7, CategoryID: category.ID}); err != nil {
		t.Fatalf("DeleteCategory() = %v", err)
	}
}

func TestCategoryServiceRejectsPermissionDeniedAndReferencedCategoryDeletion(t *testing.T) {
	store := &categoryStoreFake{categories: map[uint]domain.MessageCategory{1: {ID: 1, OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}}}
	denied := application.NewService(application.Dependencies{
		Categories:    store,
		Authorization: categoryAuthorizationFake{allowed: false, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
	})
	if _, err := denied.CreateCategory(context.Background(), application.CategoryCreateRequest{ActorID: 7, OrganizationID: 10, Code: "other", Name: "Other", Enabled: true}); err != application.ErrPermissionDenied {
		t.Fatalf("permission denied error = %v, want ErrPermissionDenied", err)
	}

	service := application.NewService(application.Dependencies{
		Categories:    store,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
	})
	if err := service.DeleteCategory(context.Background(), application.CategoryDeleteRequest{ActorID: 7, CategoryID: 1}); err != application.ErrCategoryInUse {
		t.Fatalf("referenced DeleteCategory() error = %v, want ErrCategoryInUse", err)
	}
}
