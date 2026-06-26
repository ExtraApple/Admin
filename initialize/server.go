package initialize


import (
	"os"
	"fmt"
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
}

var conf Config

func InitConfig() *Config{
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		panic(fmt.Sprintf("read config file failed: %v", err))
	}
     if err := yaml.Unmarshal(data, &conf); err != nil {
		panic(fmt.Sprintf("unmarshal config file failed: %v", err))
	}
	return &conf
}
