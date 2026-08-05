package uploadsecurity

import (
	"context"
	"io"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

type managedFileValidator struct{}

// NewManagedFileValidator returns the V1 validator for administrator-managed
// files. Successful results own a staged Reader that callers should close when
// it also implements io.Closer.
func NewManagedFileValidator() Validator {
	return managedFileValidator{}
}

func (managedFileValidator) Validate(
	ctx context.Context,
	input Input,
) (Result, error) {
	if input.Purpose != PurposeManagedFile {
		return Result{}, NewError(CodeUploadBodyInvalid, nil)
	}
	if input.Size > input.MaxBytes {
		return Result{}, NewError(CodeFileTooLarge, nil)
	}
	if err := ValidateExtensionChain(input.FileName); err != nil {
		return Result{}, err
	}

	fileName := path.Base(strings.ReplaceAll(input.FileName, `\`, "/"))
	expectedType, ok := LookupTypeByExtension(path.Ext(fileName))
	if !ok || !IsManagedFileType(expectedType) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	declaredType, ok := LookupTypeByMIME(input.DeclaredMIME)
	if !ok || !IsManagedFileType(declaredType) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	if declaredType != expectedType {
		return Result{}, NewError(CodeFileTypeMismatch, nil)
	}

	staged, err := Stage(ctx, input.Reader, input.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	keepStaged := false
	defer func() {
		if !keepStaged {
			_ = staged.Close()
		}
	}()

	if input.Size >= 0 && input.Size != staged.Size() {
		return Result{}, NewError(CodeUploadBodyInvalid, nil)
	}

	detected, err := mimetype.DetectReader(staged)
	if err != nil {
		return Result{}, NewError(CodeFileContentInvalid, err)
	}
	detectedMIME := detected.String()

	validatedType, err := ValidateContent(staged, expectedType)
	if err != nil {
		return Result{}, err
	}

	definition, err := ResolveCanonicalType(TypeEvidence{
		FileName:      input.FileName,
		DeclaredMIME:  input.DeclaredMIME,
		DetectedMIME:  detectedMIME,
		ValidatedType: validatedType,
	})
	if err != nil {
		return Result{}, err
	}
	if !IsManagedFileType(definition.Type) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}

	displayName, err := SanitizeDisplayName(
		input.FileName,
		PurposeManagedFile,
		definition.CanonicalExtension,
	)
	if err != nil {
		return Result{}, err
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return Result{}, NewError(CodeFileContentInvalid, err)
	}

	keepStaged = true
	return Result{
		Purpose:            PurposeManagedFile,
		FileName:           displayName,
		CanonicalType:      definition.Type,
		CanonicalExtension: definition.CanonicalExtension,
		CanonicalMIME:      definition.MIME,
		DetectedMIME:       detectedMIME,
		Size:               staged.Size(),
		ContentSHA256:      staged.ContentSHA256(),
		PolicyVersion:      PolicyVersionV1,
		Reader:             staged,
	}, nil
}
