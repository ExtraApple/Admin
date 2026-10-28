package application

import (
	"context"
	"sort"
	"strings"

	"admin/internal/apimetadata/domain"
	"admin/internal/platform/configimpact"
)

type ImpactPreviewInput struct {
	Operation      ImpactOperation
	APIID          uint
	Update         UpdateInput
	Candidate      CreateInput
	SyncFacts      []RouteFact
	ButtonParentID uint
	ButtonName     string
	ButtonSort     int
}

type apiImpactTarget struct {
	APIID          uint        `json:"api_id,omitempty"`
	Name           string      `json:"name,omitempty"`
	Method         string      `json:"method,omitempty"`
	Path           string      `json:"path,omitempty"`
	Group          string      `json:"group,omitempty"`
	PermissionCode string      `json:"permission_code,omitempty"`
	Status         *int        `json:"status,omitempty"`
	NeedAuth       *int        `json:"need_auth,omitempty"`
	NeedAudit      *int        `json:"need_audit,omitempty"`
	Routes         []routeFact `json:"routes,omitempty"`
	ParentID       uint        `json:"parent_id,omitempty"`
	ButtonName     string      `json:"button_name,omitempty"`
	ButtonSort     int         `json:"button_sort,omitempty"`
}

type routeFact struct {
	Method                string `json:"method"`
	Path                  string `json:"path"`
	Name                  string `json:"name,omitempty"`
	Group                 string `json:"group,omitempty"`
	Authenticated         bool   `json:"authenticated"`
	PermissionControlled  bool   `json:"permission_controlled"`
	DefaultPermissionCode string `json:"default_permission_code,omitempty"`
	NeedAudit             bool   `json:"need_audit"`
	RouteState            string `json:"route_state"`
}

type apiImpactFacts struct {
	APIs        []apiImpactAPIFact `json:"apis,omitempty"`
	RouteKeys   []string           `json:"route_keys,omitempty"`
	RouteStates []string           `json:"route_states,omitempty"`
}

type apiImpactAPIFact struct {
	ID             uint   `json:"id"`
	Name           string `json:"name,omitempty"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	Group          string `json:"group,omitempty"`
	Remark         string `json:"remark,omitempty"`
	Sort           int    `json:"sort"`
	PermissionCode string `json:"permission_code,omitempty"`
	Status         int    `json:"status"`
	NeedAuth       int    `json:"need_auth"`
	NeedAudit      int    `json:"need_audit"`
	Deleted        bool   `json:"deleted"`
}

func (service *Service) PreviewImpact(ctx context.Context, input ImpactPreviewInput) (ImpactPreview, error) {
	plan, err := service.buildImpactPlan(ctx, input)
	if err != nil {
		return ImpactPreview{}, err
	}
	fingerprint, err := plan.Fingerprint()
	if err != nil {
		return ImpactPreview{}, NewError(CodeInternalError, err)
	}
	return ImpactPreview{Operation: input.Operation, NormalizedTarget: plan.NormalizedTarget, Fingerprint: fingerprint, Summary: plan.Summary}, nil
}

func (service *Service) buildImpactPlan(ctx context.Context, input ImpactPreviewInput) (configimpact.Plan, error) {
	switch input.Operation {
	case ImpactUpdateAPI:
		return service.buildAPIUpdatePlan(ctx, input.APIID, input.Update)
	case ImpactDeleteAPI:
		return service.buildAPIDeletePlan(ctx, input.APIID)
	case ImpactSyncAPIs:
		return service.buildAPISyncPlan(ctx, input.SyncFacts)
	case ImpactSyncPermissions:
		return service.buildPermissionSyncPlan(ctx)
	case ImpactGenerateMenuButton:
		return service.buildButtonPlan(ctx, input)
	default:
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
}

func (service *Service) buildAPIUpdatePlan(ctx context.Context, apiID uint, input UpdateInput) (configimpact.Plan, error) {
	api, err := service.core.Get(ctx, apiID)
	if err != nil {
		return configimpact.Plan{}, err
	}
	candidate := apiImpactTarget{APIID: apiID}
	current := apiImpactAPIFactFromDomain(api)
	if input.Name != nil {
		candidate.Name = strings.TrimSpace(*input.Name)
	}
	if input.Method != nil {
		candidate.Method = strings.ToUpper(strings.TrimSpace(*input.Method))
	}
	if input.Path != nil {
		candidate.Path = normalizeRoutePath(*input.Path)
	}
	if input.Group != nil {
		candidate.Group = strings.TrimSpace(*input.Group)
	}
	if input.PermissionCode != nil {
		candidate.PermissionCode = strings.TrimSpace(*input.PermissionCode)
	}
	candidate.Status, candidate.NeedAuth, candidate.NeedAudit = input.Status, input.NeedAuth, input.NeedAudit
	facts := apiImpactFacts{APIs: []apiImpactAPIFact{current}}
	state, err := service.routeStateFacts(ctx, []domain.API{api})
	if err != nil {
		return configimpact.Plan{}, err
	}
	facts.RouteStates = state
	return configimpact.NewPlan(string(ImpactUpdateAPI), candidate, facts, configimpact.Summary{AffectedAPIIDs: []uint{apiID}, AccessVersionChange: true, RequiresConfirmation: true})
}

func (service *Service) buildAPIDeletePlan(ctx context.Context, apiID uint) (configimpact.Plan, error) {
	api, err := service.core.Get(ctx, apiID)
	if err != nil {
		return configimpact.Plan{}, err
	}
	facts := apiImpactFacts{APIs: []apiImpactAPIFact{apiImpactAPIFactFromDomain(api)}}
	state, err := service.routeStateFacts(ctx, []domain.API{api})
	if err != nil {
		return configimpact.Plan{}, err
	}
	facts.RouteStates = state
	return configimpact.NewPlan(string(ImpactDeleteAPI), apiImpactTarget{APIID: apiID}, facts, configimpact.Summary{AffectedAPIIDs: []uint{apiID}, AccessVersionChange: true, RequiresConfirmation: true})
}

func (service *Service) buildAPISyncPlan(ctx context.Context, facts []RouteFact) (configimpact.Plan, error) {
	if service.routes == nil && len(facts) == 0 {
		return configimpact.Plan{}, NewError(CodeInternalError, nil)
	}
	if len(facts) == 0 {
		var err error
		facts, err = service.routes.Routes(ctx)
		if err != nil {
			return configimpact.Plan{}, NewError(CodeInternalError, err)
		}
	}
	apis, _, err := service.core.repository.List(ctx, 0, -1, Filter{})
	if err != nil {
		return configimpact.Plan{}, NewError(CodeInternalError, err)
	}
	known := make(map[string]domain.API, len(apis))
	for _, api := range apis {
		known[routeKey(api.Method, api.Path)] = api
	}
	created, skipped, conflicts := 0, 0, 0
	routes := make([]routeFact, 0, len(facts)+len(apis))
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if !strings.HasPrefix(normalizeRoutePath(fact.Path), "/api/") {
			continue
		}
		key := routeKey(fact.Method, fact.Path)
		route := routeFactFromDomain(fact)
		if api, exists := known[key]; exists {
			route.RouteState = routeStateForCandidate(fact, &api)
			skipped++
			if route.RouteState == RouteStateAccessMismatch {
				conflicts++
			}
		} else {
			route.RouteState = RouteStateCatalogOnly
			created++
		}
		routes = append(routes, route)
		seen[key] = struct{}{}
	}
	for key, api := range known {
		if _, exists := seen[key]; exists {
			continue
		}
		routes = append(routes, routeFact{Method: strings.ToUpper(strings.TrimSpace(api.Method)), Path: normalizeRoutePath(api.Path), Name: api.Name, Group: api.Group, RouteState: RouteStateMetadataOnly})
	}
	sort.Slice(routes, func(i, j int) bool {
		return routeKey(routes[i].Method, routes[i].Path) < routeKey(routes[j].Method, routes[j].Path)
	})
	routeKeys := make(map[string]struct{}, len(known))
	for key := range known {
		routeKeys[key] = struct{}{}
	}
	return configimpact.NewPlan(string(ImpactSyncAPIs), apiImpactTarget{Routes: routes}, apiImpactFacts{RouteKeys: sortedRouteKeys(routeKeys)}, configimpact.Summary{CreatedCount: created, SkippedCount: skipped, ConflictCount: conflicts, AccessVersionChange: created > 0, RequiresConfirmation: created > 0 || conflicts > 0})
}

func (service *Service) buildPermissionSyncPlan(ctx context.Context) (configimpact.Plan, error) {
	apis, _, err := service.core.repository.List(ctx, 0, -1, Filter{})
	if err != nil {
		return configimpact.Plan{}, NewError(CodeInternalError, err)
	}
	ids := make([]uint, 0, len(apis))
	for _, api := range apis {
		if api.NeedAuth == 1 {
			ids = append(ids, api.ID)
		}
	}
	return configimpact.NewPlan(string(ImpactSyncPermissions), apiImpactTarget{}, apiImpactFacts{APIs: apiImpactAPIFacts(apis)}, configimpact.Summary{AffectedAPIIDs: ids, AccessVersionChange: len(ids) > 0, RequiresConfirmation: len(ids) > 0})
}

func (service *Service) buildButtonPlan(ctx context.Context, input ImpactPreviewInput) (configimpact.Plan, error) {
	if service.coordinator == nil {
		return configimpact.Plan{}, NewError(CodeInternalError, nil)
	}
	api, err := service.core.Get(ctx, input.APIID)
	if err != nil {
		return configimpact.Plan{}, err
	}
	if api.Status != 1 || api.NeedAuth != 1 {
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
	name := strings.TrimSpace(input.ButtonName)
	if name == "" || input.ButtonParentID == 0 {
		return configimpact.Plan{}, NewError(CodeValidationInvalid, nil)
	}
	if err := service.coordinator.ValidateMenuButtonParent(ctx, input.ButtonParentID); err != nil {
		return configimpact.Plan{}, err
	}
	return configimpact.NewPlan(string(ImpactGenerateMenuButton), apiImpactTarget{APIID: input.APIID, ParentID: input.ButtonParentID, ButtonName: name, ButtonSort: input.ButtonSort}, apiImpactFacts{APIs: []apiImpactAPIFact{apiImpactAPIFactFromDomain(api)}}, configimpact.Summary{AffectedAPIIDs: []uint{input.APIID}, AccessVersionChange: true, RequiresConfirmation: true})
}

func (service *Service) routeStateFacts(ctx context.Context, apis []domain.API) ([]string, error) {
	if service.routes == nil {
		return make([]string, len(apis)), nil
	}
	facts, err := service.routes.Routes(ctx)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	return routeCatalogStates(facts, apis), nil
}

func apiImpactAPIFactFromDomain(api domain.API) apiImpactAPIFact {
	return apiImpactAPIFact{ID: api.ID, Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group, Remark: api.Remark, Sort: api.Sort, PermissionCode: api.PermissionCode, Status: api.Status, NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit, Deleted: api.Deleted}
}

func apiImpactAPIFacts(apis []domain.API) []apiImpactAPIFact {
	facts := make([]apiImpactAPIFact, 0, len(apis))
	for _, api := range apis {
		facts = append(facts, apiImpactAPIFactFromDomain(api))
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	return facts
}

func routeFactFromDomain(fact RouteFact) routeFact {
	return routeFact{Method: strings.ToUpper(strings.TrimSpace(fact.Method)), Path: normalizeRoutePath(fact.Path), Name: strings.TrimSpace(fact.Name), Group: strings.TrimSpace(fact.Group), Authenticated: fact.Authenticated, PermissionControlled: fact.PermissionControlled, DefaultPermissionCode: strings.TrimSpace(fact.DefaultPermissionCode), NeedAudit: fact.NeedAudit}
}

func sortedRouteKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
