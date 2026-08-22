package dictionary

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type TransactionRunner interface {
	Run(ctx context.Context, operation func(context.Context) error) error
}

type Service struct {
	repository   Repository
	transactions TransactionRunner
}

func NewService(repository Repository, transactions TransactionRunner) *Service {
	return &Service{repository: repository, transactions: transactions}
}

func (service *Service) CreateType(ctx context.Context, request CreateTypeRequest) (TypeInfo, error) {
	exists, err := service.repository.TypeCodeExists(ctx, request.Code, 0)
	if err != nil {
		return TypeInfo{}, NewError(CodeInternalError, err)
	}
	if exists {
		return TypeInfo{}, NewError(CodeConflict, nil)
	}
	dictionaryType := Type{
		Name: request.Name, Code: request.Code, Remark: request.Remark,
		Sort: request.Sort, Status: defaultStatus(request.Status),
	}
	if err := service.repository.CreateType(ctx, &dictionaryType); err != nil {
		return TypeInfo{}, NewError(CodeInternalError, err)
	}
	return typeInfo(dictionaryType), nil
}

func (service *Service) ListTypes(ctx context.Context, page, pageSize int, keyword string, status *int) ([]TypeInfo, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	dictionaryTypes, total, err := service.repository.ListTypes(ctx, (page-1)*pageSize, pageSize, keyword, status)
	if err != nil {
		return nil, 0, NewError(CodeInternalError, err)
	}
	result := make([]TypeInfo, len(dictionaryTypes))
	for index, dictionaryType := range dictionaryTypes {
		result[index] = typeInfo(dictionaryType)
	}
	return result, total, nil
}

func (service *Service) UpdateType(ctx context.Context, typeID uint, request UpdateTypeRequest) (TypeInfo, error) {
	dictionaryType, err := service.repository.FindTypeByID(ctx, typeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return TypeInfo{}, NewError(CodeNotFound, err)
		}
		return TypeInfo{}, NewError(CodeInternalError, err)
	}

	updates := make(map[string]any, 5)
	if request.Name != "" {
		updates["name"] = request.Name
	}
	if request.Code != "" {
		exists, err := service.repository.TypeCodeExists(ctx, request.Code, typeID)
		if err != nil {
			return TypeInfo{}, NewError(CodeInternalError, err)
		}
		if exists {
			return TypeInfo{}, NewError(CodeConflict, nil)
		}
		updates["code"] = request.Code
	}
	if request.Remark != "" {
		updates["remark"] = request.Remark
	}
	if request.Sort != nil {
		updates["sort"] = *request.Sort
	}
	if request.Status != nil {
		updates["status"] = *request.Status
	}
	if len(updates) == 0 {
		return TypeInfo{}, NewError(CodeValidationInvalid, nil)
	}

	oldCode := dictionaryType.Code
	if err := service.transactions.Run(ctx, func(txContext context.Context) error {
		if err := service.repository.UpdateType(txContext, typeID, updates); err != nil {
			return err
		}
		if request.Code != "" && request.Code != oldCode {
			return service.repository.UpdateItemsTypeCode(txContext, oldCode, request.Code)
		}
		return nil
	}); err != nil {
		return TypeInfo{}, NewError(CodeInternalError, err)
	}
	dictionaryType, err = service.repository.FindTypeByID(ctx, typeID)
	if err != nil {
		return TypeInfo{}, NewError(CodeInternalError, err)
	}
	return typeInfo(dictionaryType), nil
}

func (service *Service) DeleteType(ctx context.Context, typeID uint) error {
	dictionaryType, err := service.repository.FindTypeByID(ctx, typeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return NewError(CodeNotFound, err)
		}
		return NewError(CodeInternalError, err)
	}
	if err := service.transactions.Run(ctx, func(txContext context.Context) error {
		if err := service.repository.DeleteItemsByTypeCode(txContext, dictionaryType.Code); err != nil {
			return err
		}
		return service.repository.DeleteType(txContext, &dictionaryType)
	}); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *Service) CreateItem(ctx context.Context, request CreateItemRequest) (ItemInfo, error) {
	if _, err := service.repository.FindTypeByCode(ctx, request.TypeCode); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ItemInfo{}, NewError(CodeNotFound, err)
		}
		return ItemInfo{}, NewError(CodeInternalError, err)
	}
	exists, err := service.repository.ItemValueExists(ctx, request.TypeCode, request.Value, 0)
	if err != nil {
		return ItemInfo{}, NewError(CodeInternalError, err)
	}
	if exists {
		return ItemInfo{}, NewError(CodeConflict, nil)
	}
	item := Item{
		TypeCode: request.TypeCode, Label: request.Label, Value: request.Value,
		Remark: request.Remark, Sort: request.Sort, Status: defaultStatus(request.Status),
	}
	if err := service.repository.CreateItem(ctx, &item); err != nil {
		return ItemInfo{}, NewError(CodeInternalError, err)
	}
	return itemInfo(item), nil
}

func (service *Service) UpdateItem(ctx context.Context, itemID uint, request UpdateItemRequest) (ItemInfo, error) {
	item, err := service.repository.FindItemByID(ctx, itemID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ItemInfo{}, NewError(CodeNotFound, err)
		}
		return ItemInfo{}, NewError(CodeInternalError, err)
	}

	targetTypeCode := item.TypeCode
	if request.TypeCode != "" {
		if _, err := service.repository.FindTypeByCode(ctx, request.TypeCode); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ItemInfo{}, NewError(CodeNotFound, err)
			}
			return ItemInfo{}, NewError(CodeInternalError, err)
		}
		targetTypeCode = request.TypeCode
	}
	targetValue := item.Value
	if request.Value != "" {
		targetValue = request.Value
	}
	if targetTypeCode != item.TypeCode || targetValue != item.Value {
		exists, err := service.repository.ItemValueExists(ctx, targetTypeCode, targetValue, itemID)
		if err != nil {
			return ItemInfo{}, NewError(CodeInternalError, err)
		}
		if exists {
			return ItemInfo{}, NewError(CodeConflict, nil)
		}
	}

	updates := make(map[string]any, 6)
	if request.TypeCode != "" {
		updates["type_code"] = request.TypeCode
	}
	if request.Label != "" {
		updates["label"] = request.Label
	}
	if request.Value != "" {
		updates["value"] = request.Value
	}
	if request.Remark != "" {
		updates["remark"] = request.Remark
	}
	if request.Sort != nil {
		updates["sort"] = *request.Sort
	}
	if request.Status != nil {
		updates["status"] = *request.Status
	}
	if len(updates) == 0 {
		return ItemInfo{}, NewError(CodeValidationInvalid, nil)
	}
	if err := service.repository.UpdateItem(ctx, itemID, updates); err != nil {
		return ItemInfo{}, NewError(CodeInternalError, err)
	}
	item, err = service.repository.FindItemByID(ctx, itemID)
	if err != nil {
		return ItemInfo{}, NewError(CodeInternalError, err)
	}
	return itemInfo(item), nil
}

func (service *Service) DeleteItem(ctx context.Context, itemID uint) error {
	item, err := service.repository.FindItemByID(ctx, itemID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return NewError(CodeNotFound, err)
		}
		return NewError(CodeInternalError, err)
	}
	if err := service.repository.DeleteItem(ctx, &item); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *Service) ListItems(ctx context.Context, page, pageSize int, typeCode, keyword string, status *int) ([]ItemInfo, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := service.repository.ListItems(ctx, (page-1)*pageSize, pageSize, typeCode, keyword, status)
	if err != nil {
		return nil, 0, NewError(CodeInternalError, err)
	}
	result := make([]ItemInfo, len(items))
	for index, item := range items {
		result[index] = itemInfo(item)
	}
	return result, total, nil
}

func (service *Service) ListEnabledItems(ctx context.Context, typeCode string) ([]ItemInfo, error) {
	if _, err := service.repository.FindEnabledTypeByCode(ctx, typeCode); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []ItemInfo{}, nil
		}
		return nil, NewError(CodeInternalError, err)
	}
	items, err := service.repository.ListEnabledItems(ctx, typeCode)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	result := make([]ItemInfo, len(items))
	for index, item := range items {
		result[index] = itemInfo(item)
	}
	return result, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	return page, pageSize
}

func defaultStatus(status *int) int {
	if status == nil {
		return 1
	}
	return *status
}

func typeInfo(dictionaryType Type) TypeInfo {
	return TypeInfo{ID: dictionaryType.ID, Name: dictionaryType.Name, Code: dictionaryType.Code, Remark: dictionaryType.Remark, Sort: dictionaryType.Sort, Status: dictionaryType.Status}
}

func itemInfo(item Item) ItemInfo {
	return ItemInfo{ID: item.ID, TypeCode: item.TypeCode, Label: item.Label, Value: item.Value, Remark: item.Remark, Sort: item.Sort, Status: item.Status}
}
