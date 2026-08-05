package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformconfig "admin/internal/platform/config"
)

func TestLoadPreservesExistingYAMLConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
server:
  port: 18080
mysql:
  host: mysql.local
  port: 3307
  user: admin
  password: mysql-secret
  db: admin_test
minio:
  host: minio.local
  port: 9001
  username: minio-user
  password: minio-secret
jwt:
  secret: jwt-secret
  expire: 15
  refresh_expire: 10080
  legacy_access_expire: 30
  legacy_refresh_expire: 20160
redis:
  host: redis.local
  port: 6380
  password: redis-secret
  db: 3
admin:
  username: root
  password: admin-secret
  email: root@example.com
  nickname: Root
logger:
  level: info
  format: json
  output: logs/test.log
  max_size: 50
  max_backups: 5
  max_age: 14
  compress: true
file_rotation:
  enabled: true
  days: 30
  hot_bucket: files
  cold_bucket: files-archive
  batch_size: 100
file_upload:
  max_size_mb: 50
  avatar_max_size_mb: 2
  download_url_expire_seconds: 300
audit_log_archive:
  enabled: true
  retention_days: 90
  batch_size: 1000
api_docs:
  enabled: true
  title: Admin API
  version: 1.0.0
  description: Admin API documentation
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := platformconfig.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got.Server.Port != 18080 || got.Mysql.Host != "mysql.local" || got.Mysql.Port != 3307 || got.Mysql.User != "admin" || got.Mysql.Password != "mysql-secret" || got.Mysql.DB != "admin_test" {
		t.Fatalf("server/mysql config changed: %+v %+v", got.Server, got.Mysql)
	}
	if got.Minio.Host != "minio.local" || got.Minio.Port != 9001 || got.Minio.Username != "minio-user" || got.Minio.Password != "minio-secret" {
		t.Fatalf("minio config changed: %+v", got.Minio)
	}
	if got.Jwt.Secret != "jwt-secret" || got.Jwt.Expire != 15 || got.Jwt.RefreshExpire != 10080 || got.Jwt.LegacyAccessExpire != 30 || got.Jwt.LegacyRefreshExpire != 20160 {
		t.Fatalf("jwt config changed: %+v", got.Jwt)
	}
	if got.Redis.Host != "redis.local" || got.Redis.Port != 6380 || got.Redis.Password != "redis-secret" || got.Redis.DB != 3 {
		t.Fatalf("redis config changed: %+v", got.Redis)
	}
	if got.Admin.Username != "root" || got.Admin.Password != "admin-secret" || got.Admin.Email != "root@example.com" || got.Admin.Nickname != "Root" {
		t.Fatalf("admin config changed: %+v", got.Admin)
	}
	if got.Logger.Level != "info" || got.Logger.Format != "json" || got.Logger.Output != "logs/test.log" || got.Logger.MaxSize != 50 || got.Logger.MaxBackups != 5 || got.Logger.MaxAge != 14 || !got.Logger.Compress {
		t.Fatalf("logger config changed: %+v", got.Logger)
	}
	if !got.FileRotation.Enabled || got.FileRotation.Days != 30 || got.FileRotation.HotBucket != "files" || got.FileRotation.ColdBucket != "files-archive" || got.FileRotation.BatchSize != 100 {
		t.Fatalf("file rotation config changed: %+v", got.FileRotation)
	}
	if got.FileUpload.MaxSizeMB != 50 || got.FileUpload.AvatarMaxSizeMB != 2 || got.FileUpload.DownloadURLExpireSeconds != 300 {
		t.Fatalf("file upload config changed: %+v", got.FileUpload)
	}
	if !got.AuditLogArchive.Enabled || got.AuditLogArchive.RetentionDays != 90 || got.AuditLogArchive.BatchSize != 1000 {
		t.Fatalf("audit archive config changed: %+v", got.AuditLogArchive)
	}
	if !got.APIDocs.Enabled || got.APIDocs.Title != "Admin API" || got.APIDocs.Version != "1.0.0" || got.APIDocs.Description != "Admin API documentation" {
		t.Fatalf("api docs config changed: %+v", got.APIDocs)
	}
}

func TestLoadResolvesExistingEnvironmentBackedSecrets(t *testing.T) {
	t.Setenv("TEST_MYSQL_PASSWORD", "mysql-from-env")
	t.Setenv("TEST_MINIO_USERNAME", "minio-user-from-env")
	t.Setenv("TEST_MINIO_PASSWORD", "minio-password-from-env")
	t.Setenv("TEST_JWT_SECRET", "jwt-from-env")
	t.Setenv("TEST_REDIS_PASSWORD", "redis-from-env")
	t.Setenv("TEST_ADMIN_USERNAME", "admin-from-env")
	t.Setenv("TEST_ADMIN_PASSWORD", "admin-password-from-env")
	t.Setenv("TEST_ADMIN_EMAIL", "admin-from-env@example.com")
	t.Setenv("TEST_ADMIN_NICKNAME", "Administrator")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
mysql:
  password_env: TEST_MYSQL_PASSWORD
minio:
  username_env: TEST_MINIO_USERNAME
  password_env: TEST_MINIO_PASSWORD
jwt:
  secret_env: TEST_JWT_SECRET
redis:
  password_env: TEST_REDIS_PASSWORD
admin:
  username_env: TEST_ADMIN_USERNAME
  password_env: TEST_ADMIN_PASSWORD
  email_env: TEST_ADMIN_EMAIL
  nickname_env: TEST_ADMIN_NICKNAME
file_upload:
  max_size_mb: 50
  avatar_max_size_mb: 2
  download_url_expire_seconds: 300
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := platformconfig.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got.Mysql.Password != "mysql-from-env" || got.Minio.Username != "minio-user-from-env" || got.Minio.Password != "minio-password-from-env" || got.Jwt.Secret != "jwt-from-env" || got.Redis.Password != "redis-from-env" {
		t.Fatalf("infrastructure secret resolution changed: mysql=%q minio=%q/%q jwt=%q redis=%q", got.Mysql.Password, got.Minio.Username, got.Minio.Password, got.Jwt.Secret, got.Redis.Password)
	}
	if got.Admin.Username != "admin-from-env" || got.Admin.Password != "admin-password-from-env" || got.Admin.Email != "admin-from-env@example.com" || got.Admin.Nickname != "Administrator" {
		t.Fatalf("admin secret resolution changed: %+v", got.Admin)
	}
}

func TestLoadRejectsExistingInvalidUploadLimits(t *testing.T) {
	tests := []struct {
		name       string
		uploadYAML string
		wantField  string
	}{
		{name: "managed file limit below minimum", uploadYAML: "max_size_mb: 0\n  avatar_max_size_mb: 2\n  download_url_expire_seconds: 300", wantField: "file_upload.max_size_mb"},
		{name: "managed file limit above maximum", uploadYAML: "max_size_mb: 101\n  avatar_max_size_mb: 2\n  download_url_expire_seconds: 300", wantField: "file_upload.max_size_mb"},
		{name: "avatar limit below minimum", uploadYAML: "max_size_mb: 50\n  avatar_max_size_mb: 0\n  download_url_expire_seconds: 300", wantField: "file_upload.avatar_max_size_mb"},
		{name: "avatar limit above maximum", uploadYAML: "max_size_mb: 50\n  avatar_max_size_mb: 11\n  download_url_expire_seconds: 300", wantField: "file_upload.avatar_max_size_mb"},
		{name: "non-positive download expiration", uploadYAML: "max_size_mb: 50\n  avatar_max_size_mb: 2\n  download_url_expire_seconds: 0", wantField: "file_upload.download_url_expire_seconds"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			data := []byte("file_upload:\n  " + test.uploadYAML + "\n")
			if err := os.WriteFile(configPath, data, 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := platformconfig.Load(configPath)
			if err == nil || !strings.Contains(err.Error(), test.wantField) {
				t.Fatalf("load error = %v, want field %s", err, test.wantField)
			}
		})
	}
}

func TestLoadReadsDotEnvBesideConfiguration(t *testing.T) {
	const envName = "TEST_PLATFORM_CONFIG_DOTENV_SECRET"
	if err := os.Unsetenv(envName); err != nil {
		t.Fatalf("unset environment: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv(envName) })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
jwt:
  secret_env: TEST_PLATFORM_CONFIG_DOTENV_SECRET
file_upload:
  max_size_mb: 50
  avatar_max_size_mb: 2
  download_url_expire_seconds: 300
`)
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envName+"=dotenv-secret\n"), 0o600); err != nil {
		t.Fatalf("write dotenv: %v", err)
	}

	got, err := platformconfig.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got.Jwt.Secret != "dotenv-secret" {
		t.Fatalf("jwt secret = %q, want dotenv-secret", got.Jwt.Secret)
	}
}
