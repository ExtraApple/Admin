package navigation

import "strings"

type syncMenuPlanItem struct {
	name           string
	path           string
	component      string
	icon           string
	permissionCode string
	parentPath     string
	sort           int
	typeValue      int
}

func normalizeSyncMenuPlan(items []SyncItem) ([]syncMenuPlanItem, error) {
	plan := make([]syncMenuPlanItem, len(items))
	pathIndexes := make(map[string]int, len(items))
	for index, item := range items {
		path := normalizeMenuPath(item.Path)
		if path == nil || strings.TrimSpace(item.Name) == "" {
			return nil, NewError(CodeValidationInvalid, nil)
		}
		if _, exists := pathIndexes[*path]; exists {
			return nil, NewError(CodeConflict, nil)
		}
		menuType := item.Type
		if menuType == 0 {
			menuType = 1
		}
		parentPath := strings.TrimSpace(item.ParentPath)
		if parentPath != "" {
			normalizedParent := normalizeMenuPath(parentPath)
			if normalizedParent == nil {
				return nil, NewError(CodeConflict, nil)
			}
			parentPath = *normalizedParent
		}
		candidate := Menu{
			Name: strings.TrimSpace(item.Name), Path: path,
			Component: strings.TrimSpace(item.Component), Type: menuType, Status: 1,
		}
		if err := validateMenuFields(candidate); err != nil {
			return nil, err
		}
		pathIndexes[*path] = index
		plan[index] = syncMenuPlanItem{
			name: candidate.Name, path: *path, component: candidate.Component,
			icon: item.Icon, permissionCode: strings.TrimSpace(item.PermissionCode),
			parentPath: parentPath, sort: item.Sort, typeValue: menuType,
		}
	}

	state := make([]uint8, len(plan))
	ordered := make([]syncMenuPlanItem, 0, len(plan))
	var visit func(int) error
	visit = func(index int) error {
		switch state[index] {
		case 1:
			return NewError(CodeConflict, nil)
		case 2:
			return nil
		}
		state[index] = 1
		if plan[index].parentPath != "" {
			if parentIndex, exists := pathIndexes[plan[index].parentPath]; exists {
				if err := visit(parentIndex); err != nil {
					return err
				}
			}
		}
		state[index] = 2
		ordered = append(ordered, plan[index])
		return nil
	}
	for index := range plan {
		if err := visit(index); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
