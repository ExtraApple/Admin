package navigation

import (
	"context"
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
	if menuType < 1 || menuType > 3 {
		menuType = 1
	}
	status := input.Status
	if status != 1 {
		status = 1
	}
	menu := Menu{
		ParentID: input.ParentID, Name: name, Path: path,
		Component: input.Component, Icon: input.Icon,
		PermissionCode: strings.TrimSpace(input.PermissionCode),
		Sort:           input.Sort, Type: menuType, Status: status,
	}
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		if err := service.repository.CreateMenu(transactionContext, &menu); err != nil {
			return NewError(CodeInternalError, err)
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, userIDs)
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
	if input.Name != "" {
		updates["name"] = strings.TrimSpace(input.Name)
	}
	if input.Path != "" {
		path := normalizeMenuPath(input.Path)
		exists, err := service.repository.PathExists(ctx, *path, menuID)
		if err != nil {
			return MenuDetail{}, err
		}
		if exists {
			return MenuDetail{}, NewError(CodeConflict, nil)
		}
		updates["path"] = path
	}
	if input.Component != "" {
		updates["component"] = input.Component
	}
	if input.Icon != "" {
		updates["icon"] = input.Icon
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
		if _, err := service.repository.LockMenu(transactionContext, menuID); err != nil {
			return err
		}
		if err := service.repository.UpdateMenu(transactionContext, menuID, updates); err != nil {
			return NewError(CodeInternalError, err)
		}
		var err error
		menu, err = service.repository.FindMenu(transactionContext, menuID)
		if err != nil {
			return err
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, userIDs)
	})
	return menuToDetail(menu), err
}

func (service *Service) DeleteMenu(ctx context.Context, menuID uint) error {
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
		if _, err := service.repository.LockMenu(transactionContext, menuID); err != nil {
			return err
		}
		childCount, err := service.repository.ChildCount(transactionContext, menuID)
		if err != nil {
			return err
		}
		if childCount > 0 {
			return NewError(CodeConflict, nil)
		}
		if err := service.repository.DeleteMenu(transactionContext, menuID); err != nil {
			return err
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, userIDs)
	})
}
func (service *Service) AssignRoleMenus(ctx context.Context, roleID uint, menuIDs []uint) error {
	exists, err := service.authorization.RoleExists(ctx, roleID)
	if err != nil {
		return err
	}
	if !exists {
		return NewError(CodeNotFound, nil)
	}
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
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
	exists, err := service.authorization.RoleExists(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, NewError(CodeNotFound, nil)
	}
	menuIDs, err := service.repository.MenuIDsByRoleIDs(ctx, []uint{roleID})
	if err != nil || len(menuIDs) == 0 {
		return []MenuDetail{}, err
	}
	menus, err := service.repository.ListMenus(ctx, true)
	if err != nil {
		return nil, err
	}
	return buildMenuTree(filterMenusByIDs(menus, menuIDs), 0), nil
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
func (service *Service) SyncMenus(ctx context.Context, items []SyncItem) (int, error) {
	created := 0
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		for _, item := range items {
			path := normalizeMenuPath(item.Path)
			if path == nil {
				continue
			}
			_, exists, err := service.repository.FindMenuByPath(transactionContext, *path)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			parentID := uint(0)
			if parentPath := strings.TrimSpace(item.ParentPath); parentPath != "" {
				parent, found, err := service.repository.FindMenuByPath(transactionContext, parentPath)
				if err != nil {
					return err
				}
				if found {
					parentID = parent.ID
				}
			}
			menuType := item.Type
			if menuType < 1 || menuType > 3 {
				menuType = 1
			}
			menu := Menu{
				ParentID: parentID, Name: strings.TrimSpace(item.Name), Path: path,
				Component: item.Component, Icon: item.Icon,
				PermissionCode: strings.TrimSpace(item.PermissionCode),
				Sort:           item.Sort, Type: menuType, Status: 1,
			}
			if err := service.repository.CreateMenu(transactionContext, &menu); err != nil {
				return err
			}
			created++
		}
		userIDs, err := service.authorization.AllUserIDs(transactionContext)
		if err != nil {
			return err
		}
		return service.authorization.IncrementAccessVersions(transactionContext, userIDs)
	})
	if err != nil {
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
func (service *Service) AssignAPIs(ctx context.Context, menuID uint, apiIDs []uint, manualCode string) error {
	apiIDs = uniqueUintIDs(apiIDs)
	if len(apiIDs) == 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	var affectedUserIDs []uint
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		menu, err := service.repository.LockMenu(transactionContext, menuID)
		if err != nil {
			return err
		}
		oldAPIIDs, err := service.repository.APIIDsByMenuIDs(transactionContext, []uint{menuID}, true)
		if err != nil {
			return err
		}
		if _, err := service.apis.LockMany(transactionContext, oldAPIIDs); err != nil {
			return err
		}
		apis, err := service.apis.LockMany(transactionContext, apiIDs)
		if err != nil {
			return err
		}
		if len(apis) != len(apiIDs) {
			return NewError(CodeNotFound, nil)
		}
		for _, api := range apis {
			if err := validateLinkableAPI(api); err != nil {
				return err
			}
		}
		permissionCode, err := resolveBindingCode(manualCode, apis)
		if err != nil {
			return err
		}
		beforeRoles, err := service.affectedRoleIDs(transactionContext, []uint{menuID}, []string{menu.PermissionCode})
		if err != nil {
			return err
		}
		if _, _, err := service.authorization.EnsurePermission(transactionContext, PermissionSeed{Code: permissionCode, Name: menu.Name, Group: inferAPIGroup(apis), Sort: menu.Sort}); err != nil {
			return err
		}
		if err := service.mergePermissionRoles(transactionContext, menu.PermissionCode, permissionCode); err != nil {
			return err
		}
		if err := service.apis.SetPermissionCode(transactionContext, apiIDs, permissionCode); err != nil {
			return NewError(CodeInternalError, err)
		}
		if err := service.repository.UpdateMenusPermissionCode(transactionContext, []uint{menuID}, permissionCode); err != nil {
			return NewError(CodeInternalError, err)
		}
		if err := service.repository.ReplaceMenuAPIs(transactionContext, menuID, apiIDs); err != nil {
			return NewError(CodeInternalError, err)
		}
		afterRoles, err := service.affectedRoleIDs(transactionContext, []uint{menuID}, []string{permissionCode})
		if err != nil {
			return err
		}
		affectedUserIDs, err = service.authorization.UserIDsByRoleIDs(transactionContext, uniqueUintIDs(append(beforeRoles, afterRoles...)))
		if err != nil {
			return err
		}
		affectedUserIDs = uniqueUintIDs(affectedUserIDs)
		if err := service.authorization.IncrementAccessVersions(transactionContext, affectedUserIDs); err != nil {
			return err
		}
		return service.cleanupPermission(transactionContext, menu.PermissionCode, permissionCode)
	})
	if err != nil {
		return err
	}
	service.postCommitInvalidate(ctx, affectedUserIDs)
	return nil
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
	_ = service.RetryCacheInvalidation(ctx, userIDs)
}
func (service *Service) ChangeAPIPermissionCode(ctx context.Context, apiID uint, requestedCode string, update func(context.Context) error) error {
	return service.transactions.Run(ctx, func(transactionContext context.Context) error {
		api, err := service.apis.Lock(transactionContext, apiID)
		if err != nil {
			return err
		}
		menuIDs, err := service.repository.MenuIDsByAPIIDs(transactionContext, []uint{apiID}, true)
		if err != nil {
			return err
		}
		for _, menuID := range menuIDs {
			if _, err := service.repository.LockMenu(transactionContext, menuID); err != nil {
				return err
			}
		}
		linkedAPIIDs, err := service.repository.APIIDsByMenuIDs(transactionContext, menuIDs, true)
		if err != nil {
			return err
		}
		if _, err := service.apis.LockMany(transactionContext, linkedAPIIDs); err != nil {
			return err
		}
		beforeRoles, err := service.affectedRoleIDs(transactionContext, menuIDs, []string{api.PermissionCode})
		if err != nil {
			return err
		}
		permissionCode := strings.TrimSpace(requestedCode)
		if permissionCode == "" && api.NeedAuth == 1 {
			permissionCode = derivePermissionCode(api.Method, api.Path)
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
		afterRoles, err := service.affectedRoleIDs(transactionContext, menuIDs, []string{permissionCode})
		if err != nil {
			return err
		}
		userIDs, err := service.authorization.UserIDsByRoleIDs(transactionContext, uniqueUintIDs(append(beforeRoles, afterRoles...)))
		if err != nil {
			return err
		}
		userIDs = uniqueUintIDs(userIDs)
		if err := service.authorization.IncrementAccessVersions(transactionContext, userIDs); err != nil {
			return err
		}
		return service.cleanupPermission(transactionContext, api.PermissionCode, permissionCode)
	})
}
func (service *Service) DeleteAPI(ctx context.Context, apiID uint, deleteAPI func(context.Context) error) error {
	var affectedUserIDs []uint
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		api, err := service.apis.Lock(transactionContext, apiID)
		if err != nil {
			return err
		}
		menuIDs, err := service.repository.MenuIDsByAPIIDs(transactionContext, []uint{apiID}, true)
		if err != nil {
			return err
		}
		for _, menuID := range menuIDs {
			if _, err := service.repository.LockMenu(transactionContext, menuID); err != nil {
				return err
			}
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
func (service *Service) GenerateMenuButton(ctx context.Context, apiID, parentID uint, name string, sort int) (MenuDetail, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MenuDetail{}, NewError(CodeValidationInvalid, nil)
	}
	var menu Menu
	err := service.transactions.Run(ctx, func(transactionContext context.Context) error {
		if _, err := service.repository.LockMenu(transactionContext, parentID); err != nil {
			return NewError(CodeNotFound, err)
		}
		api, err := service.apis.Lock(transactionContext, apiID)
		if err != nil {
			return err
		}
		if err := validateLinkableAPI(api); err != nil {
			return err
		}
		permissionCode := strings.TrimSpace(api.PermissionCode)
		if permissionCode == "" {
			permissionCode = derivePermissionCode(api.Method, api.Path)
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
		return service.authorization.IncrementAccessVersions(transactionContext, userIDs)
	})
	return menuToDetail(menu), err
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
		_, createdResult, err := service.authorization.EnsurePermission(transactionContext, PermissionSeed{Code: api.PermissionCode, Name: api.Name, Group: api.Group, Sort: api.Sort})
		created = createdResult
		return err
	})
	return created, err
}
