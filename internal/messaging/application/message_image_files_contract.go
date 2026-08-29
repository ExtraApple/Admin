package application

import (
	"context"
	"io"
	"time"
)

// MessageImageFiles is the Messaging-owned view of Files. It provides only
// temporary media upload, atomic message binding, and an already-authorized
// content read; it exposes neither storage addresses nor Files models.
type MessageImageFiles interface {
	UploadMessageImage(context.Context, MessageImageUpload) (TemporaryMessageImage, error)
	BindMessageImages(context.Context, MessageImageBinding) error
	OpenMessageImage(context.Context, VisibleMessageImage) (MessageImageContent, error)
}

// MessageImageUpload is an unbound image owned by its uploader until it is
// bound to a logical message. Files determines the binding deadline.
type MessageImageUpload struct {
	UploaderID            uint
	FileName, ContentType string
	Size                  int64
	Reader                io.Reader
}

// TemporaryMessageImage is a validated unbound File Record. It expires unless
// an eligible actor binds it to a logical message before ExpiresAt.
type TemporaryMessageImage struct {
	ID        uint
	ExpiresAt time.Time
}

// MessageImageBinding atomically gives the selected temporary images one
// logical message owner. Files accepts only unexpired images owned by ActorID.
type MessageImageBinding struct {
	ActorID          uint
	MessageLogicalID string
	ImageIDs         []uint
}

// VisibleMessageImage is constructed only after Messaging has determined that
// the current viewer may see MessageLogicalID. Files verifies that the image
// is bound to that exact logical message before opening it.
type VisibleMessageImage struct {
	ID               uint
	MessageLogicalID string
}

// MessageImageContent contains a validated image stream and canonical MIME.
type MessageImageContent struct {
	Reader      io.ReadCloser
	ContentType string
	Size        int64
}
