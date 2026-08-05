package app

import (
	"net/http"
	"strings"

	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type TechnicalHTTP struct {
	APIDocsEnabled bool
	Docs           gin.HandlerFunc
	OpenAPI        gin.HandlerFunc
}

func RegisterTechnicalHTTP(engine *gin.Engine, technical TechnicalHTTP) {
	engine.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"msg": "pong"})
	})
	if technical.APIDocsEnabled {
		engine.GET("/docs", technical.Docs)
		engine.GET("/docs/openapi.json", technical.OpenAPI)
	}
}

type HTTPMiddleware struct {
	API                  gin.HandlerFunc
	Authenticated        gin.HandlerFunc
	PermissionControlled gin.HandlerFunc
}

func RegisterHTTP(engine *gin.Engine, catalog *routecatalog.Catalog, middleware HTTPMiddleware) {
	for _, descriptor := range catalog.Snapshot() {
		handlers := make([]gin.HandlerFunc, 0, 4)
		if middleware.API != nil && strings.HasPrefix(descriptor.Path, "/api/") {
			handlers = append(handlers, middleware.API)
		}
		switch descriptor.Access {
		case routecatalog.Authenticated:
			if middleware.Authenticated != nil {
				handlers = append(handlers, middleware.Authenticated)
			}
		case routecatalog.PermissionControlled:
			if middleware.Authenticated != nil {
				handlers = append(handlers, middleware.Authenticated)
			}
			if middleware.PermissionControlled != nil {
				handlers = append(handlers, middleware.PermissionControlled)
			}
		}
		handlers = append(handlers, descriptor.Handler)
		engine.Handle(descriptor.Method, descriptor.Path, handlers...)
	}
}
