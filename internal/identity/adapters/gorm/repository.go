package gormadapter

import (
	"context"
	"errors"
	"time"

	"admin/internal/identity"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserModel = identity.User

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, repository.db)
}

func (repository *Repository) FindByUsername(ctx context.Context, username string) (domain.User, error) {
	var user UserModel
	if err := repository.connection(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.User{}, application.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return toDomain(user), nil
}

func (repository *Repository) FindByID(ctx context.Context, userID uint) (domain.User, error) {
	var user UserModel
	if err := repository.connection(ctx).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.User{}, application.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return toDomain(user), nil
}

func (repository *Repository) FindByIDForUpdate(ctx context.Context, userID uint) (domain.User, error) {
	var user UserModel
	if err := repository.connection(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.User{}, application.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return toDomain(user), nil
}

func (repository *Repository) ExistsByUsernameOrEmail(ctx context.Context, username, email string) (bool, error) {
	var count int64
	err := repository.connection(ctx).Model(&UserModel{}).Where("username = ? OR email = ?", username, email).Count(&count).Error
	return count > 0, err
}

func (repository *Repository) Create(ctx context.Context, user *domain.User) error {
	record := fromDomain(*user)
	if err := repository.connection(ctx).Create(&record).Error; err != nil {
		return err
	}
	*user = toDomain(record)
	return nil
}
func toDomain(user UserModel) domain.User {
	return domain.User{ID: user.ID, Username: user.Username, Password: user.Password, Nickname: user.Nickname, Avatar: user.Avatar, AvatarObjectName: user.AvatarObjectName, AvatarContentType: user.AvatarContentType, AvatarContentSHA256: user.AvatarContentSHA256, AvatarValidationStatus: user.AvatarValidationStatus, AvatarValidatedAt: user.AvatarValidatedAt, Role: user.Role, Status: user.Status, Email: user.Email, PendingEmail: user.PendingEmail, EmailVerifiedAt: user.EmailVerifiedAt}
}

func fromDomain(user domain.User) UserModel {
	return UserModel{Username: user.Username, Password: user.Password, Nickname: user.Nickname, Avatar: user.Avatar, AvatarObjectName: user.AvatarObjectName, AvatarContentType: user.AvatarContentType, AvatarContentSHA256: user.AvatarContentSHA256, AvatarValidationStatus: user.AvatarValidationStatus, AvatarValidatedAt: user.AvatarValidatedAt, Role: user.Role, Status: user.Status, Email: user.Email, PendingEmail: user.PendingEmail, EmailVerifiedAt: user.EmailVerifiedAt}
}

var _ application.UserRepository = (*Repository)(nil)

func (repository *Repository) ListUsersByIDs(ctx context.Context, userIDs []uint) ([]application.DirectoryUserRecord, error) {
	if len(userIDs) == 0 {
		return []application.DirectoryUserRecord{}, nil
	}
	var users []UserModel
	if err := repository.connection(ctx).
		Select("id", "username", "nickname", "email", "role", "status", "avatar_object_name", "avatar_content_type", "avatar_validation_status").
		Where("id IN ?", userIDs).
		Order("id asc").
		Find(&users).Error; err != nil {
		return nil, err
	}
	records := make([]application.DirectoryUserRecord, len(users))
	for index, user := range users {
		_, avatarTrusted := domain.TrustedAvatarObjectName(toDomain(user), user.ID)
		records[index] = application.DirectoryUserRecord{
			ID: user.ID, Username: user.Username, Nickname: user.Nickname,
			Email: user.Email, Role: user.Role, Status: user.Status,
			AvatarTrusted: avatarTrusted,
		}
	}
	return records, nil
}

func (repository *Repository) ListUserIDs(ctx context.Context) ([]uint, error) {
	var userIDs []uint
	if err := repository.connection(ctx).Model(&UserModel{}).Order("id asc").Pluck("id", &userIDs).Error; err != nil {
		return nil, err
	}
	if userIDs == nil {
		return []uint{}, nil
	}
	return userIDs, nil
}

var _ application.DirectoryRepository = (*Repository)(nil)

func (repository *Repository) List(ctx context.Context, offset, limit int, scope domain.UserScope) ([]domain.User, int64, error) {
	query := repository.connection(ctx).Model(&UserModel{})
	if !scope.All {
		if len(scope.UserIDs) == 0 {
			return []domain.User{}, 0, nil
		}
		query = query.Where("id IN ?", scope.UserIDs)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []UserModel
	if err := query.Order("id asc").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	users := make([]domain.User, len(records))
	for index := range records {
		users[index] = toDomain(records[index])
	}
	return users, total, nil
}

func (repository *Repository) EmailExists(ctx context.Context, email string, exceptID uint) (bool, error) {
	query := repository.connection(ctx).Model(&UserModel{}).Where("email = ? OR pending_email = ?", email, email)
	if exceptID != 0 {
		query = query.Where("id != ?", exceptID)
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (repository *Repository) Update(ctx context.Context, userID uint, changes application.UserChanges) error {
	updates := make(map[string]any, 6)
	if changes.Nickname != nil {
		updates["nickname"] = *changes.Nickname
	}
	if changes.Email != nil {
		updates["email"] = *changes.Email
	}
	if changes.PendingEmail != nil {
		updates["pending_email"] = *changes.PendingEmail
	}
	if changes.ClearEmailVerifiedAt {
		updates["email_verified_at"] = nil
	}
	if changes.Role != nil {
		updates["role"] = *changes.Role
	}
	if changes.Status != nil {
		updates["status"] = *changes.Status
	}
	return repository.connection(ctx).Model(&UserModel{}).Where("id = ?", userID).Updates(updates).Error
}
func (repository *Repository) Delete(ctx context.Context, userID uint) error {
	return repository.connection(ctx).Delete(&UserModel{}, userID).Error
}

func (repository *Repository) UpdatePassword(ctx context.Context, userID uint, password string) error {
	return repository.connection(ctx).Model(&UserModel{}).Where("id = ?", userID).Update("password", password).Error
}

var _ application.UserManagementRepository = (*Repository)(nil)

func (repository *Repository) UpdateAvatar(ctx context.Context, userID uint, update application.AvatarUpdate) (application.AvatarUpdateOutcome, error) {
	result := repository.connection(ctx).Model(&UserModel{}).Where("id = ?", userID).Updates(map[string]any{
		"avatar_object_name":       update.ObjectName,
		"avatar_content_type":      update.ContentType,
		"avatar_content_sha256":    update.ContentSHA256,
		"avatar_validation_status": update.ValidationStatus,
		"avatar_validated_at":      update.ValidatedAt,
	})
	if result.Error != nil {
		return application.AvatarUpdateUnknown, result.Error
	}
	if result.RowsAffected != 1 {
		return application.AvatarUpdateNotCommitted, gorm.ErrRecordNotFound
	}
	return application.AvatarUpdateCommitted, nil
}

var _ application.AvatarRepository = (*Repository)(nil)

func (repository *Repository) CountIssuedSince(ctx context.Context, userID uint, since time.Time) (int, error) {
	var count int64
	err := repository.connection(ctx).Model(&identity.EmailVerificationCredential{}).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Count(&count).Error
	return int(count), err
}

func (repository *Repository) IssueCredential(ctx context.Context, credential domain.EmailVerificationCredential, since time.Time, limit int) (bool, error) {
	if credential.UserID == 0 || limit <= 0 {
		return false, nil
	}
	issued := false
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		var user UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, credential.UserID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&identity.EmailVerificationCredential{}).
			Where("user_id = ? AND created_at >= ?", credential.UserID, since).
			Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return nil
		}
		now := credential.CreatedAt
		if err := tx.Model(&identity.EmailVerificationCredential{}).
			Where("user_id = ? AND used_at IS NULL", credential.UserID).
			Update("used_at", now).Error; err != nil {
			return err
		}
		record := identity.EmailVerificationCredential{
			Model:  gorm.Model{CreatedAt: credential.CreatedAt},
			UserID: credential.UserID, Email: credential.Email, TokenHash: credential.TokenHash,
			ExpiresAt: credential.ExpiresAt, UsedAt: credential.UsedAt,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		issued = true
		return nil
	})
	return issued, err
}

func (repository *Repository) ReplaceActive(ctx context.Context, credential domain.EmailVerificationCredential) error {
	db := repository.connection(ctx)
	now := credential.CreatedAt
	if err := db.Model(&identity.EmailVerificationCredential{}).
		Where("user_id = ? AND used_at IS NULL", credential.UserID).
		Update("used_at", now).Error; err != nil {
		return err
	}
	record := identity.EmailVerificationCredential{
		Model:  gorm.Model{CreatedAt: credential.CreatedAt},
		UserID: credential.UserID, Email: credential.Email, TokenHash: credential.TokenHash,
		ExpiresAt: credential.ExpiresAt, UsedAt: credential.UsedAt,
	}
	return db.Create(&record).Error
}

var _ application.EmailVerificationRepository = (*Repository)(nil)

func (repository *Repository) DeleteTerminalBefore(ctx context.Context, before time.Time) error {
	return repository.connection(ctx).Unscoped().
		Where("(used_at IS NOT NULL AND used_at <= ?) OR (used_at IS NULL AND expires_at <= ?)", before, before).
		Delete(&identity.EmailVerificationCredential{}).Error
}

func (repository *Repository) InvalidateActive(ctx context.Context, userID uint, email string, at time.Time) error {
	return repository.connection(ctx).Model(&identity.EmailVerificationCredential{}).
		Where("user_id = ? AND email = ? AND used_at IS NULL", userID, email).
		Update("used_at", at).Error
}

func (repository *Repository) Confirm(ctx context.Context, userID uint, tokenHash string, at time.Time) (application.EmailVerificationConfirmation, error) {
	db := repository.connection(ctx)
	var credential identity.EmailVerificationCredential
	if err := db.Where("user_id = ? AND token_hash = ?", userID, tokenHash).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.EmailVerificationConfirmation{}, application.ErrEmailVerificationInvalid
		}
		return application.EmailVerificationConfirmation{}, err
	}
	if credential.UsedAt != nil || !at.Before(credential.ExpiresAt) {
		return application.EmailVerificationConfirmation{}, application.ErrEmailVerificationInvalid
	}
	var user identity.User
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.EmailVerificationConfirmation{}, application.ErrEmailVerificationInvalid
		}
		return application.EmailVerificationConfirmation{}, err
	}
	if user.PendingEmail != credential.Email && !(user.PendingEmail == "" && user.Email == credential.Email && user.EmailVerifiedAt == nil) {
		return application.EmailVerificationConfirmation{}, application.ErrEmailVerificationInvalid
	}
	var occupied int64
	if err := db.Model(&identity.User{}).
		Where("(email = ? OR pending_email = ?) AND id <> ?", credential.Email, credential.Email, userID).
		Count(&occupied).Error; err != nil {
		return application.EmailVerificationConfirmation{}, err
	}
	if occupied > 0 {
		result := db.Model(&identity.EmailVerificationCredential{}).Where("id = ? AND used_at IS NULL", credential.ID).Update("used_at", at)
		if result.Error != nil {
			return application.EmailVerificationConfirmation{}, result.Error
		}
		if result.RowsAffected != 1 {
			return application.EmailVerificationConfirmation{}, application.ErrEmailVerificationInvalid
		}
		return application.EmailVerificationConfirmation{Conflict: true}, nil
	}
	updates := map[string]any{"email_verified_at": at}
	if user.PendingEmail == credential.Email {
		updates["email"] = credential.Email
		updates["pending_email"] = ""
	}
	result := db.Model(&identity.User{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		return application.EmailVerificationConfirmation{}, result.Error
	}
	if result.RowsAffected != 1 {
		return application.EmailVerificationConfirmation{}, application.ErrEmailVerificationInvalid
	}
	result = db.Model(&identity.EmailVerificationCredential{}).Where("id = ? AND used_at IS NULL", credential.ID).Update("used_at", at)
	if result.Error != nil {
		return application.EmailVerificationConfirmation{}, result.Error
	}
	if result.RowsAffected != 1 {
		return application.EmailVerificationConfirmation{}, application.ErrEmailVerificationInvalid
	}
	return application.EmailVerificationConfirmation{}, nil
}
