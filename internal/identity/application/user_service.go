package application

import (
	"context"

	"admin/internal/identity/domain"
)

type UpdateSelfRequest struct {
	Nickname      string
	Email         string
	AvatarPresent bool
}

type ChangePasswordRequest struct {
	OldPassword     string
	NewPassword     string
	ConfirmPassword string
}

type AdminUpdateUserRequest struct {
	Nickname string
	Email    string
	Role     string
	Status   *int
}

type UserPage struct {
	List  []domain.User
	Total int64
	Page  int
	Size  int
}

type UserService struct {
	users        UserManagementRepository
	passwords    PasswordHasher
	transactions TransactionRunner
	access       AccessManager
}

func NewUserService(users UserManagementRepository, passwords PasswordHasher, transactions TransactionRunner, access AccessManager) *UserService {
	return &UserService{users: users, passwords: passwords, transactions: transactions, access: access}
}

func (service *UserService) Get(ctx context.Context, userID uint) (domain.User, error) {
	user, err := service.users.FindByID(ctx, userID)
	if err != nil {
		return domain.User{}, ErrUserNotFound
	}
	return user, nil
}

func (service *UserService) UpdateSelf(ctx context.Context, userID uint, request UpdateSelfRequest) (domain.User, error) {
	if request.AvatarPresent {
		return domain.User{}, ErrAvatarFieldNotWritable
	}
	changes := UserChanges{}
	if request.Nickname != "" {
		value := request.Nickname
		changes.Nickname = &value
	}
	if request.Email != "" {
		exists, err := service.users.EmailExists(ctx, request.Email, userID)
		if err != nil {
			return domain.User{}, NewError(CodeInternalError, err)
		}
		if exists {
			return domain.User{}, NewError(CodeConflict, nil)
		}
		value := request.Email
		changes.Email = &value
	}
	if changes.Nickname == nil && changes.Email == nil {
		return domain.User{}, NewError(CodeValidationInvalid, nil)
	}
	if err := service.users.Update(ctx, userID, changes); err != nil {
		return domain.User{}, NewError(CodeInternalError, err)
	}
	return service.Get(ctx, userID)
}

func (service *UserService) ChangePassword(ctx context.Context, userID uint, request ChangePasswordRequest) error {
	if request.NewPassword != request.ConfirmPassword {
		return NewValidationError([]FieldError{{Field: "confirm_password", ErrorCode: "IDENTITY_PASSWORD_CONFIRMATION_INVALID", Message: "password confirmation does not match"}}, nil)
	}
	if err := validatePassword(request.NewPassword); err != nil {
		return err
	}
	if request.OldPassword == request.NewPassword {
		return NewValidationError([]FieldError{{Field: "new_password", ErrorCode: "IDENTITY_PASSWORD_REUSE", Message: "new password must differ from the old password"}}, nil)
	}
	user, err := service.users.FindByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}
	if err := service.passwords.Compare(user.Password, request.OldPassword); err != nil {
		return NewValidationError([]FieldError{{Field: "old_password", ErrorCode: "IDENTITY_OLD_PASSWORD_INVALID", Message: "old password is invalid"}}, nil)
	}
	hashed, err := service.passwords.Hash(request.NewPassword)
	if err != nil {
		return NewError(CodeInternalError, err)
	}
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		if err := service.users.UpdatePassword(tx, userID, hashed); err != nil {
			return err
		}
		return service.access.IncrementVersions(tx, []uint{userID})
	}); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *UserService) List(ctx context.Context, operatorID uint, page, size int) (UserPage, error) {
	page, size = normalizeUserPage(page, size)
	scope, err := service.access.UserScope(ctx, operatorID)
	if err != nil {
		return UserPage{}, NewError(CodeInternalError, err)
	}
	users, total, err := service.users.List(ctx, (page-1)*size, size, scope)
	if err != nil {
		return UserPage{}, NewError(CodeInternalError, err)
	}
	if users == nil {
		users = []domain.User{}
	}
	return UserPage{List: users, Total: total, Page: page, Size: size}, nil
}

func (service *UserService) UpdateByAdmin(ctx context.Context, operatorID, targetID uint, request AdminUpdateUserRequest) (domain.User, error) {
	if operatorID == targetID {
		return domain.User{}, NewError(CodeValidationInvalid, nil)
	}
	if err := service.ensureManagedTarget(ctx, operatorID, targetID); err != nil {
		return domain.User{}, err
	}
	changes := UserChanges{}
	if request.Nickname != "" {
		value := request.Nickname
		changes.Nickname = &value
	}
	if request.Email != "" {
		exists, err := service.users.EmailExists(ctx, request.Email, targetID)
		if err != nil {
			return domain.User{}, NewError(CodeInternalError, err)
		}
		if exists {
			return domain.User{}, NewError(CodeConflict, nil)
		}
		value := request.Email
		changes.Email = &value
	}
	if request.Role != "" {
		value := request.Role
		changes.Role = &value
	}
	changes.Status = request.Status
	if changes.Nickname == nil && changes.Email == nil && changes.Role == nil && changes.Status == nil {
		return domain.User{}, NewError(CodeValidationInvalid, nil)
	}
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		if err := service.users.Update(tx, targetID, changes); err != nil {
			return err
		}
		return service.access.IncrementVersions(tx, []uint{targetID})
	}); err != nil {
		return domain.User{}, NewError(CodeInternalError, err)
	}
	return service.Get(ctx, targetID)
}

func (service *UserService) Delete(ctx context.Context, operatorID, targetID uint) error {
	if operatorID == targetID {
		return NewError(CodeValidationInvalid, nil)
	}
	if err := service.ensureManagedTarget(ctx, operatorID, targetID); err != nil {
		return err
	}
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		if err := service.users.Delete(tx, targetID); err != nil {
			return err
		}
		return service.access.IncrementVersions(tx, []uint{targetID})
	}); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *UserService) ToggleStatus(ctx context.Context, operatorID, targetID uint) (int, error) {
	if operatorID == targetID {
		return 0, NewError(CodeValidationInvalid, nil)
	}
	if err := service.ensureManagedTarget(ctx, operatorID, targetID); err != nil {
		return 0, err
	}
	user, err := service.users.FindByID(ctx, targetID)
	if err != nil {
		return 0, ErrUserNotFound
	}
	status := 1
	if user.Status == 1 {
		status = 0
	}
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		if err := service.users.Update(tx, targetID, UserChanges{Status: &status}); err != nil {
			return err
		}
		return service.access.IncrementVersions(tx, []uint{targetID})
	}); err != nil {
		return 0, NewError(CodeInternalError, err)
	}
	return status, nil
}

func (service *UserService) Kick(ctx context.Context, operatorID, targetID uint) error {
	if operatorID == targetID {
		return NewError(CodeValidationInvalid, nil)
	}
	if err := service.ensureManagedTarget(ctx, operatorID, targetID); err != nil {
		return err
	}
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		return service.access.IncrementVersions(tx, []uint{targetID})
	}); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *UserService) ensureManagedTarget(ctx context.Context, operatorID, targetID uint) error {
	if _, err := service.users.FindByID(ctx, targetID); err != nil {
		return ErrUserNotFound
	}
	scope, err := service.access.UserScope(ctx, operatorID)
	if err != nil {
		return NewError(CodeInternalError, err)
	}
	if !scope.All && !containsUserID(scope.UserIDs, targetID) {
		return NewError(CodePermissionDenied, nil)
	}
	administrator, err := service.access.IsAdministrator(ctx, targetID)
	if err != nil {
		return NewError(CodeInternalError, err)
	}
	if administrator {
		return NewError(CodePermissionDenied, nil)
	}
	return nil
}

func containsUserID(ids []uint, target uint) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
func normalizeUserPage(page, size int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	return page, size
}
