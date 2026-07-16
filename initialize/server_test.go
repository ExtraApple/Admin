package initialize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitConfigLoadsFileUploadConfig 验证文件上传配置从 config.yaml 进入公开配置对象。
func TestInitConfigLoadsFileUploadConfig(t *testing.T) {
	configDir := t.TempDir()
	configData := []byte(`
file_upload:
  max_size_mb: 50
  avatar_max_size_mb: 2
  download_url_expire_seconds: 300
`)
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), configData, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(configDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	got := InitConfig()
	if got.FileUpload.MaxSizeMB != 50 {
		t.Fatalf("max_size_mb: got %d, want 50", got.FileUpload.MaxSizeMB)
	}
	if got.FileUpload.AvatarMaxSizeMB != 2 {
		t.Fatalf("avatar_max_size_mb: got %d, want 2", got.FileUpload.AvatarMaxSizeMB)
	}
	if got.FileUpload.DownloadURLExpireSeconds != 300 {
		t.Fatalf("download_url_expire_seconds: got %d, want 300", got.FileUpload.DownloadURLExpireSeconds)
	}
}

// TestInitConfigAcceptsFileUploadBoundaryValues 验证程序硬上限边界仍可正常启动。
func TestInitConfigAcceptsFileUploadBoundaryValues(t *testing.T) {
	withTestConfig(t, `
file_upload:
  max_size_mb: 100
  avatar_max_size_mb: 10
  download_url_expire_seconds: 1
`)

	got := InitConfig()
	if got.FileUpload.MaxSizeMB != 100 ||
		got.FileUpload.AvatarMaxSizeMB != 10 ||
		got.FileUpload.DownloadURLExpireSeconds != 1 {
		t.Fatalf("unexpected boundary config: %+v", got.FileUpload)
	}
}

// TestInitConfigRejectsInvalidFileUploadConfig 验证非法上传配置会阻止服务启动。
func TestInitConfigRejectsInvalidFileUploadConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantConfig string
	}{
		{
			name: "zero managed file limit",
			config: `
file_upload:
  max_size_mb: 0
  avatar_max_size_mb: 2
  download_url_expire_seconds: 300
`,
			wantConfig: "file_upload.max_size_mb",
		},
		{
			name: "managed file limit above hard maximum",
			config: `
file_upload:
  max_size_mb: 101
  avatar_max_size_mb: 2
  download_url_expire_seconds: 300
`,
			wantConfig: "file_upload.max_size_mb",
		},
		{
			name: "zero avatar limit",
			config: `
file_upload:
  max_size_mb: 50
  avatar_max_size_mb: 0
  download_url_expire_seconds: 300
`,
			wantConfig: "file_upload.avatar_max_size_mb",
		},
		{
			name: "avatar limit above hard maximum",
			config: `
file_upload:
  max_size_mb: 50
  avatar_max_size_mb: 11
  download_url_expire_seconds: 300
`,
			wantConfig: "file_upload.avatar_max_size_mb",
		},
		{
			name: "non-positive URL expiration",
			config: `
file_upload:
  max_size_mb: 50
  avatar_max_size_mb: 2
  download_url_expire_seconds: 0
`,
			wantConfig: "file_upload.download_url_expire_seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withTestConfig(t, tt.config)

			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatalf("InitConfig should panic for %s", tt.wantConfig)
				}
				if !strings.Contains(fmt.Sprint(recovered), tt.wantConfig) {
					t.Fatalf("panic %q should identify %s", recovered, tt.wantConfig)
				}
			}()

			InitConfig()
		})
	}
}

// withTestConfig 在隔离目录中写入配置，让 InitConfig 通过公开入口读取。
func withTestConfig(t *testing.T, data string) {
	t.Helper()

	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(data), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(configDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}
