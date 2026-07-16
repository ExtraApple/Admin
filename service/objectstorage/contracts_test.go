package objectstorage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeObject struct {
	info ObjectInfo
	data []byte
}

type fakeStore struct {
	objects map[string]fakeObject
}

func newFakeStore() *fakeStore {
	return &fakeStore{objects: make(map[string]fakeObject)}
}

func (s *fakeStore) Put(_ context.Context, input PutInput) (ObjectInfo, error) {
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return ObjectInfo{}, err
	}

	info := ObjectInfo{
		Bucket:       input.Bucket,
		Name:         input.Name,
		Size:         int64(len(content)),
		ContentType:  input.ContentType,
		LastModified: time.Unix(1_700_000_000, 0).UTC(),
	}
	s.objects[input.Bucket+"/"+input.Name] = fakeObject{
		info: info,
		data: append([]byte(nil), content...),
	}
	return info, nil
}

func (s *fakeStore) Open(_ context.Context, bucket, name string) (io.ReadCloser, error) {
	object := s.objects[bucket+"/"+name]
	return io.NopCloser(bytes.NewReader(object.data)), nil
}

func (s *fakeStore) Stat(_ context.Context, bucket, name string) (ObjectInfo, error) {
	return s.objects[bucket+"/"+name].info, nil
}

func (s *fakeStore) Delete(_ context.Context, bucket, name string) error {
	delete(s.objects, bucket+"/"+name)
	return nil
}

func (s *fakeStore) List(_ context.Context, bucket string, options ListOptions) ([]ObjectInfo, error) {
	objects := make([]ObjectInfo, 0)
	for _, object := range s.objects {
		if object.info.Bucket == bucket && strings.HasPrefix(object.info.Name, options.Prefix) {
			objects = append(objects, object.info)
		}
	}
	return objects, nil
}

func TestStoreCanBeReplacedWithAServiceFake(t *testing.T) {
	var store Store = newFakeStore()
	ctx := context.Background()
	input := PutInput{
		Bucket:      "files",
		Name:        "generated-id.pdf",
		Reader:      strings.NewReader("safe document"),
		Size:        int64(len("safe document")),
		ContentType: "application/pdf",
	}

	uploaded, err := store.Put(ctx, input)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if uploaded.Bucket != input.Bucket || uploaded.Name != input.Name {
		t.Fatalf("Put() object = %#v, want bucket %q and name %q", uploaded, input.Bucket, input.Name)
	}

	reader, err := store.Open(ctx, input.Bucket, input.Name)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(content) != "safe document" {
		t.Fatalf("Open() content = %q, want %q", content, "safe document")
	}

	info, err := store.Stat(ctx, input.Bucket, input.Name)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.ContentType != input.ContentType || info.Size != input.Size {
		t.Fatalf("Stat() object = %#v, want MIME %q and size %d", info, input.ContentType, input.Size)
	}

	objects, err := store.List(ctx, input.Bucket, ListOptions{Prefix: "generated-"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(objects) != 1 || objects[0].Name != input.Name {
		t.Fatalf("List() objects = %#v, want uploaded object", objects)
	}

	if err := store.Delete(ctx, input.Bucket, input.Name); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	objects, err = store.List(ctx, input.Bucket, ListOptions{})
	if err != nil {
		t.Fatalf("List() after Delete() error = %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("List() after Delete() = %#v, want empty", objects)
	}
}
