package app

import (
	"context"

	identityobjectstorage "admin/internal/identity/adapters/objectstorage"
	identityapplication "admin/internal/identity/application"
	"admin/internal/uploadsecurity"
)

func newAvatarService(resources Resources, users identityapplication.AvatarRepository) *identityapplication.AvatarService {
	return identityapplication.NewAvatarService(
		avatarValidator{validator: uploadsecurity.NewAvatarValidator()},
		identityobjectstorage.New(resources.MinIO),
		users,
		nil,
	)
}

type avatarValidator struct {
	validator uploadsecurity.Validator
}

func (adapter avatarValidator) Validate(ctx context.Context, input identityapplication.AvatarValidationInput) (identityapplication.AvatarValidationResult, error) {
	result, err := adapter.validator.Validate(ctx, uploadsecurity.Input{Purpose: uploadsecurity.PurposeAvatar, FileName: input.FileName, DeclaredMIME: input.DeclaredMIME, Size: input.Size, MaxBytes: input.MaxBytes, Reader: input.Reader})
	if err != nil {
		return identityapplication.AvatarValidationResult{}, mapAvatarError(err)
	}
	return identityapplication.AvatarValidationResult{FileName: result.FileName, Size: result.Size, DetectedMIME: result.DetectedMIME, CanonicalMIME: result.CanonicalMIME, CanonicalExtension: result.CanonicalExtension, ContentSHA256: result.ContentSHA256, PolicyVersion: result.PolicyVersion, Reader: result.Reader}, nil
}

func mapAvatarError(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := uploadsecurity.CodeOf(err); ok {
		return identityapplication.NewAvatarError(string(code), err)
	}
	return err
}

var _ identityapplication.AvatarValidator = avatarValidator{}
