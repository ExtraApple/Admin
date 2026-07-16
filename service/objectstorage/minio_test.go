package objectstorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type minioTransport struct {
	content []byte
	deleted bool
}

func (t *minioTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	switch {
	case request.Method == http.MethodPut:
		content, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		t.content = content
		return minioResponse(request, http.StatusOK, "", http.Header{
			"ETag": []string{`"test-etag"`},
		}), nil
	case request.Method == http.MethodGet && request.URL.Query().Get("list-type") == "2":
		body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>files</Name>
  <Prefix>generated-</Prefix>
  <KeyCount>1</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>generated-id.pdf</Key>
    <LastModified>2023-11-14T22:13:20Z</LastModified>
    <ETag>&quot;test-etag&quot;</ETag>
    <Size>%d</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`, len(t.content))
		return minioResponse(request, http.StatusOK, body, http.Header{
			"Content-Type": []string{"application/xml"},
		}), nil
	case request.Method == http.MethodGet:
		return minioResponse(request, http.StatusOK, string(t.content), objectHeaders(int64(len(t.content)))), nil
	case request.Method == http.MethodHead:
		return minioResponse(request, http.StatusOK, "", objectHeaders(int64(len(t.content)))), nil
	case request.Method == http.MethodDelete:
		t.deleted = true
		return minioResponse(request, http.StatusNoContent, "", nil), nil
	default:
		return nil, fmt.Errorf("unexpected MinIO request: %s %s", request.Method, request.URL.String())
	}
}

func minioResponse(request *http.Request, status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func objectHeaders(size int64) http.Header {
	return http.Header{
		"Content-Length": []string{strconv.FormatInt(size, 10)},
		"Content-Type":   []string{"application/pdf"},
		"ETag":           []string{`"test-etag"`},
		"Last-Modified":  []string{time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat)},
	}
}

func TestMinIOStoreImplementsStreamingStorageOperations(t *testing.T) {
	transport := &minioTransport{}
	client, err := minio.New("storage.test", &minio.Options{
		Creds:        credentials.NewStatic("", "", "", credentials.SignatureAnonymous),
		Region:       "us-east-1",
		Transport:    transport,
		BucketLookup: minio.BucketLookupPath,
		MaxRetries:   1,
	})
	if err != nil {
		t.Fatalf("minio.New() error = %v", err)
	}

	var store Store = NewMinIOStore(client)
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
	if uploaded.Bucket != input.Bucket || uploaded.Name != input.Name ||
		uploaded.Size != input.Size || uploaded.ContentType != input.ContentType {
		t.Fatalf("Put() object = %#v, want input metadata", uploaded)
	}
	if string(transport.content) != "safe document" {
		t.Fatalf("MinIO request body = %q, want %q", transport.content, "safe document")
	}

	reader, err := store.Open(ctx, input.Bucket, input.Name)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	content, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("Open() read error = %v", err)
	}
	if string(content) != "safe document" {
		t.Fatalf("Open() content = %q, want %q", content, "safe document")
	}

	info, err := store.Stat(ctx, input.Bucket, input.Name)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Bucket != input.Bucket || info.Name != input.Name ||
		info.Size != input.Size || info.ContentType != input.ContentType {
		t.Fatalf("Stat() object = %#v, want stored metadata", info)
	}

	objects, err := store.List(ctx, input.Bucket, ListOptions{
		Prefix:    "generated-",
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(objects) != 1 || objects[0].Bucket != input.Bucket ||
		objects[0].Name != input.Name || objects[0].Size != input.Size {
		t.Fatalf("List() objects = %#v, want uploaded object metadata", objects)
	}

	if err := store.Delete(ctx, input.Bucket, input.Name); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !transport.deleted {
		t.Fatal("Delete() did not call MinIO")
	}
}
