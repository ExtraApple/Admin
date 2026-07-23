package uploadsecurity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"admin/service/uploadsecurity"
)

type acceptingValidator struct{}

func (acceptingValidator) Validate(_ context.Context, input uploadsecurity.Input) (uploadsecurity.Result, error) {
	return uploadsecurity.Result{
		Purpose:            input.Purpose,
		FileName:           input.FileName,
		CanonicalType:      uploadsecurity.CanonicalType("pdf"),
		CanonicalExtension: ".pdf",
		CanonicalMIME:      "application/pdf",
		DetectedMIME:       "application/pdf",
		Size:               input.Size,
		PolicyVersion:      "file-upload-v1",
	}, nil
}

func TestPurposeValuesAreStable(t *testing.T) {
	if uploadsecurity.PurposeManagedFile != "managed_file" {
		t.Fatalf("managed file purpose: got %q", uploadsecurity.PurposeManagedFile)
	}
	if uploadsecurity.PurposeAvatar != "avatar" {
		t.Fatalf("avatar purpose: got %q", uploadsecurity.PurposeAvatar)
	}
}

func TestValidatorContractAcceptsRestrictedInputAndReturnsCanonicalResult(t *testing.T) {
	input := uploadsecurity.Input{
		Purpose:      uploadsecurity.PurposeManagedFile,
		FileName:     "report.pdf",
		DeclaredMIME: "application/pdf",
		Size:         9,
		MaxBytes:     1024,
		Reader:       bytes.NewBufferString("%PDF-1.7"),
	}

	var validator uploadsecurity.Validator = acceptingValidator{}
	result, err := validator.Validate(context.Background(), input)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result.Purpose != uploadsecurity.PurposeManagedFile {
		t.Fatalf("purpose: got %q", result.Purpose)
	}
	if result.CanonicalType != "pdf" {
		t.Fatalf("canonical type: got %q", result.CanonicalType)
	}
	if result.CanonicalExtension != ".pdf" || result.CanonicalMIME != "application/pdf" {
		t.Fatalf("unexpected canonical result: %+v", result)
	}
	if input.MaxBytes != 1024 {
		t.Fatalf("input limit changed: got %d", input.MaxBytes)
	}
}

func TestManagedFileResultDigestMatchesCompleteReaderBytes(t *testing.T) {
	content := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\r\n")
	result, err := uploadsecurity.NewManagedFileValidator().Validate(
		context.Background(),
		uploadsecurity.Input{
			Purpose:      uploadsecurity.PurposeManagedFile,
			FileName:     "report.pdf",
			DeclaredMIME: "application/pdf",
			Size:         int64(len(content)),
			MaxBytes:     1024,
			Reader:       bytes.NewReader(content),
		},
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if closer, ok := result.Reader.(io.Closer); ok {
		defer closer.Close()
	}

	readBack, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("read validated result: %v", err)
	}
	sum := sha256.Sum256(content)
	wantDigest := hex.EncodeToString(sum[:])
	if string(readBack) != string(content) {
		t.Fatalf("validated reader bytes: got %q, want %q", readBack, content)
	}
	if result.ContentSHA256 != wantDigest {
		t.Fatalf("content sha256: got %q, want %q", result.ContentSHA256, wantDigest)
	}
	if len(result.ContentSHA256) != sha256.Size*2 {
		t.Fatalf("content sha256 length: got %d, want %d", len(result.ContentSHA256), sha256.Size*2)
	}
}
