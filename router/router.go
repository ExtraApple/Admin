package router

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"admin/handler"
	"admin/middleware"
	"admin/service"
)

// InitRouter 注册全局中间件、公开接口、用户接口和管理员接口。
func InitRouter(jwtCfg service.JWTConfig) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), middleware.ZapLogger())

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
	userHandler := &handler.UserHandler{JwtCfg: jwtCfg}
	captchaHandler := &handler.CaptchaHandler{}
	adminUserHandler := &handler.AdminUserHandler{}
	roleHandler := &handler.RoleHandler{}
	permHandler := &handler.PermissionHandler{Engine: r}
	fileHandler := &handler.FileHandler{}
	menuHandler := &handler.MenuHandler{}
	auditLogHandler := &handler.AuditLogHandler{}
	dictHandler := &handler.DictHandler{}
	organizationHandler := &handler.OrganizationHandler{}
	apiHandler := &handler.APIHandler{Engine: r}
	auth := middleware.JWTAuth(jwtCfg.Secret)
	apiPermission := middleware.APIPermission()

	// ========== 路由 ==========
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "pong"})
	})

	api := r.Group("/api")
	api.Use(middleware.AuditLog())
	{
		// --- 验证码 ---
		api.GET("/captcha", captchaHandler.Generate)

		// --- 公开路由 ---
		api.POST("/register", userHandler.Register)
		api.POST("/login", userHandler.Login)
		api.GET("/dicts/:type_code/items", dictHandler.ListEnabledItemsByTypeCode)

		// --- 需认证路由 ---
		user := api.Group("/user").Use(auth)
		{
			user.GET("/context", userHandler.InitialContext)
			user.GET("/info", userHandler.GetInfo)
			user.PUT("/info", userHandler.UpdateSelf)
			user.PUT("/password", userHandler.ChangePassword)
			user.POST("/avatar", userHandler.UploadAvatar)
			user.POST("/logout", userHandler.Logout)
		}

		// --- 管理员路由 ---
		admin := api.Group("/admin").Use(auth, apiPermission)
		{
			admin.GET("/users", adminUserHandler.ListUsers)
			admin.PUT("/users/:id", adminUserHandler.UpdateUser)
			admin.DELETE("/users/:id", adminUserHandler.DeleteUser)
			admin.PUT("/users/:id/status", adminUserHandler.ToggleStatus)
			admin.PUT("/users/:id/kick", adminUserHandler.KickUser)

			// 角色管理
			admin.GET("/roles", roleHandler.ListRoles)
			admin.POST("/roles", roleHandler.CreateRole)
			admin.PUT("/roles/:id", roleHandler.UpdateRole)
			admin.DELETE("/roles/:id", roleHandler.DeleteRole)
			admin.POST("/roles/:id/users", roleHandler.AssignUsers)
			admin.GET("/roles/:id/users", roleHandler.GetRoleUsers)
			admin.POST("/roles/:id/data-scope", roleHandler.AssignDataScope)
			admin.GET("/roles/:id/data-scope", roleHandler.GetDataScope)

			// 权限管理
			admin.GET("/permissions", permHandler.ListPermissions)
			admin.POST("/permissions", permHandler.CreatePermission)
			admin.PUT("/permissions/:id", permHandler.UpdatePermission)
			admin.DELETE("/permissions/:id", permHandler.DeletePermission)
			admin.GET("/permission-codes", permHandler.GetPermissionCodes)
			admin.POST("/permissions/sync", permHandler.SyncPermissions)
			admin.POST("/roles/:id/permissions", permHandler.AssignPermissions)
			admin.GET("/roles/:id/permissions", permHandler.GetRolePermissions)

			// 权限分组管理
			admin.GET("/permission-groups", permHandler.ListPermGroups)
			admin.POST("/permission-groups", permHandler.CreatePermGroup)
			admin.PUT("/permission-groups/:id", permHandler.UpdatePermGroup)
			admin.DELETE("/permission-groups/:id", permHandler.DeletePermGroup)

			// 文件管理
			admin.POST("/files", fileHandler.Upload)
			admin.GET("/files", fileHandler.ListFiles)
			admin.GET("/files/:id", fileHandler.GetFile)
			admin.PUT("/files/:id", fileHandler.UpdateFile)
			admin.DELETE("/files/:id", fileHandler.DeleteFile)
			admin.GET("/files-browse", fileHandler.BrowseFiles)

			admin.GET("/menus", menuHandler.ListMenus)
			admin.POST("/menus", menuHandler.CreateMenu)
			admin.PUT("/menus/:id", menuHandler.UpdateMenu)
			admin.DELETE("/menus/:id", menuHandler.DeleteMenu)
			admin.POST("/menus/sync", menuHandler.SyncMenus)
			admin.POST("/menus/:id/apis", menuHandler.AssignAPIs)
			admin.GET("/menus/:id/apis", menuHandler.GetAPIs)
			admin.POST("/roles/:id/menus", menuHandler.AssignRoleMenus)
			admin.GET("/roles/:id/menus", menuHandler.GetRoleMenus)

			// 审计日志
			admin.GET("/audit-logs", auditLogHandler.ListAuditLogs)
			admin.GET("/login-logs", auditLogHandler.ListLoginLogs)
			admin.GET("/operation-logs", auditLogHandler.ListOperationLogs)
			admin.GET("/permission-logs", auditLogHandler.ListPermissionLogs)
			admin.GET("/data-access-logs", auditLogHandler.ListDataAccessLogs)

			// 字典管理
			admin.GET("/dict-types", dictHandler.ListDictTypes)
			admin.POST("/dict-types", dictHandler.CreateDictType)
			admin.PUT("/dict-types/:id", dictHandler.UpdateDictType)
			admin.DELETE("/dict-types/:id", dictHandler.DeleteDictType)
			admin.GET("/dict-items", dictHandler.ListDictItems)
			admin.POST("/dict-items", dictHandler.CreateDictItem)
			admin.PUT("/dict-items/:id", dictHandler.UpdateDictItem)
			admin.DELETE("/dict-items/:id", dictHandler.DeleteDictItem)

			// 组织管理
			admin.GET("/organizations", organizationHandler.ListOrganizations)
			admin.GET("/organizations/tree", organizationHandler.GetOrganizationTree)
			admin.POST("/organizations", organizationHandler.CreateOrganization)
			admin.PUT("/organizations/:id", organizationHandler.UpdateOrganization)
			admin.DELETE("/organizations/:id", organizationHandler.DeleteOrganization)
			admin.POST("/organizations/:id/users", organizationHandler.AssignUsers)
			admin.GET("/organizations/:id/users", organizationHandler.GetUsers)

			// API管理
			admin.GET("/apis", apiHandler.ListAPIs)
			admin.POST("/apis", apiHandler.CreateAPI)
			admin.PUT("/apis/:id", apiHandler.UpdateAPI)
			admin.DELETE("/apis/:id", apiHandler.DeleteAPI)
			admin.POST("/apis/:id/menu-button", apiHandler.GenerateMenuButton)
			admin.POST("/apis/sync", apiHandler.SyncAPIs)
			admin.POST("/apis/sync-permissions", apiHandler.SyncAPIPermissions)
		}
	}

	return r
}
