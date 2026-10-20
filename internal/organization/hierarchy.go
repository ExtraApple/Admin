package organization

import (
	"context"
	"sort"
)

type Hierarchy struct {
	repository Repository
}

type HierarchyReader interface {
	MemberOrganizationIDs(context.Context, uint) ([]uint, error)
	DescendantOrganizationIDs(context.Context, []uint) ([]uint, error)
	AncestorOrganizationIDs(context.Context, []uint) ([]uint, error)
}

func NewHierarchy(repository Repository) *Hierarchy {
	return &Hierarchy{repository: repository}
}

func (hierarchy *Hierarchy) MemberOrganizationIDs(ctx context.Context, userID uint) ([]uint, error) {
	return hierarchy.repository.MemberOrganizationIDs(ctx, userID)
}

func (hierarchy *Hierarchy) DescendantOrganizationIDs(ctx context.Context, rootIDs []uint) ([]uint, error) {
	roots := uniqueIDs(rootIDs)
	if len(roots) == 0 {
		return []uint{}, nil
	}
	organizations, err := hierarchy.repository.OrganizationParents(ctx)
	if err != nil {
		return nil, err
	}
	childrenByParent := make(map[uint][]uint)
	for _, organization := range organizations {
		childrenByParent[organization.ParentID] = append(childrenByParent[organization.ParentID], organization.ID)
	}
	result := make([]uint, 0, len(roots))
	queue := append([]uint(nil), roots...)
	seen := make(map[uint]struct{}, len(roots))
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if _, exists := seen[currentID]; exists {
			continue
		}
		seen[currentID] = struct{}{}
		result = append(result, currentID)
		queue = append(queue, childrenByParent[currentID]...)
	}
	return result, nil
}

func (hierarchy *Hierarchy) AncestorOrganizationIDs(ctx context.Context, organizationIDs []uint) ([]uint, error) {
	roots := uniqueIDs(organizationIDs)
	if len(roots) == 0 {
		return []uint{}, nil
	}
	organizations, err := hierarchy.repository.OrganizationParents(ctx)
	if err != nil {
		return nil, err
	}
	parentByID := make(map[uint]uint, len(organizations))
	for _, organization := range organizations {
		parentByID[organization.ID] = organization.ParentID
	}
	result := append([]uint(nil), roots...)
	seen := make(map[uint]struct{}, len(result))
	for _, id := range result {
		seen[id] = struct{}{}
	}
	for _, id := range roots {
		for parentID := parentByID[id]; parentID != 0; parentID = parentByID[parentID] {
			if _, exists := seen[parentID]; exists {
				break
			}
			seen[parentID] = struct{}{}
			result = append(result, parentID)
		}
	}
	return uniqueIDs(result), nil
}

// Only ancestors present on exactly one parent path change their descendant set.
// The moved node and common ancestors retain the same descendants.
func changedHierarchyRootIDs(parentByID map[uint]uint, unitID, newParentID uint) []uint {
	paths := make(map[uint]uint8)
	for index, parentID := range [2]uint{parentByID[unitID], newParentID} {
		path := uint8(1 << index)
		for ancestorID := parentID; ancestorID != 0; ancestorID = parentByID[ancestorID] {
			if paths[ancestorID]&path != 0 {
				break
			}
			paths[ancestorID] |= path
		}
	}
	result := make([]uint, 0, len(paths))
	for id, paths := range paths {
		if id != unitID && paths != 3 {
			result = append(result, id)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func uniqueIDs(ids []uint) []uint {
	result := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
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
