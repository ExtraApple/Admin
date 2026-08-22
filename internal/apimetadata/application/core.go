package application

import (
	"context"
	"net/http"
	"strings"

	"admin/internal/apimetadata/domain"
)

var supportedMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodPost: {}, http.MethodPut: {},
	http.MethodPatch: {}, http.MethodDelete: {}, http.MethodOptions: {}, http.MethodHead: {},
}

type Core struct{ repository Repository }

func NewCore(repository Repository) *Core { return &Core{repository: repository} }

func (core *Core) Create(ctx context.Context, input CreateInput) (domain.API, error) {
	method, err := NormalizeMethod(input.Method)
	if err != nil {
		return domain.API{}, err
	}
	path, err := NormalizePath(input.Path)
	if err != nil {
		return domain.API{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.API{}, NewError(CodeValidationInvalid, nil)
	}
	exists, err := core.repository.ExistsMethodPath(ctx, 0, method, path)
	if err != nil {
		return domain.API{}, wrapError(err)
	}
	if exists {
		return domain.API{}, NewError(CodeConflict, nil)
	}
	status, needAuth, needAudit := 1, 1, 1
	if input.Status != nil {
		status = *input.Status
	}
	if input.NeedAuth != nil {
		needAuth = *input.NeedAuth
	}
	if input.NeedAudit != nil {
		needAudit = *input.NeedAudit
	}
	group := strings.TrimSpace(input.Group)
	if group == "" {
		group = InferGroup(path)
	}
	api := domain.API{
		Name: name, Method: method, Path: path, Group: group,
		PermissionCode: strings.TrimSpace(input.PermissionCode),
		Remark:         strings.TrimSpace(input.Remark), Sort: input.Sort,
		Status: status, NeedAuth: needAuth, NeedAudit: needAudit,
	}
	if err := core.repository.Create(ctx, &api); err != nil {
		return domain.API{}, NewError(CodeInternalError, err)
	}
	return api, nil
}
func (core *Core) List(ctx context.Context, page, size int, filter Filter) ([]domain.API, int64, error) {
	page, size = normalizePage(page, size)
	if filter.Method != "" {
		method, err := NormalizeMethod(filter.Method)
		if err != nil {
			return nil, 0, err
		}
		filter.Method = method
	}
	return core.repository.List(ctx, (page-1)*size, size, filter)
}
func (core *Core) Snapshot(ctx context.Context) ([]domain.API, error) {
	apis, _, err := core.repository.List(ctx, 0, -1, Filter{})
	return apis, err
}
func (core *Core) ListByIDs(ctx context.Context, ids []uint) ([]domain.API, error) {
	return core.repository.ListByIDs(ctx, ids)
}

func (core *Core) Lock(ctx context.Context, id uint) (domain.API, error) {
	apis, err := core.repository.ListByIDsForUpdate(ctx, []uint{id})
	if err != nil {
		return domain.API{}, err
	}
	if len(apis) == 0 {
		return domain.API{}, ErrNotFound
	}
	return apis[0], nil
}

func (core *Core) LockMany(ctx context.Context, ids []uint) ([]domain.API, error) {
	return core.repository.ListByIDsForUpdate(ctx, ids)
}

func (core *Core) SetPermissionCode(ctx context.Context, ids []uint, code string) error {
	return core.repository.SetPermissionCode(ctx, ids, code)
}

func (core *Core) CountPermissionCode(ctx context.Context, code string) (int64, error) {
	return core.repository.CountPermissionCode(ctx, code)
}

func (core *Core) Get(ctx context.Context, id uint) (domain.API, error) {
	api, err := core.repository.Find(ctx, id)
	if err != nil {
		return domain.API{}, wrapError(err)
	}
	return api, nil
}

func (core *Core) Update(ctx context.Context, id uint, input UpdateInput) (domain.API, error) {
	current, err := core.repository.Find(ctx, id)
	if err != nil {
		return domain.API{}, wrapError(err)
	}
	targetMethod, targetPath := current.Method, current.Path
	if input.Method != nil {
		targetMethod, err = NormalizeMethod(*input.Method)
		if err != nil {
			return domain.API{}, err
		}
	}
	if input.Path != nil {
		targetPath, err = NormalizePath(*input.Path)
		if err != nil {
			return domain.API{}, err
		}
	}
	if targetMethod != current.Method || targetPath != current.Path {
		exists, err := core.repository.ExistsMethodPath(ctx, id, targetMethod, targetPath)
		if err != nil {
			return domain.API{}, wrapError(err)
		}
		if exists {
			return domain.API{}, NewError(CodeConflict, nil)
		}
	}
	updates := make(map[string]any)
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return domain.API{}, NewError(CodeValidationInvalid, nil)
		}
		updates["name"] = name
	}
	if input.Method != nil {
		updates["method"] = targetMethod
	}
	if input.Path != nil {
		updates["path"] = targetPath
	}
	if input.Group != nil {
		updates["api_group"] = strings.TrimSpace(*input.Group)
	}
	if input.PermissionCode != nil {
		updates["permission_code"] = strings.TrimSpace(*input.PermissionCode)
	}
	if input.Remark != nil {
		updates["remark"] = strings.TrimSpace(*input.Remark)
	}
	if input.Sort != nil {
		updates["sort"] = *input.Sort
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}
	if input.NeedAuth != nil {
		updates["need_auth"] = *input.NeedAuth
	}
	if input.NeedAudit != nil {
		updates["need_audit"] = *input.NeedAudit
	}
	if len(updates) == 0 {
		return domain.API{}, NewError(CodeValidationInvalid, nil)
	}
	if err := core.repository.Update(ctx, id, updates); err != nil {
		return domain.API{}, NewError(CodeInternalError, err)
	}
	updated, err := core.repository.Find(ctx, id)
	if err != nil {
		return domain.API{}, wrapError(err)
	}
	return updated, nil
}

func (core *Core) Delete(ctx context.Context, id uint) error {
	if _, err := core.repository.Find(ctx, id); err != nil {
		return wrapError(err)
	}
	if err := core.repository.Delete(ctx, id); err != nil {
		return wrapError(err)
	}
	return nil
}

func (core *Core) Groups(ctx context.Context) ([]domain.GroupOption, error) {
	return core.repository.Groups(ctx)
}

func (core *Core) Methods() []MethodOption {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead}
	result := make([]MethodOption, len(methods))
	for index, method := range methods {
		result[index] = MethodOption{Label: method, Value: method}
	}
	return result
}

func (core *Core) Policy(ctx context.Context, method, path string) (domain.Policy, error) {
	normalizedMethod, err := NormalizeMethod(method)
	if err != nil {
		return domain.Policy{}, err
	}
	normalizedPath, err := NormalizePath(path)
	if err != nil {
		return domain.Policy{}, err
	}
	return core.repository.FindPolicyByRoute(ctx, normalizedMethod, normalizedPath)
}

func NormalizeMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "", NewError(CodeValidationInvalid, nil)
	}
	if _, supported := supportedMethods[method]; !supported {
		return "", NewError(CodeValidationInvalid, nil)
	}
	return method, nil
}

func NormalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", NewError(CodeValidationInvalid, nil)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path, nil
}

func InferGroup(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/api/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "api"
	}
	segment := parts[0]
	if segment == "admin" && len(parts) > 1 {
		segment = parts[1]
	}
	switch segment {
	case "login", "register", "captcha":
		return "auth"
	case "user", "users":
		return "user"
	case "roles":
		return "role"
	case "permissions", "permission-groups", "permission-codes":
		return "permission"
	case "menus":
		return "menu"
	case "files", "files-browse":
		return "file"
	case "audit-logs", "login-logs", "operation-logs", "permission-logs", "data-access-logs":
		return "audit"
	case "dicts", "dict-types", "dict-items":
		return "dict"
	case "organizations":
		return "organization"
	case "apis", "api-groups", "api-methods":
		return "api"
	default:
		return segment
	}
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
