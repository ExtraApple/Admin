package files

import (
	"admin/internal/files/adapters/http"
	"admin/internal/files/application"
	"admin/internal/routecatalog"
)

// Routes exposes the complete Files HTTP descriptor collection without
// registering it in an application root.
func Routes(service *application.Service, maxUploadBytes int64) []routecatalog.Descriptor {
	return httpadapter.Routes(service, maxUploadBytes)
}
