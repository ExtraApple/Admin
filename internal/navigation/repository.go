package navigation

import (
	"context"
	"errors"
	"fmt"
	"sort"

	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrMenuNotFound = errors.New("菜单不存在")

type Repository interface {
	ListMenus(context.Context, bool) ([]Menu, error)
	CountMenusByIDs(context.Context, []uint) (int64, error)
	FindMenu(context.Context, uint) (Menu, error)
	LockMenu(context.Context, uint) (Menu, error)
	ListByPermissionCode(context.Context, string) ([]Menu, error)
	LockDependencies(context.Context, []uint, []string) ([]Menu, error)
	LockMenusByPaths(context.Context, []string) ([]Menu, error)
	PathExists(context.Context, string, uint) (bool, error)
	CreateMenu(context.Context, *Menu) error
	UpdateMenu(context.Context, uint, map[string]any) error
	LockMenuDeletionTargets(context.Context, uint) ([]Menu, error)
	DeleteMenu(context.Context, uint) error
	ReplaceRoleMenus(context.Context, uint, []uint) error
	MenuIDsByRoleIDs(context.Context, []uint) ([]uint, error)
	RoleMenuAssignments(context.Context, []uint) ([]RoleMenuAssignment, error)
	RoleIDsByMenuIDs(context.Context, []uint) ([]uint, error)
	APIIDsByMenuIDs(context.Context, []uint, bool) ([]uint, error)
	MenuIDsByAPIIDs(context.Context, []uint, bool) ([]uint, error)
	ReplaceMenuAPIs(context.Context, uint, []uint) error
	DeleteMenuAPIsByAPI(context.Context, uint) error
	UpdateMenusPermissionCode(context.Context, []uint, string) error
	CountPermissionCode(context.Context, string) (int64, error)
}

type GORMRepository struct{ db *gorm.DB }

func NewGORMRepository(db *gorm.DB) *GORMRepository { return &GORMRepository{db: db} }
func (repository *GORMRepository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, repository.db)
}

func (repository *GORMRepository) ListMenus(ctx context.Context, enabledOnly bool) ([]Menu, error) {
	query := repository.connection(ctx).Order("sort asc, id asc")
	if enabledOnly {
		query = query.Where("status = 1")
	}
	var records []MenuModel
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return menuModelsToDomain(records), nil
}

func (repository *GORMRepository) CountMenusByIDs(ctx context.Context, menuIDs []uint) (int64, error) {
	if len(menuIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := repository.connection(ctx).Model(&MenuModel{}).Where("id IN ?", menuIDs).Count(&count).Error
	return count, err
}
func (repository *GORMRepository) FindMenu(ctx context.Context, id uint) (Menu, error) {
	var record MenuModel
	if err := repository.connection(ctx).First(&record, id).Error; err != nil {
		return Menu{}, mapMenuError(err)
	}
	return menuModelToDomain(record), nil
}
func (repository *GORMRepository) LockMenu(ctx context.Context, id uint) (Menu, error) {
	var record MenuModel
	if err := repository.connection(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, id).Error; err != nil {
		return Menu{}, mapMenuError(err)
	}
	return menuModelToDomain(record), nil
}

func (repository *GORMRepository) ListByPermissionCode(ctx context.Context, code string) ([]Menu, error) {
	var records []MenuModel
	if err := repository.connection(ctx).Unscoped().Where("permission_code = ?", code).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	return menuModelsToDomain(records), nil
}

func (repository *GORMRepository) LockDependencies(ctx context.Context, ids []uint, codes []string) ([]Menu, error) {
	if len(ids) == 0 && len(codes) == 0 {
		return []Menu{}, nil
	}
	var records []MenuModel
	if err := repository.connection(ctx).Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ? OR permission_code IN ?", uniqueUintIDs(ids), codes).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	return menuModelsToDomain(records), nil
}
func (repository *GORMRepository) LockMenusByPaths(ctx context.Context, paths []string) ([]Menu, error) {
	if len(paths) == 0 {
		return []Menu{}, nil
	}
	var records []MenuModel
	if err := repository.connection(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("path IN ?", paths).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	return menuModelsToDomain(records), nil
}
func (repository *GORMRepository) PathExists(ctx context.Context, path string, exceptID uint) (bool, error) {
	query := repository.connection(ctx).Model(&MenuModel{}).Where("path = ?", path)
	if exceptID != 0 {
		query = query.Where("id != ?", exceptID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
func (repository *GORMRepository) CreateMenu(ctx context.Context, menu *Menu) error {
	db := repository.connection(ctx)
	now := db.NowFunc()
	record := map[string]any{
		"parent_id":       menu.ParentID,
		"name":            menu.Name,
		"path":            menu.Path,
		"component":       menu.Component,
		"icon":            menu.Icon,
		"permission_code": menu.PermissionCode,
		"sort":            menu.Sort,
		"type":            menu.Type,
		"status":          menu.Status,
		"created_at":      now,
		"updated_at":      now,
	}
	if err := db.Model(&MenuModel{}).Create(record).Error; err != nil {
		return err
	}
	// MySQL uses LastInsertId (int64); RETURNING scans the schema's uint ID.
	switch id := record["id"].(type) {
	case int64:
		menu.ID = uint(id)
	case uint:
		menu.ID = id
	default:
		return fmt.Errorf("create menu: unexpected inserted ID type %T", id)
	}
	if menu.ID == 0 {
		return errors.New("create menu: inserted ID is zero")
	}
	return nil
}
func (repository *GORMRepository) UpdateMenu(ctx context.Context, id uint, updates map[string]any) error {
	return repository.connection(ctx).Model(&MenuModel{}).Where("id = ?", id).Updates(updates).Error
}

// LockMenuDeletionTargets includes children in the first ordered acquisition;
// a reparented lower-ID child is never locked after its higher-ID parent.
func (repository *GORMRepository) LockMenuDeletionTargets(ctx context.Context, id uint) ([]Menu, error) {
	var records []MenuModel
	err := repository.connection(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? OR parent_id = ?", id, id).Order("id asc").Find(&records).Error
	return menuModelsToDomain(records), err
}
func (repository *GORMRepository) DeleteMenu(ctx context.Context, id uint) error {
	db := repository.connection(ctx)
	if err := db.Where("menu_id = ?", id).Delete(&RoleMenuModel{}).Error; err != nil {
		return err
	}
	if err := db.Where("menu_id = ?", id).Delete(&MenuAPIModel{}).Error; err != nil {
		return err
	}
	return db.Unscoped().Delete(&MenuModel{}, id).Error
}

// DeleteRoleMenus removes only Navigation-owned references in the caller's transaction.
// Authorization holds the role lifecycle lock before invoking this capability.
func (repository *GORMRepository) DeleteRoleMenus(ctx context.Context, roleID uint) error {
	return repository.connection(ctx).Where("role_id = ?", roleID).Delete(&RoleMenuModel{}).Error
}
func (repository *GORMRepository) ReplaceRoleMenus(ctx context.Context, roleID uint, ids []uint) error {
	db := repository.connection(ctx)
	if err := db.Where("role_id = ?", roleID).Delete(&RoleMenuModel{}).Error; err != nil {
		return err
	}
	ids = uniqueUintIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	records := make([]RoleMenuModel, len(ids))
	for index, id := range ids {
		records[index] = RoleMenuModel{RoleID: roleID, MenuID: id}
	}
	return db.Create(&records).Error
}
func (repository *GORMRepository) MenuIDsByRoleIDs(ctx context.Context, roleIDs []uint) ([]uint, error) {
	roleIDs = uniqueUintIDs(roleIDs)
	if len(roleIDs) == 0 {
		return []uint{}, nil
	}
	var ids []uint
	err := repository.connection(ctx).Model(&RoleMenuModel{}).Distinct("menu_id").Where("role_id IN ?", roleIDs).Order("menu_id asc").Pluck("menu_id", &ids).Error
	return nonNilUintIDs(ids), err
}
func (repository *GORMRepository) RoleMenuAssignments(ctx context.Context, roleIDs []uint) ([]RoleMenuAssignment, error) {
	roleIDs = uniqueUintIDs(roleIDs)
	if len(roleIDs) == 0 {
		return []RoleMenuAssignment{}, nil
	}
	var assignments []RoleMenuAssignment
	err := repository.connection(ctx).Model(&RoleMenuModel{}).
		Select("role_id, menu_id").
		Where("role_id IN ?", roleIDs).
		Order("role_id asc, menu_id asc").
		Scan(&assignments).Error
	if err != nil {
		return nil, err
	}
	if assignments == nil {
		assignments = []RoleMenuAssignment{}
	}
	return assignments, nil
}

func (repository *GORMRepository) RoleIDsByMenuIDs(ctx context.Context, menuIDs []uint) ([]uint, error) {
	menuIDs = uniqueUintIDs(menuIDs)
	if len(menuIDs) == 0 {
		return []uint{}, nil
	}
	var ids []uint
	err := repository.connection(ctx).Model(&RoleMenuModel{}).Clauses(clause.Locking{Strength: "UPDATE"}).
		Distinct("role_id").Where("menu_id IN ?", menuIDs).Order("role_id asc").Pluck("role_id", &ids).Error
	return nonNilUintIDs(ids), err
}
func (repository *GORMRepository) APIIDsByMenuIDs(ctx context.Context, menuIDs []uint, lock bool) ([]uint, error) {
	menuIDs = uniqueUintIDs(menuIDs)
	if len(menuIDs) == 0 {
		return []uint{}, nil
	}
	query := repository.connection(ctx).Model(&MenuAPIModel{}).Where("menu_id IN ?", menuIDs)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var ids []uint
	err := query.Distinct("api_id").Order("api_id asc").Pluck("api_id", &ids).Error
	return nonNilUintIDs(ids), err
}
func (repository *GORMRepository) MenuIDsByAPIIDs(ctx context.Context, apiIDs []uint, lock bool) ([]uint, error) {
	apiIDs = uniqueUintIDs(apiIDs)
	if len(apiIDs) == 0 {
		return []uint{}, nil
	}
	query := repository.connection(ctx).Model(&MenuAPIModel{}).Where("api_id IN ?", apiIDs)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var ids []uint
	err := query.Distinct("menu_id").Order("menu_id asc").Pluck("menu_id", &ids).Error
	return nonNilUintIDs(ids), err
}
func (repository *GORMRepository) ReplaceMenuAPIs(ctx context.Context, menuID uint, apiIDs []uint) error {
	db := repository.connection(ctx)
	if err := db.Where("menu_id = ?", menuID).Delete(&MenuAPIModel{}).Error; err != nil {
		return err
	}
	apiIDs = uniqueUintIDs(apiIDs)
	if len(apiIDs) == 0 {
		return nil
	}
	records := make([]MenuAPIModel, len(apiIDs))
	for index, apiID := range apiIDs {
		records[index] = MenuAPIModel{MenuID: menuID, APIID: apiID}
	}
	return db.Create(&records).Error
}
func (repository *GORMRepository) DeleteMenuAPIsByAPI(ctx context.Context, apiID uint) error {
	return repository.connection(ctx).Where("api_id = ?", apiID).Delete(&MenuAPIModel{}).Error
}
func (repository *GORMRepository) UpdateMenusPermissionCode(ctx context.Context, ids []uint, code string) error {
	ids = uniqueUintIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return repository.connection(ctx).Model(&MenuModel{}).Where("id IN ?", ids).Update("permission_code", code).Error
}
func (repository *GORMRepository) CountPermissionCode(ctx context.Context, code string) (int64, error) {
	var ids []uint
	err := repository.connection(ctx).Model(&MenuModel{}).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("permission_code = ?", code).Order("id asc").Pluck("id", &ids).Error
	return int64(len(ids)), err
}

func menuModelToDomain(record MenuModel) Menu {
	return Menu{ID: record.ID, ParentID: record.ParentID, Name: record.Name, Path: record.Path, Component: record.Component, Icon: record.Icon, PermissionCode: record.PermissionCode, Sort: record.Sort, Type: record.Type, Status: record.Status, Deleted: record.DeletedAt.Valid}
}
func menuModelsToDomain(records []MenuModel) []Menu {
	result := make([]Menu, len(records))
	for i := range records {
		result[i] = menuModelToDomain(records[i])
	}
	return result
}
func mapMenuError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMenuNotFound
	}
	return err
}
func uniqueUintIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
func nonNilUintIDs(ids []uint) []uint {
	if ids == nil {
		return []uint{}
	}
	return ids
}
