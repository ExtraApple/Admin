package main

import (
	"fmt"
	"time"

	"admin/global"
	"admin/initialize"
	"admin/router"
	"admin/service"

	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	conf := initialize.InitConfig()
	initialize.InitLogger(conf)
	defer global.Logger.Sync()

	global.Logger.Info("config loaded")

	// 2. 初始化 MySQL + 自动迁移
	initialize.InitMysql(conf)
	global.Logger.Info("mysql initialized")

	// 3. 初始化 Redis
	initialize.InitRedis(conf)
	global.Logger.Info("redis initialized")

	// 3.5 初始化 MinIO
	initialize.InitMinio(conf)
	global.Logger.Info("minio initialized")

	// 3.6 启动文件轮转定时任务
	if conf.FileRotation.Enabled {
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				service.RotateFiles(conf)
			}
		}()
		global.Logger.Info("file rotation started")
	}

	// 3.7 启动审计日志冷热归档定时任务
	if conf.AuditLogArchive.Enabled {
		go func() {
			service.ArchiveAuditLogs(conf)
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				service.ArchiveAuditLogs(conf)
			}
		}()
		global.Logger.Info("audit log archive started")
	}

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
	global.Logger.Info("server started", zap.String("addr", "http://localhost"+addr))
	if err := r.Run(addr); err != nil {
		global.Logger.Fatal("server start failed", zap.Error(err))
	}
}
