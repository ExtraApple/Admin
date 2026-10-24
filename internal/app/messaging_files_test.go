package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	filesapplication "admin/internal/files/application"
	filesdomain "admin/internal/files/domain"
	messagingapplication "admin/internal/messaging/application"
	"admin/internal/uploadsecurity"
)

type messagingImageRepositoryProbe struct {
	filesapplication.MessageImageRepository
	file    filesdomain.File
	findErr error
}

func (probe messagingImageRepositoryProbe) FindMessageImage(context.Context, uint) (filesdomain.File, error) {
	if probe.findErr != nil {
		return filesdomain.File{}, probe.findErr
	}
	return probe.file, nil
}

type messagingImageStorageProbe struct {
	openErr error
}

func (messagingImageStorageProbe) Put(context.Context, filesapplication.ObjectInput) error {
	return nil
}

func (messagingImageStorageProbe) Stat(context.Context, string, string) (filesapplication.Object, error) {
	return filesapplication.Object{}, nil
}

func (probe messagingImageStorageProbe) Open(context.Context, string, string) (io.ReadCloser, error) {
	if probe.openErr != nil {
		return nil, probe.openErr
	}
	return io.NopCloser(bytes.NewReader([]byte("image"))), nil
}

func (messagingImageStorageProbe) Delete(context.Context, string, string) error { return nil }
func (messagingImageStorageProbe) List(context.Context, string, filesapplication.ListOptions) ([]filesapplication.Object, error) {
	return nil, nil
}
func (messagingImageStorageProbe) Move(context.Context, string, string, string) error { return nil }

func TestMessagingFilesReaderMapsFilesImageErrorsAtModuleBoundary(t *testing.T) {
	validFile := filesdomain.File{Purpose: "message_image", LogicalMessageID: "logical", ValidationStatus: filesdomain.ValidationStatusValidated, ContentType: "image/png", Bucket: "files", ObjectName: "message-images/image.png", Size: 5}
	cases := []struct {
		name    string
		findErr error
		openErr error
		want    error
	}{
		{name: "file access invalid", findErr: filesapplication.ErrFileNotFound, want: messagingapplication.ErrNotFound},
		{name: "storage object missing", openErr: uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, nil), want: messagingapplication.ErrNotFound},
		{name: "storage unavailable", openErr: uploadsecurity.NewError(uploadsecurity.CodeStorageUnavailable, nil), want: messagingapplication.ErrMessagingDependency},
		{name: "persistence failure", findErr: errors.New("database details"), want: messagingapplication.ErrMessagingDependency},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service := filesapplication.NewService(filesapplication.Dependencies{
				MessageImages: messagingImageRepositoryProbe{file: validFile, findErr: test.findErr},
				Storage:       messagingImageStorageProbe{openErr: test.openErr},
			})
			content, err := (messagingFilesReader{service: service}).OpenMessageImage(context.Background(), messagingapplication.VisibleMessageImage{ID: 9, MessageLogicalID: "logical"})
			if !errors.Is(err, test.want) || content.Reader != nil {
				t.Fatalf("content=%#v error=%v want=%v", content, err, test.want)
			}
			if strings := err.Error(); strings == "database details" {
				t.Fatal("Files persistence error crossed the module boundary")
			}
		})
	}
}

func TestMessagingFilesReaderPreservesValidatedImageContent(t *testing.T) {
	service := filesapplication.NewService(filesapplication.Dependencies{
		MessageImages: messagingImageRepositoryProbe{file: filesdomain.File{Purpose: "message_image", LogicalMessageID: "logical", ValidationStatus: filesdomain.ValidationStatusValidated, ContentType: "image/webp", Bucket: "files", ObjectName: "message-images/image.webp", Size: 5}},
		Storage:       messagingImageStorageProbe{},
	})
	content, err := (messagingFilesReader{service: service}).OpenMessageImage(context.Background(), messagingapplication.VisibleMessageImage{ID: 9, MessageLogicalID: "logical"})
	if err != nil || content.ContentType != "image/webp" || content.Size != 5 || content.Reader == nil {
		t.Fatalf("content=%#v error=%v", content, err)
	}
	if err := content.Reader.Close(); err != nil {
		t.Fatal(err)
	}
}
