package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformconfig "admin/internal/platform/config"
	"admin/internal/platform/logging"

	"go.uber.org/zap"
)

func TestNewCreatesConfiguredZapLogger(t *testing.T) {
	output := filepath.Join(t.TempDir(), "nested", "app.log")
	logger, err := logging.New(platformconfig.LoggerConfig{
		Level:      "error",
		Format:     "json",
		Output:     output,
		MaxSize:    10,
		MaxBackups: 2,
		MaxAge:     3,
		Compress:   true,
	})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(); err != nil {
			t.Errorf("close logger: %v", err)
		}
	})

	logger.Info("filtered-message")
	logger.Error("visible-message", zap.String("component", "platform"))
	if err := logger.Sync(); err != nil {
		t.Fatalf("sync logger: %v", err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "filtered-message") {
		t.Fatalf("info message was not filtered: %s", text)
	}
	if !strings.Contains(text, "visible-message") || !strings.Contains(text, `"component":"platform"`) {
		t.Fatalf("configured error log missing: %s", text)
	}
}

func TestNewPreservesExistingLoggerDefaults(t *testing.T) {
	workingDirectory := t.TempDir()
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	logger, err := logging.New(platformconfig.LoggerConfig{})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = logger.Close()
		}
	})

	logger.Debug("default-debug-message")
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}
	closed = true

	data, err := os.ReadFile(filepath.Join("logs", "app.log"))
	if err != nil {
		t.Fatalf("read default log file: %v", err)
	}
	if !strings.Contains(string(data), "default-debug-message") {
		t.Fatalf("default debug log missing: %s", data)
	}
}
