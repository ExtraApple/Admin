package database

import (
	"fmt"

	platformconfig "admin/internal/platform/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func OpenMySQL(conf platformconfig.MySQLConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		conf.User,
		conf.Password,
		conf.Host,
		conf.Port,
		conf.DB,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	return db, nil
}
