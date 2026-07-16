package objectstorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"admin/service/uploadsecurity"
)

type minioErrorTransport struct {
	status int
	code   string
	detail string
}

func (t minioErrorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := fmt.Sprintf(
		`<Error><Code>%s</Code><Message>%s</Message><BucketName>private-bucket</BucketName><Key>private-key</Key></Error>`,
		t.code,
		t.detail,
	)
	return &http.Response{
		StatusCode: t.status,
		Status:     strconv.Itoa(t.status) + " " + http.StatusText(t.status),
		Header: http.Header{
			"Content-Type": []string{"application/xml"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: request,
	}, nil
}

func newMinIOErrorStore(t *testing.T, transport http.RoundTripper) Store {
	t.Helper()
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
	return NewMinIOStore(client)
}

func TestMinIOStoreNormalizesMissingObjectErrors(t *testing.T) {
	const providerDetail = "missing private-key with access-key=minio-secret"
	store := newMinIOErrorStore(t, minioErrorTransport{
		status: http.StatusNotFound,
		code:   minio.NoSuchKey,
		detail: providerDetail,
	})

	_, err := store.Stat(context.Background(), "files", "generated-id.pdf")
	assertStorageCode(t, err, uploadsecurity.CodeStorageObjectNotFound, providerDetail)
}

func TestMinIOStoreDoesNotClassifyBucketOrUnknown404AsMissingObject(t *testing.T) {
	for _, testCase := range []struct {
		name string
		code string
	}{
		{
			name: "missing bucket",
			code: minio.NoSuchBucket,
		},
		{
			name: "unknown not found response",
			code: "RouteNotFound",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			const providerDetail = "storage routing failure with access-key=minio-secret"
			store := newMinIOErrorStore(t, minioErrorTransport{
				status: http.StatusNotFound,
				code:   testCase.code,
				detail: providerDetail,
			})

			_, err := store.Stat(context.Background(), "files", "generated-id.pdf")
			assertStorageCode(
				t,
				err,
				uploadsecurity.CodeStorageUnavailable,
				providerDetail,
			)
		})
	}
}

func TestMinIOStoreNormalizesUnavailableErrors(t *testing.T) {
	const providerDetail = "backend unavailable at storage.internal with access-key=minio-secret"
	store := newMinIOErrorStore(t, minioErrorTransport{
		status: http.StatusServiceUnavailable,
		code:   "ServiceUnavailable",
		detail: providerDetail,
	})

	_, err := store.Stat(context.Background(), "files", "generated-id.pdf")
	assertStorageCode(t, err, uploadsecurity.CodeStorageUnavailable, providerDetail)
}

func TestMinIOStoreNormalizesErrorsRaisedDuringStreamingRead(t *testing.T) {
	const providerDetail = "missing private-key with access-key=minio-secret"
	store := newMinIOErrorStore(t, minioErrorTransport{
		status: http.StatusNotFound,
		code:   minio.NoSuchKey,
		detail: providerDetail,
	})

	reader, err := store.Open(context.Background(), "files", "generated-id.pdf")
	if err != nil {
		t.Fatalf("Open() error = %v, want lazy streaming reader", err)
	}
	defer reader.Close()

	buffer := make([]byte, 1)
	_, err = reader.Read(buffer)
	assertStorageCode(t, err, uploadsecurity.CodeStorageObjectNotFound, providerDetail)
}

func assertStorageCode(t *testing.T, err error, want uploadsecurity.Code, providerDetail string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	got, ok := uploadsecurity.CodeOf(err)
	if !ok || got != want {
		t.Fatalf("error code = %q, found=%v, want %q (error: %v)", got, ok, want, err)
	}
	if strings.Contains(err.Error(), providerDetail) ||
		strings.Contains(err.Error(), "private-key") ||
		strings.Contains(err.Error(), "minio-secret") {
		t.Fatalf("public error %q leaked provider details", err.Error())
	}
}
