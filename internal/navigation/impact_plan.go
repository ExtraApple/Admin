package navigation

import (
	"context"
	"sort"
	"strings"

	"admin/internal/platform/configimpact"
)

type ImpactPreviewInput struct {
	Operation      ImpactOperation
	MenuID         uint
	Create         CreateInput
	Update         UpdateInput
	APIIDs         []uint
	PermissionCode string
	SyncItems      []SyncItem
}

type menuImpactTarget struct {
	MenuID         uint                 `json:"menu_id,omitempty"`
	ParentID       uint                 `json:"parent_id"`
	Name           string               `json:"name,omitempty"`
	Path           string               `json:"path,omitempty"`
	Component      string               `json:"component,omitempty"`
	Icon           string               `json:"icon,omitempty"`
	PermissionCode string               `json:"permission_code,omitempty"`
	Sort           int                  `json:"sort"`
	Type           int                  `json:"type"`
	Status         int                  `json:"status"`
	APIIDs         []uint               `json:"api_ids,omitempty"`
	Clear          bool                 `json:"clear,omitempty"`
	Routes         []menuImpactSyncItem `json:"routes,omitempty"`
}

type menuImpactSyncItem struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Component      string `json:"component,omitempty"`
	Icon           string `json:"icon,omitempty"`
	PermissionCode string `json:"permission_code,omitempty"`
	ParentPath     string `json:"parent_path,omitempty"`
	Sort           int    `json:"sort"`
	Type           int    `json:"type"`
}

type menuImpactMenuFact struct {
	ID             uint   `json:"id"`
	ParentID       uint   `json:"parent_id"`
	Name           string `json:"name"`
	Path           string `json:"path,omitempty"`
	Component      string `json:"component,omitempty"`
	Icon           string `json:"icon,omitempty"`
	PermissionCode string `json:"permission_code,omitempty"`
	Sort           int    `json:"sort"`
	Type           int    `json:"type"`
	Status         int    `json:"status"`
}

type menuImpactAPIFact struct {
	ID             uint   `json:"id"`
	PermissionCode string `json:"permission_code,omitempty"`
	Status         int    `json:"status"`
	NeedAuth       int    `json:"need_auth"`
	Deleted        bool   `json:"deleted"`
}

type menuImpactFacts struct {
	Menus   []menuImpactMenuFact `json:"menus,omitempty"`
	APIIDs  []uint               `json:"api_ids,omitempty"`
	APIs    []menuImpactAPIFact  `json:"apis,omitempty"`
	RoleIDs []uint               `json:"role_ids,omitempty"`
	UserIDs []uint               `json:"user_ids,omitempty"`
}

func (service *Service) PreviewImpact(ctx context.Context, input ImpactPreviewInput) (ImpactPreview, error) {
	plan, err := service.buildMenuImpactPlan(ctx, input)
	if err != nil {
		return ImpactPreview{}, err
	}
	fingerprint, err := plan.Fingerprint()
	if err != nil {
		return ImpactPreview{}, NewError(CodeInternalError, err)
	}
	return ImpactPreview{
		Operation:        input.Operation,
		NormalizedTarget: plan.NormalizedTarget,
		Fingerprint:      fingerprint,
		Summary:          plan.Summary,
	}, nil
}

func (service *Service) buildMenuImpactPlan(ctx context.Context, input ImpactPreviewInput) (configimpact.Plan, error) {
	switch input.Operation {
	case ImpactCreateMenu:
		return service.buildCreateImpactPlan(ctx, input.Create)
	case ImpactUpdateMenu:
		return service.buildUpdateImpactPlan(ctx, input.MenuID, input.Update)
	case ImpactDeleteMenu:
		return service.buildDeleteImpactPlan(ctx, input.MenuID)
	case ImpactBindAPIs:
		return service.buildBindImpactPlan(ctx, input.MenuID, input.APIIDs, input.PermissionCode)
	case ImpactClearAPIs:
		return service.buildClearImpactPlan(ctx, input.MenuID)
	case ImpactSyncMenus:
		return service.buildSyncImpactPlan(ctx, input.SyncItems)
	default:
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
}

func (service *Service) buildCreateImpactPlan(ctx context.Context, input CreateInput) (configimpact.Plan, error) {
	name := strings.TrimSpace(input.Name)
	menuType := input.Type
	if menuType == 0 {
		menuType = 1
	}
	status := 1
	if input.Status != nil {
		status = *input.Status
	}
	path := normalizeMenuPath(input.Path)
	candidate := Menu{
		ParentID: input.ParentID, Name: name, Path: path,
		Component: strings.TrimSpace(input.Component), Icon: input.Icon,
		PermissionCode: strings.TrimSpace(input.PermissionCode), Sort: input.Sort,
		Type: menuType, Status: status,
	}
	if name == "" {
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
	if err := validateMenuFields(candidate); err != nil {
		return configimpact.Plan{}, err
	}
	if path != nil {
		exists, err := service.repository.PathExists(ctx, *path, 0)
		if err != nil {
			return configimpact.Plan{}, err
		}
		if exists {
			return configimpact.Plan{}, NewError(CodeConflict, nil)
		}
	}
	parent, err := service.impactParent(ctx, candidate.ParentID, 0, candidate.Type)
	if err != nil {
		return configimpact.Plan{}, err
	}
	userIDs, err := service.authorization.AllUserIDs(ctx)
	if err != nil {
		return configimpact.Plan{}, err
	}
	target := menuImpactTargetFromMenu(candidate)
	facts := menuImpactFacts{Menus: nonEmptyMenuFacts(parent), UserIDs: uniqueUintIDs(userIDs)}
	summary := configimpact.Summary{
		AffectedUserCount: len(facts.UserIDs), AccessVersionChange: true,
		RequiresConfirmation: true,
	}
	return newMenuImpactPlan(string(ImpactCreateMenu), target, facts, summary)
}

func (service *Service) buildUpdateImpactPlan(ctx context.Context, menuID uint, input UpdateInput) (configimpact.Plan, error) {
	current, err := service.repository.FindMenu(ctx, menuID)
	if err != nil {
		if err == ErrMenuNotFound {
			return configimpact.Plan{}, NewError(CodeNotFound, err)
		}
		return configimpact.Plan{}, err
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
	target := menuAfterUpdate(current, input)
	if input.Name != nil {
		target.Name = strings.TrimSpace(*input.Name)
	}
	if input.Icon != nil {
		target.Icon = *input.Icon
	}
	if input.PermissionCode != nil {
		target.PermissionCode = strings.TrimSpace(*input.PermissionCode)
	}
	if input.Sort != nil {
		target.Sort = *input.Sort
	}
	if err := validateMenuFields(target); err != nil {
		return configimpact.Plan{}, err
	}
	if target.Path != nil {
		exists, err := service.repository.PathExists(ctx, *target.Path, menuID)
		if err != nil {
			return configimpact.Plan{}, err
		}
		if exists {
			return configimpact.Plan{}, NewError(CodeConflict, nil)
		}
	}
	parent, err := service.impactParent(ctx, target.ParentID, menuID, target.Type)
	if err != nil {
		return configimpact.Plan{}, err
	}
	currentAPIIDs, err := service.repository.APIIDsByMenuIDs(ctx, []uint{menuID}, true)
	if err != nil {
		return configimpact.Plan{}, err
	}
	if len(currentAPIIDs) > 0 && target.Type != 3 {
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
	apiFacts, err := service.impactAPIFacts(ctx, currentAPIIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	userIDs, err := service.authorization.AllUserIDs(ctx)
	if err != nil {
		return configimpact.Plan{}, err
	}
	same := sameMenuFacts(current, target)
	facts := menuImpactFacts{
		Menus:  append(nonEmptyMenuFacts(current), nonEmptyMenuFacts(parent)...),
		APIIDs: uniqueUintIDs(currentAPIIDs), APIs: apiFacts,
		UserIDs: uniqueUintIDs(userIDs),
	}
	summary := configimpact.Summary{
		AffectedMenuIDs: []uint{menuID}, AffectedAPIIDs: uniqueUintIDs(currentAPIIDs),
		AffectedUserCount: len(facts.UserIDs), AccessVersionChange: !same,
		RequiresConfirmation: !same,
	}
	return newMenuImpactPlan(string(ImpactUpdateMenu), menuImpactTargetFromMenu(target), facts, summary)
}

func (service *Service) buildDeleteImpactPlan(ctx context.Context, menuID uint) (configimpact.Plan, error) {
	current, err := service.repository.FindMenu(ctx, menuID)
	if err != nil {
		return configimpact.Plan{}, NewError(CodeNotFound, ErrMenuNotFound)
	}
	menus, err := service.repository.ListMenus(ctx, false)
	if err != nil {
		return configimpact.Plan{}, err
	}
	for _, menu := range menus {
		if menu.ParentID == menuID && !menu.Deleted {
			return configimpact.Plan{}, NewError(CodeConflict, nil)
		}
	}
	roleIDs, err := service.affectedRoleIDs(ctx, []uint{menuID}, []string{current.PermissionCode})
	if err != nil {
		return configimpact.Plan{}, err
	}
	userIDs, err := service.authorization.UserIDsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	apiIDs, err := service.repository.APIIDsByMenuIDs(ctx, []uint{menuID}, true)
	if err != nil {
		return configimpact.Plan{}, err
	}
	apiFacts, err := service.impactAPIFacts(ctx, apiIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	facts := menuImpactFacts{
		Menus: []menuImpactMenuFact{menuImpactMenuFactFromMenu(current)}, APIIDs: uniqueUintIDs(apiIDs),
		APIs: apiFacts, RoleIDs: uniqueUintIDs(roleIDs), UserIDs: uniqueUintIDs(userIDs),
	}
	summary := configimpact.Summary{
		AffectedMenuIDs: []uint{menuID}, AffectedAPIIDs: uniqueUintIDs(apiIDs),
		AffectedRoleCount: len(facts.RoleIDs), AffectedUserCount: len(facts.UserIDs),
		AccessVersionChange: true, RequiresConfirmation: true,
	}
	return newMenuImpactPlan(string(ImpactDeleteMenu), menuImpactTarget{MenuID: menuID}, facts, summary)
}

func (service *Service) buildBindImpactPlan(ctx context.Context, menuID uint, requestedIDs []uint, manualCode string) (configimpact.Plan, error) {
	for _, apiID := range requestedIDs {
		if apiID == 0 {
			return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
		}
	}
	requestedIDs = uniqueUintIDs(requestedIDs)
	if len(requestedIDs) == 0 {
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
	menu, err := service.repository.FindMenu(ctx, menuID)
	if err != nil {
		return configimpact.Plan{}, NewError(CodeNotFound, ErrMenuNotFound)
	}
	if menu.Type != 3 {
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
	apis, err := service.apis.ListByIDs(ctx, requestedIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	if len(apis) != len(requestedIDs) {
		return configimpact.Plan{}, NewError(CodeNotFound, nil)
	}
	for _, api := range apis {
		if err := validateLinkableAPI(api); err != nil {
			return configimpact.Plan{}, err
		}
	}
	permissionCode, err := resolveBindingCode(manualCode, apis)
	if err != nil {
		return configimpact.Plan{}, err
	}
	currentIDs, err := service.repository.APIIDsByMenuIDs(ctx, []uint{menuID}, true)
	if err != nil {
		return configimpact.Plan{}, err
	}
	allAPIIDs := uniqueUintIDs(append(append([]uint{}, currentIDs...), requestedIDs...))
	apiFacts, err := service.impactAPIFacts(ctx, allAPIIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	existingCodes, err := service.authorization.ExistingPermissionCodes(ctx, []string{permissionCode}, false)
	if err != nil {
		return configimpact.Plan{}, err
	}
	roleIDs, err := service.affectedRoleIDs(ctx, []uint{menuID}, []string{menu.PermissionCode, permissionCode})
	if err != nil {
		return configimpact.Plan{}, err
	}
	userIDs, err := service.authorization.UserIDsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	codeChanged := menu.PermissionCode != permissionCode
	for _, api := range apis {
		codeChanged = codeChanged || api.PermissionCode != permissionCode
	}
	changed := !sameUintIDs(uniqueUintIDs(currentIDs), requestedIDs) || codeChanged || len(existingCodes) == 0
	facts := menuImpactFacts{
		Menus: []menuImpactMenuFact{menuImpactMenuFactFromMenu(menu)}, APIIDs: uniqueUintIDs(currentIDs),
		APIs: apiFacts, RoleIDs: uniqueUintIDs(roleIDs), UserIDs: uniqueUintIDs(userIDs),
	}
	summary := configimpact.Summary{
		AffectedMenuIDs: []uint{menuID}, AffectedAPIIDs: allAPIIDs,
		AffectedRoleCount: len(facts.RoleIDs), AffectedUserCount: len(facts.UserIDs),
		AccessVersionChange: changed, RequiresConfirmation: changed,
	}
	target := menuImpactTarget{MenuID: menuID, APIIDs: requestedIDs, PermissionCode: permissionCode}
	return newMenuImpactPlan(string(ImpactBindAPIs), target, facts, summary)
}

func (service *Service) buildClearImpactPlan(ctx context.Context, menuID uint) (configimpact.Plan, error) {
	menu, err := service.repository.FindMenu(ctx, menuID)
	if err != nil {
		return configimpact.Plan{}, NewError(CodeNotFound, ErrMenuNotFound)
	}
	apiIDs, err := service.repository.APIIDsByMenuIDs(ctx, []uint{menuID}, true)
	if err != nil {
		return configimpact.Plan{}, err
	}
	apiFacts, err := service.impactAPIFacts(ctx, apiIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	roleIDs, err := service.affectedRoleIDs(ctx, []uint{menuID}, []string{menu.PermissionCode})
	if err != nil {
		return configimpact.Plan{}, err
	}
	userIDs, err := service.authorization.UserIDsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return configimpact.Plan{}, err
	}
	changed := len(apiIDs) > 0
	facts := menuImpactFacts{
		Menus: []menuImpactMenuFact{menuImpactMenuFactFromMenu(menu)}, APIIDs: uniqueUintIDs(apiIDs),
		APIs: apiFacts, RoleIDs: uniqueUintIDs(roleIDs), UserIDs: uniqueUintIDs(userIDs),
	}
	summary := configimpact.Summary{
		AffectedMenuIDs: []uint{menuID}, AffectedAPIIDs: uniqueUintIDs(apiIDs),
		AffectedRoleCount: len(facts.RoleIDs), AffectedUserCount: len(facts.UserIDs),
		AccessVersionChange: changed, RequiresConfirmation: changed,
	}
	return newMenuImpactPlan(string(ImpactClearAPIs), menuImpactTarget{MenuID: menuID, Clear: true}, facts, summary)
}

func (service *Service) buildSyncImpactPlan(ctx context.Context, items []SyncItem) (configimpact.Plan, error) {
	planItems, err := normalizeSyncMenuPlan(items)
	if err != nil {
		return configimpact.Plan{}, err
	}
	paths := make([]string, 0, len(planItems)*2)
	for _, item := range planItems {
		paths = append(paths, item.path)
		if item.parentPath != "" {
			paths = append(paths, item.parentPath)
		}
	}
	locked, err := service.repository.LockMenusByPaths(ctx, paths)
	if err != nil {
		return configimpact.Plan{}, err
	}
	byPath := make(map[string]Menu, len(locked)+len(planItems))
	for _, menu := range locked {
		if menu.Path != nil {
			byPath[*menu.Path] = menu
		}
	}
	created, skipped := 0, 0
	affectedIDs := make([]uint, 0, len(locked))
	for _, item := range planItems {
		if item.parentPath != "" {
			parent, found := byPath[item.parentPath]
			if !found {
				return configimpact.Plan{}, NewError(CodeConflict, nil)
			}
			if item.typeValue == 3 && parent.Type != 1 && parent.Type != 2 {
				return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
			}
		}
		if existing, exists := byPath[item.path]; exists {
			skipped++
			affectedIDs = append(affectedIDs, existing.ID)
			continue
		}
		created++
		path := item.path
		menu := Menu{Name: item.name, Path: &path, Component: item.component, Type: item.typeValue, Status: 1}
		if item.parentPath != "" {
			menu.ParentID = byPath[item.parentPath].ID
		}
		byPath[item.path] = menu
	}
	userIDs, err := service.authorization.AllUserIDs(ctx)
	if err != nil {
		return configimpact.Plan{}, err
	}
	routes := make([]menuImpactSyncItem, len(planItems))
	for index, item := range planItems {
		routes[index] = menuImpactSyncItem{Name: item.name, Path: item.path, Component: item.component, Icon: item.icon, PermissionCode: item.permissionCode, ParentPath: item.parentPath, Sort: item.sort, Type: item.typeValue}
	}
	facts := menuImpactFacts{Menus: menuImpactMenuFacts(locked), UserIDs: uniqueUintIDs(userIDs)}
	summary := configimpact.Summary{
		AffectedMenuIDs: uniqueUintIDs(affectedIDs), CreatedCount: created, SkippedCount: skipped,
		AffectedUserCount: len(facts.UserIDs), AccessVersionChange: created > 0,
		RequiresConfirmation: created > 0,
	}
	target := menuImpactTarget{Routes: routes}
	return newMenuImpactPlan(string(ImpactSyncMenus), target, facts, summary)
}

func (service *Service) impactParent(ctx context.Context, parentID, menuID uint, menuType int) (Menu, error) {
	if parentID == 0 {
		return Menu{}, nil
	}
	if parentID == menuID {
		return Menu{}, NewError(CodeValidationInvalid, nil)
	}
	parent, err := service.repository.FindMenu(ctx, parentID)
	if err != nil {
		return Menu{}, NewError(CodeNotFound, ErrMenuNotFound)
	}
	if parent.Deleted {
		return Menu{}, NewError(CodeNotFound, ErrMenuNotFound)
	}
	if menuType == 3 && parent.Type != 1 && parent.Type != 2 {
		return Menu{}, NewError(CodeValidationInvalid, nil)
	}
	return parent, nil
}

func (service *Service) impactAPIFacts(ctx context.Context, ids []uint) ([]menuImpactAPIFact, error) {
	ids = uniqueUintIDs(ids)
	if len(ids) == 0 {
		return []menuImpactAPIFact{}, nil
	}
	apis, err := service.apis.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	facts := make([]menuImpactAPIFact, 0, len(apis))
	for _, api := range apis {
		facts = append(facts, menuImpactAPIFact{ID: api.ID, PermissionCode: api.PermissionCode, Status: api.Status, NeedAuth: api.NeedAuth, Deleted: api.Deleted})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	return facts, nil
}

func newMenuImpactPlan(operation string, target any, facts menuImpactFacts, summary configimpact.Summary) (configimpact.Plan, error) {
	return configimpact.NewPlan(operation, target, facts, summary)
}

func menuImpactTargetFromMenu(menu Menu) menuImpactTarget {
	target := menuImpactTarget{MenuID: menu.ID, ParentID: menu.ParentID, Name: menu.Name, Component: menu.Component, Icon: menu.Icon, PermissionCode: menu.PermissionCode, Sort: menu.Sort, Type: menu.Type, Status: menu.Status}
	if menu.Path != nil {
		target.Path = strings.TrimSpace(*menu.Path)
	}
	return target
}

func menuImpactMenuFactFromMenu(menu Menu) menuImpactMenuFact {
	fact := menuImpactMenuFact{ID: menu.ID, ParentID: menu.ParentID, Name: menu.Name, Component: menu.Component, Icon: menu.Icon, PermissionCode: menu.PermissionCode, Sort: menu.Sort, Type: menu.Type, Status: menu.Status}
	if menu.Path != nil {
		fact.Path = strings.TrimSpace(*menu.Path)
	}
	return fact
}

func nonEmptyMenuFacts(menu Menu) []menuImpactMenuFact {
	if menu.ID == 0 {
		return []menuImpactMenuFact{}
	}
	return []menuImpactMenuFact{menuImpactMenuFactFromMenu(menu)}
}

func menuImpactMenuFacts(menus []Menu) []menuImpactMenuFact {
	facts := make([]menuImpactMenuFact, 0, len(menus))
	for _, menu := range menus {
		facts = append(facts, menuImpactMenuFactFromMenu(menu))
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	return facts
}

func sameMenuFacts(left, right Menu) bool {
	leftPath, rightPath := "", ""
	if left.Path != nil {
		leftPath = *left.Path
	}
	if right.Path != nil {
		rightPath = *right.Path
	}
	return left.ParentID == right.ParentID && left.Name == right.Name && leftPath == rightPath && left.Component == right.Component && left.Icon == right.Icon && left.PermissionCode == right.PermissionCode && left.Sort == right.Sort && left.Type == right.Type && left.Status == right.Status
}
