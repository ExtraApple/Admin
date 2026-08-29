package app

import (
	"net/http"
	"strings"
	"time"

	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type TechnicalHTTP struct {
	APIDocsEnabled bool
	Docs           gin.HandlerFunc
	OpenAPI        gin.HandlerFunc
}

func RegisterTechnicalHTTP(engine *gin.Engine, technical TechnicalHTTP) {
	engine.GET("/ping", func(c *gin.Context) {
		httpresponse.WriteSuccess(c, http.StatusOK, map[string]string{"msg": "pong"})
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
	engine.HandleMethodNotAllowed = true
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
	engine.NoRoute(writeHTTPNotFound)
	engine.NoMethod(writeHTTPMethodNotAllowed)
}

func writeHTTPNotFound(c *gin.Context) {
	httpresponse.WriteError(c, httpresponse.NotFoundDefinition(), nil, nil)
}

func writeHTTPMethodNotAllowed(c *gin.Context) {
	httpresponse.WriteError(c, httpresponse.MethodNotAllowedDefinition(), nil, nil)
}

func requestLoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		fields := []zap.Field{
			zap.String("method", c.Request.Method), zap.String("path", path), zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(started)), zap.String("client_ip", c.ClientIP()), zap.String("user_agent", c.Request.UserAgent()),
		}
		if c.Writer.Status() >= 500 {
			errorCode := httpresponse.InternalErrorDefinition().Code
			if errorContext, ok := httpresponse.ErrorContextOf(c); ok && errorContext.Definition.Code != "" {
				errorCode = errorContext.Definition.Code
			}
			logger.Error("http request failed", append(fields, zap.String("error_code", errorCode))...)
			return
		}
		logger.Info("http request", fields...)
	}
}

func recoveryMiddleware(logger *zap.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("http panic recovered", zap.String("error_code", httpresponse.InternalErrorDefinition().Code), zap.String("method", c.Request.Method), zap.String("path", c.Request.URL.Path))
				if !c.Writer.Written() {
					httpresponse.WriteError(c, httpresponse.InternalErrorDefinition(), nil, nil)
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
