package uploadsecurity_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"admin/internal/uploadsecurity"
)

func TestUploadSecurityCoreValidationMatrix(t *testing.T) {
	validPDF := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\r\n")

	tests := []struct {
		name     string
		run      func() error
		wantCode uploadsecurity.Code
	}{
		{
			name: "filename is cleaned to its final path segment",
			run: func() error {
				got, err := uploadsecurity.SanitizeDisplayName(
					`C:\fakepath\report.PDF`,
					uploadsecurity.PurposeManagedFile,
					".pdf",
				)
				if err != nil {
					return err
				}
				if got != "report.pdf" {
					return fmt.Errorf("sanitized name: got %q", got)
				}
				return nil
			},
		},
		{
			name: "invalid canonical extension is rejected",
			run: func() error {
				_, err := uploadsecurity.SanitizeDisplayName(
					"report.pdf",
					uploadsecurity.PurposeManagedFile,
					"../../exe",
				)
				return err
			},
			wantCode: uploadsecurity.CodeFileNameInvalid,
		},
		{
			name: "dangerous double extension is rejected",
			run: func() error {
				return uploadsecurity.ValidateExtensionChain("payload.exe.pdf")
			},
			wantCode: uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name: "format character cannot hide dangerous double extension",
			run: func() error {
				return uploadsecurity.ValidateExtensionChain("payload.e\u200bxe.pdf")
			},
			wantCode: uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name: "ordinary multi-dot filename is accepted",
			run: func() error {
				return uploadsecurity.ValidateExtensionChain("annual.report.final.pdf")
			},
		},
		{
			name: "allowed type evidence is accepted",
			run: func() error {
				_, err := uploadsecurity.ResolveCanonicalType(uploadsecurity.TypeEvidence{
					FileName:      "photo.jpeg",
					DeclaredMIME:  "image/jpeg",
					DetectedMIME:  "image/jpeg",
					ValidatedType: uploadsecurity.TypeJPEG,
				})
				return err
			},
		},
		{
			name: "unlisted type is rejected",
			run: func() error {
				_, err := uploadsecurity.ResolveCanonicalType(uploadsecurity.TypeEvidence{
					FileName:      "archive.zip",
					DeclaredMIME:  "application/zip",
					DetectedMIME:  "application/zip",
					ValidatedType: uploadsecurity.CanonicalType("zip"),
				})
				return err
			},
			wantCode: uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name: "MIME mismatch is rejected",
			run: func() error {
				_, err := uploadsecurity.ResolveCanonicalType(uploadsecurity.TypeEvidence{
					FileName:      "photo.jpg",
					DeclaredMIME:  "image/png",
					DetectedMIME:  "image/jpeg",
					ValidatedType: uploadsecurity.TypeJPEG,
				})
				return err
			},
			wantCode: uploadsecurity.CodeFileTypeMismatch,
		},
		{
			name: "empty file is rejected",
			run: func() error {
				_, err := uploadsecurity.Stage(context.Background(), bytes.NewReader(nil), 4)
				return err
			},
			wantCode: uploadsecurity.CodeFileEmpty,
		},
		{
			name: "exact size boundary is accepted",
			run: func() error {
				staged, err := uploadsecurity.Stage(
					context.Background(),
					bytes.NewReader([]byte("1234")),
					4,
				)
				if err != nil {
					return err
				}
				return staged.Close()
			},
		},
		{
			name: "size above boundary is rejected",
			run: func() error {
				_, err := uploadsecurity.Stage(
					context.Background(),
					bytes.NewReader([]byte("12345")),
					4,
				)
				return err
			},
			wantCode: uploadsecurity.CodeFileTooLarge,
		},
		{
			name: "valid image is accepted",
			run: func() error {
				_, err := uploadsecurity.ValidateContent(
					bytes.NewReader(encodeTestJPEG(t)),
					uploadsecurity.TypeJPEG,
				)
				return err
			},
		},
		{
			name: "corrupt image is rejected",
			run: func() error {
				_, err := uploadsecurity.ValidateContent(
					bytes.NewReader([]byte("\xff\xd8broken")),
					uploadsecurity.TypeJPEG,
				)
				return err
			},
			wantCode: uploadsecurity.CodeImageDecodeInvalid,
		},
		{
			name: "valid PDF markers are accepted",
			run: func() error {
				_, err := uploadsecurity.ValidateContent(
					bytes.NewReader(validPDF),
					uploadsecurity.TypePDF,
				)
				return err
			},
		},
		{
			name: "PDF without EOF marker is rejected",
			run: func() error {
				_, err := uploadsecurity.ValidateContent(
					bytes.NewReader([]byte("%PDF-1.7\nmissing EOF")),
					uploadsecurity.TypePDF,
				)
				return err
			},
			wantCode: uploadsecurity.CodeFileContentInvalid,
		},
		{
			name: "UTF-8 text with BOM is accepted",
			run: func() error {
				_, err := uploadsecurity.ValidateContent(
					bytes.NewReader(append([]byte{0xef, 0xbb, 0xbf}, []byte("文本")...)),
					uploadsecurity.TypeTXT,
				)
				return err
			},
		},
		{
			name: "UTF-16 text is rejected",
			run: func() error {
				_, err := uploadsecurity.ValidateContent(
					bytes.NewReader([]byte{0xff, 0xfe, 'a', 0}),
					uploadsecurity.TypeTXT,
				)
				return err
			},
			wantCode: uploadsecurity.CodeFileEncodingInvalid,
		},
		{
			name: "text containing NUL is rejected",
			run: func() error {
				_, err := uploadsecurity.ValidateContent(
					bytes.NewReader([]byte{'a', 0, 'b'}),
					uploadsecurity.TypeCSV,
				)
				return err
			},
			wantCode: uploadsecurity.CodeFileEncodingInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != tt.wantCode {
				t.Fatalf("code: got %q, classified=%v, want %q", code, ok, tt.wantCode)
			}
		})
	}
}
