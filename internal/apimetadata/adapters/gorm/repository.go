package gormadapter

import (
	"context"
	"errors"
	"strings"

	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"
	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (repository *Repository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, repository.db)
}

func (repository *Repository) List(ctx context.Context, offset, limit int, filter application.Filter) ([]domain.API, int64, error) {
	query := repository.connection(ctx).Model(&API{})
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR path LIKE ? OR permission_code LIKE ?", like, like, like)
	}
	if group := strings.TrimSpace(filter.Group); group != "" {
		query = query.Where("api_group = ?", group)
	}
	if filter.Method != "" {
		query = query.Where("method = ?", filter.Method)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.NeedAuth != nil {
		query = query.Where("need_auth = ?", *filter.NeedAuth)
	}
	if filter.NeedAudit != nil {
		query = query.Where("need_audit = ?", *filter.NeedAudit)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []API
	if err := query.Order("api_group asc, sort asc, id asc").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return toDomainList(records), total, nil
}

func (repository *Repository) Find(ctx context.Context, id uint) (domain.API, error) {
	var record API
	if err := repository.connection(ctx).First(&record, id).Error; err != nil {
		return domain.API{}, mapError(err)
	}
	return toDomain(record), nil
}
func (repository *Repository) ExistsMethodPath(ctx context.Context, exceptID uint, method, path string) (bool, error) {
	query := repository.connection(ctx).Model(&API{}).Where("method = ? AND path = ?", method, path)
	if exceptID != 0 {
		query = query.Where("id != ?", exceptID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (repository *Repository) ListByIDsForUpdate(ctx context.Context, ids []uint) ([]domain.API, error) {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return []domain.API{}, nil
	}
	var records []API
	if err := repository.connection(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	return toDomainList(records), nil
}

func (repository *Repository) SetPermissionCode(ctx context.Context, ids []uint, code string) error {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return repository.connection(ctx).Model(&API{}).Where("id IN ?", ids).Update("permission_code", strings.TrimSpace(code)).Error
}

func (repository *Repository) CountPermissionCode(ctx context.Context, code string) (int64, error) {
	var count int64
	err := repository.connection(ctx).Model(&API{}).Where("permission_code = ?", strings.TrimSpace(code)).Count(&count).Error
	return count, err
}
func (repository *Repository) ListByIDs(ctx context.Context, ids []uint) ([]domain.API, error) {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return []domain.API{}, nil
	}
	var records []API
	if err := repository.connection(ctx).Where("id IN ?", ids).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	return toDomainList(records), nil
}

func (repository *Repository) FindPolicyByRoute(ctx context.Context, method, path string) (domain.Policy, error) {
	var record API
	if err := repository.connection(ctx).Where("method = ? AND path = ?", method, path).First(&record).Error; err != nil {
		return domain.Policy{}, mapError(err)
	}
	return domain.Policy{Status: record.Status, NeedAuth: record.NeedAuth, NeedAudit: record.NeedAudit, PermissionCode: record.PermissionCode}, nil
}

func (repository *Repository) Groups(ctx context.Context) ([]domain.GroupOption, error) {
	var result []domain.GroupOption
	if err := repository.connection(ctx).Model(&API{}).Select("api_group AS `group`, COUNT(*) AS count").Group("api_group").Order("api_group asc").Scan(&result).Error; err != nil {
		return nil, err
	}
	for index := range result {
		if strings.TrimSpace(result[index].Group) == "" {
			result[index].Group = "api"
		}
	}
	return result, nil
}

func (repository *Repository) Create(ctx context.Context, api *domain.API) error {
	record := fromDomain(*api)
	db := repository.connection(ctx)
	if err := db.Create(&record).Error; err != nil {
		return err
	}
	zeroValues := make(map[string]any)
	if api.Status == 0 {
		zeroValues["status"] = 0
	}
	if api.NeedAuth == 0 {
		zeroValues["need_auth"] = 0
	}
	if api.NeedAudit == 0 {
		zeroValues["need_audit"] = 0
	}
	if len(zeroValues) > 0 {
		if err := db.Model(&record).Updates(zeroValues).Error; err != nil {
			return err
		}
		record.Status, record.NeedAuth, record.NeedAudit = api.Status, api.NeedAuth, api.NeedAudit
	}
	*api = toDomain(record)
	return nil
}

func (repository *Repository) FindByMethodPathUnscoped(ctx context.Context, method, path string) (domain.API, bool, error) {
	var record API
	err := repository.connection(ctx).Unscoped().Where("method = ? AND path = ?", method, path).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.API{}, false, nil
	}
	if err != nil {
		return domain.API{}, false, err
	}
	return toDomain(record), record.DeletedAt.Valid, nil
}

func (repository *Repository) Restore(ctx context.Context, id uint, updates map[string]any) error {
	return repository.connection(ctx).Unscoped().Model(&API{}).Where("id = ?", id).Updates(updates).Error
}

func (repository *Repository) Update(ctx context.Context, id uint, updates map[string]any) error {
	return repository.connection(ctx).Model(&API{}).Where("id = ?", id).Updates(updates).Error
}

func (repository *Repository) Delete(ctx context.Context, id uint) error {
	return repository.connection(ctx).Unscoped().Delete(&API{}, id).Error
}

func toDomain(record API) domain.API {
	return domain.API{ID: record.ID, Name: record.Name, Method: record.Method, Path: record.Path, Group: record.Group, PermissionCode: record.PermissionCode, Remark: record.Remark, Sort: record.Sort, Status: record.Status, NeedAuth: record.NeedAuth, NeedAudit: record.NeedAudit, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}
func toDomainList(records []API) []domain.API {
	result := make([]domain.API, len(records))
	for index := range records {
		result[index] = toDomain(records[index])
	}
	return result
}
func fromDomain(api domain.API) API {
	return API{Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group, PermissionCode: api.PermissionCode, Remark: api.Remark, Sort: api.Sort, Status: api.Status, NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit}
}
func mapError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}
func uniqueIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
