package gormadapter

import (
	"context"
	"errors"

	"admin/internal/identity"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
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
	return domain.User{ID: user.ID, Username: user.Username, Password: user.Password, Nickname: user.Nickname, Avatar: user.Avatar, AvatarObjectName: user.AvatarObjectName, AvatarContentType: user.AvatarContentType, AvatarContentSHA256: user.AvatarContentSHA256, AvatarValidationStatus: user.AvatarValidationStatus, AvatarValidatedAt: user.AvatarValidatedAt, Role: user.Role, Status: user.Status, Email: user.Email}
}

func fromDomain(user domain.User) UserModel {
	return UserModel{Username: user.Username, Password: user.Password, Nickname: user.Nickname, Avatar: user.Avatar, AvatarObjectName: user.AvatarObjectName, AvatarContentType: user.AvatarContentType, AvatarContentSHA256: user.AvatarContentSHA256, AvatarValidationStatus: user.AvatarValidationStatus, AvatarValidatedAt: user.AvatarValidatedAt, Role: user.Role, Status: user.Status, Email: user.Email}
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
	query := repository.connection(ctx).Model(&UserModel{}).Where("email = ?", email)
	if exceptID != 0 {
		query = query.Where("id != ?", exceptID)
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (repository *Repository) Update(ctx context.Context, userID uint, changes application.UserChanges) error {
	updates := make(map[string]any, 4)
	if changes.Nickname != nil {
		updates["nickname"] = *changes.Nickname
	}
	if changes.Email != nil {
		updates["email"] = *changes.Email
	}
	if changes.Role != nil {
		updates["role"] = *changes.Role
	}
	if changes.Status != nil {
		updates["status"] = *changes.Status
	}
	return repository.connection(ctx).Model(&UserModel{}).Where("id = ?", userID).Updates(updates).Error
}

func (repository *Repository) UpdatePassword(ctx context.Context, userID uint, password string) error {
	return repository.connection(ctx).Model(&UserModel{}).Where("id = ?", userID).Update("password", password).Error
}

func (repository *Repository) Delete(ctx context.Context, userID uint) error {
	return repository.connection(ctx).Delete(&UserModel{}, userID).Error
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
