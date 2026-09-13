package domain_test

import (
	"strings"
	"testing"

	"admin/internal/messaging/domain"
)

func TestCompileMessageContentBuildsSafeExternalLinks(t *testing.T) {
	content, err := domain.CompileMessageContent("公告", "安全 [链接](https://example.test/path)", domain.DefaultContentLimits())
	if err != nil {
		t.Fatalf("CompileMessageContent() error = %v", err)
	}
	if !strings.Contains(content.HTML, `<a href="https://example.test/path">`) {
		t.Fatalf("compiled HTML lost safe link: %q", content.HTML)
	}
}
func TestCompileMessageContentCountsUnicodeLimits(t *testing.T) {
	_, err := domain.CompileMessageContent(strings.Repeat("标", domain.MaxMessageTitleRunes+1), "正文", domain.DefaultContentLimits())
	if err == nil {
		t.Fatal("title over the Unicode limit was accepted")
	}

	_, err = domain.CompileMessageContent("标题", strings.Repeat("文", domain.MaxMessageBodyRunes+1), domain.DefaultContentLimits())
	if err == nil {
		t.Fatal("body over the Unicode limit was accepted")
	}
}

func TestCompileMessageContentAcceptsExactUnicodeLimitsAndRejectsInvalidText(t *testing.T) {
	content, err := domain.CompileMessageContent(strings.Repeat("标", domain.MaxMessageTitleRunes), strings.Repeat("文", domain.MaxMessageBodyRunes), domain.DefaultContentLimits())
	if err != nil || content.Title == "" || content.HTML == "" {
		t.Fatalf("CompileMessageContent() = %#v, %v", content, err)
	}
	invalidUTF8 := string([]byte{0xff})
	if _, err := domain.CompileMessageContent(invalidUTF8, "正文", domain.DefaultContentLimits()); err != domain.ErrMessageTextInvalid {
		t.Fatalf("invalid title error = %v", err)
	}
	if _, err := domain.CompileMessageContent("标题", invalidUTF8, domain.DefaultContentLimits()); err != domain.ErrMessageTextInvalid {
		t.Fatalf("invalid body error = %v", err)
	}
}

func TestCompileMessageContentDefines128KiBCleanHTMLLimit(t *testing.T) {
	if domain.MaxMessageHTMLBytes != 128*1024 {
		t.Fatalf("MaxMessageHTMLBytes = %d, want 131072", domain.MaxMessageHTMLBytes)
	}
	content, err := domain.CompileMessageContent("标题", strings.Repeat("&", domain.MaxMessageBodyRunes), domain.DefaultContentLimits())
	if err != nil || len(content.HTML) > domain.MaxMessageHTMLBytes {
		t.Fatalf("CompileMessageContent() = %d bytes, %v", len(content.HTML), err)
	}
}

func TestCompileMessageContentUsesInjectedLimits(t *testing.T) {
	limits := domain.ContentLimits{MaxTitleRunes: 120, MaxBodyRunes: 25_000}
	content, err := domain.CompileMessageContent(strings.Repeat("标", 120), strings.Repeat("文", 25_000), limits)
	if err != nil || content.Title == "" || content.HTML == "" {
		t.Fatalf("CompileMessageContent() with raised limits = %#v, %v", content, err)
	}
	if _, err := domain.CompileMessageContent(strings.Repeat("标", 121), "正文", limits); err != domain.ErrMessageTitleTooLong {
		t.Fatalf("title over the injected limit error = %v", err)
	}
	if _, err := domain.CompileMessageContent("标题", strings.Repeat("文", 25_001), limits); err != domain.ErrMessageBodyTooLong {
		t.Fatalf("body over the injected limit error = %v", err)
	}
}

func TestCompileMessageContentRejectsTightenedLimits(t *testing.T) {
	limits := domain.ContentLimits{MaxTitleRunes: 50, MaxBodyRunes: 500}
	if _, err := domain.CompileMessageContent(strings.Repeat("标", 50), strings.Repeat("文", 500), limits); err != nil {
		t.Fatalf("CompileMessageContent() at the tightened limits error = %v", err)
	}
	if _, err := domain.CompileMessageContent(strings.Repeat("标", 51), "正文", limits); err != domain.ErrMessageTitleTooLong {
		t.Fatalf("title over the tightened limit error = %v", err)
	}
	if _, err := domain.CompileMessageContent("标题", strings.Repeat("文", 501), limits); err != domain.ErrMessageBodyTooLong {
		t.Fatalf("body over the tightened limit error = %v", err)
	}
}

func TestCompileMessageContentRejectsInvalidLimits(t *testing.T) {
	for name, limits := range map[string]domain.ContentLimits{
		"zero":           {},
		"zero title":     {MaxTitleRunes: 0, MaxBodyRunes: 20_000},
		"zero body":      {MaxTitleRunes: 100, MaxBodyRunes: 0},
		"negative title": {MaxTitleRunes: -1, MaxBodyRunes: 20_000},
		"negative body":  {MaxTitleRunes: 100, MaxBodyRunes: -1},
		"both negative":  {MaxTitleRunes: -1, MaxBodyRunes: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.CompileMessageContent("标题", "正文", limits); err != domain.ErrMessageLimitsInvalid {
				t.Fatalf("CompileMessageContent() with %#v error = %v", limits, err)
			}
		})
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
			if _, err := domain.CompileMessageContent("标题", markdown, domain.DefaultContentLimits()); err != domain.ErrMessageContentUnsafe {
				t.Fatalf("CompileMessageContent() error = %v", err)
			}
		})
	}
}
