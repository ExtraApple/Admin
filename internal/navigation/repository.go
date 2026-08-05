package navigation

import (
	"context"
	"errors"

	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrMenuNotFound = errors.New("菜单不存在")

type Repository interface {
	ListMenus(context.Context, bool) ([]Menu, error)
	FindMenu(context.Context, uint) (Menu, error)
	LockMenu(context.Context, uint) (Menu, error)
	FindMenuByPath(context.Context, string) (Menu, bool, error)
	PathExists(context.Context, string, uint) (bool, error)
	CreateMenu(context.Context, *Menu) error
	UpdateMenu(context.Context, uint, map[string]any) error
	ChildCount(context.Context, uint) (int64, error)
	DeleteMenu(context.Context, uint) error
	ReplaceRoleMenus(context.Context, uint, []uint) error
	MenuIDsByRoleIDs(context.Context, []uint) ([]uint, error)
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
func (repository *GORMRepository) FindMenuByPath(ctx context.Context, path string) (Menu, bool, error) {
	var record MenuModel
	err := repository.connection(ctx).Where("path = ?", path).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Menu{}, false, nil
	}
	if err != nil {
		return Menu{}, false, err
	}
	return menuModelToDomain(record), true, nil
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
	record := menuDomainToModel(*menu)
	if err := repository.connection(ctx).Create(&record).Error; err != nil {
		return err
	}
	*menu = menuModelToDomain(record)
	return nil
}
func (repository *GORMRepository) UpdateMenu(ctx context.Context, id uint, updates map[string]any) error {
	return repository.connection(ctx).Model(&MenuModel{}).Where("id = ?", id).Updates(updates).Error
}
func (repository *GORMRepository) ChildCount(ctx context.Context, id uint) (int64, error) {
	var count int64
	err := repository.connection(ctx).Model(&MenuModel{}).Where("parent_id = ?", id).Count(&count).Error
	return count, err
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
func (repository *GORMRepository) RoleIDsByMenuIDs(ctx context.Context, menuIDs []uint) ([]uint, error) {
	menuIDs = uniqueUintIDs(menuIDs)
	if len(menuIDs) == 0 {
		return []uint{}, nil
	}
	var ids []uint
	err := repository.connection(ctx).Model(&RoleMenuModel{}).Distinct("role_id").Where("menu_id IN ?", menuIDs).Order("role_id asc").Pluck("role_id", &ids).Error
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
	var count int64
	err := repository.connection(ctx).Model(&MenuModel{}).Where("permission_code = ?", code).Count(&count).Error
	return count, err
}

func menuModelToDomain(record MenuModel) Menu {
	return Menu{ID: record.ID, ParentID: record.ParentID, Name: record.Name, Path: record.Path, Component: record.Component, Icon: record.Icon, PermissionCode: record.PermissionCode, Sort: record.Sort, Type: record.Type, Status: record.Status}
}
func menuModelsToDomain(records []MenuModel) []Menu {
	result := make([]Menu, len(records))
	for i := range records {
		result[i] = menuModelToDomain(records[i])
	}
	return result
}
func menuDomainToModel(menu Menu) MenuModel {
	return MenuModel{ParentID: menu.ParentID, Name: menu.Name, Path: menu.Path, Component: menu.Component, Icon: menu.Icon, PermissionCode: menu.PermissionCode, Sort: menu.Sort, Type: menu.Type, Status: menu.Status}
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
	return result
}
func nonNilUintIDs(ids []uint) []uint {
	if ids == nil {
		return []uint{}
	}
	return ids
}
