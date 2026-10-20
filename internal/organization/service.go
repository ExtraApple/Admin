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
	policy       UserManagementPolicy
	references   OrganizationReferencePolicy
	dependencies HierarchyDependencyReader
}

func NewService(
	repository Repository,
	hierarchy HierarchyReader,
	visibility VisibilityProvider,
	users UserDirectory,
	versions AccessVersionInvalidator,
	transactions TransactionRunner,
	policy UserManagementPolicy,
	references OrganizationReferencePolicy,
	dependencies HierarchyDependencyReader,
) *Service {
	return &Service{
		repository:   repository,
		hierarchy:    hierarchy,
		visibility:   visibility,
		users:        users,
		versions:     versions,
		transactions: transactions,
		policy:       policy,
		references:   references,
		dependencies: dependencies,
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
	return buildTree(units, 0, scope), nil
}

func (service *Service) CreateUnit(ctx context.Context, operatorID uint, request CreateUnitRequest) (UnitInfo, error) {
	var result UnitInfo
	err := service.withLockedTopology(ctx, func(transactionContext context.Context, parents []OrganizationParent) error {
		scope, err := service.visibility.CurrentScope(transactionContext, operatorID)
		if err != nil {
			return err
		}
		if request.ParentID == 0 {
			if !scope.All {
				return NewError(CodeValidationInvalid, nil)
			}
		} else {
			if !scope.All && !containsID(scope.OrganizationIDs, request.ParentID) {
				return NewError(CodeValidationInvalid, nil)
			}
			if !organizationParentExists(parents, request.ParentID) {
				return NewError(CodeNotFound, ErrNotFound)
			}
		}
		if exists, err := service.repository.CodeExists(transactionContext, request.Code, 0); err != nil {
			return err
		} else if exists {
			return NewError(CodeConflict, nil)
		}
		unit := Unit{
			ParentID: request.ParentID,
			Name:     request.Name,
			Code:     request.Code,
			Remark:   request.Remark,
			Sort:     request.Sort,
			Status:   defaultStatus(request.Status),
		}
		if err := service.repository.CreateUnit(transactionContext, &unit); err != nil {
			return err
		}
		result = unitInfo(unit)
		return nil
	})
	if err != nil {
		return UnitInfo{}, err
	}
	return result, nil
}

func (service *Service) UpdateUnit(ctx context.Context, operatorID, unitID uint, request UpdateUnitRequest) (UnitInfo, error) {
	if request.ParentID == nil {
		return service.updateUnit(ctx, operatorID, unitID, request, nil)
	}
	var result UnitInfo
	err := service.withLockedTopology(ctx, func(transactionContext context.Context, parents []OrganizationParent) error {
		var err error
		result, err = service.updateUnit(transactionContext, operatorID, unitID, request, parents)
		return err
	})
	if err != nil {
		return UnitInfo{}, err
	}
	return result, nil
}

func (service *Service) updateUnit(ctx context.Context, operatorID, unitID uint, request UpdateUnitRequest, parents []OrganizationParent) (UnitInfo, error) {
	var parentByID map[uint]uint
	if request.ParentID != nil {
		parentByID = make(map[uint]uint, len(parents))
		for _, parent := range parents {
			parentByID[parent.ID] = parent.ParentID
		}
		if _, exists := parentByID[unitID]; !exists {
			return UnitInfo{}, NewError(CodeNotFound, ErrNotFound)
		}
	} else if _, err := service.repository.FindUnitByID(ctx, unitID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return UnitInfo{}, NewError(CodeNotFound, err)
		}
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	var scope OrganizationScope
	var err error
	if request.ParentID != nil {
		scope, err = service.visibility.CurrentScope(ctx, operatorID)
	} else {
		scope, err = service.visibility.Scope(ctx, operatorID)
	}
	if err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	if !scope.All && !containsID(scope.OrganizationIDs, unitID) {
		return UnitInfo{}, NewError(CodeValidationInvalid, nil)
	}
	updates := make(map[string]any, 6)
	if request.ParentID != nil {
		parentID := *request.ParentID
		if parentID != 0 {
			if parentID == unitID {
				return UnitInfo{}, NewError(CodeValidationInvalid, nil)
			}
			if !scope.All && !containsID(scope.OrganizationIDs, parentID) {
				return UnitInfo{}, NewError(CodeValidationInvalid, nil)
			}
			if _, exists := parentByID[parentID]; !exists {
				return UnitInfo{}, NewError(CodeNotFound, ErrNotFound)
			}
			// Follow only the proposed parent's chain in the locked graph.
			// The bound also terminates traversal of pre-existing corrupt cycles.
			for ancestorID, remaining := parentID, len(parents); ancestorID != 0 && remaining > 0; remaining-- {
				if ancestorID == unitID {
					return UnitInfo{}, NewError(CodeValidationInvalid, nil)
				}
				ancestorID = parentByID[ancestorID]
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
	var dependentUserIDs []uint
	if request.ParentID != nil && parentByID[unitID] != *request.ParentID {
		roots := changedHierarchyRootIDs(parentByID, unitID, *request.ParentID)
		dependentUserIDs, err = service.dependencies.HierarchyDependentUserIDs(ctx, roots)
		if err != nil {
			if _, classified := CodeOf(err); classified {
				return UnitInfo{}, err
			}
			return UnitInfo{}, NewError(CodeInternalError, err)
		}
		dependentUserIDs = uniqueIDs(dependentUserIDs)
		sort.Slice(dependentUserIDs, func(left, right int) bool { return dependentUserIDs[left] < dependentUserIDs[right] })
	}
	if err := service.repository.UpdateUnit(ctx, unitID, updates); err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	if err := service.incrementVersions(ctx, dependentUserIDs); err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	unit, err := service.repository.FindUnitByID(ctx, unitID)
	if err != nil {
		return UnitInfo{}, NewError(CodeInternalError, err)
	}
	return unitInfo(unit), nil
}

func (service *Service) DeleteUnit(ctx context.Context, operatorID, unitID uint) error {
	if operatorID == 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	return service.withLockedTopology(ctx, func(transactionContext context.Context, parents []OrganizationParent) error {
		if !organizationParentExists(parents, unitID) {
			return NewError(CodeNotFound, ErrNotFound)
		}
		userIDs, err := service.repository.CurrentMemberUserIDs(transactionContext, unitID)
		if err != nil {
			return err
		}
		referenced, err := service.references.LockOrganizationReferences(transactionContext, operatorID, unitID, userIDs)
		if err != nil {
			return err
		}
		if err := service.ensureVisible(transactionContext, operatorID, unitID, true); err != nil {
			return err
		}
		if referenced {
			return NewError(CodeConflict, nil)
		}
		for _, parent := range parents {
			if parent.ParentID == unitID {
				return NewError(CodeValidationInvalid, nil)
			}
		}
		unit, err := service.repository.FindUnitByID(transactionContext, unitID)
		if err != nil {
			return err
		}
		users, err := service.policy.ManagedUsers(transactionContext, operatorID, userIDs)
		if err != nil {
			return err
		}
		for _, userID := range userIDs {
			if err := service.validateMember(operatorID, userID, users[userID], true, true); err != nil {
				return err
			}
		}
		if err := service.repository.DeleteMemberships(transactionContext, unitID); err != nil {
			return err
		}
		if err := service.repository.DeleteUnit(transactionContext, &unit); err != nil {
			return err
		}
		return service.incrementVersions(transactionContext, userIDs)
	})
}

func (service *Service) withLockedTopology(ctx context.Context, operation func(context.Context, []OrganizationParent) error) error {
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		parents, err := service.repository.LockOrganizationParents(transactionContext)
		if err != nil {
			return err
		}
		return operation(withOrganizationParents(transactionContext, parents), parents)
	})
	if err != nil {
		if _, classified := CodeOf(err); classified {
			return err
		}
		return NewError(CodeInternalError, err)
	}
	return nil
}

func organizationParentExists(parents []OrganizationParent, unitID uint) bool {
	for _, parent := range parents {
		if parent.ID == unitID {
			return true
		}
	}
	return false
}

func (service *Service) SetUsers(ctx context.Context, operatorID, unitID uint, userIDs []uint) error {
	if operatorID == 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	for _, userID := range userIDs {
		if userID == 0 {
			return NewError(CodeValidationInvalid, nil)
		}
	}
	userIDs = uniqueIDs(userIDs)
	err := service.withLockedTopology(ctx, func(transactionContext context.Context, parents []OrganizationParent) error {
		if !organizationParentExists(parents, unitID) {
			return NewError(CodeNotFound, ErrNotFound)
		}
		oldUserIDs, err := service.repository.CurrentMemberUserIDs(transactionContext, unitID)
		if err != nil {
			return err
		}
		checkedIDs := uniqueIDs(append(oldUserIDs, userIDs...))
		users, err := service.policy.ManagedUsers(transactionContext, operatorID, checkedIDs)
		if err != nil {
			return err
		}
		if err := service.ensureVisible(transactionContext, operatorID, unitID, true); err != nil {
			return err
		}
		for _, userID := range userIDs {
			if err := service.validateMember(operatorID, userID, users[userID], !containsID(oldUserIDs, userID), false); err != nil {
				return err
			}
		}
		for _, userID := range oldUserIDs {
			if !containsID(userIDs, userID) {
				if err := service.validateMember(operatorID, userID, users[userID], true, true); err != nil {
					return err
				}
			}
		}
		added, removed := membershipDelta(oldUserIDs, userIDs)
		if len(added) == 0 && len(removed) == 0 {
			return nil
		}
		if err := service.repository.ApplyMembershipDelta(transactionContext, unitID, added, removed); err != nil {
			return err
		}
		changed := append(added, removed...)
		sort.Slice(changed, func(left, right int) bool { return changed[left] < changed[right] })
		return service.incrementVersions(transactionContext, changed)
	})
	if err != nil {
		if _, classified := CodeOf(err); classified {
			return err
		}
		return NewError(CodeInternalError, err)
	}
	return nil
}

// membershipDelta compares the applicable current and selected organization links.
// Both inputs contain unique IDs; retained links never enter the write set.
func membershipDelta(current, selected []uint) (added, removed []uint) {
	currentSet := make(map[uint]bool, len(current))
	for _, id := range current {
		currentSet[id] = false
	}
	for _, id := range selected {
		if _, exists := currentSet[id]; exists {
			currentSet[id] = true
		} else {
			added = append(added, id)
		}
	}
	for _, id := range current {
		if !currentSet[id] {
			removed = append(removed, id)
		}
	}
	return added, removed
}

func (service *Service) validateMember(operatorID, userID uint, user ManagedUser, changed, allowDeleted bool) error {
	if operatorID == 0 || userID == 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	if !user.Exists || (user.Deleted && !allowDeleted) {
		return NewError(CodeValidationInvalid, nil)
	}
	if !user.Visible {
		return NewError(CodePermissionDenied, nil)
	}
	if changed && operatorID == userID {
		return NewError(CodeValidationInvalid, nil)
	}
	if changed && user.Protected {
		return NewError(CodeConflict, nil)
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
	if err := service.ensureVisible(ctx, operatorID, unitID, false); err != nil {
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

func (service *Service) ensureVisible(ctx context.Context, operatorID, unitID uint, current bool) error {
	var scope OrganizationScope
	var err error
	if current {
		scope, err = service.visibility.CurrentScope(ctx, operatorID)
	} else {
		scope, err = service.visibility.Scope(ctx, operatorID)
	}
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

func buildTree(units []Unit, parentID uint, scope OrganizationScope) []TreeNode {
	tree := make([]TreeNode, 0)
	for _, unit := range units {
		if unit.ParentID != parentID {
			continue
		}
		node := TreeNode{ID: unit.ID, ParentID: unit.ParentID, Name: unit.Name, Code: unit.Code, Remark: unit.Remark, Sort: unit.Sort, Status: unit.Status}
		node.Manageable = scope.All || containsID(scope.OrganizationIDs, unit.ID)
		node.Children = buildTree(units, unit.ID, scope)
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
