package organization

import (
	"context"
	"errors"
	"sort"

	identitydomain "admin/internal/identity/domain"
)

type Service struct {
	repository   Repository
	hierarchy    HierarchyReader
	visibility   VisibilityProvider
	users        UserDirectory
	versions     AccessVersionInvalidator
	transactions TransactionRunner
}

func NewService(
	repository Repository,
	hierarchy HierarchyReader,
	visibility VisibilityProvider,
	users UserDirectory,
	versions AccessVersionInvalidator,
	transactions TransactionRunner,
) *Service {
	return &Service{
		repository:   repository,
		hierarchy:    hierarchy,
		visibility:   visibility,
		users:        users,
		versions:     versions,
		transactions: transactions,
	}
}

func (service *Service) ListUnits(ctx context.Context, operatorID uint, page, pageSize int, keyword string, status *int) ([]UnitInfo, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	scope, err := service.visibility.Scope(ctx, operatorID)
	if err != nil {
		return nil, 0, NewError(CodeInternalError, err)
	}
	units, total, err := service.repository.ListUnits(ctx, (page-1)*pageSize, pageSize, keyword, status, scope)
	if err != nil {
		return nil, 0, NewError(CodeInternalError, err)
	}
	result := make([]UnitInfo, len(units))
	for index, unit := range units {
		result[index] = unitInfo(unit)
	}
	return result, total, nil
}

func (service *Service) Tree(ctx context.Context, operatorID uint) ([]TreeNode, error) {
	scope, err := service.visibility.Scope(ctx, operatorID)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	var units []Unit
	if scope.All {
		units, err = service.repository.ListAllUnits(ctx)
	} else {
		ancestorIDs, ancestorErr := service.hierarchy.AncestorOrganizationIDs(ctx, scope.OrganizationIDs)
		if ancestorErr != nil {
			return nil, NewError(CodeInternalError, ancestorErr)
		}
		units, err = service.repository.ListUnitsByIDs(ctx, ancestorIDs)
	}
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	return buildTree(units, 0), nil
}

func (service *Service) CreateUnit(ctx context.Context, operatorID uint, request CreateUnitRequest) (UnitInfo, error) {
	if request.ParentID != 0 {
		if err := service.ensureVisible(ctx, operatorID, request.ParentID); err != nil {
			return UnitInfo{}, err
		}
		if _, err := service.repository.FindUnitByID(ctx, request.ParentID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return UnitInfo{}, NewError(CodeNotFound, err)
			}
			return UnitInfo{}, NewError(CodeInternalError, err)
		}
	} else {
		scope, err := service.visibility.Scope(ctx, operatorID)
		if err != nil {
			return UnitInfo{}, NewError(CodeInternalError, err)
		}
		if !scope.All {
			return UnitInfo{}, NewError(CodeValidationInvalid, nil)
		}
	}
	if exists, err := service.repository.CodeExists(ctx, request.Code, 0); err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	} else if exists {
		return UnitInfo{}, NewError(CodeConflict, nil)
	}
	unit := Unit{
		ParentID: request.ParentID,
		Name:     request.Name,
		Code:     request.Code,
		Remark:   request.Remark,
		Sort:     request.Sort,
		Status:   defaultStatus(request.Status),
	}
	if err := service.repository.CreateUnit(ctx, &unit); err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	return unitInfo(unit), nil
}

func (service *Service) UpdateUnit(ctx context.Context, operatorID, unitID uint, request UpdateUnitRequest) (UnitInfo, error) {
	unit, err := service.repository.FindUnitByID(ctx, unitID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return UnitInfo{}, NewError(CodeNotFound, err)
		}
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return UnitInfo{}, err
	}
	updates := make(map[string]any, 6)
	if request.ParentID != nil {
		parentID := *request.ParentID
		if parentID != 0 {
			if parentID == unitID {
				return UnitInfo{}, NewError(CodeValidationInvalid, nil)
			}
			if err := service.ensureVisible(ctx, operatorID, parentID); err != nil {
				return UnitInfo{}, err
			}
			if _, err := service.repository.FindUnitByID(ctx, parentID); err != nil {
				if errors.Is(err, ErrNotFound) {
					return UnitInfo{}, NewError(CodeNotFound, err)
				}
				return UnitInfo{}, NewError(CodeInternalError, err)
			}
			descendants, err := service.hierarchy.DescendantOrganizationIDs(ctx, []uint{unitID})
			if err != nil {
				return UnitInfo{}, NewError(CodeInternalError, err)
			}
			if containsID(descendants, parentID) {
				return UnitInfo{}, NewError(CodeValidationInvalid, nil)
			}
		}
		updates["parent_id"] = parentID
	}
	if request.Name != "" {
		updates["name"] = request.Name
	}
	if request.Code != "" {
		if exists, err := service.repository.CodeExists(ctx, request.Code, unitID); err != nil {
			return UnitInfo{}, NewError(CodeInternalError, err)
		} else if exists {
			return UnitInfo{}, NewError(CodeConflict, nil)
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
		return UnitInfo{}, NewError(CodeValidationInvalid, nil)
	}
	if err := service.repository.UpdateUnit(ctx, unitID, updates); err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	unit, err = service.repository.FindUnitByID(ctx, unitID)
	if err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	return unitInfo(unit), nil
}

func (service *Service) DeleteUnit(ctx context.Context, operatorID, unitID uint) error {
	unit, err := service.repository.FindUnitByID(ctx, unitID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return NewError(CodeNotFound, err)
		}
		return NewError(CodeInternalError, err)
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return err
	}
	childCount, err := service.repository.ChildCount(ctx, unitID)
	if err != nil {
		return NewError(CodeInternalError, err)
	}
	if childCount > 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	if err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		userIDs, err := service.repository.MemberUserIDs(transactionContext, unitID)
		if err != nil {
			return err
		}
		if err := service.repository.DeleteMemberships(transactionContext, unitID); err != nil {
			return err
		}
		if err := service.repository.DeleteUnit(transactionContext, &unit); err != nil {
			return err
		}
		return service.incrementVersions(transactionContext, userIDs)
	}); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *Service) SetUsers(ctx context.Context, operatorID, unitID uint, userIDs []uint) error {
	if _, err := service.repository.FindUnitByID(ctx, unitID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return NewError(CodeNotFound, err)
		}
		return NewError(CodeInternalError, err)
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return err
	}
	userIDs = uniqueIDs(userIDs)
	if len(userIDs) > 0 {
		users, err := service.users.ListUsersByIDs(ctx, userIDs)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		existingIDs := make([]uint, len(users))
		for index, user := range users {
			existingIDs[index] = user.ID
		}
		if !sameIDs(existingIDs, userIDs) {
			return NewError(CodeValidationInvalid, nil)
		}
	}
	if err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		oldUserIDs, err := service.repository.MemberUserIDs(transactionContext, unitID)
		if err != nil {
			return err
		}
		if err := service.repository.ReplaceMemberships(transactionContext, unitID, userIDs); err != nil {
			return err
		}
		affected := append(oldUserIDs, userIDs...)
		return service.incrementVersions(transactionContext, uniqueIDs(affected))
	}); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func (service *Service) Users(ctx context.Context, operatorID, unitID uint) ([]identitydomain.DirectoryUser, error) {
	if _, err := service.repository.FindUnitByID(ctx, unitID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, NewError(CodeNotFound, err)
		}
		return nil, NewError(CodeInternalError, err)
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return nil, err
	}
	userIDs, err := service.repository.MemberUserIDs(ctx, unitID)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	if len(userIDs) == 0 {
		return []identitydomain.DirectoryUser{}, nil
	}
	users, err := service.users.ListUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	return users, nil
}

func (service *Service) ensureVisible(ctx context.Context, operatorID, unitID uint) error {
	scope, err := service.visibility.Scope(ctx, operatorID)
	if err != nil {
		return NewError(CodeInternalError, err)
	}
	if scope.All || containsID(scope.OrganizationIDs, unitID) {
		return nil
	}
	return NewError(CodeValidationInvalid, nil)
}

func (service *Service) incrementVersions(ctx context.Context, userIDs []uint) error {
	if len(userIDs) == 0 || service.versions == nil {
		return nil
	}
	return service.versions.Increment(ctx, userIDs)
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

func unitInfo(unit Unit) UnitInfo {
	return UnitInfo{ID: unit.ID, ParentID: unit.ParentID, Name: unit.Name, Code: unit.Code, Remark: unit.Remark, Sort: unit.Sort, Status: unit.Status}
}

func buildTree(units []Unit, parentID uint) []TreeNode {
	tree := make([]TreeNode, 0)
	for _, unit := range units {
		if unit.ParentID != parentID {
			continue
		}
		node := TreeNode{ID: unit.ID, ParentID: unit.ParentID, Name: unit.Name, Code: unit.Code, Remark: unit.Remark, Sort: unit.Sort, Status: unit.Status}
		node.Children = buildTree(units, unit.ID)
		tree = append(tree, node)
	}
	sort.SliceStable(tree, func(left, right int) bool {
		if tree[left].Sort != tree[right].Sort {
			return tree[left].Sort < tree[right].Sort
		}
		return tree[left].ID < tree[right].ID
	})
	return tree
}

func containsID(ids []uint, target uint) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func sameIDs(left, right []uint) bool {
	left = uniqueIDs(left)
	right = uniqueIDs(right)
	if len(left) != len(right) {
		return false
	}
	for _, id := range right {
		if !containsID(left, id) {
			return false
		}
	}
	return true
}
