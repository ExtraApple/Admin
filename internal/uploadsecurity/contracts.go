// Package uploadsecurity defines upload validation contracts shared by managed
// files and user avatars. It intentionally has no dependency on HTTP handlers
// or object storage clients.
package uploadsecurity

import (
	"context"
	"io"
)

// Purpose identifies the policy applied to an upload.
type Purpose string

const (
	PurposeManagedFile  Purpose = "managed_file"
	PurposeAvatar       Purpose = "avatar"
	PurposeMessageImage Purpose = "message_image"
	PolicyVersionV1             = "file-upload-v1"
)

// CanonicalType identifies a file type after all declarations and content
// checks have been normalized to the same policy type.
type CanonicalType string

// Input is the bounded upload data presented to a Validator.
type Input struct {
	Purpose      Purpose
	FileName     string
	DeclaredMIME string
	Size         int64
	MaxBytes     int64
	Reader       io.Reader
}

// Result contains trusted metadata and the data stream that may be persisted
// after validation succeeds.
type Result struct {
	Purpose            Purpose
	FileName           string
	CanonicalType      CanonicalType
	CanonicalExtension string
	CanonicalMIME      string
	DetectedMIME       string
	Size               int64
	ContentSHA256      string
	PolicyVersion      string
	Reader             io.Reader
}

// Validator validates one bounded upload without depending on Gin or MinIO.
type Validator interface {
	Validate(ctx context.Context, input Input) (Result, error)
}
