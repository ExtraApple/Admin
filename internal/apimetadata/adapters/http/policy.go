package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"
	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

type PolicyReader interface {
	Policy(context.Context, string, string) (domain.Policy, error)
}

func PermissionMiddleware(policies PolicyReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := strings.ToUpper(c.Request.Method)
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		policy, err := policies.Policy(c.Request.Context(), method, path)
		if err != nil {
			if errors.Is(err, application.ErrNotFound) && isBootstrapRoute(method, path) && contextContains(c, "roles", "admin") {
				c.Next()
				return
			}
			c.Abort()
			if errors.Is(err, application.ErrNotFound) {
				httpresponse.WriteError(c, apiMetaPermissionNotConfigured(), err, nil)
			} else {
				httpresponse.WriteError(c, apiMetaInternal(), err, nil)
			}
			return
		}
		if policy.Status != 1 {
			c.Abort()
			httpresponse.WriteError(c, apiMetaDisabled(), nil, nil)
			return
		}
		if isBootstrapRoute(method, path) && contextContains(c, "roles", "admin") {
			c.Next()
			return
		}
		if policy.NeedAuth == 0 || contextContains(c, "roles", "admin") {
			c.Next()
			return
		}
		permissionCode := strings.TrimSpace(policy.PermissionCode)
		if permissionCode == "" {
			c.Abort()
			httpresponse.WriteError(c, apiMetaPermissionMissing(), nil, nil)
			return
		}
		if !contextContains(c, "permissions", permissionCode) {
			c.Abort()
			httpresponse.WriteError(c, apiMetaPermissionDenied(), nil, nil)
			return
		}
		c.Next()
	}
}

func isBootstrapRoute(method, path string) bool {
	return method == http.MethodPost && (path == "/api/admin/apis/sync" || path == "/api/admin/apis/sync-permissions")
}

func contextContains(c *gin.Context, key, expected string) bool {
	values, _ := c.Get(key)
	list, ok := values.([]string)
	if !ok {
		return false
	}
	for _, value := range list {
		if value == expected {
			return true
		}
	}
	return false
}

var _ PolicyReader = (*application.Core)(nil)
