package application

import (
	"context"
	"strconv"

	"admin/internal/identity/domain"
)

// DirectoryService projects Identity persistence into the safe value type
// shared by member and administrator read models.
type DirectoryService struct {
	repository DirectoryRepository
}

func NewDirectoryService(repository DirectoryRepository) *DirectoryService {
	return &DirectoryService{repository: repository}
}

func (service *DirectoryService) ListUsersByIDs(ctx context.Context, userIDs []uint) ([]domain.DirectoryUser, error) {
	if len(userIDs) == 0 {
		return []domain.DirectoryUser{}, nil
	}
	records, err := service.repository.ListUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	users := make([]domain.DirectoryUser, len(records))
	for index, record := range records {
		avatar := "/api/avatars/default"
		if record.AvatarTrusted {
			avatar = "/api/avatars/" + strconv.FormatUint(uint64(record.ID), 10)
		}
		users[index] = domain.DirectoryUser{
			ID:       record.ID,
			Username: record.Username,
			Nickname: record.Nickname,
			Avatar:   avatar,
			Email:    record.Email,
			Role:     record.Role,
			Status:   record.Status,
		}
	}
	return users, nil
}

func (service *DirectoryService) ListUserIDs(ctx context.Context) ([]uint, error) {
	userIDs, err := service.repository.ListUserIDs(ctx)
	if err != nil {
		return nil, err
	}
	if userIDs == nil {
		return []uint{}, nil
	}
	return userIDs, nil
}

var _ UserDirectory = (*DirectoryService)(nil)
