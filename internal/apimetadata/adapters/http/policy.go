package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"

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
			if errors.Is(err, application.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "API未配置权限"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "API权限校验失败"})
			return
		}
		if policy.Status != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "API已禁用"})
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
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "API未绑定权限码"})
			return
		}
		if !contextContains(c, "permissions", permissionCode) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无操作权限"})
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
