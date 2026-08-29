package application

import (
	"context"

	"admin/internal/messaging/domain"
)

const PermissionCategoryManage = "admin.messages.category.manage"

// AllOrganizationAudienceReader is the optional Organization capability needed
// only by the super-administrator's system-wide audience and category scope.
type AllOrganizationAudienceReader interface {
	AllOrganizationIDs(context.Context) ([]uint, error)
}

type CategoryCreateRequest struct {
	ActorID        uint
	OrganizationID uint
	Code           string
	Name           string
	Sort           int
	Enabled        bool
}

type CategoryUpdateRequest struct {
	ActorID    uint
	CategoryID uint
	Name       *string
	Sort       *int
	Enabled    *bool
}

type CategoryDeleteRequest struct {
	ActorID    uint
	CategoryID uint
}

type CategoryListRequest struct {
	ActorID     uint
	EnabledOnly bool
	Offset      int
	Limit       int
}

func (service *Service) CreateCategory(ctx context.Context, request CategoryCreateRequest) (domain.MessageCategory, error) {
	if service == nil || service.categories == nil {
		return domain.MessageCategory{}, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionCategoryManage); err != nil {
		return domain.MessageCategory{}, err
	}
	if err := service.requireManagedOrganization(ctx, request.ActorID, request.OrganizationID); err != nil {
		return domain.MessageCategory{}, err
	}
	return service.categories.CreateCategory(ctx, domain.MessageCategory{OrganizationID: request.OrganizationID, Code: request.Code, Name: request.Name, Sort: request.Sort, Enabled: request.Enabled})
}

func (service *Service) UpdateCategory(ctx context.Context, request CategoryUpdateRequest) (domain.MessageCategory, error) {
	if service == nil || service.categories == nil {
		return domain.MessageCategory{}, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionCategoryManage); err != nil {
		return domain.MessageCategory{}, err
	}
	if request.CategoryID == 0 {
		return domain.MessageCategory{}, ErrNotFound
	}
	category, err := service.categories.FindCategory(ctx, request.CategoryID)
	if err != nil {
		return domain.MessageCategory{}, err
	}
	if err := service.requireManagedOrganization(ctx, request.ActorID, category.OrganizationID); err != nil {
		return domain.MessageCategory{}, err
	}
	if request.Name != nil {
		category.Name = *request.Name
	}
	if request.Sort != nil {
		category.Sort = *request.Sort
	}
	if request.Enabled != nil {
		category.Enabled = *request.Enabled
	}
	if err := service.categories.UpdateCategory(ctx, category); err != nil {
		return domain.MessageCategory{}, err
	}
	return category, nil
}

func (service *Service) DeleteCategory(ctx context.Context, request CategoryDeleteRequest) error {
	if service == nil || service.categories == nil {
		return ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionCategoryManage); err != nil {
		return err
	}
	if request.CategoryID == 0 {
		return ErrNotFound
	}
	category, err := service.categories.FindCategory(ctx, request.CategoryID)
	if err != nil {
		return err
	}
	if err := service.requireManagedOrganization(ctx, request.ActorID, category.OrganizationID); err != nil {
		return err
	}
	deleted, err := service.categories.DeleteCategoryIfUnused(ctx, request.CategoryID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrCategoryInUse
	}
	return nil
}

func (service *Service) ListCategories(ctx context.Context, request CategoryListRequest) ([]domain.MessageCategory, int64, error) {
	if service == nil || service.categories == nil {
		return nil, 0, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionCategoryManage); err != nil {
		return nil, 0, err
	}
	if request.Offset < 0 || request.Limit < 0 {
		return nil, 0, ErrInboxRequestInvalid
	}
	organizationIDs, err := service.managedOrganizationIDs(ctx, request.ActorID)
	if err != nil {
		return nil, 0, err
	}
	return service.categories.ListCategories(ctx, CategoryListQuery{OrganizationIDs: organizationIDs, EnabledOnly: request.EnabledOnly, Offset: request.Offset, Limit: request.Limit})
}

func (service *Service) requirePermission(ctx context.Context, actorID uint, permission string) error {
	if service == nil || service.authorization == nil || actorID == 0 {
		return ErrPermissionDenied
	}
	allowed, err := service.authorization.HasPermission(ctx, actorID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func (service *Service) requireManagedOrganization(ctx context.Context, actorID, organizationID uint) error {
	if organizationID == 0 {
		return ErrOrganizationNotManaged
	}
	organizationIDs, err := service.managedOrganizationIDs(ctx, actorID)
	if err != nil {
		return err
	}
	for _, candidate := range organizationIDs {
		if candidate == organizationID {
			return nil
		}
	}
	return ErrOrganizationNotManaged
}

func (service *Service) managedOrganizationIDs(ctx context.Context, actorID uint) ([]uint, error) {
	if service == nil || service.authorization == nil || service.organizations == nil || actorID == 0 {
		return nil, ErrMessagingDependency
	}
	scope, err := service.authorization.OrganizationScope(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if scope.All {
		allOrganizations, ok := service.organizations.(AllOrganizationAudienceReader)
		if !ok {
			return nil, ErrMessagingDependency
		}
		return allOrganizations.AllOrganizationIDs(ctx)
	}
	return service.organizations.DescendantOrganizationIDs(ctx, scope.OrganizationIDs)
}
