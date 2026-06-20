package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HasRole 校验当前用户是否拥有指定角色之一
func HasRole(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, _ := c.Get("roles")

		roleList, ok := roles.([]string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无操作权限"})
			return
		}

		for _, r := range roleList {
			for _, a := range allowed {
				if r == a {
					c.Next()
					return
				}
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无操作权限"})
	}
}
