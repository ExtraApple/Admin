package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"admin/dto"
	"admin/model"
	"admin/service"
	"admin/service/fileaccess"
	"admin/service/objectstorage"
	"admin/service/uploadsecurity"
)

type recordingFileDetailGetter struct {
	userID uint
	fileID uint
	result *dto.FileDetailResp
	err    error
}

type recordingFileUploader struct {
	calls   int
	input   service.UploadFileInput
	content []byte
	result  *dto.FileInfo
	err     error
}

type recordingFileContentOpener struct {
	calls  int
	input  service.FileAccessInput
	result *service.FileContent
	err    error
}

type recordingFileRevalidator struct {
	calls  int
	fileID uint
	result *dto.FileInfo
	err    error
}

type closingReadCloser struct {
	io.Reader
	closed bool
}

type firstReadErrorStore struct {
	openCalls  int
	openReader io.ReadCloser
	statResult objectstorage.ObjectInfo
}

type firstReadErrorCloser struct {
	err    error
	closed bool
}

type fileAccessRepositoryStub struct {
	file *model.File
	err  error
}

func (r *closingReadCloser) Close() error {
	r.closed = true
	return nil
}

func (r *firstReadErrorCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r *firstReadErrorCloser) Close() error {
	r.closed = true
	return nil
}

func (s *firstReadErrorStore) Put(
	_ context.Context,
	_ objectstorage.PutInput,
) (objectstorage.ObjectInfo, error) {
	panic("unexpected Put call")
}

func (s *firstReadErrorStore) Open(
	_ context.Context,
	_, _ string,
) (io.ReadCloser, error) {
	s.openCalls++
	return s.openReader, nil
}

func (s *firstReadErrorStore) Stat(
	_ context.Context,
	_, _ string,
) (objectstorage.ObjectInfo, error) {
	return s.statResult, nil
}

func (s *firstReadErrorStore) Delete(
	_ context.Context,
	_, _ string,
) error {
	panic("unexpected Delete call")
}

func (s *firstReadErrorStore) List(
	_ context.Context,
	_ string,
	_ objectstorage.ListOptions,
) ([]objectstorage.ObjectInfo, error) {
	panic("unexpected List call")
}

func (r *fileAccessRepositoryStub) FindByID(
	_ context.Context,
	fileID uint,
) (*model.File, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.file == nil || r.file.ID != fileID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.file
	return &copy, nil
}

func (o *recordingFileContentOpener) Open(
	_ context.Context,
	input service.FileAccessInput,
) (*service.FileContent, error) {
	o.calls++
	o.input = input
	return o.result, o.err
}

func (r *recordingFileRevalidator) Revalidate(
	_ context.Context,
	fileID uint,
) (*dto.FileInfo, error) {
	r.calls++
	r.fileID = fileID
	return r.result, r.err
}

func (u *recordingFileUploader) Upload(
	_ context.Context,
	input service.UploadFileInput,
) (*dto.FileInfo, error) {
	u.calls++
	u.input = input
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return nil, err
	}
	u.content = content
	return u.result, u.err
}

func (g *recordingFileDetailGetter) Get(
	_ context.Context,
	userID, fileID uint,
) (*dto.FileDetailResp, error) {
	g.userID = userID
	g.fileID = fileID
	return g.result, g.err
}

func TestFileHandlerDownloadStreamsControlledAttachmentWithSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reader := &closingReadCloser{Reader: strings.NewReader("safe file content")}
	content := &service.FileContent{
		FileName:    "季度 报告.pdf",
		ContentType: "application/pdf",
		Disposition: service.FileDispositionAttachment,
		Reader:      reader,
	}
	opener := &recordingFileContentOpener{result: content}
	fileHandler := &FileHandler{Contents: opener}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/admin/files/7/download?expires=1700000300&signature=signed-value",
		nil,
	)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Set("userID", uint(42))

	fileHandler.Download(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if opener.calls != 1 {
		t.Fatalf("Open() calls = %d, want 1", opener.calls)
	}
	if opener.input.UserID != 42 ||
		opener.input.FileID != 7 ||
		opener.input.ExpiresAt != 1_700_000_300 ||
		opener.input.Signature != "signed-value" ||
		opener.input.Mode != fileaccess.ModeDownload {
		t.Fatalf("Open() input = %#v, want signed download access input", opener.input)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", got)
	}
	wantDisposition := mime.FormatMediaType("attachment", map[string]string{
		"filename": "季度 报告.pdf",
	})
	if got := recorder.Header().Get("Content-Disposition"); got != wantDisposition {
		t.Fatalf("Content-Disposition = %q, want %q", got, wantDisposition)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
	if got := recorder.Body.String(); got != "safe file content" {
		t.Fatalf("body = %q, want streamed file content", got)
	}
	if !reader.closed {
		t.Fatal("file content reader was not closed")
	}
}

func TestFileHandlerDownloadSurfacesFirstStorageReadFailureBeforeCommittingSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Unix(1_700_000_000, 0)
	signer, err := fileaccess.NewSigner(
		[]byte("0123456789abcdef0123456789abcdef"),
	)
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}

	tests := []struct {
		name        string
		mode        fileaccess.Mode
		fileName    string
		contentType string
		target      string
		invoke      func(*FileHandler, *gin.Context)
	}{
		{
			name:        "download",
			mode:        fileaccess.ModeDownload,
			fileName:    "report.pdf",
			contentType: "application/pdf",
			target:      "/api/admin/files/7/download",
			invoke:      func(h *FileHandler, c *gin.Context) { h.Download(c) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &model.File{
				Model:            gorm.Model{ID: 7},
				Name:             tt.fileName,
				Bucket:           "files",
				ObjectName:       "private-object",
				ContentType:      tt.contentType,
				ValidationStatus: model.FileValidationStatusValidated,
			}
			expiresAt := now.Add(5 * time.Minute).Unix()
			signature, err := signer.Sign(fileaccess.Claims{
				UserID:           42,
				FileID:           file.ID,
				Mode:             tt.mode,
				ExpiresAt:        expiresAt,
				ValidationStatus: file.ValidationStatus,
			})
			if err != nil {
				t.Fatalf("Sign() error = %v", err)
			}

			reader := &firstReadErrorCloser{
				err: uploadsecurity.NewError(
					uploadsecurity.CodeStorageUnavailable,
					errors.New("provider unavailable on first read"),
				),
			}
			store := &firstReadErrorStore{openReader: reader}
			contents := service.NewFileContentService(
				signer,
				store,
				&fileAccessRepositoryStub{file: file},
				func() time.Time { return now },
			)
			fileHandler := &FileHandler{Contents: contents}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodGet,
				tt.target+"?expires="+strconv.FormatInt(expiresAt, 10)+
					"&signature="+signature,
				nil,
			)
			c.Params = gin.Params{{Key: "id", Value: "7"}}
			c.Set("userID", uint(42))

			tt.invoke(fileHandler, c)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf(
					"stable repro: first storage Read() failed, but %s returned "+
						"status=%d body=%q; want %d/%s before success is committed",
					tt.name,
					recorder.Code,
					recorder.Body.String(),
					http.StatusServiceUnavailable,
					uploadsecurity.CodeStorageUnavailable,
				)
			}
			assertUploadErrorResponse(
				t,
				recorder,
				http.StatusServiceUnavailable,
				uploadsecurity.CodeStorageUnavailable,
			)
			if store.openCalls != 1 {
				t.Fatalf("storage Open() calls = %d, want 1", store.openCalls)
			}
			if !reader.closed {
				t.Fatal("storage reader was not closed after first-read failure")
			}
		})
	}
}

func TestFileHandlerDownloadMapsInvalidSignatureToStableForbiddenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	opener := &recordingFileContentOpener{
		err: uploadsecurity.NewError(
			uploadsecurity.CodeFileAccessInvalid,
			errors.New("signature contained internal detail"),
		),
	}
	fileHandler := &FileHandler{Contents: opener}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/admin/files/7/download?expires=1700000300&signature=invalid",
		nil,
	)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Set("userID", uint(42))

	fileHandler.Download(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusForbidden,
		uploadsecurity.CodeFileAccessInvalid,
	)
	if strings.Contains(recorder.Body.String(), "internal detail") {
		t.Fatalf("response leaked signature detail: %s", recorder.Body.String())
	}
}

func TestFileHandlerRejectsInvalidFileIDAndDownloadAccessParametersBeforeOpeningContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		target string
		id     string
		invoke func(*FileHandler, *gin.Context)
	}{
		{
			name:   "download invalid file ID",
			target: "/api/admin/files/not-an-id/download?expires=1700000300&signature=signed",
			id:     "not-an-id",
			invoke: func(h *FileHandler, c *gin.Context) { h.Download(c) },
		},
		{
			name:   "preview invalid file ID",
			target: "/api/admin/files/not-an-id/preview?expires=1700000300&signature=signed",
			id:     "not-an-id",
			invoke: func(h *FileHandler, c *gin.Context) { h.Preview(c) },
		},
		{
			name:   "download missing expiry",
			target: "/api/admin/files/7/download?signature=signed",
			id:     "7",
			invoke: func(h *FileHandler, c *gin.Context) { h.Download(c) },
		},
		{
			name:   "download missing signature",
			target: "/api/admin/files/7/download?expires=1700000300",
			id:     "7",
			invoke: func(h *FileHandler, c *gin.Context) { h.Download(c) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opener := &recordingFileContentOpener{}
			fileHandler := &FileHandler{Contents: opener}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, tt.target, nil)
			c.Params = gin.Params{{Key: "id", Value: tt.id}}
			c.Set("userID", uint(42))

			tt.invoke(fileHandler, c)

			assertUploadErrorResponse(
				t,
				recorder,
				http.StatusBadRequest,
				uploadsecurity.CodeRequestInvalid,
			)
			if opener.calls != 0 {
				t.Fatalf("Open() calls = %d, want 0 for invalid parameters", opener.calls)
			}
		})
	}
}

func TestFileHandlerPreviewReturnsConflictWithoutSignedParametersBeforeOpeningObjectStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	file := &model.File{
		Model:            gorm.Model{ID: 8},
		Name:             "historical-image.png",
		Bucket:           "files",
		ObjectName:       "historical-image.png",
		ContentType:      "image/png",
		ValidationStatus: model.FileValidationStatusLegacyUnverified,
	}
	store := &firstReadErrorStore{}
	signer, err := fileaccess.NewSigner(
		[]byte("0123456789abcdef0123456789abcdef"),
	)
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	contents := service.NewFileContentService(
		signer,
		store,
		&fileAccessRepositoryStub{file: file},
		func() time.Time { return time.Unix(1_700_000_000, 0) },
	)
	fileHandler := &FileHandler{Contents: contents}

	tests := []struct {
		name   string
		target string
	}{
		{
			name:   "missing expiry and signature",
			target: "/api/admin/files/8/preview",
		},
		{
			name:   "invalid expiry and missing signature",
			target: "/api/admin/files/8/preview?expires=not-a-number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, tt.target, nil)
			c.Params = gin.Params{{Key: "id", Value: "8"}}
			c.Set("userID", uint(43))

			fileHandler.Preview(c)

			assertUploadErrorResponse(
				t,
				recorder,
				http.StatusConflict,
				uploadsecurity.CodeFileStateConflict,
			)
		})
	}
	if store.openCalls != 0 {
		t.Fatalf("storage Open() calls = %d, want 0 for rejected previews", store.openCalls)
	}
}

func TestFileHandlerPreviewMapsRestrictionsAndInvalidSignaturesToStableErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const internalDetail = "signature-key=private object=files/internal/preview.png"

	tests := []struct {
		name     string
		err      error
		wantHTTP int
		wantCode uploadsecurity.Code
	}{
		{
			name: "non-previewable file state",
			err: uploadsecurity.NewError(
				uploadsecurity.CodeFileStateConflict,
				errors.New(internalDetail),
			),
			wantHTTP: http.StatusConflict,
			wantCode: uploadsecurity.CodeFileStateConflict,
		},
		{
			name: "blocked file state",
			err: uploadsecurity.NewError(
				uploadsecurity.CodeFileStateBlocked,
				errors.New(internalDetail),
			),
			wantHTTP: http.StatusConflict,
			wantCode: uploadsecurity.CodeFileStateBlocked,
		},
		{
			name: "invalid preview signature",
			err: uploadsecurity.NewError(
				uploadsecurity.CodeFileAccessInvalid,
				errors.New(internalDetail),
			),
			wantHTTP: http.StatusForbidden,
			wantCode: uploadsecurity.CodeFileAccessInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opener := &recordingFileContentOpener{err: tt.err}
			fileHandler := &FileHandler{Contents: opener}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodGet,
				"/api/admin/files/8/preview?expires=1700000300&signature=preview-signature",
				nil,
			)
			c.Params = gin.Params{{Key: "id", Value: "8"}}
			c.Set("userID", uint(43))

			fileHandler.Preview(c)

			assertUploadErrorResponse(
				t,
				recorder,
				tt.wantHTTP,
				tt.wantCode,
			)
			if opener.calls != 1 {
				t.Fatalf("Open() calls = %d, want 1", opener.calls)
			}
			for _, forbidden := range []string{
				"signature-key",
				"private",
				"files/internal",
				"preview.png",
			} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestFileHandlerRevalidateReturnsUpdatedFileMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	revalidator := &recordingFileRevalidator{
		result: &dto.FileInfo{
			ID:                      9,
			Name:                    "historical.pdf",
			ContentType:             "application/pdf",
			DetectedContentType:     "application/pdf",
			ValidationStatus:        "validated",
			ValidationPolicyVersion: uploadsecurity.PolicyVersionV1,
		},
	}
	fileHandler := &FileHandler{Revalidations: revalidator}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/admin/files/9/revalidate",
		nil,
	)
	c.Params = gin.Params{{Key: "id", Value: "9"}}

	fileHandler.Revalidate(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if revalidator.calls != 1 || revalidator.fileID != 9 {
		t.Fatalf("Revalidate() calls=%d fileID=%d, want 1 and 9",
			revalidator.calls, revalidator.fileID)
	}
	var response struct {
		Code int          `json:"code"`
		Data dto.FileInfo `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusOK ||
		response.Data.ID != revalidator.result.ID ||
		response.Data.ValidationStatus != "validated" {
		t.Fatalf("response = %#v, want updated validated file", response)
	}
}

func TestFileHandlerRevalidateMapsValidationAndInfrastructureFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		id        string
		err       error
		wantCalls int
		wantHTTP  int
		wantCode  uploadsecurity.Code
	}{
		{
			name:      "invalid file ID",
			id:        "invalid",
			wantCalls: 0,
			wantHTTP:  http.StatusBadRequest,
			wantCode:  uploadsecurity.CodeRequestInvalid,
		},
		{
			name:      "state does not permit revalidation",
			id:        "10",
			err:       uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil),
			wantCalls: 1,
			wantHTTP:  http.StatusConflict,
			wantCode:  uploadsecurity.CodeFileStateConflict,
		},
		{
			name:      "stored object no longer exists",
			id:        "11",
			err:       uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, nil),
			wantCalls: 1,
			wantHTTP:  http.StatusNotFound,
			wantCode:  uploadsecurity.CodeStorageObjectNotFound,
		},
		{
			name:      "policy rejects file content",
			id:        "12",
			err:       uploadsecurity.NewError(uploadsecurity.CodeFileContentInvalid, nil),
			wantCalls: 1,
			wantHTTP:  http.StatusUnprocessableEntity,
			wantCode:  uploadsecurity.CodeFileContentInvalid,
		},
		{
			name:      "storage is temporarily unavailable",
			id:        "13",
			err:       uploadsecurity.NewError(uploadsecurity.CodeStorageUnavailable, errors.New("secret provider detail")),
			wantCalls: 1,
			wantHTTP:  http.StatusServiceUnavailable,
			wantCode:  uploadsecurity.CodeStorageUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			revalidator := &recordingFileRevalidator{err: tt.err}
			fileHandler := &FileHandler{Revalidations: revalidator}

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/admin/files/"+tt.id+"/revalidate",
				nil,
			)
			c.Params = gin.Params{{Key: "id", Value: tt.id}}

			fileHandler.Revalidate(c)

			assertUploadErrorResponse(t, recorder, tt.wantHTTP, tt.wantCode)
			if revalidator.calls != tt.wantCalls {
				t.Fatalf("Revalidate() calls = %d, want %d", revalidator.calls, tt.wantCalls)
			}
			if strings.Contains(recorder.Body.String(), "secret provider detail") {
				t.Fatalf("response leaked infrastructure detail: %s", recorder.Body.String())
			}
		})
	}
}

func TestFileHandlerGetFileReturnsControlledDownloadURLWithoutPreviewURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	details := &recordingFileDetailGetter{
		result: &dto.FileDetailResp{
			File: &dto.FileInfo{
				ID:               7,
				Name:             "photo.png",
				ContentType:      "image/png",
				ValidationStatus: "validated",
			},
			DownloadURL: "/api/admin/files/7/download?expires=1700000300&signature=download",
		},
	}
	fileHandler := &FileHandler{Details: details}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/admin/files/7", nil)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Set("userID", uint(42))

	fileHandler.GetFile(c)

	if details.userID != 42 || details.fileID != 7 {
		t.Fatalf("Get() received userID=%d fileID=%d, want 42 and 7", details.userID, details.fileID)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response struct {
		Data dto.FileDetailResp `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.DownloadURL != details.result.DownloadURL {
		t.Fatalf("response URL = %#v, want controlled detail URL %#v", response.Data, details.result)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("response data = %#v, want object", payload["data"])
	}
	if _, ok := data["preview_url"]; ok {
		t.Fatalf("response unexpectedly exposes preview_url: %s", recorder.Body.String())
	}
}

func TestFileHandlerGetFileReturnsStableErrorsWithoutExposingServiceDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		id         string
		serviceErr error
		wantHTTP   int
		wantCode   uploadsecurity.Code
	}{
		{
			name:     "invalid file ID",
			id:       "invalid",
			wantHTTP: http.StatusBadRequest,
			wantCode: uploadsecurity.CodeRequestInvalid,
		},
		{
			name: "file not found",
			id:   "7",
			serviceErr: uploadsecurity.NewError(
				uploadsecurity.CodeFileNotFound,
				errors.New("query files object_name=private/internal.pdf"),
			),
			wantHTTP: http.StatusNotFound,
			wantCode: uploadsecurity.CodeFileNotFound,
		},
		{
			name:       "unclassified persistence failure",
			id:         "8",
			serviceErr: errors.New("mysql dsn password=private object_name=files/internal.pdf"),
			wantHTTP:   http.StatusInternalServerError,
			wantCode:   uploadsecurity.CodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := &recordingFileDetailGetter{err: tt.serviceErr}
			fileHandler := &FileHandler{Details: details}

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodGet,
				"/api/admin/files/"+tt.id,
				nil,
			)
			c.Params = gin.Params{{Key: "id", Value: tt.id}}
			c.Set("userID", uint(42))

			fileHandler.GetFile(c)

			assertUploadErrorResponse(t, recorder, tt.wantHTTP, tt.wantCode)
			if strings.Contains(recorder.Body.String(), "private") ||
				strings.Contains(recorder.Body.String(), "object_name") ||
				strings.Contains(recorder.Body.String(), "internal.pdf") {
				t.Fatalf("response leaked file detail service error: %s", recorder.Body.String())
			}
		})
	}
}

func TestFileHandlerUploadRejectsBodyAboveHardLimitBeforeMultipartParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxUploadBytes = int64(8)
	fileHandler := &FileHandler{MaxUploadBytes: maxUploadBytes}
	hardLimit := maxUploadBytes + managedFileMultipartOverheadBytes

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartUploadRequest(
		t,
		"/api/admin/files",
		"payload.exe",
		hardLimit,
	)

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusRequestEntityTooLarge,
		uploadsecurity.CodeUploadBodyTooLarge,
	)
}

func TestFileHandlerUploadRejectsMultipartWithoutFilePart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fileHandler := &FileHandler{MaxUploadBytes: 1024}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", nil)

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusBadRequest,
		uploadsecurity.CodeUploadFileMissing,
	)
}

func TestFileHandlerUploadRejectsMultipleFileParts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fileHandler := &FileHandler{MaxUploadBytes: 1024}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", []multipartUploadPart{
		{
			FieldName: "file",
			FileName:  "first.exe",
			Content:   []byte("first"),
		},
		{
			FieldName: "file",
			FileName:  "second.exe",
			Content:   []byte("second"),
		},
	})

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusBadRequest,
		uploadsecurity.CodeUploadMultipleFiles,
	)
}

func TestFileHandlerUploadRejectsExtraFilePartUnderAnotherFieldName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fileHandler := &FileHandler{MaxUploadBytes: 1024}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", []multipartUploadPart{
		{
			FieldName: "file",
			FileName:  "first.exe",
			Content:   []byte("first"),
		},
		{
			FieldName: "attachment",
			FileName:  "second.exe",
			Content:   []byte("second"),
		},
	})

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusBadRequest,
		uploadsecurity.CodeUploadMultipleFiles,
	)
}

func TestFileHandlerUploadRejectsEmptyFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fileHandler := &FileHandler{MaxUploadBytes: 1024}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", []multipartUploadPart{{
		FieldName: "file",
		FileName:  "empty.txt",
	}})

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusBadRequest,
		uploadsecurity.CodeFileEmpty,
	)
}

func TestFileHandlerUploadRejectsDamagedMultipartBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fileHandler := &FileHandler{MaxUploadBytes: 1024}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/admin/files",
		strings.NewReader("--broken-boundary\r\nincomplete"),
	)
	c.Request.Header.Set(
		"Content-Type",
		"multipart/form-data; boundary=expected-boundary",
	)

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusBadRequest,
		uploadsecurity.CodeUploadBodyInvalid,
	)
	metadataValue, exists := c.Get(service.UploadAuditMetadataContextKey)
	if !exists {
		t.Fatal("rejected upload audit metadata was not added to Gin context")
	}
	metadata, ok := metadataValue.(service.UploadAuditMetadata)
	if !ok {
		t.Fatalf("upload audit metadata type = %T", metadataValue)
	}
	if metadata.Purpose != string(uploadsecurity.PurposeManagedFile) ||
		metadata.FileName != "" ||
		metadata.ValidationResult != service.UploadValidationRejected ||
		metadata.ReasonCode != string(uploadsecurity.CodeUploadBodyInvalid) {
		t.Fatalf("upload audit metadata = %#v, want filename-free rejected result", metadata)
	}
}

func TestFileHandlerUploadRejectsFileAboveConfiguredLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxUploadBytes = int64(8)
	fileHandler := &FileHandler{MaxUploadBytes: maxUploadBytes}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", []multipartUploadPart{{
		FieldName: "file",
		FileName:  "payload.exe",
		Content:   make([]byte, maxUploadBytes+1),
	}})

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusRequestEntityTooLarge,
		uploadsecurity.CodeFileTooLarge,
	)
}

func TestFileHandlerUploadPassesBoundedInputToFileService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxUploadBytes = int64(1024)
	uploader := &recordingFileUploader{
		result: &dto.FileInfo{
			ID:                      7,
			Name:                    "report.pdf",
			ContentType:             "application/pdf",
			DetectedContentType:     "application/pdf",
			Size:                    int64(len("safe-pdf")),
			ValidationStatus:        "validated",
			ValidationPolicyVersion: uploadsecurity.PolicyVersionV1,
		},
	}
	fileHandler := &FileHandler{
		Uploads:        uploader,
		MaxUploadBytes: maxUploadBytes,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", []multipartUploadPart{{
		FieldName: "file",
		FileName:  "report.pdf",
		Content:   []byte("safe-pdf"),
	}})
	c.Set("userID", uint(42))

	fileHandler.Upload(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if uploader.calls != 1 {
		t.Fatalf("Upload() calls = %d, want 1", uploader.calls)
	}
	if uploader.input.UploaderID != 42 ||
		uploader.input.FileName != "report.pdf" ||
		uploader.input.ContentType != "application/octet-stream" ||
		uploader.input.Size != int64(len("safe-pdf")) ||
		uploader.input.MaxBytes != maxUploadBytes {
		t.Fatalf("Upload() input = %#v, want bounded request metadata", uploader.input)
	}
	if string(uploader.content) != "safe-pdf" {
		t.Fatalf("Upload() content = %q, want %q", uploader.content, "safe-pdf")
	}

	var response struct {
		Code int          `json:"code"`
		Data dto.FileInfo `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusOK || response.Data.ID != uploader.result.ID {
		t.Fatalf("response = %#v, want uploaded file metadata", response)
	}

	metadataValue, exists := c.Get(service.UploadAuditMetadataContextKey)
	if !exists {
		t.Fatal("upload audit metadata was not added to Gin context")
	}
	metadata, ok := metadataValue.(service.UploadAuditMetadata)
	if !ok {
		t.Fatalf("upload audit metadata type = %T", metadataValue)
	}
	if metadata.Purpose != string(uploadsecurity.PurposeManagedFile) ||
		metadata.FileName != uploader.result.Name ||
		metadata.FileSize != uploader.result.Size ||
		metadata.DeclaredMIME != "application/octet-stream" ||
		metadata.DetectedMIME != uploader.result.DetectedContentType ||
		metadata.ValidationResult != service.UploadValidationAccepted ||
		metadata.PolicyVersion != uploader.result.ValidationPolicyVersion ||
		metadata.ReasonCode != "" {
		t.Fatalf("upload audit metadata = %#v, want controlled accepted result", metadata)
	}
}

func TestFileHandlerUploadRejectsAdministratorImageAsUnsupportedMediaType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uploader := &recordingFileUploader{
		err: uploadsecurity.NewError(
			uploadsecurity.CodeFileTypeNotAllowed,
			nil,
		),
	}
	fileHandler := &FileHandler{
		Uploads:        uploader,
		MaxUploadBytes: 1024,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", []multipartUploadPart{{
		FieldName: "file",
		FileName:  "avatar.png",
		Content:   []byte("png-image-content"),
	}})
	c.Set("userID", uint(42))

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusUnsupportedMediaType,
		uploadsecurity.CodeFileTypeNotAllowed,
	)
	if uploader.calls != 1 {
		t.Fatalf("Upload() calls = %d, want 1", uploader.calls)
	}
}

func TestFileHandlerUploadMapsServiceErrorAndRecordsRejectedAuditMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const internalDetail = "minio access-key=private-secret"
	uploader := &recordingFileUploader{
		err: uploadsecurity.NewError(
			uploadsecurity.CodeStorageUnavailable,
			errors.New(internalDetail),
		),
	}
	fileHandler := &FileHandler{
		Uploads:        uploader,
		MaxUploadBytes: 1024,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/admin/files", []multipartUploadPart{{
		FieldName: "file",
		FileName:  `C:\fakepath\report.pdf`,
		Content:   []byte("safe-pdf"),
	}})
	c.Set("userID", uint(42))

	fileHandler.Upload(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusServiceUnavailable,
		uploadsecurity.CodeStorageUnavailable,
	)
	if strings.Contains(recorder.Body.String(), internalDetail) ||
		strings.Contains(recorder.Body.String(), "private-secret") {
		t.Fatalf("response leaked internal storage detail: %s", recorder.Body.String())
	}

	metadataValue, exists := c.Get(service.UploadAuditMetadataContextKey)
	if !exists {
		t.Fatal("rejected upload audit metadata was not added to Gin context")
	}
	metadata, ok := metadataValue.(service.UploadAuditMetadata)
	if !ok {
		t.Fatalf("upload audit metadata type = %T", metadataValue)
	}
	if metadata.Purpose != string(uploadsecurity.PurposeManagedFile) ||
		metadata.FileName != "report.pdf" ||
		metadata.FileSize != int64(len("safe-pdf")) ||
		metadata.DeclaredMIME != "application/octet-stream" ||
		metadata.ValidationResult != service.UploadValidationRejected ||
		metadata.ReasonCode != string(uploadsecurity.CodeStorageUnavailable) {
		t.Fatalf("upload audit metadata = %#v, want controlled rejected result", metadata)
	}
}

func TestFileHandlerManagementEndpointsReturnStableErrorsWithoutLeakingDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const internalDetail = "mysql password=private object_name=files/internal/report.pdf"

	tests := []struct {
		name     string
		method   string
		target   string
		id       string
		body     string
		handler  *FileHandler
		invoke   func(*FileHandler, *gin.Context)
		wantHTTP int
		wantCode uploadsecurity.Code
	}{
		{
			name:   "list persistence failure",
			method: http.MethodGet,
			target: "/api/admin/files",
			handler: &FileHandler{
				List: func(_ context.Context, _, _ int, _ string) ([]dto.FileInfo, int64, error) {
					return nil, 0, uploadsecurity.NewError(
						uploadsecurity.CodePersistenceFailed,
						errors.New(internalDetail),
					)
				},
			},
			invoke:   func(h *FileHandler, c *gin.Context) { h.ListFiles(c) },
			wantHTTP: http.StatusInternalServerError,
			wantCode: uploadsecurity.CodePersistenceFailed,
		},
		{
			name:     "update invalid file ID",
			method:   http.MethodPut,
			target:   "/api/admin/files/not-an-id",
			id:       "not-an-id",
			body:     `{"name":"report"}`,
			handler:  &FileHandler{},
			invoke:   func(h *FileHandler, c *gin.Context) { h.UpdateFile(c) },
			wantHTTP: http.StatusBadRequest,
			wantCode: uploadsecurity.CodeRequestInvalid,
		},
		{
			name:     "update invalid JSON",
			method:   http.MethodPut,
			target:   "/api/admin/files/7",
			id:       "7",
			body:     `{"name":`,
			handler:  &FileHandler{},
			invoke:   func(h *FileHandler, c *gin.Context) { h.UpdateFile(c) },
			wantHTTP: http.StatusBadRequest,
			wantCode: uploadsecurity.CodeRequestInvalid,
		},
		{
			name:   "update persistence failure",
			method: http.MethodPut,
			target: "/api/admin/files/7",
			id:     "7",
			body:   `{"name":"report"}`,
			handler: &FileHandler{
				Update: func(_ context.Context, _ uint, _ dto.UpdateFileReq) (*dto.FileInfo, error) {
					return nil, uploadsecurity.NewError(
						uploadsecurity.CodePersistenceFailed,
						errors.New(internalDetail),
					)
				},
			},
			invoke:   func(h *FileHandler, c *gin.Context) { h.UpdateFile(c) },
			wantHTTP: http.StatusInternalServerError,
			wantCode: uploadsecurity.CodePersistenceFailed,
		},
		{
			name:   "delete storage failure",
			method: http.MethodDelete,
			target: "/api/admin/files/7",
			id:     "7",
			handler: &FileHandler{
				Delete: func(_ context.Context, _ uint) error {
					return uploadsecurity.NewError(
						uploadsecurity.CodeStorageUnavailable,
						errors.New(internalDetail),
					)
				},
			},
			invoke:   func(h *FileHandler, c *gin.Context) { h.DeleteFile(c) },
			wantHTTP: http.StatusServiceUnavailable,
			wantCode: uploadsecurity.CodeStorageUnavailable,
		},
		{
			name:   "browse storage failure",
			method: http.MethodGet,
			target: "/api/admin/files-browse?prefix=private/",
			handler: &FileHandler{
				Browse: func(_ context.Context, _ string) ([]dto.FileObjectInfo, error) {
					return nil, uploadsecurity.NewError(
						uploadsecurity.CodeStorageUnavailable,
						errors.New(internalDetail),
					)
				},
			},
			invoke:   func(h *FileHandler, c *gin.Context) { h.BrowseFiles(c) },
			wantHTTP: http.StatusServiceUnavailable,
			wantCode: uploadsecurity.CodeStorageUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.body != "" {
				c.Request.Header.Set("Content-Type", "application/json")
			}
			if tt.id != "" {
				c.Params = gin.Params{{Key: "id", Value: tt.id}}
			}

			tt.invoke(tt.handler, c)

			assertUploadErrorResponse(t, recorder, tt.wantHTTP, tt.wantCode)
			for _, forbidden := range []string{
				"password",
				"private",
				"object_name",
				"files/internal",
				"report.pdf",
			} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestFileHandlerListRejectsInvalidPaginationBeforeQueryingFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		target string
	}{
		{
			name:   "page is not an integer",
			target: "/api/admin/files?page=invalid&size=10",
		},
		{
			name:   "page is not positive",
			target: "/api/admin/files?page=0&size=10",
		},
		{
			name:   "size is not an integer",
			target: "/api/admin/files?page=1&size=invalid",
		},
		{
			name:   "size is not positive",
			target: "/api/admin/files?page=1&size=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listCalls := 0
			fileHandler := &FileHandler{
				List: func(
					_ context.Context,
					_, _ int,
					_ string,
				) ([]dto.FileInfo, int64, error) {
					listCalls++
					return nil, 0, nil
				},
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, tt.target, nil)

			fileHandler.ListFiles(c)

			assertUploadErrorResponse(
				t,
				recorder,
				http.StatusBadRequest,
				uploadsecurity.CodeRequestInvalid,
			)
			if listCalls != 0 {
				t.Fatalf("List() calls = %d, want 0 for invalid pagination", listCalls)
			}
		})
	}
}
