package app

import (
	authgorm "admin/internal/authorization/adapters/gorm"
	identityapplication "admin/internal/identity/application"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
)

func organizationDescriptorsWithVisibility(resources Resources, visibility organization.VisibilityProvider, users organization.UserDirectory) []routecatalog.Descriptor {
	repository := organization.NewGORMRepository(resources.DB)
	hierarchy := organization.NewHierarchy(repository)
	application := organization.NewService(
		repository,
		hierarchy,
		visibility,
		users,
		authgorm.NewAccessVersions(resources.DB),
		platformdatabase.NewTransactionRunner(resources.DB),
	)
	return organization.Routes(application)
}

var _ organization.UserDirectory = (*identityapplication.DirectoryService)(nil)
