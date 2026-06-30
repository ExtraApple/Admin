package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"admin/global"
	"admin/service"
	"admin/utils"
)

// JWTAuth 返回一个 Gin 中间件，验证请求头中的 Bearer Token
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "缺少 Authorization 头"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Authorization 格式错误，需要 Bearer Token"})
			return
		}

		tokenStr := parts[1]
		claims, err := utils.ParseToken(tokenStr, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 无效或已过期"})
			return
		}

		// 检查黑名单（已登出的 token）
		exist, _ := global.Redis.Exists(context.Background(), "blacklist:"+tokenStr).Result()
		if exist > 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 已失效"})
			return
		}
		if err := service.IsTokenVersionValid(claims.UserID, claims.TokenVersion); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": err.Error()})
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("roles", claims.Roles)
		c.Set("permissions", claims.Permissions)
		c.Next()
	}
}
