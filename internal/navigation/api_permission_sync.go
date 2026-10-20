package navigation

import (
	"context"
	"sort"
	"strings"
)

type apiPermissionSyncContextKey struct{}

type apiPermissionSyncRoot struct {
	code         string
	menuIDs      []uint
	linkedAPIIDs []uint
}

// This state belongs to one transaction attempt. Inspection rows are held for
// cleanup only; roots retain their original direct propagation graph.
type preparedAPIPermissionSync struct {
	apis  map[uint]APIRecord
	roots map[uint]apiPermissionSyncRoot
	codes map[string]struct{}
	users map[uint]struct{}
}

func apiPermissionSyncFromContext(ctx context.Context) *preparedAPIPermissionSync {
	prepared, _ := ctx.Value(apiPermissionSyncContextKey{}).(*preparedAPIPermissionSync)
	return prepared
}

func (service *Service) ExistingPermissionCodes(ctx context.Context, codes []string, includeDeleted bool) ([]string, error) {
	return service.authorization.ExistingPermissionCodes(ctx, codes, includeDeleted)
}

// WithAPIPermissionSync acquires each resource family for the whole request
// before allowing the original, deliberately unreordered business loop to run.
func (service *Service) WithAPIPermissionSync(ctx context.Context, targets []APIPermissionSyncTarget, work func(context.Context) error) error {
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
		if apiPermissionSyncFromContext(transactionContext) != nil {
			return NewError(CodeConflict, nil)
		}
		prepared, err := service.prepareAPIPermissionSync(transactionContext, targets)
		if err != nil {
			return err
		}
		return work(context.WithValue(transactionContext, apiPermissionSyncContextKey{}, prepared))
	})
}

func (service *Service) prepareAPIPermissionSync(ctx context.Context, targets []APIPermissionSyncTarget) (*preparedAPIPermissionSync, error) {
	prepared := &preparedAPIPermissionSync{
		apis:  make(map[uint]APIRecord),
		roots: make(map[uint]apiPermissionSyncRoot), codes: make(map[string]struct{}), users: make(map[uint]struct{}),
	}
	var apiIDs, menuIDs []uint
	mutationCodeSet := make(map[string]struct{})
	for _, target := range targets {
		api := target.API
		if api.NeedAuth != 1 {
			continue
		}
		apiIDs = append(apiIDs, api.ID)
		code := strings.TrimSpace(target.Code)
		prepared.codes[code] = struct{}{}
		if strings.TrimSpace(api.PermissionCode) != "" {
			continue
		}
		root := apiPermissionSyncRoot{code: code}
		var err error
		root.menuIDs, err = service.repository.MenuIDsByAPIIDs(ctx, []uint{api.ID}, false)
		if err != nil {
			return nil, err
		}
		root.menuIDs = uniqueUintIDs(root.menuIDs)
		root.linkedAPIIDs, err = service.repository.APIIDsByMenuIDs(ctx, root.menuIDs, false)
		if err != nil {
			return nil, err
		}
		root.linkedAPIIDs = uniqueUintIDs(root.linkedAPIIDs)
		prepared.roots[api.ID] = root
		apiIDs = append(apiIDs, root.linkedAPIIDs...)
		menuIDs = append(menuIDs, root.menuIDs...)
		if code != "" {
			mutationCodeSet[code] = struct{}{}
		}
	}

	// A prior root can propagate its new code to a later root, making that
	// code a cleanup target later in the original loop. Discover all of them
	// now, without turning inspection-only references into mutation roots.
	codes := make([]string, 0, len(prepared.codes))
	for code := range prepared.codes {
		if code != "" {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	mutationCodes := make([]string, 0, len(mutationCodeSet))
	for code := range mutationCodeSet {
		mutationCodes = append(mutationCodes, code)
	}
	sort.Strings(mutationCodes)
	menuReferences := make(map[string][]uint, len(mutationCodes))
	apiReferences := make(map[string][]uint, len(mutationCodes))
	for _, code := range mutationCodes {
		menus, err := service.repository.ListByPermissionCode(ctx, code)
		if err != nil {
			return nil, err
		}
		apis, err := service.apis.ListByPermissionCode(ctx, code)
		if err != nil {
			return nil, err
		}
		menuReferences[code] = menuIDsWithCode(menus, code)
		apiReferences[code] = apiIDsWithCode(apis, code)
	}
	if err := service.authorization.LockPermissions(ctx, codes); err != nil {
		return nil, err
	}
	apis, err := service.apis.LockDependencies(ctx, uniqueUintIDs(apiIDs), mutationCodes)
	if err != nil {
		return nil, err
	}
	menus, err := service.repository.LockDependencies(ctx, uniqueUintIDs(menuIDs), mutationCodes)
	if err != nil {
		return nil, err
	}
	for _, code := range mutationCodes {
		if !sameUintIDs(menuReferences[code], menuIDsWithCode(menus, code)) ||
			!sameUintIDs(apiReferences[code], apiIDsWithCode(apis, code)) {
			return nil, NewError(CodeConflict, nil)
		}
	}
	for _, api := range apis {
		prepared.apis[api.ID] = api
	}
	currentMenus := make(map[uint]bool, len(menus))
	for _, menu := range menus {
		currentMenus[menu.ID] = menu.Deleted
	}
	for _, target := range targets {
		if target.API.NeedAuth != 1 {
			continue
		}
		if current, exists := prepared.apis[target.API.ID]; !exists || current != target.API || current.Deleted {
			return nil, NewError(CodeConflict, nil)
		}
	}
	for _, id := range uniqueUintIDs(menuIDs) {
		if deleted, exists := currentMenus[id]; !exists || deleted {
			return nil, NewError(CodeConflict, nil)
		}
	}
	for _, id := range uniqueUintIDs(apiIDs) {
		if api, exists := prepared.apis[id]; !exists || api.Deleted {
			return nil, NewError(CodeConflict, nil)
		}
	}
	for _, target := range targets {
		root, exists := prepared.roots[target.API.ID]
		if !exists {
			continue
		}
		if err := service.recheckAPIPermissionSyncRelations(ctx, target.API.ID, root); err != nil {
			return nil, err
		}
	}

	// Only actual mutation menus and possible old/new mutation codes contribute
	// users. Inspection-only references and Ensure-only APIs must not do so.
	roles, err := service.affectedRoleIDs(ctx, uniqueUintIDs(menuIDs), mutationCodes)
	if err != nil {
		return nil, err
	}
	users, err := service.authorization.UserIDsByRoleIDs(ctx, roles)
	if err != nil {
		return nil, err
	}
	users = uniqueUintIDs(users)
	if err := service.authorization.LockAccessVersions(ctx, users); err != nil {
		return nil, err
	}
	for _, id := range users {
		prepared.users[id] = struct{}{}
	}
	return prepared, nil
}

func (service *Service) recheckAPIPermissionSyncRelations(ctx context.Context, apiID uint, root apiPermissionSyncRoot) error {
	menus, err := service.repository.MenuIDsByAPIIDs(ctx, []uint{apiID}, true)
	if err != nil {
		return err
	}
	apis, err := service.repository.APIIDsByMenuIDs(ctx, root.menuIDs, true)
	if err != nil {
		return err
	}
	if !sameUintIDs(root.menuIDs, uniqueUintIDs(menus)) || !sameUintIDs(root.linkedAPIIDs, uniqueUintIDs(apis)) {
		return NewError(CodeConflict, nil)
	}
	return nil
}

func (prepared *preparedAPIPermissionSync) requireCodes(codes ...string) error {
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, exists := prepared.codes[code]; !exists {
			return NewError(CodeConflict, nil)
		}
	}
	return nil
}

func (prepared *preparedAPIPermissionSync) requireUsers(ids []uint) error {
	for _, id := range ids {
		if _, exists := prepared.users[id]; !exists {
			return NewError(CodeConflict, nil)
		}
	}
	return nil
}

func (prepared *preparedAPIPermissionSync) recordPropagation(apiID uint, linkedAPIIDs []uint, code string) {
	api := prepared.apis[apiID]
	api.PermissionCode = code
	prepared.apis[apiID] = api
	for _, id := range linkedAPIIDs {
		api := prepared.apis[id]
		api.PermissionCode = code
		prepared.apis[id] = api
	}
}

func (service *Service) lockPreparedAPICoordinatorTargets(ctx context.Context, prepared *preparedAPIPermissionSync, apiID uint, requestedCode *string) (APIRecord, []uint, []uint, error) {
	root, exists := prepared.roots[apiID]
	if !exists || requestedCode == nil || strings.TrimSpace(*requestedCode) != root.code {
		return APIRecord{}, nil, nil, NewError(CodeConflict, nil)
	}
	expected, exists := prepared.apis[apiID]
	if !exists {
		return APIRecord{}, nil, nil, NewError(CodeConflict, nil)
	}
	if err := prepared.requireCodes(expected.PermissionCode, root.code); err != nil {
		return APIRecord{}, nil, nil, err
	}
	// Lock is a current read of an already held ID, never a new acquisition.
	current, err := service.apis.Lock(ctx, apiID)
	if err != nil {
		return APIRecord{}, nil, nil, err
	}
	if current != expected || current.Deleted {
		return APIRecord{}, nil, nil, NewError(CodeConflict, nil)
	}
	if err := service.recheckAPIPermissionSyncRelations(ctx, apiID, root); err != nil {
		return APIRecord{}, nil, nil, err
	}
	return current, root.menuIDs, root.linkedAPIIDs, nil
}
