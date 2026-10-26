package app_test

import platformconfig "admin/internal/platform/config"

func testAppConfig() platformconfig.Config {
	config := platformconfig.Config{}
	config.Jwt.Secret = "app-test-file-signing-secret"
	config.Jwt.Expire = 15
	config.Jwt.RefreshExpire = 10080
	config.BrowserSession.Origin = "https://admin.example.test"
	config.FileUpload.MaxSizeMB = 50
	config.FileUpload.AvatarMaxSizeMB = 2
	config.FileUpload.DownloadURLExpireSeconds = 300
	return config
}
