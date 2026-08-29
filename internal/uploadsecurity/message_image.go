package uploadsecurity

import (
	"context"
	"image"
	"io"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

const (
	MaxMessageImageBytes int64 = 5 * 1024 * 1024
	MaxMessageImageSide        = 4096
)

type messageImageValidator struct{}

// NewMessageImageValidator returns the validator for the dedicated message
// image purpose. It accepts only validated JPEG, PNG, and WebP content.
func NewMessageImageValidator() Validator {
	return messageImageValidator{}
}

func (messageImageValidator) Validate(ctx context.Context, input Input) (Result, error) {
	if input.Purpose != PurposeMessageImage {
		return Result{}, NewError(CodeUploadBodyInvalid, nil)
	}
	maxBytes := MaxMessageImageBytes
	if input.MaxBytes > 0 && input.MaxBytes < maxBytes {
		maxBytes = input.MaxBytes
	}
	if input.Size > maxBytes {
		return Result{}, NewError(CodeFileTooLarge, nil)
	}
	if err := ValidateExtensionChain(input.FileName); err != nil {
		return Result{}, err
	}

	fileName := path.Base(strings.ReplaceAll(input.FileName, `\`, "/"))
	expectedType, ok := LookupTypeByExtension(path.Ext(fileName))
	if !ok || !isMessageImageType(expectedType) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	declaredType, ok := LookupTypeByMIME(input.DeclaredMIME)
	if !ok || !isMessageImageType(declaredType) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	if declaredType != expectedType {
		return Result{}, NewError(CodeFileTypeMismatch, nil)
	}

	staged, err := Stage(ctx, input.Reader, maxBytes)
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
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return Result{}, NewError(CodeFileContentInvalid, err)
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
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return Result{}, NewError(CodeFileContentInvalid, err)
	}
	if err := validateMessageImageDimensions(staged); err != nil {
		return Result{}, err
	}

	definition, err := ResolveCanonicalType(TypeEvidence{
		FileName: input.FileName, DeclaredMIME: input.DeclaredMIME,
		DetectedMIME: detectedMIME, ValidatedType: validatedType,
	})
	if err != nil {
		return Result{}, err
	}
	if !isMessageImageType(definition.Type) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	displayName, err := SanitizeDisplayName(input.FileName, PurposeMessageImage, definition.CanonicalExtension)
	if err != nil {
		return Result{}, err
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return Result{}, NewError(CodeFileContentInvalid, err)
	}

	keepStaged = true
	return Result{
		Purpose:            PurposeMessageImage,
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

func isMessageImageType(canonicalType CanonicalType) bool {
	switch canonicalType {
	case TypeJPEG, TypePNG, TypeWebP:
		return true
	default:
		return false
	}
}

func validateMessageImageDimensions(source io.ReadSeeker) error {
	config, _, err := image.DecodeConfig(source)
	if err != nil {
		return NewError(CodeImageDecodeInvalid, err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > MaxMessageImageSide || config.Height > MaxMessageImageSide {
		return NewError(CodeImageDimensionLimit, nil)
	}
	return nil
}
