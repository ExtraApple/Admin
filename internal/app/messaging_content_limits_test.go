package app_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"admin/internal/messaging/domain"

	"gopkg.in/yaml.v3"
)

// The shipped configuration must keep the documented defaults, so a deployment
// that does not override them behaves exactly as before this change.
func TestRepositoryConfigurationKeepsDocumentedMessagingContentLimits(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository configuration test path")
	}
	configPath := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(currentFile))), "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read repository config: %v", err)
	}
	// Parse only the messaging limits: the full loader resolves environment
	// backed secrets, which are not available in every environment.
	var parsed struct {
		Messaging struct {
			MaxTitleRunes int `yaml:"max_title_runes"`
			MaxBodyRunes  int `yaml:"max_body_runes"`
		} `yaml:"messaging"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse repository config: %v", err)
	}
	if parsed.Messaging.MaxTitleRunes != domain.MaxMessageTitleRunes || parsed.Messaging.MaxBodyRunes != domain.MaxMessageBodyRunes {
		t.Fatalf("config.yaml messaging limits = %d/%d, want %d/%d",
			parsed.Messaging.MaxTitleRunes, parsed.Messaging.MaxBodyRunes,
			domain.MaxMessageTitleRunes, domain.MaxMessageBodyRunes)
	}
	defaults := domain.DefaultContentLimits()
	if defaults.MaxTitleRunes != parsed.Messaging.MaxTitleRunes || defaults.MaxBodyRunes != parsed.Messaging.MaxBodyRunes {
		t.Fatalf("DefaultContentLimits() = %+v, does not match the shipped configuration", defaults)
	}
}

// A message compiled with the shipped values must behave exactly as documented:
// 100 title runes and 20,000 body runes pass, one rune more is rejected.
func TestShippedContentLimitsEnforceDocumentedBoundaries(t *testing.T) {
	limits := domain.DefaultContentLimits()
	if _, err := domain.CompileMessageContent(strings.Repeat("标", 100), strings.Repeat("文", 20000), limits); err != nil {
		t.Fatalf("a message exactly at the documented limits was rejected: %v", err)
	}
	if _, err := domain.CompileMessageContent(strings.Repeat("标", 101), "正文", limits); err != domain.ErrMessageTitleTooLong {
		t.Fatalf("101-rune title error = %v, want ErrMessageTitleTooLong", err)
	}
	if _, err := domain.CompileMessageContent("标题", strings.Repeat("文", 20001), limits); err != domain.ErrMessageBodyTooLong {
		t.Fatalf("20001-rune body error = %v, want ErrMessageBodyTooLong", err)
	}
}
