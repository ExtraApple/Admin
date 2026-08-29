package application_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type uploadMessageImageFilesFake struct {
	request application.MessageImageUpload
}

func (fake *uploadMessageImageFilesFake) UploadMessageImage(_ context.Context, request application.MessageImageUpload) (application.TemporaryMessageImage, error) {
	fake.request = request
	return application.TemporaryMessageImage{ID: 9, ExpiresAt: time.Date(2026, 8, 24, 1, 15, 0, 0, time.UTC)}, nil
}
func (*uploadMessageImageFilesFake) BindMessageImages(context.Context, application.MessageImageBinding) error {
	panic("unused")
}
func (*uploadMessageImageFilesFake) OpenMessageImage(context.Context, application.VisibleMessageImage) (application.MessageImageContent, error) {
	panic("unused")
}

func TestMessageImageUploadApplicationUsesFilesContractAndAuthenticatedUser(t *testing.T) {
	files := &uploadMessageImageFilesFake{}
	service := application.NewService(application.Dependencies{Files: files})
	reader := strings.NewReader("image")
	image, err := service.UploadMessageImage(context.Background(), application.MessageImageUploadRequest{UserID: 7, FileName: "notice.png", ContentType: "image/png", Size: 5, Reader: reader})
	if err != nil || image.ID != 9 || files.request.UploaderID != 7 || files.request.FileName != "notice.png" || files.request.ContentType != "image/png" || files.request.Size != 5 || files.request.Reader != reader {
		t.Fatalf("UploadMessageImage() = %#v, %v request=%#v", image, err, files.request)
	}
}

var _ io.Reader = (*strings.Reader)(nil)
