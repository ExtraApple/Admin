package app

import (
	"context"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"

	"gorm.io/gorm"
)

func organizationDescriptorsWithVisibility(resources Resources, visibility organization.VisibilityProvider) []routecatalog.Descriptor {
	repository := organization.NewGORMRepository(resources.DB)
	hierarchy := organization.NewHierarchy(repository)
	application := organization.NewService(
		repository,
		hierarchy,
		visibility,
		organizationIdentityUsers{db: resources.DB},
		authgorm.NewAccessVersions(resources.DB),
		platformdatabase.NewTransactionRunner(resources.DB),
	)
	return organization.Routes(application)
}

type organizationIdentityUsers struct{ db *gorm.DB }

func (adapter organizationIdentityUsers) ExistingUserIDs(ctx context.Context, userIDs []uint) ([]uint, error) {
	if len(userIDs) == 0 {
		return []uint{}, nil
	}
	var existingIDs []uint
	err := platformdatabase.FromContext(ctx, adapter.db).
		Model(&identity.User{}).
		Where("id IN ?", userIDs).
		Order("id asc").
		Pluck("id", &existingIDs).Error
	return existingIDs, err
}

func (adapter organizationIdentityUsers) ListUsersByIDs(ctx context.Context, userIDs []uint) ([]organization.MemberInfo, error) {
	if len(userIDs) == 0 {
		return []organization.MemberInfo{}, nil
	}
	var users []identity.User
	if err := platformdatabase.FromContext(ctx, adapter.db).Where("id IN ?", userIDs).Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	result := make([]organization.MemberInfo, len(users))
	for index, user := range users {
		result[index] = organization.MemberInfo{ID: user.ID, Username: user.Username, Nickname: user.Nickname, Avatar: user.Avatar, Email: user.Email, Role: user.Role, Status: user.Status}
	}
	return result, nil
}

var _ organization.UserDirectory = organizationIdentityUsers{}
