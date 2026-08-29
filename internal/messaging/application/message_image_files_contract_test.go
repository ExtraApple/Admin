package application_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type messageImageFilesFake struct {
	uploaded application.TemporaryMessageImage
	bound    application.MessageImageBinding
	content  application.MessageImageContent
}

func (fake *messageImageFilesFake) UploadMessageImage(_ context.Context, _ application.MessageImageUpload) (application.TemporaryMessageImage, error) {
	return fake.uploaded, nil
}

func (fake *messageImageFilesFake) BindMessageImages(_ context.Context, binding application.MessageImageBinding) error {
	fake.bound = binding
	return nil
}

func (fake *messageImageFilesFake) OpenMessageImage(_ context.Context, _ application.VisibleMessageImage) (application.MessageImageContent, error) {
	return fake.content, nil
}

var _ application.MessageImageFiles = (*messageImageFilesFake)(nil)

func TestMessageImageFilesContractBindsTemporaryImagesToLogicalMessage(t *testing.T) {
	expiresAt := time.Date(2026, 8, 24, 1, 15, 0, 0, time.UTC)
	files := &messageImageFilesFake{
		uploaded: application.TemporaryMessageImage{ID: 8, ExpiresAt: expiresAt},
		content:  application.MessageImageContent{Reader: io.NopCloser(strings.NewReader("image")), ContentType: "image/png", Size: 5},
	}
	image, err := files.UploadMessageImage(context.Background(), application.MessageImageUpload{UploaderID: 7, FileName: "notice.png", ContentType: "image/png", Size: 5, Reader: strings.NewReader("image")})
	if err != nil || image.ID != 8 || !image.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("UploadMessageImage() = %#v, %v", image, err)
	}
	binding := application.MessageImageBinding{ActorID: 7, MessageLogicalID: "db9c4f50-af2e-4c7d-8f7d-55a55ec1f614", ImageIDs: []uint{image.ID}}
	if err := files.BindMessageImages(context.Background(), binding); err != nil {
		t.Fatalf("BindMessageImages() = %v", err)
	}
	if files.bound.ActorID != binding.ActorID || files.bound.MessageLogicalID != binding.MessageLogicalID || len(files.bound.ImageIDs) != 1 || files.bound.ImageIDs[0] != image.ID {
		t.Fatalf("bound = %#v", files.bound)
	}
	content, err := files.OpenMessageImage(context.Background(), application.VisibleMessageImage{ID: image.ID, MessageLogicalID: binding.MessageLogicalID})
	if err != nil || content.ContentType != "image/png" || content.Size != 5 {
		t.Fatalf("OpenMessageImage() = %#v, %v", content, err)
	}
	_ = content.Reader.Close()
}
