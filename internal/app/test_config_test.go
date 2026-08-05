package app_test

import platformconfig "admin/internal/platform/config"

func testAppConfig() platformconfig.Config {
	config := platformconfig.Config{}
	config.Jwt.Secret = "app-test-file-signing-secret"
	config.FileUpload.MaxSizeMB = 50
	config.FileUpload.AvatarMaxSizeMB = 2
	config.FileUpload.DownloadURLExpireSeconds = 300
	return config
}
