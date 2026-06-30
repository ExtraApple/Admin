package initialize

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port int
	}
	Mysql struct {
		User     string
		Password string
		Host     string
		Port     int
		DB       string
	}
	Minio struct {
		Host     string
		Port     int
		Username string
		Password string
	}
	Jwt struct {
		Secret        string
		Expire        int
		RefreshExpire int
	}
	Redis struct {
		Host     string
		Port     int
		Password string
		DB       int
	}
	FileRotation struct {
		Enabled    bool   `yaml:"enabled"`
		Days       int    `yaml:"days"`
		HotBucket  string `yaml:"hot_bucket"`
		ColdBucket string `yaml:"cold_bucket"`
		BatchSize  int    `yaml:"batch_size"`
	} `yaml:"file_rotation"`
	Logger          LoggerConfig `yaml:"logger"`
	AuditLogArchive struct {
		Enabled       bool `yaml:"enabled"`
		RetentionDays int  `yaml:"retention_days"`
		BatchSize     int  `yaml:"batch_size"`
	} `yaml:"audit_log_archive"`
}

type LoggerConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	Output     string `yaml:"output"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
	Compress   bool   `yaml:"compress"`
}

var conf Config

// InitConfig 读取 config.yaml 并解析为全局配置结构。
func InitConfig() *Config {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		panic(fmt.Sprintf("read config file failed: %v", err))
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		panic(fmt.Sprintf("unmarshal config file failed: %v", err))
	}
	return &conf
}
