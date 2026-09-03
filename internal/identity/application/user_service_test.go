package application_test

import (
	"context"
	"testing"

	"admin/internal/identity/application"
	"admin/internal/identity/domain"
)

type managementRepositoryFake struct {
	userRepositoryFake
	password          string
	changes           application.UserChanges
	deleted           bool
	emailExists       bool
	emailExistsValues []bool
	updateErr         error
	lockCalls         int
}

func (fake *managementRepositoryFake) FindByIDForUpdate(_ context.Context, _ uint) (domain.User, error) {
	fake.lockCalls++
	return fake.user, nil
}

func (fake *managementRepositoryFake) List(context.Context, int, int, domain.UserScope) ([]domain.User, int64, error) {
	return []domain.User{fake.user}, 1, nil
}
func (fake *managementRepositoryFake) EmailExists(_ context.Context, _ string, _ uint) (bool, error) {
	if len(fake.emailExistsValues) > 0 {
		exists := fake.emailExistsValues[0]
		fake.emailExistsValues = fake.emailExistsValues[1:]
		return exists, nil
	}
	return fake.emailExists, nil
}
func (fake *managementRepositoryFake) Update(_ context.Context, _ uint, changes application.UserChanges) error {
	if fake.updateErr != nil {
		return fake.updateErr
	}
	fake.changes = changes
	if changes.Email != nil {
		fake.user.Email = *changes.Email
	}
	if changes.PendingEmail != nil {
		fake.user.PendingEmail = *changes.PendingEmail
	}
	if changes.ClearEmailVerifiedAt {
		fake.user.EmailVerifiedAt = nil
	}
	return nil
}
func (fake *managementRepositoryFake) UpdatePassword(_ context.Context, _ uint, password string) error {
	fake.password = password
	return nil
}
func (fake *managementRepositoryFake) Delete(context.Context, uint) error {
	fake.deleted = true
	return nil
}

type transactionFake struct{ calls int }

func (fake *transactionFake) Run(ctx context.Context, callback func(context.Context) error) error {
	fake.calls++
	return callback(ctx)
}

type accessManagementFake struct {
	incremented   []uint
	scope         domain.UserScope
	administrator bool
}

func (fake *accessManagementFake) UserScope(context.Context, uint) (domain.UserScope, error) {
	if fake.scope.All || fake.scope.UserIDs != nil {
		return fake.scope, nil
	}
	return domain.UserScope{All: true}, nil
}
func (fake *accessManagementFake) IncrementVersions(_ context.Context, ids []uint) error {
	fake.incremented = append([]uint(nil), ids...)
	return nil
}
func (fake *accessManagementFake) IsAdministrator(context.Context, uint) (bool, error) {
	return fake.administrator, nil
}
func TestUserServiceChangesPasswordAndInvalidatesAccessVersion(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7, Password: "old-hash", Status: 1}}}
	transactions := &transactionFake{}
	access := &accessManagementFake{}
	service := application.NewUserService(repository, passwordFake{}, transactions, access)
	err := service.ChangePassword(context.Background(), 7, application.ChangePasswordRequest{OldPassword: "OldPass123!", NewPassword: "NewPass123!", ConfirmPassword: "NewPass123!"})
	if err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if repository.password != "hash" || transactions.calls != 1 || len(access.incremented) != 1 || access.incremented[0] != 7 {
		t.Fatalf("password change state: password=%q transactions=%d invalidated=%v", repository.password, transactions.calls, access.incremented)
	}
}

func TestUserServiceRejectsDirectAvatarMutation(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 7}}}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, &accessManagementFake{})
	_, err := service.UpdateSelf(context.Background(), 7, application.UpdateSelfRequest{Nickname: "new-name", AvatarPresent: true})
	if err == nil || repository.changes.Nickname != nil {
		t.Fatalf("UpdateSelf() err = %v changes = %#v, want avatar rejection before persistence", err, repository.changes)
	}
}

func TestUserServiceAdminStatusChangeHonorsScopeAndInvalidatesTarget(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 42, Status: 1}}}
	transactions := &transactionFake{}
	access := &accessManagementFake{scope: domain.UserScope{UserIDs: []uint{42}}}
	service := application.NewUserService(repository, passwordFake{}, transactions, access)
	status, err := service.ToggleStatus(context.Background(), 7, 42)
	if err != nil {
		t.Fatalf("ToggleStatus() error = %v", err)
	}
	if status != 0 || repository.changes.Status == nil || *repository.changes.Status != 0 || transactions.calls != 1 || len(access.incremented) != 1 || access.incremented[0] != 42 {
		t.Fatalf("status change state: status=%d changes=%#v transactions=%d invalidated=%v", status, repository.changes, transactions.calls, access.incremented)
	}
}

func TestUserServiceRejectsTargetOutsideOperatorScope(t *testing.T) {
	repository := &managementRepositoryFake{userRepositoryFake: userRepositoryFake{user: domain.User{ID: 42, Status: 1}}}
	access := &accessManagementFake{scope: domain.UserScope{UserIDs: []uint{99}}}
	service := application.NewUserService(repository, passwordFake{}, &transactionFake{}, access)
	if _, err := service.ToggleStatus(context.Background(), 7, 42); err == nil {
		t.Fatal("ToggleStatus() accepted target outside operator scope")
	}
}
