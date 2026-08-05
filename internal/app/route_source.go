package app

import (
	"context"
	"sync"

	apiapplication "admin/internal/apimetadata/application"
	"admin/internal/routecatalog"
)

type catalogRouteSource struct {
	mutex       sync.RWMutex
	descriptors []routecatalog.Descriptor
}

func (source *catalogRouteSource) Set(descriptors []routecatalog.Descriptor) {
	source.mutex.Lock()
	source.descriptors = append([]routecatalog.Descriptor(nil), descriptors...)
	source.mutex.Unlock()
}

func (source *catalogRouteSource) Routes(_ context.Context) ([]apiapplication.RouteFact, error) {
	source.mutex.RLock()
	facts := make([]apiapplication.RouteFact, len(source.descriptors))
	for index, descriptor := range source.descriptors {
		facts[index] = apiapplication.RouteFact{
			Method:                descriptor.Method,
			Path:                  descriptor.Path,
			Name:                  descriptor.Name,
			Group:                 descriptor.Group,
			Authenticated:         descriptor.Access == routecatalog.Authenticated,
			PermissionControlled:  descriptor.Access == routecatalog.PermissionControlled,
			DefaultPermissionCode: descriptor.DefaultPermissionCode,
			NeedAudit:             descriptor.DefaultAuditCategory != "",
		}
	}
	source.mutex.RUnlock()
	return facts, nil
}

var _ apiapplication.RouteSource = (*catalogRouteSource)(nil)
