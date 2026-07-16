package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin/service/uploadsecurity"
)

func newMultipartUploadRequest(
	t *testing.T,
	target, fileName string,
	fileSize int64,
) *http.Request {
	t.Helper()

	return newMultipartRequest(t, target, []multipartUploadPart{{
		FieldName: "file",
		FileName:  fileName,
		Content:   make([]byte, int(fileSize)),
	}})
}

type multipartUploadPart struct {
	FieldName string
	FileName  string
	Content   []byte
}

func newMultipartRequest(
	t *testing.T,
	target string,
	parts []multipartUploadPart,
) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, uploadPart := range parts {
		part, err := writer.CreateFormFile(uploadPart.FieldName, uploadPart.FileName)
		if err != nil {
			t.Fatalf("create multipart file: %v", err)
		}
		if _, err := part.Write(uploadPart.Content); err != nil {
			t.Fatalf("write multipart file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func assertUploadErrorResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantCode uploadsecurity.Code,
) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			recorder.Code,
			wantStatus,
			recorder.Body.String(),
		)
	}

	var response struct {
		Code      int                 `json:"code"`
		ErrorCode uploadsecurity.Code `json:"error_code"`
		Msg       string              `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != wantStatus {
		t.Fatalf("numeric code = %d, want %d", response.Code, wantStatus)
	}
	if response.ErrorCode != wantCode {
		t.Fatalf("error_code = %q, want %q", response.ErrorCode, wantCode)
	}
	if response.Msg == "" {
		t.Fatal("msg should not be empty")
	}
}
