package uploadsecurity_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"admin/service/uploadsecurity"
)

func TestSanitizeDisplayName(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		purpose   uploadsecurity.Purpose
		extension string
		want      string
	}{
		{
			name:      "keeps only final path segment",
			raw:       `C:\fakepath\reports\quarterly.PDF`,
			purpose:   uploadsecurity.PurposeManagedFile,
			extension: ".pdf",
			want:      "quarterly.pdf",
		},
		{
			name:      "removes control and bidi characters",
			raw:       "safe\u0000\u202Ename\u2066.txt",
			purpose:   uploadsecurity.PurposeManagedFile,
			extension: ".txt",
			want:      "safename.txt",
		},
		{
			name:      "normalizes unicode whitespace and surrounding dots",
			raw:       "..  销售\u00a0\u3000报表  ..CSV",
			purpose:   uploadsecurity.PurposeManagedFile,
			extension: ".csv",
			want:      "销售 报表.csv",
		},
		{
			name:      "removes characters outside display allowlist",
			raw:       `sales<>:"|?*.csv`,
			purpose:   uploadsecurity.PurposeManagedFile,
			extension: ".csv",
			want:      "sales.csv",
		},
		{
			name:      "keeps common chinese and ascii brackets",
			raw:       "报表（最终）[已审].xlsx",
			purpose:   uploadsecurity.PurposeManagedFile,
			extension: ".xlsx",
			want:      "报表（最终）[已审].xlsx",
		},
		{
			name:      "uses managed file fallback for empty body",
			raw:       "...pdf",
			purpose:   uploadsecurity.PurposeManagedFile,
			extension: ".pdf",
			want:      "file.pdf",
		},
		{
			name:      "uses avatar fallback for empty body",
			raw:       ".png",
			purpose:   uploadsecurity.PurposeAvatar,
			extension: ".png",
			want:      "avatar.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := uploadsecurity.SanitizeDisplayName(tt.raw, tt.purpose, tt.extension)
			if err != nil {
				t.Fatalf("sanitize: %v", err)
			}
			if got != tt.want {
				t.Fatalf("name: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeDisplayNameLimitsUnicodeCharactersAndPreservesExtension(t *testing.T) {
	raw := strings.Repeat("文", 300) + ".pdf"

	got, err := uploadsecurity.SanitizeDisplayName(
		raw,
		uploadsecurity.PurposeManagedFile,
		".pdf",
	)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	if utf8.RuneCountInString(got) != 255 {
		t.Fatalf("rune count: got %d, want 255", utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, ".pdf") {
		t.Fatalf("canonical extension was not preserved: %q", got)
	}
}

func TestSanitizeDisplayNameRejectsInvalidCanonicalExtension(t *testing.T) {
	_, err := uploadsecurity.SanitizeDisplayName(
		"report.pdf",
		uploadsecurity.PurposeManagedFile,
		"../../exe",
	)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileNameInvalid {
		t.Fatalf("code: got %q, classified=%v", code, ok)
	}
}

func TestValidateExtensionChain(t *testing.T) {
	rejected := []string{
		"payload.exe.pdf",
		"script.JS.txt",
		"image.svg.png",
		"archive.tar.gz.pdf",
		`C:\fakepath\payload.ps1.PDF`,
		"payload.e\u202Exe.pdf",
		"macro.docm.docx",
	}
	for _, name := range rejected {
		t.Run("reject "+name, func(t *testing.T) {
			err := uploadsecurity.ValidateExtensionChain(name)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeFileTypeNotAllowed {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}

	allowed := []string{
		"annual.report.final.pdf",
		"photo.family.2026.jpg",
		"数据.最终.版本.xlsx",
		".bashrc.txt",
		"simple.csv",
	}
	for _, name := range allowed {
		t.Run("allow "+name, func(t *testing.T) {
			if err := uploadsecurity.ValidateExtensionChain(name); err != nil {
				t.Fatalf("validate %q: %v", name, err)
			}
		})
	}
}
