package organization

import (
	"context"
	"errors"
	"time"

	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("organization not found")

type Repository interface {
	MemberOrganizationIDs(context.Context, uint) ([]uint, error)
	OrganizationParents(context.Context) ([]OrganizationParent, error)
	ListUnits(context.Context, int, int, string, *int, OrganizationScope) ([]Unit, int64, error)
	ListAllUnits(context.Context) ([]Unit, error)
	ListUnitsByIDs(context.Context, []uint) ([]Unit, error)
	FindUnitByID(context.Context, uint) (Unit, error)
	CodeExists(context.Context, string, uint) (bool, error)
	CreateUnit(context.Context, *Unit) error
	UpdateUnit(context.Context, uint, map[string]any) error
	DeleteUnit(context.Context, *Unit) error
	ChildCount(context.Context, uint) (int64, error)
	MemberUserIDs(context.Context, uint) ([]uint, error)
	MemberOrganizationMemberships(context.Context, uint) ([]MembershipFact, error)
	DeleteMemberships(context.Context, uint) error
	CreateMemberships(context.Context, []Membership) error
	ReplaceMemberships(context.Context, uint, []uint) error
}

type OrganizationParent struct {
	ID       uint
	ParentID uint
}

// MembershipFact is the caller-safe current membership record. JoinedAt is
// preserved when a membership remains in an administrative replacement.
type MembershipFact struct {
	OrganizationID uint
	JoinedAt       time.Time
}

type gormRepository struct {
	db *gorm.DB
}

func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (repository *gormRepository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, repository.db)
}

func (repository *gormRepository) MemberOrganizationIDs(ctx context.Context, userID uint) ([]uint, error) {
	var organizationIDs []uint
	err := repository.connection(ctx).
		Model(&Membership{}).
		Where("user_id = ?", userID).
		Order("organization_id asc").
		Pluck("organization_id", &organizationIDs).Error
	return organizationIDs, err
}

func (repository *gormRepository) MemberOrganizationMemberships(ctx context.Context, userID uint) ([]MembershipFact, error) {
	var records []MembershipFact
	err := repository.connection(ctx).
		Model(&Membership{}).
		Select("organization_id", "created_at AS joined_at").
		Where("user_id = ?", userID).
		Order("organization_id asc").
		Scan(&records).Error
	return records, err
}

func (repository *gormRepository) OrganizationParents(ctx context.Context) ([]OrganizationParent, error) {
	var organizations []OrganizationParent
	err := repository.connection(ctx).
		Model(&Unit{}).
		Select("id", "parent_id").
		Order("id asc").
		Scan(&organizations).Error
	return organizations, err
}

func (repository *gormRepository) ListUnits(ctx context.Context, offset, limit int, keyword string, status *int, scope OrganizationScope) ([]Unit, int64, error) {
	var units []Unit
	var total int64
	query := repository.connection(ctx).Model(&Unit{})
	if !scope.All {
		if len(scope.OrganizationIDs) == 0 {
			return []Unit{}, 0, nil
		}
		query = query.Where("id IN ?", scope.OrganizationIDs)
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("sort asc, id asc").Limit(limit).Offset(offset).Find(&units).Error; err != nil {
		return nil, 0, err
	}
	return units, total, nil
}

func (repository *gormRepository) ListAllUnits(ctx context.Context) ([]Unit, error) {
	var units []Unit
	err := repository.connection(ctx).Order("sort asc, id asc").Find(&units).Error
	return units, err
}

func (repository *gormRepository) ListUnitsByIDs(ctx context.Context, ids []uint) ([]Unit, error) {
	if len(ids) == 0 {
		return []Unit{}, nil
	}
	var units []Unit
	err := repository.connection(ctx).Where("id IN ?", ids).Order("sort asc, id asc").Find(&units).Error
	return units, err
}

func (repository *gormRepository) FindUnitByID(ctx context.Context, unitID uint) (Unit, error) {
	var unit Unit
	err := repository.connection(ctx).First(&unit, unitID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Unit{}, ErrNotFound
	}
	return unit, err
}

func (repository *gormRepository) CodeExists(ctx context.Context, code string, exceptID uint) (bool, error) {
	var count int64
	query := repository.connection(ctx).Model(&Unit{}).Where("code = ?", code)
	if exceptID > 0 {
		query = query.Where("id != ?", exceptID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (repository *gormRepository) CreateUnit(ctx context.Context, unit *Unit) error {
	return repository.connection(ctx).Create(unit).Error
}

func (repository *gormRepository) UpdateUnit(ctx context.Context, unitID uint, updates map[string]any) error {
	return repository.connection(ctx).Model(&Unit{}).Where("id = ?", unitID).Updates(updates).Error
}

func (repository *gormRepository) DeleteUnit(ctx context.Context, unit *Unit) error {
	return repository.connection(ctx).Unscoped().Delete(unit).Error
}

func (repository *gormRepository) ChildCount(ctx context.Context, unitID uint) (int64, error) {
	var count int64
	err := repository.connection(ctx).Model(&Unit{}).Where("parent_id = ?", unitID).Count(&count).Error
	return count, err
}

func (repository *gormRepository) MemberUserIDs(ctx context.Context, unitID uint) ([]uint, error) {
	var userIDs []uint
	err := repository.connection(ctx).Model(&Membership{}).Where("organization_id = ?", unitID).Order("user_id asc").Pluck("user_id", &userIDs).Error
	return userIDs, err
}

func (repository *gormRepository) DeleteMemberships(ctx context.Context, unitID uint) error {
	return repository.connection(ctx).Where("organization_id = ?", unitID).Delete(&Membership{}).Error
}

func (repository *gormRepository) CreateMemberships(ctx context.Context, memberships []Membership) error {
	if len(memberships) == 0 {
		return nil
	}
	return repository.connection(ctx).Create(&memberships).Error
}

// ReplaceMemberships preserves the membership record, and therefore its
// JoinedAt, for every user that remains assigned to the organization.
func (repository *gormRepository) ReplaceMemberships(ctx context.Context, unitID uint, userIDs []uint) error {
	db := repository.connection(ctx)
	userIDs = uniqueMembershipUserIDs(userIDs)
	if len(userIDs) == 0 {
		return db.Where("organization_id = ?", unitID).Delete(&Membership{}).Error
	}
	if err := db.Where("organization_id = ? AND user_id NOT IN ?", unitID, userIDs).Delete(&Membership{}).Error; err != nil {
		return err
	}
	memberships := make([]Membership, len(userIDs))
	for index, userID := range userIDs {
		memberships[index] = Membership{UserID: userID, OrganizationID: unitID}
	}
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "organization_id"}}, DoNothing: true}).Create(&memberships).Error
}

func uniqueMembershipUserIDs(userIDs []uint) []uint {
	unique := make([]uint, 0, len(userIDs))
	seen := make(map[uint]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		unique = append(unique, userID)
	}
	return unique
}
