package initialize

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port int `yaml:"port"`
	}
	Mysql struct {
		User        string `yaml:"user"`
		Password    string `yaml:"password"`
		PasswordEnv string `yaml:"password_env"`
		Host        string `yaml:"host"`
		Port        int    `yaml:"port"`
		DB          string `yaml:"db"`
	} `yaml:"mysql"`
	Minio struct {
		Host        string `yaml:"host"`
		Port        int    `yaml:"port"`
		Username    string `yaml:"username"`
		UsernameEnv string `yaml:"username_env"`
		Password    string `yaml:"password"`
		PasswordEnv string `yaml:"password_env"`
	} `yaml:"minio"`
	Jwt struct {
		Secret        string `yaml:"secret"`
		SecretEnv     string `yaml:"secret_env"`
		Expire        int    `yaml:"expire"`
		RefreshExpire int    `yaml:"refresh_expire"`
	} `yaml:"jwt"`
	Redis struct {
		Host        string `yaml:"host"`
		Port        int    `yaml:"port"`
		Password    string `yaml:"password"`
		PasswordEnv string `yaml:"password_env"`
		DB          int    `yaml:"db"`
	} `yaml:"redis"`
	Admin struct {
		Username    string `yaml:"username"`
		UsernameEnv string `yaml:"username_env"`
		Password    string `yaml:"password"`
		PasswordEnv string `yaml:"password_env"`
		Email       string `yaml:"email"`
		EmailEnv    string `yaml:"email_env"`
		Nickname    string `yaml:"nickname"`
		NicknameEnv string `yaml:"nickname_env"`
	} `yaml:"admin"`
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
	loadDotEnv()

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		panic(fmt.Sprintf("read config file failed: %v", err))
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		panic(fmt.Sprintf("unmarshal config file failed: %v", err))
	}
	if err := applyEnvConfig(&conf); err != nil {
		panic(fmt.Sprintf("load env config failed: %v", err))
	}
	return &conf
}

func loadDotEnv() {
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(".env"); err != nil {
			panic(fmt.Sprintf("load .env file failed: %v", err))
		}
	}
}

func applyEnvConfig(conf *Config) error {
	var err error

	if conf.Mysql.Password, err = envValue(conf.Mysql.PasswordEnv, conf.Mysql.Password, false); err != nil {
		return fmt.Errorf("mysql.password_env: %w", err)
	}
	if conf.Redis.Password, err = envValue(conf.Redis.PasswordEnv, conf.Redis.Password, true); err != nil {
		return fmt.Errorf("redis.password_env: %w", err)
	}
	if conf.Minio.Username, err = envValue(conf.Minio.UsernameEnv, conf.Minio.Username, false); err != nil {
		return fmt.Errorf("minio.username_env: %w", err)
	}
	if conf.Minio.Password, err = envValue(conf.Minio.PasswordEnv, conf.Minio.Password, false); err != nil {
		return fmt.Errorf("minio.password_env: %w", err)
	}
	if conf.Jwt.Secret, err = envValue(conf.Jwt.SecretEnv, conf.Jwt.Secret, false); err != nil {
		return fmt.Errorf("jwt.secret_env: %w", err)
	}
	if conf.Admin.Username, err = envValue(conf.Admin.UsernameEnv, conf.Admin.Username, false); err != nil {
		return fmt.Errorf("admin.username_env: %w", err)
	}
	if conf.Admin.Password, err = envValue(conf.Admin.PasswordEnv, conf.Admin.Password, false); err != nil {
		return fmt.Errorf("admin.password_env: %w", err)
	}
	if conf.Admin.Email, err = envValue(conf.Admin.EmailEnv, conf.Admin.Email, false); err != nil {
		return fmt.Errorf("admin.email_env: %w", err)
	}
	if conf.Admin.Nickname, err = envValue(conf.Admin.NicknameEnv, conf.Admin.Nickname, false); err != nil {
		return fmt.Errorf("admin.nickname_env: %w", err)
	}

	return nil
}

func envValue(envName, fallback string, allowEmpty bool) (string, error) {
	if envName == "" {
		return fallback, nil
	}
	value, ok := os.LookupEnv(envName)
	if !ok {
		return "", fmt.Errorf("%s is not set", envName)
	}
	if value == "" && fallback == "" && !allowEmpty {
		return "", errors.New(envName + " is empty")
	}
	return value, nil
}
