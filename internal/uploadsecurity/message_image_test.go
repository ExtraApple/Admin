package uploadsecurity_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"admin/internal/uploadsecurity"
)

func TestMessageImageValidatorAcceptsValidatedPNGForDedicatedPurpose(t *testing.T) {
	content := pngContent(t, 4, 3)
	result, err := uploadsecurity.NewMessageImageValidator().Validate(context.Background(), uploadsecurity.Input{Purpose: uploadsecurity.PurposeMessageImage, FileName: "notice.png", DeclaredMIME: "image/png", Size: int64(len(content)), MaxBytes: uploadsecurity.MaxMessageImageBytes, Reader: bytes.NewReader(content)})
	if err != nil || result.Purpose != uploadsecurity.PurposeMessageImage || result.CanonicalType != uploadsecurity.TypePNG || result.CanonicalMIME != "image/png" || result.Size != int64(len(content)) || result.Reader == nil {
		t.Fatalf("Validate() = %#v, %v", result, err)
	}
}

func TestMessageImageValidatorRejectsWrongPurposeAndOversizedDimensions(t *testing.T) {
	validator := uploadsecurity.NewMessageImageValidator()
	content := pngContent(t, 4, 3)
	if _, err := validator.Validate(context.Background(), uploadsecurity.Input{Purpose: uploadsecurity.PurposeManagedFile, FileName: "notice.png", DeclaredMIME: "image/png", Size: int64(len(content)), MaxBytes: uploadsecurity.MaxMessageImageBytes, Reader: bytes.NewReader(content)}); uploadCode(err) != uploadsecurity.CodeUploadBodyInvalid {
		t.Fatalf("wrong purpose error = %v", err)
	}
	tooWide := pngContent(t, uploadsecurity.MaxMessageImageSide+1, 1)
	if _, err := validator.Validate(context.Background(), uploadsecurity.Input{Purpose: uploadsecurity.PurposeMessageImage, FileName: "notice.png", DeclaredMIME: "image/png", Size: int64(len(tooWide)), MaxBytes: uploadsecurity.MaxMessageImageBytes, Reader: bytes.NewReader(tooWide)}); uploadCode(err) != uploadsecurity.CodeImageDimensionLimit {
		t.Fatalf("oversized dimensions error = %v", err)
	}
}
func TestMessageImageValidatorAcceptsJPEGAndWebP(t *testing.T) {
	tests := []struct {
		name          string
		fileName      string
		contentType   string
		canonicalType uploadsecurity.CanonicalType
		content       []byte
	}{
		{"jpeg", "notice.jpg", "image/jpeg", uploadsecurity.TypeJPEG, encodeTestJPEG(t)},
		{"webp", "notice.webp", "image/webp", uploadsecurity.TypeWebP, decodeTestWebP(t)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := uploadsecurity.NewMessageImageValidator().Validate(context.Background(), uploadsecurity.Input{Purpose: uploadsecurity.PurposeMessageImage, FileName: tt.fileName, DeclaredMIME: tt.contentType, Size: int64(len(tt.content)), MaxBytes: uploadsecurity.MaxMessageImageBytes, Reader: bytes.NewReader(tt.content)})
			if err != nil || result.CanonicalType != tt.canonicalType || result.CanonicalMIME != tt.contentType {
				t.Fatalf("Validate() = %#v, %v", result, err)
			}
		})
	}
}

func TestMessageImageValidatorRejectsOversizedAndTruncatedImages(t *testing.T) {
	validator := uploadsecurity.NewMessageImageValidator()
	tooLarge := make([]byte, uploadsecurity.MaxMessageImageBytes+1)
	if _, err := validator.Validate(context.Background(), uploadsecurity.Input{Purpose: uploadsecurity.PurposeMessageImage, FileName: "notice.png", DeclaredMIME: "image/png", Size: int64(len(tooLarge)), MaxBytes: uploadsecurity.MaxMessageImageBytes, Reader: bytes.NewReader(tooLarge)}); uploadCode(err) != uploadsecurity.CodeFileTooLarge {
		t.Fatalf("oversized image error = %v", err)
	}
	truncated := pngContent(t, 4, 3)
	truncated = truncated[:len(truncated)-1]
	if _, err := validator.Validate(context.Background(), uploadsecurity.Input{Purpose: uploadsecurity.PurposeMessageImage, FileName: "notice.png", DeclaredMIME: "image/png", Size: int64(len(truncated)), MaxBytes: uploadsecurity.MaxMessageImageBytes, Reader: bytes.NewReader(truncated)}); uploadCode(err) != uploadsecurity.CodeImageDecodeInvalid {
		t.Fatalf("truncated image error = %v", err)
	}
}

func pngContent(t *testing.T, width, height int) []byte {
	t.Helper()
	imageData := image.NewRGBA(image.Rect(0, 0, width, height))
	imageData.Set(0, 0, color.RGBA{R: 255, A: 255})
	var content bytes.Buffer
	if err := png.Encode(&content, imageData); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return content.Bytes()
}

func uploadCode(err error) uploadsecurity.Code {
	code, _ := uploadsecurity.CodeOf(err)
	return code
}
