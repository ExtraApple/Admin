package uploadsecurity_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"admin/service/uploadsecurity"
)

func TestValidateContentAcceptsCompleteStaticImages(t *testing.T) {
	tests := []struct {
		name          string
		canonicalType uploadsecurity.CanonicalType
		content       []byte
	}{
		{"jpeg", uploadsecurity.TypeJPEG, encodeTestJPEG(t)},
		{"png", uploadsecurity.TypePNG, encodeTestPNG(t)},
		{"webp", uploadsecurity.TypeWebP, decodeTestWebP(t)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := uploadsecurity.ValidateContent(bytes.NewReader(tt.content), tt.canonicalType)
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if got != tt.canonicalType {
				t.Fatalf("validated type: got %q, want %q", got, tt.canonicalType)
			}
		})
	}
}

func TestValidateContentRejectsCorruptOrMismatchedImages(t *testing.T) {
	_, err := uploadsecurity.ValidateContent(bytes.NewReader([]byte("\xff\xd8broken")), uploadsecurity.TypeJPEG)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeImageDecodeInvalid {
		t.Fatalf("corrupt image code: got %q, classified=%v", code, ok)
	}

	_, err = uploadsecurity.ValidateContent(bytes.NewReader(encodeTestPNG(t)), uploadsecurity.TypeJPEG)
	code, ok = uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileTypeMismatch {
		t.Fatalf("mismatched image code: got %q, classified=%v", code, ok)
	}
}

func TestValidateContentChecksPDFHeaderAndEOFMarker(t *testing.T) {
	valid := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\r\n")
	got, err := uploadsecurity.ValidateContent(bytes.NewReader(valid), uploadsecurity.TypePDF)
	if err != nil {
		t.Fatalf("validate PDF: %v", err)
	}
	if got != uploadsecurity.TypePDF {
		t.Fatalf("validated type: got %q", got)
	}

	invalid := [][]byte{
		[]byte("not a pdf\n%%EOF"),
		[]byte("%PDF-1.7\nmissing eof"),
	}
	for _, content := range invalid {
		_, err := uploadsecurity.ValidateContent(bytes.NewReader(content), uploadsecurity.TypePDF)
		code, ok := uploadsecurity.CodeOf(err)
		if !ok || code != uploadsecurity.CodeFileContentInvalid {
			t.Fatalf("invalid PDF code: got %q, classified=%v", code, ok)
		}
	}
}

func TestValidateContentAcceptsStreamingUTF8TextWithOptionalBOM(t *testing.T) {
	tests := []struct {
		canonicalType uploadsecurity.CanonicalType
		content       []byte
	}{
		{uploadsecurity.TypeTXT, []byte("plain UTF-8 文本\n")},
		{uploadsecurity.TypeTXT, append([]byte{0xef, 0xbb, 0xbf}, []byte("带 BOM 文本")...)},
		{uploadsecurity.TypeCSV, []byte("name,value\n测试,1\n")},
	}

	for _, tt := range tests {
		got, err := uploadsecurity.ValidateContent(bytes.NewReader(tt.content), tt.canonicalType)
		if err != nil {
			t.Fatalf("validate %q: %v", tt.canonicalType, err)
		}
		if got != tt.canonicalType {
			t.Fatalf("validated type: got %q", got)
		}
	}
}

func TestValidateContentRejectsInvalidTextEncodingAndNUL(t *testing.T) {
	invalid := [][]byte{
		{0xff, 0xfe, 'a', 0},
		{0xfe, 0xff, 0, 'a'},
		{'a', 0, 'b'},
		{0xff, 0xff},
	}

	for _, content := range invalid {
		_, err := uploadsecurity.ValidateContent(bytes.NewReader(content), uploadsecurity.TypeTXT)
		code, ok := uploadsecurity.CodeOf(err)
		if !ok || code != uploadsecurity.CodeFileEncodingInvalid {
			t.Fatalf("% x code: got %q, classified=%v", content, code, ok)
		}
	}
}

func encodeTestJPEG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(&buffer, img, nil); err != nil {
		t.Fatalf("encode JPEG: %v", err)
	}
	return buffer.Bytes()
}

func encodeTestPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return buffer.Bytes()
}

func decodeTestWebP(t *testing.T) []byte {
	t.Helper()
	const fixture = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
	content, err := base64.StdEncoding.DecodeString(fixture)
	if err != nil {
		t.Fatalf("decode WebP fixture: %v", err)
	}
	return content
}
