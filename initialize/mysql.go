package initialize

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"admin/global"
	"admin/model"

	"go.uber.org/zap"
)

// InitMysql 初始化 MySQL 连接，并执行模型自动迁移。
func InitMysql(conf *Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		conf.Mysql.User,
		conf.Mysql.Password,
		conf.Mysql.Host,
		conf.Mysql.Port,
		conf.Mysql.DB,
	)

	var err error
	global.DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		global.Logger.Fatal("open mysql failed", zap.Error(err))
	}

	if err := global.DB.AutoMigrate(model.Models...); err != nil {
		global.Logger.Fatal("auto migrate failed", zap.Error(err))
	}
}
