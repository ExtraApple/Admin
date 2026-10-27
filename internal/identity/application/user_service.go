package application

import (
	"admin/internal/identity/domain"
	"context"
	"errors"
)

type UpdateSelfRequest struct {
	Nickname        string
	Email           string
	CurrentPassword string
	AvatarPresent   bool
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

type UserService struct {
	users                   UserManagementRepository
	passwords               PasswordHasher
	transactions            TransactionRunner
	access                  AccessManager
	emailVerificationIssuer EmailVerificationIssuer
	verificationEmailSender VerificationEmailSender
}

func NewUserService(users UserManagementRepository, passwords PasswordHasher, transactions TransactionRunner, access AccessManager) *UserService {
	return &UserService{users: users, passwords: passwords, transactions: transactions, access: access}
}

func (service *UserService) ConfigureEmailVerification(issuer EmailVerificationIssuer, sender VerificationEmailSender) {
	service.emailVerificationIssuer = issuer
	service.verificationEmailSender = sender
}

func (service *UserService) Get(ctx context.Context, userID uint) (domain.User, error) {

	user, err := service.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return domain.User{}, ErrUserNotFound
		}
		return domain.User{}, NewError(CodeInternalError, err)
	}
	return user, nil
}

func (service *UserService) UpdateSelf(ctx context.Context, userID uint, request UpdateSelfRequest) (domain.User, error) {
	if request.AvatarPresent {
		return domain.User{}, ErrAvatarFieldNotWritable
	}
	user, err := service.Get(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	changes := UserChanges{}
	if request.Nickname != "" {
		value := request.Nickname
		changes.Nickname = &value
	}
	emailRequested := request.Email != ""
	emailChanged := emailRequested && request.Email != user.Email
	if emailRequested {
		if err := service.passwords.Compare(user.Password, request.CurrentPassword); err != nil {
			return domain.User{}, NewValidationError([]FieldError{{Field: "current_password", ErrorCode: "IDENTITY_CURRENT_PASSWORD_INVALID", Message: "current password is invalid"}}, nil)
		}
	}
	if !emailChanged {
		if changes.Nickname == nil {
			return domain.User{}, NewError(CodeValidationInvalid, nil)
		}
		if err := service.users.Update(ctx, userID, changes); err != nil {
			return domain.User{}, NewError(CodeInternalError, err)
		}
		return service.Get(ctx, userID)
	}
	if service.emailVerificationIssuer == nil || service.verificationEmailSender == nil {
		return domain.User{}, NewError(CodeInternalError, nil)
	}

	var verificationToken string
	updateEmail := func(tx context.Context) error {
		lockedUser, err := service.users.FindByIDForUpdate(tx, userID)
		if err != nil {
			return err
		}
		if err := service.passwords.Compare(lockedUser.Password, request.CurrentPassword); err != nil {
			return NewValidationError([]FieldError{{Field: "current_password", ErrorCode: "IDENTITY_CURRENT_PASSWORD_INVALID", Message: "current password is invalid"}}, nil)
		}
		if request.Email == lockedUser.Email {
			emailChanged = false
			return nil
		}
		exists, err := service.users.EmailExists(tx, request.Email, userID)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		if exists {
			return NewError(CodeConflict, nil)
		}
		verificationToken, err = service.emailVerificationIssuer.Issue(tx, userID, request.Email)
		if err != nil {
			return err
		}
		value := request.Email
		if lockedUser.EmailVerifiedAt == nil {
			changes.Email = &value
			pending := ""
			changes.PendingEmail = &pending
			changes.ClearEmailVerifiedAt = true
		} else {
			changes.PendingEmail = &value
		}
		return service.users.Update(tx, userID, changes)
	}
	if service.transactions != nil {
		err = service.transactions.Run(ctx, updateEmail)
	} else {
		err = updateEmail(ctx)
	}
	if err != nil {
		if verificationToken != "" {
			_ = service.emailVerificationIssuer.Invalidate(ctx, userID, request.Email)
		}
		if code, ok := CodeOf(err); ok && code != CodeInternalError {
			return domain.User{}, err
		}
		return domain.User{}, service.classifyEmailUpdateFailure(ctx, userID, request.Email, err)
	}
	if emailChanged {
		if err := service.verificationEmailSender.SendVerification(ctx, request.Email, verificationToken); err != nil {
			_ = service.emailVerificationIssuer.Invalidate(ctx, userID, request.Email)
			return user, NewError(CodeEmailVerificationDeliveryFailed, err)
		}
	}
	return service.Get(ctx, userID)
}

func (service *UserService) classifyEmailUpdateFailure(ctx context.Context, userID uint, email string, cause error) *Error {
	if exists, err := service.users.EmailExists(ctx, email, userID); err == nil && exists {
		return NewError(CodeConflict, nil)
	}
	return NewError(CodeInternalError, cause)
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
	user, err := service.Get(ctx, userID)
	if err != nil {
		return err
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

func (service *UserService) UpdateByAdmin(ctx context.Context, operatorID, targetID uint, request AdminUpdateUserRequest) (domain.User, error) {
	err := service.mutateManagedTarget(ctx, operatorID, targetID, func(tx context.Context, user domain.User) error {
		if request.Email != "" {
			return NewValidationError([]FieldError{{Field: "email", ErrorCode: "IDENTITY_ADMIN_EMAIL_NOT_WRITABLE", Message: "administrator email changes use the identity email flow"}}, nil)
		}
		if request.Nickname == "" && request.Role == "" && request.Status == nil {
			return NewError(CodeValidationInvalid, nil)
		}
		var changes UserChanges
		if request.Nickname != "" && request.Nickname != user.Nickname {
			changes.Nickname = &request.Nickname
		}
		if request.Role != "" && request.Role != user.Role {
			changes.Role = &request.Role
		}
		if request.Status != nil && *request.Status != user.Status {
			changes.Status = request.Status
		}
		if changes.Nickname == nil && changes.Role == nil && changes.Status == nil {
			return nil
		}
		if err := service.users.Update(tx, targetID, changes); err != nil {
			return err
		}
		if changes.Role != nil || changes.Status != nil {
			return service.access.IncrementVersions(tx, []uint{targetID})
		}
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	return service.Get(ctx, targetID)
}

func (service *UserService) Delete(ctx context.Context, operatorID, targetID uint) error {
	return service.mutateManagedTarget(ctx, operatorID, targetID, func(tx context.Context, _ domain.User) error {
		if err := service.users.Delete(tx, targetID); err != nil {
			return err
		}
		return service.access.IncrementVersions(tx, []uint{targetID})
	})
}

func (service *UserService) ToggleStatus(ctx context.Context, operatorID, targetID uint) (int, error) {
	var status int
	err := service.mutateManagedTarget(ctx, operatorID, targetID, func(tx context.Context, user domain.User) error {
		status = 1
		if user.Status == 1 {
			status = 0
		}
		if err := service.users.Update(tx, targetID, UserChanges{Status: &status}); err != nil {
			return err
		}
		return service.access.IncrementVersions(tx, []uint{targetID})
	})
	if err != nil {
		return 0, err
	}
	return status, nil
}

func (service *UserService) Kick(ctx context.Context, operatorID, targetID uint) error {
	return service.mutateManagedTarget(ctx, operatorID, targetID, func(tx context.Context, _ domain.User) error {
		return service.access.IncrementVersions(tx, []uint{targetID})
	})
}

func (service *UserService) mutateManagedTarget(ctx context.Context, operatorID, targetID uint, mutate func(context.Context, domain.User) error) error {
	if operatorID == targetID {
		return NewError(CodeValidationInvalid, nil)
	}
	err := service.transactions.Run(ctx, func(tx context.Context) error {
		user, err := service.ensureManagedTarget(tx, operatorID, targetID)
		if err != nil {
			return err
		}
		return mutate(tx, user)
	})
	if err != nil {
		if _, classified := CodeOf(err); classified {
			return err
		}
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *UserService) ensureManagedTarget(ctx context.Context, operatorID, targetID uint) (domain.User, error) {
	if _, err := service.Get(ctx, targetID); err != nil {
		return domain.User{}, err
	}
	// Gather operator and target identity/role locks together before revisiting
	// the target or resolving scope, so both checks use the same current facts.
	protected, err := service.access.IsProtectedUser(ctx, operatorID, targetID)
	if err != nil {
		return domain.User{}, NewError(CodeInternalError, err)
	}
	user, err := service.users.FindByIDForUpdate(ctx, targetID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return domain.User{}, ErrUserNotFound
		}
		return domain.User{}, NewError(CodeInternalError, err)
	}
	scope, err := service.access.ManagementUserScope(ctx, operatorID)
	if err != nil {
		return domain.User{}, NewError(CodeInternalError, err)
	}
	if !scope.All && !containsUserID(scope.UserIDs, targetID) {
		return domain.User{}, NewError(CodePermissionDenied, nil)
	}
	if protected {
		return domain.User{}, NewError(CodePermissionDenied, nil)
	}
	return user, nil
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
