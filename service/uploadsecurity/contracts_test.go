package uploadsecurity_test

import (
	"bytes"
	"context"
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
