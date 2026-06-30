package middleware

import (
	"errors"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"admin/global"
	"admin/model"

	"github.com/gin-gonic/gin"
)

// APIPermission 根据 apis 表中的接口元数据动态校验访问权限。
func APIPermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := strings.ToUpper(c.Request.Method)
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		if isAPIPermissionBootstrapRoute(method, path) && hasContextRole(c, "admin") {
			c.Next()
			return
		}

		var api model.API
		err := global.DB.Where("method = ? AND path = ?", method, path).First(&api).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "API未配置权限"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "API权限校验失败"})
			return
		}

		if api.Status != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "API已禁用"})
			return
		}

		if api.NeedAuth != 1 {
			c.Next()
			return
		}

		if hasContextRole(c, "admin") {
			c.Next()
			return
		}

		permissionCode := strings.TrimSpace(api.PermissionCode)
		if permissionCode == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "API未绑定权限码"})
			return
		}

		if !hasContextPermission(c, permissionCode) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无操作权限"})
			return
		}

		c.Next()
	}
}

func isAPIPermissionBootstrapRoute(method, path string) bool {
	return method == http.MethodPost && (path == "/api/admin/apis/sync" || path == "/api/admin/apis/sync-permissions")
}

func hasContextRole(c *gin.Context, allowed string) bool {
	roles, _ := c.Get("roles")
	roleList, ok := roles.([]string)
	if !ok {
		return false
	}

	for _, role := range roleList {
		if role == allowed {
			return true
		}
	}
	return false
}

func hasContextPermission(c *gin.Context, allowed string) bool {
	permissions, _ := c.Get("permissions")
	permissionList, ok := permissions.([]string)
	if !ok {
		return false
	}

	for _, permission := range permissionList {
		if permission == allowed {
			return true
		}
	}
	return false
}
