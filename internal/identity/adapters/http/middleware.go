package httpadapter

import (
	"net/http"
	"strings"

	"admin/internal/identity/application"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(service *application.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "缺少 Authorization 头"})
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Authorization 格式错误，需要 Bearer Token"})
			return
		}
		if service == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 无效或已过期"})
			return
		}
		identity, err := service.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": err.Error()})
			return
		}
		c.Set("userID", identity.UserID)
		c.Set("roles", identity.Roles)
		c.Set("permissions", identity.Permissions)
		c.Next()
	}
}
