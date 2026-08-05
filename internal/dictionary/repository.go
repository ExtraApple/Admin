package dictionary

import (
	"context"

	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	TypeCodeExists(ctx context.Context, code string, exceptID uint) (bool, error)
	CreateType(ctx context.Context, dictionaryType *Type) error
	ListTypes(ctx context.Context, offset, limit int, keyword string, status *int) ([]Type, int64, error)
	FindTypeByCode(ctx context.Context, code string) (Type, error)
	FindTypeByID(ctx context.Context, typeID uint) (Type, error)
	UpdateType(ctx context.Context, typeID uint, updates map[string]any) error
	UpdateItemsTypeCode(ctx context.Context, oldCode, newCode string) error
	DeleteItemsByTypeCode(ctx context.Context, typeCode string) error
	DeleteType(ctx context.Context, dictionaryType *Type) error
	FindEnabledTypeByCode(ctx context.Context, code string) (Type, error)
	ItemValueExists(ctx context.Context, typeCode, value string, exceptID uint) (bool, error)
	FindItemByID(ctx context.Context, itemID uint) (Item, error)
	UpdateItem(ctx context.Context, itemID uint, updates map[string]any) error
	DeleteItem(ctx context.Context, item *Item) error
	ListItems(ctx context.Context, offset, limit int, typeCode, keyword string, status *int) ([]Item, int64, error)
	CreateItem(ctx context.Context, item *Item) error
	ListEnabledItems(ctx context.Context, typeCode string) ([]Item, error)
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

func (repository *gormRepository) TypeCodeExists(ctx context.Context, code string, exceptID uint) (bool, error) {
	var count int64
	query := repository.connection(ctx).Model(&Type{}).Where("code = ?", code)
	if exceptID > 0 {
		query = query.Where("id != ?", exceptID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (repository *gormRepository) CreateType(ctx context.Context, dictionaryType *Type) error {
	return repository.connection(ctx).Create(dictionaryType).Error
}

func (repository *gormRepository) ListTypes(ctx context.Context, offset, limit int, keyword string, status *int) ([]Type, int64, error) {
	var dictionaryTypes []Type
	var total int64
	query := repository.connection(ctx).Model(&Type{})
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("sort asc, id asc").Limit(limit).Offset(offset).Find(&dictionaryTypes).Error; err != nil {
		return nil, 0, err
	}
	return dictionaryTypes, total, nil
}

func (repository *gormRepository) FindTypeByCode(ctx context.Context, code string) (Type, error) {
	var dictionaryType Type
	err := repository.connection(ctx).Where("code = ?", code).First(&dictionaryType).Error
	return dictionaryType, err
}

func (repository *gormRepository) FindTypeByID(ctx context.Context, typeID uint) (Type, error) {
	var dictionaryType Type
	err := repository.connection(ctx).First(&dictionaryType, typeID).Error
	return dictionaryType, err
}

func (repository *gormRepository) UpdateType(ctx context.Context, typeID uint, updates map[string]any) error {
	return repository.connection(ctx).Model(&Type{}).Where("id = ?", typeID).Updates(updates).Error
}

func (repository *gormRepository) UpdateItemsTypeCode(ctx context.Context, oldCode, newCode string) error {
	return repository.connection(ctx).Model(&Item{}).Where("type_code = ?", oldCode).Update("type_code", newCode).Error
}

func (repository *gormRepository) DeleteItemsByTypeCode(ctx context.Context, typeCode string) error {
	return repository.connection(ctx).Unscoped().Where("type_code = ?", typeCode).Delete(&Item{}).Error
}

func (repository *gormRepository) DeleteType(ctx context.Context, dictionaryType *Type) error {
	return repository.connection(ctx).Unscoped().Delete(dictionaryType).Error
}

func (repository *gormRepository) FindEnabledTypeByCode(ctx context.Context, code string) (Type, error) {
	var dictionaryType Type
	err := repository.connection(ctx).Where("code = ? AND status = 1", code).First(&dictionaryType).Error
	return dictionaryType, err
}

func (repository *gormRepository) ItemValueExists(ctx context.Context, typeCode, value string, exceptID uint) (bool, error) {
	var count int64
	query := repository.connection(ctx).Model(&Item{}).Where("type_code = ? AND value = ?", typeCode, value)
	if exceptID > 0 {
		query = query.Where("id != ?", exceptID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (repository *gormRepository) CreateItem(ctx context.Context, item *Item) error {
	return repository.connection(ctx).Create(item).Error
}

func (repository *gormRepository) FindItemByID(ctx context.Context, itemID uint) (Item, error) {
	var item Item
	err := repository.connection(ctx).First(&item, itemID).Error
	return item, err
}

func (repository *gormRepository) UpdateItem(ctx context.Context, itemID uint, updates map[string]any) error {
	return repository.connection(ctx).Model(&Item{}).Where("id = ?", itemID).Updates(updates).Error
}

func (repository *gormRepository) DeleteItem(ctx context.Context, item *Item) error {
	return repository.connection(ctx).Unscoped().Delete(item).Error
}

func (repository *gormRepository) ListItems(ctx context.Context, offset, limit int, typeCode, keyword string, status *int) ([]Item, int64, error) {
	var items []Item
	var total int64
	query := repository.connection(ctx).Model(&Item{})
	if typeCode != "" {
		query = query.Where("type_code = ?", typeCode)
	}
	if keyword != "" {
		query = query.Where("label LIKE ? OR value LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("type_code asc, sort asc, id asc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (repository *gormRepository) ListEnabledItems(ctx context.Context, typeCode string) ([]Item, error) {
	var items []Item
	err := repository.connection(ctx).
		Where("type_code = ? AND status = 1", typeCode).
		Order("sort asc, id asc").
		Find(&items).Error
	return items, err
}
