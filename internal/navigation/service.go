package navigation

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	repository    Repository
	transactions  TransactionRunner
	authorization Authorization
	apis          APIMetadataReader
	cache         CacheInvalidator
	metrics       Metrics
}

func NewService(repository Repository, transactions TransactionRunner, authorization Authorization, apis APIMetadataReader, options ...ServiceOption) *Service {
	service := &Service{repository: repository, transactions: transactions, authorization: authorization, apis: apis}
	for _, option := range options {
		option(service)
	}
	return service
}

func (service *Service) MenuTree(ctx context.Context) ([]MenuDetail, error) {
	menus, err := service.repository.ListMenus(ctx, false)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	return buildMenuTree(menus, 0), nil
}

func (service *Service) CreateMenu(ctx context.Context, input CreateInput) (MenuDetail, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return MenuDetail{}, NewError(CodeValidationInvalid, nil)
	}
	path := normalizeMenuPath(input.Path)
	if path != nil {
		exists, err := service.repository.PathExists(ctx, *path, 0)
		if err != nil {
			return MenuDetail{}, err
		}
		if exists {
			return MenuDetail{}, NewError(CodeConflict, nil)
		}
	}
	menuType := input.Type
	if menuType == 0 {
		menuType = 1
	}
	status := 1
	if input.Status != nil {
		status = *input.Status
	}
	menu := Menu{
		ParentID: input.ParentID, Name: name, Path: path,
		Component: strings.TrimSpace(input.Component), Icon: input.Icon,
		PermissionCode: strings.TrimSpace(input.PermissionCode),
		Sort:           input.Sort, Type: menuType, Status: status,
	}
	if err := validateMenuFields(menu); err != nil {
		return MenuDetail{}, err
	}
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		if err := service.authorization.LockPermissions(transactionContext, []string{menu.PermissionCode}); err != nil {
			return err
		}
		if _, err := service.lockMenuTargets(transactionContext, 0, input.ParentID); err != nil {
			return err
		}
		if err := service.validateMenuParent(transactionContext, 0, menu.ParentID, menu.Type); err != nil {
			return err
		}
		if err := service.verifyMenuFingerprint(transactionContext, input.ExpectedFingerprint, ImpactPreviewInput{Operation: ImpactCreateMenu, Create: input}); err != nil {
			return err
		}
		if err := service.repository.CreateMenu(transactionContext, &menu); err != nil {
			return NewError(CodeInternalError, err)
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, uniqueUintIDs(userIDs))
	})
	return menuToDetail(menu), err
}
func (service *Service) UpdateMenu(ctx context.Context, menuID uint, input UpdateInput) (MenuDetail, error) {
	if input.ParentID != nil && *input.ParentID == menuID {
		return MenuDetail{}, NewError(CodeValidationInvalid, nil)
	}
	updates := make(map[string]any)
	if input.ParentID != nil {
		updates["parent_id"] = *input.ParentID
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return MenuDetail{}, NewError(CodeValidationInvalid, nil)
		}
		updates["name"] = name
	}
	if input.Path != nil {
		path := normalizeMenuPath(*input.Path)
		if path != nil {
			exists, err := service.repository.PathExists(ctx, *path, menuID)
			if err != nil {
				return MenuDetail{}, err
			}
			if exists {
				return MenuDetail{}, NewError(CodeConflict, nil)
			}
		}
		updates["path"] = path
	}
	if input.Component != nil {
		updates["component"] = strings.TrimSpace(*input.Component)
	}
	if input.Icon != nil {
		updates["icon"] = *input.Icon
	}
	if input.PermissionCode != nil {
		updates["permission_code"] = strings.TrimSpace(*input.PermissionCode)
	}
	if input.Sort != nil {
		updates["sort"] = *input.Sort
	}
	if input.Type != nil {
		updates["type"] = *input.Type
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}
	if len(updates) == 0 {
		return MenuDetail{}, NewError(CodeValidationInvalid, nil)
	}
	var menu Menu
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		parentID := uint(0)
		if input.ParentID != nil {
			parentID = *input.ParentID
		}
		var lockedMenu Menu
		var apiIDs []uint
		var apis []APIRecord
		var permissionCode string
		var err error
		if input.PermissionCode != nil {
			snapshot, findErr := service.repository.FindMenu(transactionContext, menuID)
			if findErr != nil {
				if errors.Is(findErr, ErrMenuNotFound) {
					return NewError(CodeNotFound, findErr)
				}
				return findErr
			}
			currentAPIIDs, idsErr := service.repository.APIIDsByMenuIDs(transactionContext, []uint{menuID}, false)
			if idsErr != nil {
				return idsErr
			}
			if snapshot.Type == 3 || len(currentAPIIDs) > 0 {
				lockedMenu, apiIDs, apis, permissionCode, err = service.lockMenuBindingTargets(transactionContext, menuID, parentID, nil, *input.PermissionCode)
			} else {
				lockedMenu, err = service.lockMenuTargets(transactionContext, menuID, parentID)
			}
			if err != nil {
				return err
			}
		} else {
			lockedMenu, err = service.lockMenuTargets(transactionContext, menuID, parentID)
			if err != nil {
				return err
			}
		}
		if err := service.verifyMenuFingerprint(transactionContext, input.ExpectedFingerprint, ImpactPreviewInput{Operation: ImpactUpdateMenu, MenuID: menuID, Update: input}); err != nil {
			return err
		}
		targetMenu := menuAfterUpdate(lockedMenu, input)
		if err := validateMenuFields(targetMenu); err != nil {
			return err
		}
		if err := service.validateMenuParent(transactionContext, menuID, targetMenu.ParentID, targetMenu.Type); err != nil {
			return err
		}
		existingAPIIDs, err := service.repository.APIIDsByMenuIDs(transactionContext, []uint{menuID}, false)
		if err != nil {
			return err
		}
		if len(existingAPIIDs) > 0 && targetMenu.Type != 3 {
			return NewError(CodeValidationInvalid, nil)
		}
		fieldUpdates := changedMenuFields(lockedMenu, updates, len(apiIDs) > 0)
		if len(fieldUpdates) > 0 {
			if err := service.repository.UpdateMenu(transactionContext, menuID, fieldUpdates); err != nil {
				return NewError(CodeInternalError, err)
			}
		}
		bindingChanged := false
		if len(apiIDs) > 0 {
			if input.Name != nil {
				lockedMenu.Name = strings.TrimSpace(*input.Name)
			}
			if input.Sort != nil {
				lockedMenu.Sort = *input.Sort
			}
			bindingChanged, err = service.applyAPIBinding(transactionContext, lockedMenu, apiIDs, apis, permissionCode)
			if err != nil {
				return err
			}
		}
		if len(fieldUpdates) == 0 && !bindingChanged {
			menu = lockedMenu
			return nil
		}
		menu, err = service.repository.LockMenu(transactionContext, menuID)
		if err != nil {
			return err
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, uniqueUintIDs(userIDs))
	})
	return menuToDetail(menu), err
}

func (service *Service) DeleteMenu(ctx context.Context, menuID uint, expectedFingerprints ...string) error {
	expectedFingerprint := optionalFingerprint(expectedFingerprints)
	if menuID == 0 {
		return NewError(CodeNotFound, ErrMenuNotFound)
	}
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
		targets, err := service.repository.LockMenuDeletionTargets(transactionContext, menuID)
		if err != nil {
			return err
		}
		found, hasChildren := false, false
		for _, menu := range targets {
			found = found || menu.ID == menuID
			hasChildren = hasChildren || menu.ParentID == menuID
		}
		if !found {
			return NewError(CodeNotFound, ErrMenuNotFound)
		}
		if hasChildren {
			return NewError(CodeConflict, nil)
		}
		if err := service.verifyMenuFingerprint(transactionContext, expectedFingerprint, ImpactPreviewInput{Operation: ImpactDeleteMenu, MenuID: menuID}); err != nil {
			return err
		}
		if err := service.repository.DeleteMenu(transactionContext, menuID); err != nil {
			return err
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, uniqueUintIDs(userIDs))
	})
}
func (service *Service) AssignRoleMenus(ctx context.Context, roleID uint, menuIDs []uint) error {
	for _, menuID := range menuIDs {
		if menuID == 0 {
			return NewError(CodeValidationInvalid, nil)
		}
	}
	menuIDs = uniqueUintIDs(menuIDs)
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
		role, err := service.authorization.LockRoleConfiguration(transactionContext, roleID)
		if err != nil {
			return err
		}
		if !role.Exists {
			return NewError(CodeNotFound, nil)
		}
		if role.Protected {
			return NewError(CodeConflict, nil)
		}
		if _, err := service.lockMenuTargets(transactionContext, 0, menuIDs...); err != nil {
			if errors.Is(err, ErrMenuNotFound) {
				return NewError(CodeValidationInvalid, err)
			}
			return err
		}
		currentIDs, err := service.repository.MenuIDsByRoleIDs(transactionContext, []uint{roleID})
		if err != nil {
			return err
		}
		if sameUintIDs(uniqueUintIDs(currentIDs), menuIDs) {
			return nil
		}
		if err := service.repository.ReplaceRoleMenus(transactionContext, roleID, menuIDs); err != nil {
			return err
		}
		userIDs, err := service.authorization.UserIDsByRoleIDs(transactionContext, []uint{roleID})
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, uniqueUintIDs(userIDs))
	})
}

func (service *Service) RoleMenus(ctx context.Context, roleID uint) ([]MenuDetail, error) {
	role, err := service.authorization.RoleConfiguration(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if !role.Exists {
		return nil, NewError(CodeNotFound, nil)
	}
	menuIDs, err := service.repository.MenuIDsByRoleIDs(ctx, []uint{roleID})
	if err != nil || len(menuIDs) == 0 {
		return []MenuDetail{}, err
	}
	menus, err := service.repository.ListMenus(ctx, false)
	if err != nil {
		return nil, err
	}
	assigned := filterMenusByIDs(menus, menuIDs)
	result := make([]MenuDetail, len(assigned))
	for index, menu := range assigned {
		result[index] = menuToDetail(menu)
	}
	return result, nil
}

func (service *Service) UserMenus(ctx context.Context, userID uint) ([]MenuDetail, error) {
	access, err := service.authorization.UserAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	menus, err := service.repository.ListMenus(ctx, true)
	if err != nil {
		return nil, err
	}
	if access.IsAdmin {
		return buildMenuTree(menus, 0), nil
	}
	assignedIDs, err := service.repository.MenuIDsByRoleIDs(ctx, access.RoleIDs)
	if err != nil || len(assignedIDs) == 0 {
		return []MenuDetail{}, err
	}
	return buildMenuTree(filterVisibleMenus(menus, assignedIDs, access.Permissions), 0), nil
}
func (service *Service) SyncMenus(ctx context.Context, items []SyncItem, expectedFingerprints ...string) (int, error) {
	expectedFingerprint := optionalFingerprint(expectedFingerprints)
	plan, err := normalizeSyncMenuPlan(items)
	if err != nil {
		return 0, err
	}
	created := 0
	err = service.transactions.Run(ctx, func(transactionContext context.Context) error {
		created = 0
		paths := make([]string, 0, len(plan)*2)
		codes := make([]string, 0, len(plan))
		for _, item := range plan {
			paths = append(paths, item.path)
			codes = append(codes, item.permissionCode)
			if item.parentPath != "" {
				paths = append(paths, item.parentPath)
			}
		}
		if err := service.authorization.LockPermissions(transactionContext, codes); err != nil {
			return err
		}
		locked, err := service.repository.LockMenusByPaths(transactionContext, paths)
		if err != nil {
			return err
		}
		byPath := make(map[string]Menu, len(locked)+len(plan))
		for _, menu := range locked {
			if menu.Path != nil {
				byPath[*menu.Path] = menu
			}
		}
		if err := service.verifyMenuFingerprint(transactionContext, expectedFingerprint, ImpactPreviewInput{Operation: ImpactSyncMenus, SyncItems: items}); err != nil {
			return err
		}
		for _, item := range plan {
			parentID := uint(0)
			if item.parentPath != "" {
				parent, found := byPath[item.parentPath]
				if !found {
					return NewError(CodeConflict, nil)
				}
				if item.typeValue == 3 && parent.Type != 1 && parent.Type != 2 {
					return NewError(CodeValidationInvalid, nil)
				}
				parentID = parent.ID
			}
			if _, exists := byPath[item.path]; exists {
				continue
			}
			exists, err := service.repository.PathExists(transactionContext, item.path, 0)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			path := item.path
			menu := Menu{
				ParentID: parentID, Name: item.name, Path: &path,
				Component: item.component, Icon: item.icon,
				PermissionCode: item.permissionCode,
				Sort:           item.sort, Type: item.typeValue, Status: 1,
			}
			if err := validateMenuFields(menu); err != nil {
				return err
			}
			if err := service.repository.CreateMenu(transactionContext, &menu); err != nil {
				return NewError(CodeInternalError, err)
			}
			created++
			byPath[item.path] = menu
		}
		if created == 0 {
			return nil
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, uniqueUintIDs(userIDs))
	})
	if err != nil {
		var classified *Error
		if errors.As(err, &classified) {
			return 0, err
		}
		return 0, NewError(CodeInternalError, err)
	}
	return created, nil
}

func (service *Service) MenuAPIs(ctx context.Context, menuID uint) ([]APIRecord, error) {
	if _, err := service.repository.FindMenu(ctx, menuID); err != nil {
		return nil, err
	}
	apiIDs, err := service.repository.APIIDsByMenuIDs(ctx, []uint{menuID}, false)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	if len(apiIDs) == 0 {
		return []APIRecord{}, nil
	}
	apis, err := service.apis.ListByIDs(ctx, apiIDs)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	return apis, nil
}
func (service *Service) AssignAPIs(ctx context.Context, menuID uint, apiIDs []uint, manualCode string, expectedFingerprints ...string) error {
	expectedFingerprint := optionalFingerprint(expectedFingerprints)
	for _, apiID := range apiIDs {
		if apiID == 0 {
			return NewError(CodeValidationInvalid, nil)
		}
	}
	apiIDs = uniqueUintIDs(apiIDs)
	if len(apiIDs) == 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	var affectedUserIDs []uint
	changed := false
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		affectedUserIDs = nil
		changed = false
		menu, _, apis, permissionCode, err := service.lockMenuBindingTargets(transactionContext, menuID, 0, apiIDs, manualCode)
		if err != nil {
			return err
		}
		if err := service.verifyMenuFingerprint(transactionContext, expectedFingerprint, ImpactPreviewInput{Operation: ImpactBindAPIs, MenuID: menuID, APIIDs: apiIDs, PermissionCode: manualCode}); err != nil {
			return err
		}
		beforeRoles, err := service.affectedRoleIDs(transactionContext, []uint{menuID}, []string{menu.PermissionCode})
		if err != nil {
			return err
		}
		currentIDs, err := service.repository.APIIDsByMenuIDs(transactionContext, []uint{menuID}, true)
		if err != nil {
			return err
		}
		bindingChanged, err := service.applyAPIBinding(transactionContext, menu, apiIDs, apis, permissionCode)
		if err != nil {
			return err
		}
		relationsChanged := !sameUintIDs(uniqueUintIDs(currentIDs), apiIDs)
		if !bindingChanged && !relationsChanged {
			return nil
		}
		if relationsChanged {
			if err := service.repository.ReplaceMenuAPIs(transactionContext, menuID, apiIDs); err != nil {
				return NewError(CodeInternalError, err)
			}
		}
		changed = true
		afterRoles, err := service.affectedRoleIDs(transactionContext, []uint{menuID}, []string{permissionCode})
		if err != nil {
			return err
		}
		affectedUserIDs, err = service.authorization.UserIDsByRoleIDs(transactionContext, uniqueUintIDs(append(beforeRoles, afterRoles...)))
		if err != nil {
			return err
		}
		affectedUserIDs = uniqueUintIDs(affectedUserIDs)
		return service.authorization.IncrementAccessVersions(transactionContext, affectedUserIDs)
	})
	if err != nil {
		return err
	}
	if changed {
		service.postCommitInvalidate(ctx, affectedUserIDs)
	}
	return nil
}
func (service *Service) ClearAPIs(ctx context.Context, menuID uint, expectedFingerprints ...string) error {
	expectedFingerprint := optionalFingerprint(expectedFingerprints)
	var affectedUserIDs []uint
	changed := false
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		menu, err := service.lockMenuTargets(transactionContext, menuID)
		if err != nil {
			return err
		}
		if err := service.verifyMenuFingerprint(transactionContext, expectedFingerprint, ImpactPreviewInput{Operation: ImpactClearAPIs, MenuID: menuID}); err != nil {
			return err
		}
		currentIDs, err := service.repository.APIIDsByMenuIDs(transactionContext, []uint{menuID}, true)
		if err != nil {
			return err
		}
		if len(currentIDs) == 0 {
			return nil
		}
		if _, _, err := service.lockReferenceTargets(transactionContext, []uint{menuID}, currentIDs, menu.PermissionCode, ""); err != nil {
			return err
		}
		roleIDs, err := service.affectedRoleIDs(transactionContext, []uint{menuID}, []string{menu.PermissionCode})
		if err != nil {
			return err
		}
		if err := service.repository.ReplaceMenuAPIs(transactionContext, menuID, nil); err != nil {
			return NewError(CodeInternalError, err)
		}
		affectedUserIDs, err = service.authorization.UserIDsByRoleIDs(transactionContext, roleIDs)
		if err != nil {
			return err
		}
		affectedUserIDs = uniqueUintIDs(affectedUserIDs)
		changed = true
		return service.authorization.IncrementAccessVersions(transactionContext, affectedUserIDs)
	})
	if err != nil {
		return err
	}
	if changed {
		service.postCommitInvalidate(ctx, affectedUserIDs)
	}
	return nil
}

// lockMenuBindingTargets discovers direct bindings before locking any API or
// menu. Cleanup-only references join the same ordered acquisition, never the
// propagation set. A changed binding/code decision is a conflict, not an
// invitation to append a lower-ID dependency after locks have been acquired.
func (service *Service) lockMenuBindingTargets(ctx context.Context, menuID, parentID uint, requestedIDs []uint, manualCode string) (Menu, []uint, []APIRecord, string, error) {
	snapshot, err := service.repository.FindMenu(ctx, menuID)
	if err != nil {
		if errors.Is(err, ErrMenuNotFound) {
			return Menu{}, nil, nil, "", NewError(CodeNotFound, err)
		}
		return Menu{}, nil, nil, "", err
	}
	oldIDs, err := service.repository.APIIDsByMenuIDs(ctx, []uint{menuID}, false)
	if err != nil {
		return Menu{}, nil, nil, "", err
	}
	targetIDs := requestedIDs
	if targetIDs == nil {
		targetIDs = oldIDs
	}
	discovered, err := service.apis.ListByIDs(ctx, targetIDs)
	if err != nil {
		return Menu{}, nil, nil, "", err
	}
	if len(discovered) != len(targetIDs) {
		return Menu{}, nil, nil, "", NewError(CodeNotFound, nil)
	}
	code := strings.TrimSpace(manualCode)
	if len(targetIDs) > 0 {
		code, err = resolveBindingCode(manualCode, discovered)
		if err != nil {
			return Menu{}, nil, nil, "", err
		}
	}
	menus, lockedAPIs, err := service.lockReferenceTargets(ctx, uniqueUintIDs([]uint{menuID, parentID}), uniqueUintIDs(append(append([]uint{}, oldIDs...), targetIDs...)), snapshot.PermissionCode, code)
	if err != nil {
		return Menu{}, nil, nil, "", err
	}
	menu, exists := menuByID(menus, menuID)
	if !exists || menu.Deleted {
		return Menu{}, nil, nil, "", NewError(CodeNotFound, ErrMenuNotFound)
	}
	if menu.Type != 3 {
		return Menu{}, nil, nil, "", NewError(CodeValidationInvalid, nil)
	}
	if parentID != 0 {
		if parent, exists := menuByID(menus, parentID); !exists || parent.Deleted {
			return Menu{}, nil, nil, "", NewError(CodeNotFound, ErrMenuNotFound)
		}
	}
	if menu.PermissionCode != snapshot.PermissionCode {
		return Menu{}, nil, nil, "", NewError(CodeConflict, nil)
	}
	currentIDs, err := service.repository.APIIDsByMenuIDs(ctx, []uint{menuID}, true)
	if err != nil {
		return Menu{}, nil, nil, "", err
	}
	if !sameUintIDs(uniqueUintIDs(oldIDs), uniqueUintIDs(currentIDs)) {
		return Menu{}, nil, nil, "", NewError(CodeConflict, nil)
	}
	apis := make([]APIRecord, 0, len(targetIDs))
	for _, id := range targetIDs {
		api, exists := apiByID(lockedAPIs, id)
		if !exists || api.Deleted {
			return Menu{}, nil, nil, "", NewError(CodeNotFound, nil)
		}
		if err := validateLinkableAPI(api); err != nil {
			return Menu{}, nil, nil, "", err
		}
		apis = append(apis, api)
	}
	if len(targetIDs) > 0 {
		currentCode, err := resolveBindingCode(manualCode, apis)
		if err != nil {
			return Menu{}, nil, nil, "", err
		}
		if currentCode != code {
			return Menu{}, nil, nil, "", NewError(CodeConflict, nil)
		}
	}
	return menu, targetIDs, apis, code, nil
}

// applyAPIBinding updates binding facts inside the caller's transaction after
// the menu and target APIs have been locked. Relationships and versions belong
// to the outer use case.
func (service *Service) applyAPIBinding(ctx context.Context, menu Menu, apiIDs []uint, apis []APIRecord, permissionCode string) (bool, error) {
	unchanged := menu.PermissionCode == permissionCode
	for _, api := range apis {
		unchanged = unchanged && api.PermissionCode == permissionCode
	}
	if unchanged {
		codes, err := service.authorization.ExistingPermissionCodes(ctx, []string{permissionCode}, false)
		if err != nil {
			return false, err
		}
		if len(codes) > 0 {
			// Equal codes make the existing grant merge and old-code cleanup
			// inapplicable. Do not invent additional grants for this binding.
			return false, nil
		}
	}
	if _, _, err := service.authorization.EnsurePermission(ctx, PermissionSeed{Code: permissionCode, Name: menu.Name, Group: inferAPIGroup(apis), Sort: menu.Sort}); err != nil {
		return false, err
	}
	if err := service.mergePermissionRoles(ctx, menu.PermissionCode, permissionCode); err != nil {
		return false, err
	}
	if err := service.apis.SetPermissionCode(ctx, apiIDs, permissionCode); err != nil {
		return false, NewError(CodeInternalError, err)
	}
	if err := service.repository.UpdateMenusPermissionCode(ctx, []uint{menu.ID}, permissionCode); err != nil {
		return false, NewError(CodeInternalError, err)
	}
	if err := service.cleanupPermission(ctx, menu.PermissionCode, permissionCode); err != nil {
		return false, err
	}
	return true, nil
}
func (service *Service) RetryCacheInvalidation(ctx context.Context, userIDs []uint) error {
	if service.cache == nil {
		return nil
	}
	if err := service.cache.Invalidate(ctx, uniqueUintIDs(userIDs)); err != nil {
		if service.metrics != nil {
			service.metrics.RecordCacheInvalidationFailure()
		}
		return err
	}
	return nil
}

func (service *Service) postCommitInvalidate(ctx context.Context, userIDs []uint) {
	service.transactions.AfterCommit(ctx, func() {
		_ = service.RetryCacheInvalidation(ctx, userIDs)
	})
}
func (service *Service) ChangeAPIPermissionCode(ctx context.Context, apiID uint, requestedCode string, update func(context.Context) error) error {
	return service.ChangeAPIPermissionCodeWithGuard(ctx, apiID, requestedCode, nil, update)
}

func (service *Service) ChangeAPIPermissionCodeWithGuard(ctx context.Context, apiID uint, requestedCode string, guard func(context.Context) error, update func(context.Context) error) error {
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
		api, menuIDs, linkedAPIIDs, err := service.lockAPICoordinatorTargets(transactionContext, apiID, &requestedCode)
		if err != nil {
			return err
		}
		permissionCode := strings.TrimSpace(requestedCode)
		if permissionCode == "" && api.NeedAuth == 1 {
			permissionCode = derivePermissionCode(api.Method, api.Path)
		}
		if guard != nil {
			if err := guard(transactionContext); err != nil {
				return err
			}
		}
		beforeRoles, err := service.affectedRoleIDs(transactionContext, menuIDs, []string{api.PermissionCode})
		if err != nil {
			return err
		}
		if permissionCode != "" {
			if _, _, err := service.authorization.EnsurePermission(transactionContext, PermissionSeed{Code: permissionCode, Name: api.Name, Group: api.Group, Sort: api.Sort}); err != nil {
				return err
			}
			if err := service.mergePermissionRoles(transactionContext, api.PermissionCode, permissionCode); err != nil {
				return err
			}
		}
		if err := update(transactionContext); err != nil {
			return err
		}
		if len(menuIDs) > 0 {
			if err := service.repository.UpdateMenusPermissionCode(transactionContext, menuIDs, permissionCode); err != nil {
				return err
			}
			if err := service.apis.SetPermissionCode(transactionContext, linkedAPIIDs, permissionCode); err != nil {
				return err
			}
		}
		if prepared := apiPermissionSyncFromContext(transactionContext); prepared != nil {
			prepared.recordPropagation(apiID, linkedAPIIDs, permissionCode)
		}
		afterRoles, err := service.affectedRoleIDs(transactionContext, menuIDs, []string{permissionCode})
		if err != nil {
			return err
		}
		userIDs, err := service.authorization.UserIDsByRoleIDs(transactionContext, uniqueUintIDs(append(beforeRoles, afterRoles...)))
		if err != nil {
			return err
		}
		userIDs = uniqueUintIDs(userIDs)
		if prepared := apiPermissionSyncFromContext(transactionContext); prepared != nil {
			if err := prepared.requireUsers(userIDs); err != nil {
				return err
			}
		}
		if err := service.authorization.IncrementAccessVersions(transactionContext, userIDs); err != nil {
			return err
		}
		return service.cleanupPermission(transactionContext, api.PermissionCode, permissionCode)
	})
}
func (service *Service) DeleteAPI(ctx context.Context, apiID uint, deleteAPI func(context.Context) error) error {
	var affectedUserIDs []uint
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		api, menuIDs, _, err := service.lockAPICoordinatorTargets(transactionContext, apiID, nil)
		if err != nil {
			return err
		}
		roleIDs, err := service.affectedRoleIDs(transactionContext, menuIDs, []string{api.PermissionCode})
		if err != nil {
			return err
		}
		if err := service.repository.DeleteMenuAPIsByAPI(transactionContext, apiID); err != nil {
			return err
		}
		if err := deleteAPI(transactionContext); err != nil {
			return err
		}
		affectedUserIDs, err = service.authorization.UserIDsByRoleIDs(transactionContext, roleIDs)
		if err != nil {
			return err
		}
		affectedUserIDs = uniqueUintIDs(affectedUserIDs)
		if err := service.authorization.IncrementAccessVersions(transactionContext, affectedUserIDs); err != nil {
			return err
		}
		return service.cleanupPermission(transactionContext, api.PermissionCode, "")
	})
	if err != nil {
		return err
	}
	service.postCommitInvalidate(ctx, affectedUserIDs)
	return nil
}
func (service *Service) ValidateMenuButtonParent(ctx context.Context, parentID uint) error {
	return service.validateMenuParent(ctx, 0, parentID, 3)
}
func (service *Service) GenerateMenuButton(ctx context.Context, apiID, parentID uint, name string, sort int) (MenuDetail, error) {
	return service.GenerateMenuButtonWithGuard(ctx, apiID, parentID, name, sort, nil)
}

func (service *Service) GenerateMenuButtonWithGuard(ctx context.Context, apiID, parentID uint, name string, sort int, guard func(context.Context) error) (MenuDetail, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MenuDetail{}, NewError(CodeValidationInvalid, nil)
	}
	var menu Menu
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		discovered, err := service.apis.ListByIDs(transactionContext, []uint{apiID})
		if err != nil {
			return err
		}
		if len(discovered) != 1 {
			return NewError(CodeNotFound, nil)
		}
		snapshot := discovered[0]
		permissionCode := strings.TrimSpace(snapshot.PermissionCode)
		if permissionCode == "" {
			permissionCode = derivePermissionCode(snapshot.Method, snapshot.Path)
		}
		if err := service.authorization.LockPermissions(transactionContext, []string{permissionCode}); err != nil {
			return err
		}
		api, err := service.apis.Lock(transactionContext, apiID)
		if err != nil {
			return err
		}
		if api.PermissionCode != snapshot.PermissionCode || api.Method != snapshot.Method || api.Path != snapshot.Path || api.NeedAuth != snapshot.NeedAuth {
			return NewError(CodeConflict, nil)
		}
		if err := validateLinkableAPI(api); err != nil {
			return err
		}
		parent, err := service.repository.LockMenu(transactionContext, parentID)
		if err != nil {
			return NewError(CodeNotFound, err)
		}
		if parent.Type != 1 && parent.Type != 2 {
			return NewError(CodeValidationInvalid, nil)
		}
		if guard != nil {
			if err := guard(transactionContext); err != nil {
				return err
			}
		}
		if _, _, err := service.authorization.EnsurePermission(transactionContext, PermissionSeed{Code: permissionCode, Name: name, Group: api.Group, Sort: sort}); err != nil {
			return err
		}
		if err := service.apis.SetPermissionCode(transactionContext, []uint{apiID}, permissionCode); err != nil {
			return err
		}
		menu = Menu{ParentID: parentID, Name: name, PermissionCode: permissionCode, Sort: sort, Type: 3, Status: 1}
		if err := service.repository.CreateMenu(transactionContext, &menu); err != nil {
			return NewError(CodeInternalError, err)
		}
		if err := service.repository.ReplaceMenuAPIs(transactionContext, menu.ID, []uint{apiID}); err != nil {
			return NewError(CodeInternalError, err)
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, uniqueUintIDs(userIDs))
	})
	return menuToDetail(menu), err
}

// Discover only the existing direct coordination graph, then acquire each
// family's targets once in ID order. An RR snapshot is suitable for discovery,
// never for proving the graph remained unchanged after a lock wait.
func (service *Service) lockAPICoordinatorTargets(ctx context.Context, apiID uint, requestedCode *string) (APIRecord, []uint, []uint, error) {
	if prepared := apiPermissionSyncFromContext(ctx); prepared != nil {
		return service.lockPreparedAPICoordinatorTargets(ctx, prepared, apiID, requestedCode)
	}
	discovered, err := service.apis.ListByIDs(ctx, []uint{apiID})
	if err != nil {
		return APIRecord{}, nil, nil, err
	}
	if len(discovered) != 1 {
		_, err := service.apis.Lock(ctx, apiID)
		if err == nil {
			err = NewError(CodeConflict, nil)
		}
		return APIRecord{}, nil, nil, err
	}
	snapshot := discovered[0]
	newCode := ""
	if requestedCode != nil {
		newCode = strings.TrimSpace(*requestedCode)
		if newCode == "" && snapshot.NeedAuth == 1 {
			newCode = derivePermissionCode(snapshot.Method, snapshot.Path)
		}
	}
	menuIDs, err := service.repository.MenuIDsByAPIIDs(ctx, []uint{apiID}, false)
	if err != nil {
		return APIRecord{}, nil, nil, err
	}
	menuIDs = uniqueUintIDs(menuIDs)
	linkedAPIIDs, err := service.repository.APIIDsByMenuIDs(ctx, menuIDs, false)
	if err != nil {
		return APIRecord{}, nil, nil, err
	}
	linkedAPIIDs = uniqueUintIDs(linkedAPIIDs)
	menus, apis, err := service.lockReferenceTargets(ctx, menuIDs, uniqueUintIDs(append([]uint{apiID}, linkedAPIIDs...)), snapshot.PermissionCode, newCode)
	if err != nil {
		return APIRecord{}, nil, nil, err
	}
	target, exists := apiByID(apis, apiID)
	if !exists || target.Deleted {
		_, err := service.apis.Lock(ctx, apiID)
		return APIRecord{}, nil, nil, err
	}
	if target.PermissionCode != snapshot.PermissionCode || target.Method != snapshot.Method || target.Path != snapshot.Path || target.NeedAuth != snapshot.NeedAuth {
		return APIRecord{}, nil, nil, NewError(CodeConflict, nil)
	}
	for _, id := range menuIDs {
		if menu, exists := menuByID(menus, id); !exists || menu.Deleted {
			return APIRecord{}, nil, nil, NewError(CodeConflict, nil)
		}
	}
	currentMenus, err := service.repository.MenuIDsByAPIIDs(ctx, []uint{apiID}, true)
	if err != nil {
		return APIRecord{}, nil, nil, err
	}
	currentAPIs, err := service.repository.APIIDsByMenuIDs(ctx, menuIDs, true)
	if err != nil {
		return APIRecord{}, nil, nil, err
	}
	if !sameUintIDs(menuIDs, uniqueUintIDs(currentMenus)) || !sameUintIDs(linkedAPIIDs, uniqueUintIDs(currentAPIs)) {
		return APIRecord{}, nil, nil, NewError(CodeConflict, nil)
	}
	return target, menuIDs, linkedAPIIDs, nil
}

// Cleanup inspections share the same ordered acquisition as mutation targets,
// but never expand propagation or affected-user cohorts.
func (service *Service) lockReferenceTargets(ctx context.Context, menuIDs, apiIDs []uint, oldCode, newCode string) ([]Menu, []APIRecord, error) {
	if apiPermissionSyncFromContext(ctx) != nil {
		return nil, nil, NewError(CodeConflict, nil)
	}
	oldCode, newCode = strings.TrimSpace(oldCode), strings.TrimSpace(newCode)
	var menuReferences []Menu
	var apiReferences []APIRecord
	var inspectionCodes []string
	if oldCode != "" && oldCode != newCode {
		var err error
		menuReferences, err = service.repository.ListByPermissionCode(ctx, oldCode)
		if err != nil {
			return nil, nil, err
		}
		apiReferences, err = service.apis.ListByPermissionCode(ctx, oldCode)
		if err != nil {
			return nil, nil, err
		}
		inspectionCodes = []string{oldCode}
	}
	if err := service.authorization.LockPermissions(ctx, []string{oldCode, newCode}); err != nil {
		return nil, nil, err
	}
	apis, err := service.apis.LockDependencies(ctx, apiIDs, inspectionCodes)
	if err != nil {
		return nil, nil, err
	}
	menus, err := service.repository.LockDependencies(ctx, menuIDs, inspectionCodes)
	if err != nil {
		return nil, nil, err
	}
	if len(inspectionCodes) != 0 {
		if !sameUintIDs(menuIDsWithCode(menuReferences, oldCode), menuIDsWithCode(menus, oldCode)) ||
			!sameUintIDs(apiIDsWithCode(apiReferences, oldCode), apiIDsWithCode(apis, oldCode)) {
			return nil, nil, NewError(CodeConflict, nil)
		}
	}
	return menus, apis, nil
}

func menuByID(menus []Menu, id uint) (Menu, bool) {
	for _, menu := range menus {
		if menu.ID == id {
			return menu, true
		}
	}
	return Menu{}, false
}

func apiByID(apis []APIRecord, id uint) (APIRecord, bool) {
	for _, api := range apis {
		if api.ID == id {
			return api, true
		}
	}
	return APIRecord{}, false
}

func menuIDsWithCode(menus []Menu, code string) []uint {
	var ids []uint
	for _, menu := range menus {
		if menu.PermissionCode == code {
			ids = append(ids, menu.ID)
		}
	}
	return uniqueUintIDs(ids)
}

func apiIDsWithCode(apis []APIRecord, code string) []uint {
	var ids []uint
	for _, api := range apis {
		if api.PermissionCode == code {
			ids = append(ids, api.ID)
		}
	}
	return uniqueUintIDs(ids)
}

func sameUintIDs(left, right []uint) bool {
	if len(left) != len(right) {
		return false
	}
	for index, id := range left {
		if id != right[index] {
			return false
		}
	}
	return true
}

// lockMenuTargets shares the deletion barrier for both the edited menu and
// referenced menus. Zero denotes the root, not a persisted target.
func (service *Service) lockMenuTargets(ctx context.Context, targetID uint, references ...uint) (Menu, error) {
	ids := uniqueUintIDs(append([]uint{targetID}, references...))
	var target Menu
	for _, id := range ids {
		menu, err := service.repository.LockMenu(ctx, id)
		if err != nil {
			if errors.Is(err, ErrMenuNotFound) {
				return Menu{}, NewError(CodeNotFound, err)
			}
			return Menu{}, err
		}
		if id == targetID {
			target = menu
		}
	}
	return target, nil
}

func buildMenuTree(menus []Menu, parentID uint) []MenuDetail {
	result := []MenuDetail{}
	for _, menu := range menus {
		if menu.ParentID != parentID {
			continue
		}
		detail := menuToDetail(menu)
		detail.Children = buildMenuTree(menus, menu.ID)
		result = append(result, detail)
	}
	return result
}
func filterMenusByIDs(menus []Menu, ids []uint) []Menu {
	selected := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		selected[id] = struct{}{}
	}
	result := make([]Menu, 0, len(ids))
	for _, menu := range menus {
		if _, exists := selected[menu.ID]; exists {
			result = append(result, menu)
		}
	}
	return result
}

func filterVisibleMenus(menus []Menu, assignedIDs []uint, permissions []string) []Menu {
	assigned := make(map[uint]struct{}, len(assignedIDs))
	for _, id := range assignedIDs {
		assigned[id] = struct{}{}
	}
	permissionSet := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		permissionSet[permission] = struct{}{}
	}
	_, allPermissions := permissionSet["*"]
	if _, admin := permissionSet["admin"]; admin {
		allPermissions = true
	}
	menuByID := make(map[uint]Menu, len(menus))
	visible := make(map[uint]struct{})
	for _, menu := range menus {
		menuByID[menu.ID] = menu
		if _, selected := assigned[menu.ID]; !selected {
			continue
		}
		if menu.PermissionCode == "" || allPermissions {
			visible[menu.ID] = struct{}{}
			continue
		}
		if _, allowed := permissionSet[menu.PermissionCode]; allowed {
			visible[menu.ID] = struct{}{}
		}
	}
	for menuID := range visible {
		parentID := menuByID[menuID].ParentID
		for parentID != 0 {
			parent, exists := menuByID[parentID]
			if !exists {
				break
			}
			visible[parent.ID] = struct{}{}
			parentID = parent.ParentID
		}
	}
	result := make([]Menu, 0, len(visible))
	for _, menu := range menus {
		if _, exists := visible[menu.ID]; exists {
			result = append(result, menu)
		}
	}
	return result
}

func menuToDetail(menu Menu) MenuDetail {
	path := ""
	if menu.Path != nil {
		path = *menu.Path
	}
	return MenuDetail{
		ID: menu.ID, ParentID: menu.ParentID, Name: menu.Name, Path: path,
		Component: menu.Component, Icon: menu.Icon,
		PermissionCode: menu.PermissionCode, Sort: menu.Sort,
		Type: menu.Type, Status: menu.Status,
	}
}

func normalizeMenuPath(path string) *string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &path
}
func validateMenuFields(menu Menu) error {
	if menu.Type < 1 || menu.Type > 3 || menu.Status < 0 || menu.Status > 1 {
		return NewError(CodeValidationInvalid, nil)
	}
	hasPath := menu.Path != nil && strings.TrimSpace(*menu.Path) != ""
	hasComponent := strings.TrimSpace(menu.Component) != ""
	switch menu.Type {
	case 1:
		if hasPath || hasComponent {
			return NewError(CodeValidationInvalid, nil)
		}
	case 2:
		if !hasPath {
			return NewError(CodeValidationInvalid, nil)
		}
	case 3:
		if hasPath || hasComponent {
			return NewError(CodeValidationInvalid, nil)
		}
	}
	return nil
}

func (service *Service) validateMenuParent(ctx context.Context, menuID, parentID uint, menuType int) error {
	if parentID == 0 {
		return nil
	}
	if menuID != 0 && menuID == parentID {
		return NewError(CodeValidationInvalid, nil)
	}
	parent, err := service.repository.LockMenu(ctx, parentID)
	if err != nil {
		if errors.Is(err, ErrMenuNotFound) {
			return NewError(CodeNotFound, err)
		}
		return err
	}
	if parent.Deleted {
		return NewError(CodeNotFound, ErrMenuNotFound)
	}
	if menuType == 3 && parent.Type != 1 && parent.Type != 2 {
		return NewError(CodeValidationInvalid, nil)
	}
	return nil
}

func menuAfterUpdate(current Menu, input UpdateInput) Menu {
	target := current
	if input.ParentID != nil {
		target.ParentID = *input.ParentID
	}
	if input.Path != nil {
		target.Path = normalizeMenuPath(*input.Path)
	}
	if input.Component != nil {
		target.Component = strings.TrimSpace(*input.Component)
	}
	if input.Type != nil {
		target.Type = *input.Type
	}
	if input.Status != nil {
		target.Status = *input.Status
	}
	return target
}

func (service *Service) affectedRoleIDs(ctx context.Context, menuIDs []uint, permissionCodes []string) ([]uint, error) {
	roleIDs, err := service.repository.RoleIDsByMenuIDs(ctx, menuIDs)
	if err != nil {
		return nil, err
	}
	for _, code := range permissionCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		permissionRoleIDs, err := service.authorization.RoleIDsByPermissionCode(ctx, code)
		if err != nil {
			return nil, err
		}
		roleIDs = append(roleIDs, permissionRoleIDs...)
	}
	return uniqueUintIDs(roleIDs), nil
}

func (service *Service) mergePermissionRoles(ctx context.Context, oldCode, newCode string) error {
	oldCode, newCode = strings.TrimSpace(oldCode), strings.TrimSpace(newCode)
	if oldCode == "" || oldCode == newCode {
		return nil
	}
	if prepared := apiPermissionSyncFromContext(ctx); prepared != nil {
		if err := prepared.requireCodes(oldCode, newCode); err != nil {
			return err
		}
	}
	oldPermission, found, err := service.authorization.LockPermission(ctx, oldCode)
	if err != nil || !found {
		return err
	}
	newPermission, _, err := service.authorization.EnsurePermission(ctx, PermissionSeed{Code: newCode, Name: newCode, Group: "api"})
	if err != nil {
		return err
	}
	return service.authorization.MergePermissionRoles(ctx, oldPermission.ID, newPermission.ID)
}

func (service *Service) cleanupPermission(ctx context.Context, oldCode, newCode string) error {
	oldCode = strings.TrimSpace(oldCode)
	if oldCode == "" || oldCode == strings.TrimSpace(newCode) {
		return nil
	}
	if prepared := apiPermissionSyncFromContext(ctx); prepared != nil {
		if err := prepared.requireCodes(oldCode, newCode); err != nil {
			return err
		}
	}
	permission, found, err := service.authorization.LockPermission(ctx, oldCode)
	if err != nil || !found {
		return err
	}
	menuCount, err := service.repository.CountPermissionCode(ctx, oldCode)
	if err != nil || menuCount > 0 {
		return err
	}
	apiCount, err := service.apis.CountPermissionCode(ctx, oldCode)
	if err != nil || apiCount > 0 {
		return err
	}
	hasRoles, err := service.authorization.PermissionHasRoleReferences(ctx, permission.ID)
	if err != nil || hasRoles {
		return err
	}
	return service.authorization.DeletePermission(ctx, permission.ID)
}

func validateLinkableAPI(api APIRecord) error {
	if api.Status != 1 {
		return NewError(CodeValidationInvalid, nil)
	}
	if api.NeedAuth != 1 {
		return NewError(CodeValidationInvalid, nil)
	}
	return nil
}

func resolveBindingCode(manual string, apis []APIRecord) (string, error) {
	if code := strings.TrimSpace(manual); code != "" {
		return code, nil
	}
	codes := make(map[string]struct{})
	permissionCode := ""
	for _, api := range apis {
		permissionCode = strings.TrimSpace(api.PermissionCode)
		if permissionCode == "" {
			permissionCode = derivePermissionCode(api.Method, api.Path)
		}
		codes[permissionCode] = struct{}{}
	}
	if len(codes) == 0 {
		return "", NewError(CodeValidationInvalid, nil)
	}
	if len(codes) > 1 {
		return "", NewError(CodeValidationInvalid, nil)
	}
	return permissionCode, nil
}

func derivePermissionCode(method, path string) string {
	code := strings.TrimPrefix(path, "/api/")
	code = strings.ReplaceAll(code, ":", "")
	code = strings.ReplaceAll(code, "/", ".")
	code = strings.Trim(code, ".")
	return strings.ToLower(code + "." + strings.ToUpper(method))
}

func inferAPIGroup(apis []APIRecord) string {
	if len(apis) == 0 {
		return "api"
	}
	group := strings.TrimSpace(apis[0].Group)
	if group == "" {
		return "api"
	}
	for _, api := range apis[1:] {
		if strings.TrimSpace(api.Group) != group {
			return "api"
		}
	}
	return group
}

func (service *Service) EnsureAPIPermission(ctx context.Context, api APIRecord) (bool, error) {
	var created bool
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		prepared := apiPermissionSyncFromContext(transactionContext)
		if prepared != nil {
			if _, exists := prepared.apis[api.ID]; !exists {
				return NewError(CodeConflict, nil)
			}
		}
		current, err := service.apis.Lock(transactionContext, api.ID)
		if err != nil {
			return err
		}
		if prepared != nil {
			if current != prepared.apis[api.ID] {
				return NewError(CodeConflict, nil)
			}
			if err := prepared.requireCodes(current.PermissionCode); err != nil {
				return err
			}
		}
		if current.NeedAuth != 1 {
			return nil
		}
		api = current
		_, createdResult, err := service.authorization.EnsurePermission(transactionContext, PermissionSeed{Code: api.PermissionCode, Name: api.Name, Group: api.Group, Sort: api.Sort})
		created = createdResult
		return err
	})
	return created, err
}
