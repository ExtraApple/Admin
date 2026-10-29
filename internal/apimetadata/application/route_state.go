package application

import (
	"context"
	"strings"

	"admin/internal/apimetadata/domain"
)

const (
	RouteStateMatched        = "matched"
	RouteStateMetadataOnly   = "metadata_only"
	RouteStateCatalogOnly    = "catalog_only"
	RouteStateAccessMismatch = "access_mismatch"
)

func isSupportedRouteState(value string) bool {
	switch value {
	case RouteStateMatched, RouteStateMetadataOnly, RouteStateCatalogOnly, RouteStateAccessMismatch:
		return true
	default:
		return false
	}
}

type routeCatalogFactSet map[string]RouteFact

func (service *Service) projectRouteStates(ctx context.Context, apis []domain.API) ([]domain.API, error) {
	if service.routes == nil {
		return apis, nil
	}
	facts, err := service.routes.Routes(ctx)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	catalog := make(routeCatalogFactSet, len(facts))
	for _, fact := range facts {
		catalog[routeKey(fact.Method, fact.Path)] = fact
	}
	for index := range apis {
		fact, exists := catalog[routeKey(apis[index].Method, apis[index].Path)]
		if !exists {
			apis[index].RouteState = RouteStateMetadataOnly
			continue
		}
		apis[index].RouteState = routeStateForCandidate(fact, &apis[index])
	}
	return apis, nil
}

func routeStateForCandidate(fact RouteFact, api *domain.API) string {
	if api == nil {
		return RouteStateCatalogOnly
	}
	if routeAccessMatches(*api, fact) {
		return RouteStateMatched
	}
	return RouteStateAccessMismatch
}

func routeKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + normalizeRoutePath(path)
}

func normalizeRoutePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func routeAccessMatches(api domain.API, fact RouteFact) bool {
	expectedNeedAuth, expectedCode := routeAccessValues(fact)
	if api.NeedAuth != expectedNeedAuth {
		return false
	}
	return strings.TrimSpace(api.PermissionCode) == expectedCode
}

func routeCatalogStates(facts []RouteFact, apis []domain.API) []string {
	catalog := make(routeCatalogFactSet, len(facts))
	for _, fact := range facts {
		catalog[routeKey(fact.Method, fact.Path)] = fact
	}
	states := make([]string, len(apis))
	for index, api := range apis {
		fact, exists := catalog[routeKey(api.Method, api.Path)]
		if !exists {
			states[index] = RouteStateMetadataOnly
		} else if routeAccessMatches(api, fact) {
			states[index] = RouteStateMatched
		} else {
			states[index] = RouteStateAccessMismatch
		}
	}
	return states
}
