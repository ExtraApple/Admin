package organization

import (
	"context"
	"errors"
	"sort"
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
		return nil, 0, err
	}
	units, total, err := service.repository.ListUnits(ctx, (page-1)*pageSize, pageSize, keyword, status, scope)
	if err != nil {
		return nil, 0, errors.New("查询组织列表失败")
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
		return nil, err
	}
	var units []Unit
	if scope.All {
		units, err = service.repository.ListAllUnits(ctx)
	} else {
		ancestorIDs, ancestorErr := service.hierarchy.AncestorOrganizationIDs(ctx, scope.OrganizationIDs)
		if ancestorErr != nil {
			return nil, ancestorErr
		}
		units, err = service.repository.ListUnitsByIDs(ctx, ancestorIDs)
	}
	if err != nil {
		return nil, errors.New("查询组织树失败")
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
				return UnitInfo{}, errors.New("父组织不存在")
			}
			return UnitInfo{}, errors.New("查询组织失败")
		}
	} else {
		scope, err := service.visibility.Scope(ctx, operatorID)
		if err != nil {
			return UnitInfo{}, err
		}
		if !scope.All {
			return UnitInfo{}, errors.New("无权创建根组织")
		}
	}
	if exists, err := service.repository.CodeExists(ctx, request.Code, 0); err != nil {
		return UnitInfo{}, errors.New("查询组织失败")
	} else if exists {
		return UnitInfo{}, errors.New("组织编码已存在")
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
		return UnitInfo{}, errors.New("创建组织失败: " + err.Error())
	}
	return unitInfo(unit), nil
}

func (service *Service) UpdateUnit(ctx context.Context, operatorID, unitID uint, request UpdateUnitRequest) (UnitInfo, error) {
	unit, err := service.repository.FindUnitByID(ctx, unitID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return UnitInfo{}, errors.New("组织不存在")
		}
		return UnitInfo{}, errors.New("查询组织失败")
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return UnitInfo{}, err
	}
	updates := make(map[string]any, 6)
	if request.ParentID != nil {
		parentID := *request.ParentID
		if parentID != 0 {
			if parentID == unitID {
				return UnitInfo{}, errors.New("不能将组织挂载到自身下面")
			}
			if err := service.ensureVisible(ctx, operatorID, parentID); err != nil {
				return UnitInfo{}, err
			}
			if _, err := service.repository.FindUnitByID(ctx, parentID); err != nil {
				if errors.Is(err, ErrNotFound) {
					return UnitInfo{}, errors.New("父组织不存在")
				}
				return UnitInfo{}, errors.New("查询组织失败")
			}
			descendants, err := service.hierarchy.DescendantOrganizationIDs(ctx, []uint{unitID})
			if err != nil {
				return UnitInfo{}, errors.New("查询组织层级失败")
			}
			if containsID(descendants, parentID) {
				return UnitInfo{}, errors.New("不能将组织挂载到自身子组织下面")
			}
		}
		updates["parent_id"] = parentID
	}
	if request.Name != "" {
		updates["name"] = request.Name
	}
	if request.Code != "" {
		if exists, err := service.repository.CodeExists(ctx, request.Code, unitID); err != nil {
			return UnitInfo{}, errors.New("查询组织失败")
		} else if exists {
			return UnitInfo{}, errors.New("组织编码已存在")
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
		return UnitInfo{}, errors.New("无修改内容")
	}
	if err := service.repository.UpdateUnit(ctx, unitID, updates); err != nil {
		return UnitInfo{}, errors.New("修改组织失败")
	}
	unit, err = service.repository.FindUnitByID(ctx, unitID)
	if err != nil {
		return UnitInfo{}, errors.New("查询组织失败")
	}
	return unitInfo(unit), nil
}

func (service *Service) DeleteUnit(ctx context.Context, operatorID, unitID uint) error {
	unit, err := service.repository.FindUnitByID(ctx, unitID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errors.New("组织不存在")
		}
		return errors.New("查询组织失败")
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return err
	}
	childCount, err := service.repository.ChildCount(ctx, unitID)
	if err != nil {
		return errors.New("查询组织子节点失败")
	}
	if childCount > 0 {
		return errors.New("该组织存在子组织，不能删除")
	}
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
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
	})
}

func (service *Service) SetUsers(ctx context.Context, operatorID, unitID uint, userIDs []uint) error {
	if _, err := service.repository.FindUnitByID(ctx, unitID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return errors.New("组织不存在")
		}
		return errors.New("查询组织失败")
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return err
	}
	userIDs = uniqueIDs(userIDs)
	if len(userIDs) > 0 {
		existingIDs, err := service.users.ExistingUserIDs(ctx, userIDs)
		if err != nil {
			return errors.New("查询用户失败")
		}
		if !sameIDs(existingIDs, userIDs) {
			return errors.New("存在无效用户")
		}
	}
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
		oldUserIDs, err := service.repository.MemberUserIDs(transactionContext, unitID)
		if err != nil {
			return err
		}
		if err := service.repository.DeleteMemberships(transactionContext, unitID); err != nil {
			return err
		}
		memberships := make([]Membership, len(userIDs))
		for index, userID := range userIDs {
			memberships[index] = Membership{UserID: userID, OrganizationID: unitID}
		}
		if err := service.repository.CreateMemberships(transactionContext, memberships); err != nil {
			return err
		}
		affected := append(oldUserIDs, userIDs...)
		return service.incrementVersions(transactionContext, uniqueIDs(affected))
	})
}

func (service *Service) Users(ctx context.Context, operatorID, unitID uint) ([]MemberInfo, error) {
	if _, err := service.repository.FindUnitByID(ctx, unitID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errors.New("组织不存在")
		}
		return nil, errors.New("查询组织失败")
	}
	if err := service.ensureVisible(ctx, operatorID, unitID); err != nil {
		return nil, err
	}
	userIDs, err := service.repository.MemberUserIDs(ctx, unitID)
	if err != nil {
		return nil, errors.New("查询组织成员失败")
	}
	if len(userIDs) == 0 {
		return []MemberInfo{}, nil
	}
	users, err := service.users.ListUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, errors.New("查询组织成员失败")
	}
	return users, nil
}

func (service *Service) ensureVisible(ctx context.Context, operatorID, unitID uint) error {
	scope, err := service.visibility.Scope(ctx, operatorID)
	if err != nil {
		return err
	}
	if scope.All || containsID(scope.OrganizationIDs, unitID) {
		return nil
	}
	return errors.New("无权操作数据范围外的组织")
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
