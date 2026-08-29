package domain_test

import (
	"strings"
	"testing"

	"admin/internal/messaging/domain"
)

func TestCompileMessageContentBuildsSafeExternalLinks(t *testing.T) {
	content, err := domain.CompileMessageContent("公告", "安全 [链接](https://example.test/path)")
	if err != nil {
		t.Fatalf("CompileMessageContent() error = %v", err)
	}
	if !strings.Contains(content.HTML, `<a href="https://example.test/path">`) {
		t.Fatalf("compiled HTML lost safe link: %q", content.HTML)
	}
}
func TestCompileMessageContentCountsUnicodeLimits(t *testing.T) {
	_, err := domain.CompileMessageContent(strings.Repeat("标", domain.MaxMessageTitleRunes+1), "正文")
	if err == nil {
		t.Fatal("title over the Unicode limit was accepted")
	}

	_, err = domain.CompileMessageContent("标题", strings.Repeat("文", domain.MaxMessageBodyRunes+1))
	if err == nil {
		t.Fatal("body over the Unicode limit was accepted")
	}
}

func TestCompileMessageContentAcceptsExactUnicodeLimitsAndRejectsInvalidText(t *testing.T) {
	content, err := domain.CompileMessageContent(strings.Repeat("标", domain.MaxMessageTitleRunes), strings.Repeat("文", domain.MaxMessageBodyRunes))
	if err != nil || content.Title == "" || content.HTML == "" {
		t.Fatalf("CompileMessageContent() = %#v, %v", content, err)
	}
	invalidUTF8 := string([]byte{0xff})
	if _, err := domain.CompileMessageContent(invalidUTF8, "正文"); err != domain.ErrMessageTextInvalid {
		t.Fatalf("invalid title error = %v", err)
	}
	if _, err := domain.CompileMessageContent("标题", invalidUTF8); err != domain.ErrMessageTextInvalid {
		t.Fatalf("invalid body error = %v", err)
	}
}

func TestCompileMessageContentDefines128KiBCleanHTMLLimit(t *testing.T) {
	if domain.MaxMessageHTMLBytes != 128*1024 {
		t.Fatalf("MaxMessageHTMLBytes = %d, want 131072", domain.MaxMessageHTMLBytes)
	}
	content, err := domain.CompileMessageContent("标题", strings.Repeat("&", domain.MaxMessageBodyRunes))
	if err != nil || len(content.HTML) > domain.MaxMessageHTMLBytes {
		t.Fatalf("CompileMessageContent() = %d bytes, %v", len(content.HTML), err)
	}
}

func TestCompileMessageContentRejectsDangerousMarkdownConstructs(t *testing.T) {
	for name, markdown := range map[string]string{
		"script tag":         "<script>alert(1)</script>",
		"event attribute":    "<img src=\"https://example.test/a.png\" onerror=\"alert(1)\">",
		"dangerous link":     "[click](javascript:alert(1))",
		"dangerous image":    "![image](data:image/svg+xml;base64,PHN2Zz4=)",
		"dangerous autolink": "<vbscript:msgbox(1)>",
		"dangerous raw URL":  "<a href=\"javascript:alert(1)\">click</a>",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.CompileMessageContent("标题", markdown); err != domain.ErrMessageContentUnsafe {
				t.Fatalf("CompileMessageContent() error = %v", err)
			}
		})
	}
}
