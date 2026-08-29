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
	RabbitMQ        RabbitMQConfig        `yaml:"rabbitmq"`
	Messaging       MessagingConfig       `yaml:"messaging"`
	SMTP            SMTPConfig            `yaml:"smtp"`
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
	UserEnv     string `yaml:"user_env"`
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

type RabbitMQConfig struct {
	Host                     string `yaml:"host"`
	Port                     int    `yaml:"port"`
	Username                 string `yaml:"username"`
	UsernameEnv              string `yaml:"username_env"`
	Password                 string `yaml:"password"`
	PasswordEnv              string `yaml:"password_env"`
	VHost                    string `yaml:"vhost"`
	VHostEnv                 string `yaml:"vhost_env"`
	TLS                      bool   `yaml:"tls"`
	CAFile                   string `yaml:"ca_file"`
	ServerName               string `yaml:"server_name"`
	ConnectionTimeoutSeconds int    `yaml:"connection_timeout_seconds"`
	ConfirmTimeoutSeconds    int    `yaml:"confirm_timeout_seconds"`
	WorkerLeaseSeconds       int    `yaml:"worker_lease_seconds"`
	RetryDelaysSeconds       []int  `yaml:"retry_delays_seconds"`
	MaxRetries               int    `yaml:"max_retries"`
	DLQRetentionDays         int    `yaml:"dlq_retention_days"`
}

type SMTPConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	Username       string `yaml:"username"`
	UsernameEnv    string `yaml:"username_env"`
	Password       string `yaml:"password"`
	PasswordEnv    string `yaml:"password_env"`
	From           string `yaml:"from"`
	FromEnv        string `yaml:"from_env"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	TLSMode        string `yaml:"tls_mode"`
}

type MessagingConfig struct {
	MaxAudienceUsers int `yaml:"max_audience_users"`
	MaxTitleRunes    int `yaml:"max_title_runes"`
	MaxBodyRunes     int `yaml:"max_body_runes"`
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
	applyMessagingDefaults(&result)
	applySMTPDefaults(&result)
	if err := applyEnvironment(&result); err != nil {
		return Config{}, fmt.Errorf("load environment config: %w", err)
	}
	if err := validateUpload(result.FileUpload); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	if err := validateMessaging(result); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	if err := validateSMTP(result.SMTP); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return result, nil

}

func applySMTPDefaults(result *Config) {
	if result.SMTP.Port == 0 {
		result.SMTP.Port = 587
	}
	if result.SMTP.TimeoutSeconds == 0 {
		result.SMTP.TimeoutSeconds = 10
	}
	if result.SMTP.TLSMode == "" {
		result.SMTP.TLSMode = "starttls_required"
	}
}

func validateSMTP(smtp SMTPConfig) error {
	if smtp.Host == "" {
		return nil
	}
	if smtp.Port < 1 || smtp.Port > 65535 {
		return errors.New("smtp.port must be between 1 and 65535")
	}
	if smtp.Username != "" && smtp.UsernameEnv == "" {
		return errors.New("smtp.username must be provided through username_env")
	}
	if smtp.Password != "" && smtp.PasswordEnv == "" {
	}
	if smtp.Username == "" || smtp.Password == "" {
		return errors.New("smtp username and password are required when smtp is configured")
	}
	if smtp.From == "" {
		return errors.New("smtp.from is required when smtp is configured")
	}
	if smtp.TimeoutSeconds < 1 {
		return errors.New("smtp.timeout_seconds must be positive")
	}
	switch smtp.TLSMode {
	case "disabled", "starttls_required", "implicit":
		return nil
	default:
		return errors.New("smtp.tls_mode must be disabled, starttls_required, or implicit")
	}
}

func applyMessagingDefaults(result *Config) {
	if result.RabbitMQ.ConnectionTimeoutSeconds == 0 {
		result.RabbitMQ.ConnectionTimeoutSeconds = 5
	}
	if result.RabbitMQ.ConfirmTimeoutSeconds == 0 {
		result.RabbitMQ.ConfirmTimeoutSeconds = 10
	}
	if result.RabbitMQ.WorkerLeaseSeconds == 0 {
		result.RabbitMQ.WorkerLeaseSeconds = 30
	}
	if len(result.RabbitMQ.RetryDelaysSeconds) == 0 {
		result.RabbitMQ.RetryDelaysSeconds = []int{1, 2, 4, 8, 16}
	}
	if result.RabbitMQ.MaxRetries == 0 {
		result.RabbitMQ.MaxRetries = 5
	}
	if result.RabbitMQ.DLQRetentionDays == 0 {
		result.RabbitMQ.DLQRetentionDays = 7
	}
	if result.Messaging.MaxAudienceUsers == 0 {
		result.Messaging.MaxAudienceUsers = 100000
	}
	if result.Messaging.MaxTitleRunes == 0 {
		result.Messaging.MaxTitleRunes = 100
	}
	if result.Messaging.MaxBodyRunes == 0 {
		result.Messaging.MaxBodyRunes = 20000
	}
}

func validateMessaging(result Config) error {
	if result.RabbitMQ.ConnectionTimeoutSeconds < 1 || result.RabbitMQ.ConfirmTimeoutSeconds < 1 || result.RabbitMQ.WorkerLeaseSeconds < 1 {
		return errors.New("rabbitmq timeout settings must be positive integers")
	}
	if len(result.RabbitMQ.RetryDelaysSeconds) != 5 || result.RabbitMQ.RetryDelaysSeconds[0] != 1 || result.RabbitMQ.RetryDelaysSeconds[1] != 2 || result.RabbitMQ.RetryDelaysSeconds[2] != 4 || result.RabbitMQ.RetryDelaysSeconds[3] != 8 || result.RabbitMQ.RetryDelaysSeconds[4] != 16 {
		return errors.New("rabbitmq.retry_delays_seconds must be the fixed sequence 1,2,4,8,16")
	}
	if result.RabbitMQ.MaxRetries != 5 {
		return errors.New("rabbitmq.max_retries must be 5")
	}
	if result.RabbitMQ.DLQRetentionDays < 1 {
		return errors.New("rabbitmq.dlq_retention_days must be positive")
	}
	if result.Messaging.MaxAudienceUsers < 1 || result.Messaging.MaxAudienceUsers > 100000 {
		return errors.New("messaging.max_audience_users must be between 1 and 100000")
	}
	if result.Messaging.MaxTitleRunes < 1 || result.Messaging.MaxBodyRunes < 1 {
		return errors.New("messaging content limits must be positive integers")
	}
	return nil
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
		{name: result.Mysql.UserEnv, fallback: result.Mysql.User, destination: &result.Mysql.User},
		{name: result.Minio.UsernameEnv, fallback: result.Minio.Username, destination: &result.Minio.Username},
		{name: result.Minio.PasswordEnv, fallback: result.Minio.Password, destination: &result.Minio.Password},
		{name: result.Jwt.SecretEnv, fallback: result.Jwt.Secret, destination: &result.Jwt.Secret},
		{name: result.Redis.PasswordEnv, fallback: result.Redis.Password, allowEmpty: true, destination: &result.Redis.Password},
		{name: result.RabbitMQ.UsernameEnv, fallback: result.RabbitMQ.Username, destination: &result.RabbitMQ.Username},
		{name: result.RabbitMQ.PasswordEnv, fallback: result.RabbitMQ.Password, destination: &result.RabbitMQ.Password},
		{name: result.RabbitMQ.VHostEnv, fallback: result.RabbitMQ.VHost, destination: &result.RabbitMQ.VHost},
		{name: result.SMTP.UsernameEnv, fallback: result.SMTP.Username, destination: &result.SMTP.Username},
		{name: result.SMTP.PasswordEnv, fallback: result.SMTP.Password, destination: &result.SMTP.Password},
		{name: result.SMTP.FromEnv, fallback: result.SMTP.From, destination: &result.SMTP.From},
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
