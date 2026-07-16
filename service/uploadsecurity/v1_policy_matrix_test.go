package uploadsecurity_test

import (
	"context"
	"io"
	"testing"

	"admin/service/uploadsecurity"
)

func TestManagedFileV1AllowedTypeMatrix(t *testing.T) {
	tests := []struct {
		name          string
		fileName      string
		declaredMIME  string
		content       func(*testing.T) []byte
		wantType      uploadsecurity.CanonicalType
		wantExtension string
		wantMIME      string
	}{
		{"JPEG", "photo.jpeg", "image/jpeg", func(t *testing.T) []byte { return encodeTestJPEG(t) }, uploadsecurity.TypeJPEG, ".jpg", "image/jpeg"},
		{"PNG", "photo.png", "image/png", func(t *testing.T) []byte { return encodeTestPNG(t) }, uploadsecurity.TypePNG, ".png", "image/png"},
		{"WebP", "photo.webp", "image/webp", func(t *testing.T) []byte { return decodeTestWebP(t) }, uploadsecurity.TypeWebP, ".webp", "image/webp"},
		{"PDF", "report.pdf", "application/pdf", func(*testing.T) []byte {
			return []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\r\n")
		}, uploadsecurity.TypePDF, ".pdf", "application/pdf"},
		{"DOCX", "report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", func(t *testing.T) []byte {
			return buildZIPFixture(t, ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX))
		}, uploadsecurity.TypeDOCX, ".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"XLSX", "report.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", func(t *testing.T) []byte {
			return buildZIPFixture(t, ooxmlFixtureEntries(t, uploadsecurity.TypeXLSX))
		}, uploadsecurity.TypeXLSX, ".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"PPTX", "report.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", func(t *testing.T) []byte {
			return buildZIPFixture(t, ooxmlFixtureEntries(t, uploadsecurity.TypePPTX))
		}, uploadsecurity.TypePPTX, ".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"UTF-8 TXT", "notes.txt", "text/plain", func(*testing.T) []byte {
			return []byte("plain UTF-8 文本\n")
		}, uploadsecurity.TypeTXT, ".txt", "text/plain"},
		{"UTF-8 CSV", "report.csv", "text/csv", func(*testing.T) []byte {
			return []byte("name,value\n测试,1\n")
		}, uploadsecurity.TypeCSV, ".csv", "text/csv"},
	}

	validator := uploadsecurity.NewManagedFileValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := tt.content(t)
			result, err := validator.Validate(context.Background(), uploadsecurity.Input{
				Purpose:      uploadsecurity.PurposeManagedFile,
				FileName:     tt.fileName,
				DeclaredMIME: tt.declaredMIME,
				Size:         int64(len(content)),
				MaxBytes:     int64(len(content)),
				Reader:       bytesReader(content),
			})
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if closer, ok := result.Reader.(io.Closer); ok {
				defer closer.Close()
			}
			if result.CanonicalType != tt.wantType ||
				result.CanonicalExtension != tt.wantExtension ||
				result.CanonicalMIME != tt.wantMIME ||
				result.PolicyVersion != uploadsecurity.PolicyVersionV1 {
				t.Fatalf("Validate() result = %#v, want type=%q extension=%q MIME=%q policy=%q",
					result, tt.wantType, tt.wantExtension, tt.wantMIME, uploadsecurity.PolicyVersionV1)
			}
		})
	}
}

func TestManagedFileV1ForbiddenTypeMatrix(t *testing.T) {
	tests := []struct {
		name         string
		fileName     string
		declaredMIME string
	}{
		{"ZIP archive", "archive.zip", "application/zip"},
		{"RAR archive", "archive.rar", "application/vnd.rar"},
		{"7Z archive", "archive.7z", "application/x-7z-compressed"},
		{"legacy Word", "report.doc", "application/msword"},
		{"legacy Excel", "report.xls", "application/vnd.ms-excel"},
		{"legacy PowerPoint", "report.ppt", "application/vnd.ms-powerpoint"},
		{"macro Word", "report.docm", "application/vnd.ms-word.document.macroenabled.12"},
		{"macro Excel", "report.xlsm", "application/vnd.ms-excel.sheet.macroenabled.12"},
		{"macro PowerPoint", "report.pptm", "application/vnd.ms-powerpoint.presentation.macroenabled.12"},
		{"HTML", "page.html", "text/html"},
		{"SVG", "image.svg", "image/svg+xml"},
		{"JavaScript", "script.js", "text/javascript"},
		{"shell script", "script.sh", "application/x-sh"},
		{"Windows executable", "program.exe", "application/vnd.microsoft.portable-executable"},
	}

	validator := uploadsecurity.NewManagedFileValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte("not allowed")
			_, err := validator.Validate(context.Background(), uploadsecurity.Input{
				Purpose:      uploadsecurity.PurposeManagedFile,
				FileName:     tt.fileName,
				DeclaredMIME: tt.declaredMIME,
				Size:         int64(len(content)),
				MaxBytes:     1024,
				Reader:       bytesReader(content),
			})
			assertUploadSecurityCode(t, err, uploadsecurity.CodeFileTypeNotAllowed)
		})
	}
}

func TestManagedFileV1RejectsMIMEAndDangerousDoubleExtensionMatrix(t *testing.T) {
	validPDF := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\r\n")
	tests := []struct {
		name         string
		fileName     string
		declaredMIME string
		wantCode     uploadsecurity.Code
	}{
		{"declared MIME disagrees with extension and content", "report.pdf", "image/png", uploadsecurity.CodeFileTypeMismatch},
		{"generic binary MIME is not accepted", "report.pdf", "application/octet-stream", uploadsecurity.CodeFileTypeNotAllowed},
		{"executable double extension", "payload.exe.pdf", "application/pdf", uploadsecurity.CodeFileTypeNotAllowed},
		{"script double extension", "payload.js.txt", "text/plain", uploadsecurity.CodeFileTypeNotAllowed},
		{"HTML double extension", "payload.html.pdf", "application/pdf", uploadsecurity.CodeFileTypeNotAllowed},
		{"SVG double extension", "payload.svg.png", "image/png", uploadsecurity.CodeFileTypeNotAllowed},
		{"archive double extension", "payload.zip.pdf", "application/pdf", uploadsecurity.CodeFileTypeNotAllowed},
	}

	validator := uploadsecurity.NewManagedFileValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.Validate(context.Background(), uploadsecurity.Input{
				Purpose:      uploadsecurity.PurposeManagedFile,
				FileName:     tt.fileName,
				DeclaredMIME: tt.declaredMIME,
				Size:         int64(len(validPDF)),
				MaxBytes:     1024,
				Reader:       bytesReader(validPDF),
			})
			assertUploadSecurityCode(t, err, tt.wantCode)
		})
	}
}

func bytesReader(content []byte) io.Reader {
	return &readOnlyBytes{content: content}
}

type readOnlyBytes struct {
	content []byte
	offset  int
}

func (r *readOnlyBytes) Read(buffer []byte) (int, error) {
	if r.offset >= len(r.content) {
		return 0, io.EOF
	}
	n := copy(buffer, r.content[r.offset:])
	r.offset += n
	return n, nil
}

func assertUploadSecurityCode(t *testing.T, err error, want uploadsecurity.Code) {
	t.Helper()
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != want {
		t.Fatalf("error code = %q, classified=%v, want %q (error: %v)", code, ok, want, err)
	}
}
