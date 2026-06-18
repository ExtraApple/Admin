package main

import (
	"fmt"

	"admin/initialize"
	"admin/router"
	"admin/service"
)

func main() {
	// 1. 加载配置
	conf := initialize.InitConfig()
	fmt.Println("Config loaded")

	// 2. 初始化 MySQL + 自动迁移
	initialize.InitMysql(conf)
	fmt.Println("MySQL initialized")

	// 3. 初始化 Redis
	initialize.InitRedis(conf)
	fmt.Println("Redis initialized")

	// 4. JWT 配置
	jwtCfg := service.JWTConfig{
		Secret:            conf.Jwt.Secret,
		ExpireMins:        conf.Jwt.Expire,
		RefreshExpireMins: conf.Jwt.RefreshExpire,
	}

	// 5. 初始化 Gin 路由
	r := router.InitRouter(jwtCfg)

	// 6. 启动服务
	addr := fmt.Sprintf(":%d", conf.Server.Port)
	fmt.Printf("Server running at http://localhost%s\n", addr)
	if err := r.Run(addr); err != nil {
		panic(fmt.Sprintf("Server start failed: %v", err))
	}
}
