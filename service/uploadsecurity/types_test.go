package uploadsecurity_test

import (
	"testing"

	"admin/service/uploadsecurity"
)

func TestCanonicalTypeMappings(t *testing.T) {
	tests := []struct {
		extension          string
		mime               string
		wantType           uploadsecurity.CanonicalType
		canonicalExtension string
	}{
		{".jpg", "image/jpeg", uploadsecurity.TypeJPEG, ".jpg"},
		{".jpeg", "image/jpeg", uploadsecurity.TypeJPEG, ".jpg"},
		{".png", "image/png", uploadsecurity.TypePNG, ".png"},
		{".webp", "image/webp", uploadsecurity.TypeWebP, ".webp"},
		{".pdf", "application/pdf", uploadsecurity.TypePDF, ".pdf"},
		{".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", uploadsecurity.TypeDOCX, ".docx"},
		{".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", uploadsecurity.TypeXLSX, ".xlsx"},
		{".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", uploadsecurity.TypePPTX, ".pptx"},
		{".txt", "text/plain", uploadsecurity.TypeTXT, ".txt"},
		{".csv", "text/csv", uploadsecurity.TypeCSV, ".csv"},
	}

	for _, tt := range tests {
		t.Run(string(tt.wantType)+" extension", func(t *testing.T) {
			got, ok := uploadsecurity.LookupTypeByExtension(tt.extension)
			if !ok || got != tt.wantType {
				t.Fatalf("extension %q: got %q, found=%v", tt.extension, got, ok)
			}
		})
		t.Run(string(tt.wantType)+" MIME", func(t *testing.T) {
			got, ok := uploadsecurity.LookupTypeByMIME(tt.mime)
			if !ok || got != tt.wantType {
				t.Fatalf("MIME %q: got %q, found=%v", tt.mime, got, ok)
			}
		})
		t.Run(string(tt.wantType)+" definition", func(t *testing.T) {
			got, ok := uploadsecurity.DefinitionForType(tt.wantType)
			if !ok {
				t.Fatalf("missing definition for %q", tt.wantType)
			}
			if got.CanonicalExtension != tt.canonicalExtension || got.MIME != tt.mime {
				t.Fatalf("definition: got %+v", got)
			}
		})
	}
}

func TestCanonicalTypeMappingsNormalizeCaseAndMIMEParameters(t *testing.T) {
	gotExtension, ok := uploadsecurity.LookupTypeByExtension(".JPEG")
	if !ok || gotExtension != uploadsecurity.TypeJPEG {
		t.Fatalf("uppercase extension: got %q, found=%v", gotExtension, ok)
	}

	gotMIME, ok := uploadsecurity.LookupTypeByMIME("text/plain; charset=utf-8")
	if !ok || gotMIME != uploadsecurity.TypeTXT {
		t.Fatalf("MIME parameters: got %q, found=%v", gotMIME, ok)
	}
}

func TestCanonicalTypeMappingsRejectUnlistedTypes(t *testing.T) {
	for _, extension := range []string{".zip", ".exe", ".doc", ".xls", ".ppt", ".svg"} {
		if got, ok := uploadsecurity.LookupTypeByExtension(extension); ok {
			t.Errorf("extension %q unexpectedly mapped to %q", extension, got)
		}
	}

	for _, mimeType := range []string{
		"application/octet-stream",
		"application/zip",
		"application/msword",
		"text/html",
		"image/svg+xml",
	} {
		if got, ok := uploadsecurity.LookupTypeByMIME(mimeType); ok {
			t.Errorf("MIME %q unexpectedly mapped to %q", mimeType, got)
		}
	}
}
