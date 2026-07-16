package uploadsecurity_test

import (
	"testing"

	"admin/service/uploadsecurity"
)

func TestResolveCanonicalTypeRequiresAllEvidenceToAgree(t *testing.T) {
	got, err := uploadsecurity.ResolveCanonicalType(uploadsecurity.TypeEvidence{
		FileName:      "report.PDF",
		DeclaredMIME:  "application/pdf",
		DetectedMIME:  "application/pdf",
		ValidatedType: uploadsecurity.TypePDF,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Type != uploadsecurity.TypePDF ||
		got.CanonicalExtension != ".pdf" ||
		got.MIME != "application/pdf" {
		t.Fatalf("definition: got %+v", got)
	}
}

func TestResolveCanonicalTypeNormalizesJPEGExtension(t *testing.T) {
	got, err := uploadsecurity.ResolveCanonicalType(uploadsecurity.TypeEvidence{
		FileName:      "photo.jpeg",
		DeclaredMIME:  "image/jpeg",
		DetectedMIME:  "image/jpeg",
		ValidatedType: uploadsecurity.TypeJPEG,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.CanonicalExtension != ".jpg" {
		t.Fatalf("canonical extension: got %q", got.CanonicalExtension)
	}
}

func TestResolveCanonicalTypeRejectsMismatchedEvidence(t *testing.T) {
	tests := []struct {
		name     string
		evidence uploadsecurity.TypeEvidence
	}{
		{
			name: "declared MIME differs",
			evidence: uploadsecurity.TypeEvidence{
				FileName: "image.jpg", DeclaredMIME: "image/png",
				DetectedMIME: "image/jpeg", ValidatedType: uploadsecurity.TypeJPEG,
			},
		},
		{
			name: "detected MIME differs",
			evidence: uploadsecurity.TypeEvidence{
				FileName: "image.jpg", DeclaredMIME: "image/jpeg",
				DetectedMIME: "image/png", ValidatedType: uploadsecurity.TypeJPEG,
			},
		},
		{
			name: "validator result differs",
			evidence: uploadsecurity.TypeEvidence{
				FileName: "image.jpg", DeclaredMIME: "image/jpeg",
				DetectedMIME: "image/jpeg", ValidatedType: uploadsecurity.TypePNG,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uploadsecurity.ResolveCanonicalType(tt.evidence)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeFileTypeMismatch {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestResolveCanonicalTypeRejectsUnlistedOrDangerousEvidence(t *testing.T) {
	tests := []uploadsecurity.TypeEvidence{
		{
			FileName: "archive.zip", DeclaredMIME: "application/zip",
			DetectedMIME: "application/zip", ValidatedType: uploadsecurity.CanonicalType("zip"),
		},
		{
			FileName: "report.pdf", DeclaredMIME: "application/octet-stream",
			DetectedMIME: "application/pdf", ValidatedType: uploadsecurity.TypePDF,
		},
		{
			FileName: "payload.exe.pdf", DeclaredMIME: "application/pdf",
			DetectedMIME: "application/pdf", ValidatedType: uploadsecurity.TypePDF,
		},
	}

	for _, evidence := range tests {
		_, err := uploadsecurity.ResolveCanonicalType(evidence)
		code, ok := uploadsecurity.CodeOf(err)
		if !ok || code != uploadsecurity.CodeFileTypeNotAllowed {
			t.Fatalf("%q code: got %q, classified=%v", evidence.FileName, code, ok)
		}
	}
}

func TestResolveCanonicalTypeRequiresDedicatedValidationResult(t *testing.T) {
	_, err := uploadsecurity.ResolveCanonicalType(uploadsecurity.TypeEvidence{
		FileName:     "report.pdf",
		DeclaredMIME: "application/pdf",
		DetectedMIME: "application/pdf",
	})
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileContentInvalid {
		t.Fatalf("code: got %q, classified=%v", code, ok)
	}
}
