package router

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"admin/handler"
	"admin/middleware"
	"admin/service"
)

func InitRouter(jwtCfg service.JWTConfig) *gin.Engine {
	r := gin.Default()

	// ========== 全局中间件 ==========
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// ========== 依赖注入 ==========
	userHandler      := &handler.UserHandler{JwtCfg: jwtCfg}
	captchaHandler   := &handler.CaptchaHandler{}
	adminUserHandler := &handler.AdminUserHandler{}
	auth             := middleware.JWTAuth(jwtCfg.Secret)
	adminOnly        := middleware.AdminOnly()

	// ========== 路由 ==========
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "pong"})
	})

	api := r.Group("/api")
	{
		// --- 验证码 ---
		api.GET("/captcha", captchaHandler.Generate)

		// --- 公开路由 ---
		api.POST("/register", userHandler.Register)
		api.POST("/login", userHandler.Login)

		// --- 需认证路由 ---
		user := api.Group("/user").Use(auth)
		{
			user.GET("/info", userHandler.GetInfo)
			user.PUT("/info", userHandler.UpdateSelf)
			user.PUT("/password", userHandler.ChangePassword)
		}

		// --- 管理员路由 ---
		admin := api.Group("/admin").Use(auth, adminOnly)
		{
			admin.GET("/users", adminUserHandler.ListUsers)
			admin.PUT("/users/:id", adminUserHandler.UpdateUser)
			admin.DELETE("/users/:id", adminUserHandler.DeleteUser)
			admin.PUT("/users/:id/status", adminUserHandler.ToggleStatus)
		}
	}

	return r
}
