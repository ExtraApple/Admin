package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const (
	minManagedFileUploadSizeMB = 1
	maxManagedFileUploadSizeMB = 100
	minAvatarUploadSizeMB      = 1
	maxAvatarUploadSizeMB      = 10
)

type Config struct {
	Server          ServerConfig          `yaml:"server"`
	Mysql           MySQLConfig           `yaml:"mysql"`
	Minio           MinIOConfig           `yaml:"minio"`
	Jwt             JWTConfig             `yaml:"jwt"`
	Redis           RedisConfig           `yaml:"redis"`
	Admin           AdminConfig           `yaml:"admin"`
	FileRotation    FileRotationConfig    `yaml:"file_rotation"`
	FileUpload      FileUploadConfig      `yaml:"file_upload"`
	Logger          LoggerConfig          `yaml:"logger"`
	AuditLogArchive AuditLogArchiveConfig `yaml:"audit_log_archive"`
	APIDocs         APIDocsConfig         `yaml:"api_docs"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type MySQLConfig struct {
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	PasswordEnv string `yaml:"password_env"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	DB          string `yaml:"db"`
}

type MinIOConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	UsernameEnv string `yaml:"username_env"`
	Password    string `yaml:"password"`
	PasswordEnv string `yaml:"password_env"`
}

type JWTConfig struct {
	Secret              string `yaml:"secret"`
	SecretEnv           string `yaml:"secret_env"`
	Expire              int    `yaml:"expire"`
	RefreshExpire       int    `yaml:"refresh_expire"`
	LegacyAccessExpire  int    `yaml:"legacy_access_expire"`
	LegacyRefreshExpire int    `yaml:"legacy_refresh_expire"`
}

type RedisConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Password    string `yaml:"password"`
	PasswordEnv string `yaml:"password_env"`
	DB          int    `yaml:"db"`
}

type AdminConfig struct {
	Username    string `yaml:"username"`
	UsernameEnv string `yaml:"username_env"`
	Password    string `yaml:"password"`
	PasswordEnv string `yaml:"password_env"`
	Email       string `yaml:"email"`
	EmailEnv    string `yaml:"email_env"`
	Nickname    string `yaml:"nickname"`
	NicknameEnv string `yaml:"nickname_env"`
}

type FileRotationConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Days       int    `yaml:"days"`
	HotBucket  string `yaml:"hot_bucket"`
	ColdBucket string `yaml:"cold_bucket"`
	BatchSize  int    `yaml:"batch_size"`
}

type FileUploadConfig struct {
	MaxSizeMB                int `yaml:"max_size_mb"`
	AvatarMaxSizeMB          int `yaml:"avatar_max_size_mb"`
	DownloadURLExpireSeconds int `yaml:"download_url_expire_seconds"`
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

type AuditLogArchiveConfig struct {
	Enabled       bool `yaml:"enabled"`
	RetentionDays int  `yaml:"retention_days"`
	BatchSize     int  `yaml:"batch_size"`
}

type APIDocsConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

func Load(path string) (Config, error) {
	if err := loadDotEnv(path); err != nil {
		return Config{}, fmt.Errorf("load .env file: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var result Config
	if err := yaml.Unmarshal(data, &result); err != nil {
		return Config{}, fmt.Errorf("unmarshal config file: %w", err)
	}
	if err := applyEnvironment(&result); err != nil {
		return Config{}, fmt.Errorf("load environment config: %w", err)
	}
	if err := validateUpload(result.FileUpload); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return result, nil
}

func loadDotEnv(configPath string) error {
	path := filepath.Join(filepath.Dir(configPath), ".env")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return godotenv.Load(path)
}

func validateUpload(upload FileUploadConfig) error {
	if upload.MaxSizeMB < minManagedFileUploadSizeMB || upload.MaxSizeMB > maxManagedFileUploadSizeMB {
		return fmt.Errorf("file_upload.max_size_mb must be between %d and %d", minManagedFileUploadSizeMB, maxManagedFileUploadSizeMB)
	}
	if upload.AvatarMaxSizeMB < minAvatarUploadSizeMB || upload.AvatarMaxSizeMB > maxAvatarUploadSizeMB {
		return fmt.Errorf("file_upload.avatar_max_size_mb must be between %d and %d", minAvatarUploadSizeMB, maxAvatarUploadSizeMB)
	}
	if upload.DownloadURLExpireSeconds < 1 {
		return errors.New("file_upload.download_url_expire_seconds must be a positive integer")
	}
	return nil
}

func applyEnvironment(result *Config) error {
	bindings := []struct {
		name        string
		fallback    string
		allowEmpty  bool
		destination *string
	}{
		{name: result.Mysql.PasswordEnv, fallback: result.Mysql.Password, destination: &result.Mysql.Password},
		{name: result.Minio.UsernameEnv, fallback: result.Minio.Username, destination: &result.Minio.Username},
		{name: result.Minio.PasswordEnv, fallback: result.Minio.Password, destination: &result.Minio.Password},
		{name: result.Jwt.SecretEnv, fallback: result.Jwt.Secret, destination: &result.Jwt.Secret},
		{name: result.Redis.PasswordEnv, fallback: result.Redis.Password, allowEmpty: true, destination: &result.Redis.Password},
		{name: result.Admin.UsernameEnv, fallback: result.Admin.Username, destination: &result.Admin.Username},
		{name: result.Admin.PasswordEnv, fallback: result.Admin.Password, destination: &result.Admin.Password},
		{name: result.Admin.EmailEnv, fallback: result.Admin.Email, destination: &result.Admin.Email},
		{name: result.Admin.NicknameEnv, fallback: result.Admin.Nickname, destination: &result.Admin.Nickname},
	}
	for _, binding := range bindings {
		value, err := environmentValue(binding.name, binding.fallback, binding.allowEmpty)
		if err != nil {
			return err
		}
		*binding.destination = value
	}
	return nil
}

func environmentValue(name, fallback string, allowEmpty bool) (string, error) {
	if name == "" {
		return fallback, nil
	}
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("%s is not set", name)
	}
	if value == "" && fallback == "" && !allowEmpty {
		return "", errors.New(name + " is empty")
	}
	return value, nil
}
