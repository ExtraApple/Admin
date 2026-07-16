package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"admin/dto"
	"admin/model"
	"admin/service"
	"admin/service/objectstorage"
	"admin/service/uploadsecurity"
)

type recordingAvatarManager struct {
	uploadCalls  int
	restoreCalls int
	input        service.UploadAvatarInput
	content      []byte
	userID       uint
	user         *model.User
	uploadResult *service.UploadAvatarResult
	err          error
}

type recordingAvatarContentOpener struct {
	calls   int
	userID  uint
	content service.AvatarContent
}

type avatarContentRepositoryStub struct {
	user *model.User
	err  error
}

type recordingUserProfileUpdater struct {
	calls  int
	userID uint
	req    dto.UpdateSelfReq
	user   *dto.UserInfo
	err    error
}

func (u *recordingUserProfileUpdater) Update(
	_ context.Context,
	userID uint,
	req dto.UpdateSelfReq,
) (*dto.UserInfo, error) {
	u.calls++
	u.userID = userID
	u.req = req
	return u.user, u.err
}

func (o *recordingAvatarContentOpener) Open(
	_ context.Context,
	userID uint,
) service.AvatarContent {
	o.calls++
	o.userID = userID
	return o.content
}

func (r *avatarContentRepositoryStub) FindByID(
	_ context.Context,
	_ uint,
) (*model.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.user == nil {
		return nil, errors.New("user not found")
	}
	copy := *r.user
	return &copy, nil
}

func (m *recordingAvatarManager) UploadWithResult(
	_ context.Context,
	input service.UploadAvatarInput,
) (*service.UploadAvatarResult, error) {
	m.uploadCalls++
	m.input = input
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return nil, err
	}
	m.content = content
	if m.uploadResult != nil {
		return m.uploadResult, m.err
	}
	if m.user != nil {
		return &service.UploadAvatarResult{User: m.user}, m.err
	}
	return nil, m.err
}

func (m *recordingAvatarManager) RestoreDefault(
	_ context.Context,
	userID uint,
) (*model.User, error) {
	m.restoreCalls++
	m.userID = userID
	return m.user, m.err
}

func TestUserHandlerUploadAvatarRejectsBodyAboveHardLimitBeforeMultipartParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxUploadBytes = int64(8)
	userHandler := &UserHandler{AvatarMaxUploadBytes: maxUploadBytes}
	hardLimit := maxUploadBytes + avatarMultipartOverheadBytes

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartUploadRequest(
		t,
		"/api/user/avatar",
		"payload.exe",
		hardLimit,
	)

	userHandler.UploadAvatar(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusRequestEntityTooLarge,
		uploadsecurity.CodeUploadBodyTooLarge,
	)
}

func TestUserHandlerUploadAvatarRejectsInvalidMultipartBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		parts    []multipartUploadPart
		wantCode uploadsecurity.Code
	}{
		{
			name:     "missing file part",
			parts:    nil,
			wantCode: uploadsecurity.CodeUploadFileMissing,
		},
		{
			name: "multiple file parts",
			parts: []multipartUploadPart{
				{
					FieldName: "file",
					FileName:  "first.png",
					Content:   []byte("first"),
				},
				{
					FieldName: "file",
					FileName:  "second.png",
					Content:   []byte("second"),
				},
			},
			wantCode: uploadsecurity.CodeUploadMultipleFiles,
		},
		{
			name: "extra file part under another field",
			parts: []multipartUploadPart{
				{
					FieldName: "file",
					FileName:  "portrait.png",
					Content:   []byte("portrait"),
				},
				{
					FieldName: "metadata",
					FileName:  "payload.txt",
					Content:   []byte("extra"),
				},
			},
			wantCode: uploadsecurity.CodeUploadMultipleFiles,
		},
		{
			name: "empty avatar file",
			parts: []multipartUploadPart{{
				FieldName: "file",
				FileName:  "empty.png",
				Content:   nil,
			}},
			wantCode: uploadsecurity.CodeFileEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &recordingAvatarManager{}
			userHandler := &UserHandler{
				Avatars:              manager,
				AvatarMaxUploadBytes: 1024,
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = newMultipartRequest(t, "/api/user/avatar", tt.parts)
			c.Set("userID", uint(42))

			userHandler.UploadAvatar(c)

			assertUploadErrorResponse(
				t,
				recorder,
				http.StatusBadRequest,
				tt.wantCode,
			)
			if manager.uploadCalls != 0 {
				t.Fatalf("Upload() calls = %d, want 0", manager.uploadCalls)
			}
			metadataValue, exists := c.Get(service.UploadAuditMetadataContextKey)
			if !exists {
				t.Fatal("rejected avatar audit metadata was not added to Gin context")
			}
			metadata, ok := metadataValue.(service.UploadAuditMetadata)
			if !ok {
				t.Fatalf("avatar upload audit metadata type = %T", metadataValue)
			}
			if metadata.Purpose != string(uploadsecurity.PurposeAvatar) ||
				metadata.FileName != "" ||
				metadata.ValidationResult != service.UploadValidationRejected ||
				metadata.ReasonCode != string(tt.wantCode) {
				t.Fatalf("avatar upload audit metadata = %#v, want filename-free rejected result", metadata)
			}
		})
	}
}

func TestUserHandlerUploadAvatarPassesBoundedInputToAvatarService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const maxUploadBytes = int64(1024)
	manager := &recordingAvatarManager{
		user: &model.User{
			Username:               "alice",
			AvatarObjectName:       "avatars/42/00000000-0000-0000-0000-000000000001.jpg",
			AvatarContentType:      "image/jpeg",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
	}
	manager.user.ID = 42
	manager.uploadResult = &service.UploadAvatarResult{
		User:          manager.user,
		FileName:      "portrait.jpg",
		FileSize:      int64(len("normalized-avatar")),
		DetectedMIME:  "image/png",
		PolicyVersion: uploadsecurity.PolicyVersionV1,
	}
	userHandler := &UserHandler{
		Avatars:              manager,
		AvatarMaxUploadBytes: maxUploadBytes,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/user/avatar", []multipartUploadPart{{
		FieldName: "file",
		FileName:  "portrait.png",
		Content:   []byte("avatar-input"),
	}})
	c.Set("userID", uint(42))

	userHandler.UploadAvatar(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if manager.uploadCalls != 1 {
		t.Fatalf("Upload() calls = %d, want 1", manager.uploadCalls)
	}
	if manager.input.UserID != 42 ||
		manager.input.FileName != "portrait.png" ||
		manager.input.ContentType != "application/octet-stream" ||
		manager.input.Size != int64(len("avatar-input")) ||
		manager.input.MaxBytes != maxUploadBytes {
		t.Fatalf("Upload() input = %#v, want bounded avatar metadata", manager.input)
	}
	if string(manager.content) != "avatar-input" {
		t.Fatalf("Upload() content = %q, want avatar-input", manager.content)
	}

	var response struct {
		Code int `json:"code"`
		Data struct {
			Avatar string `json:"avatar"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusOK ||
		response.Data.Avatar != "/api/avatars/42" {
		t.Fatalf("response = %#v, want controlled avatar URL", response)
	}
	if strings.Contains(recorder.Body.String(), manager.user.AvatarObjectName) {
		t.Fatalf("response leaked avatar object name: %s", recorder.Body.String())
	}

	metadataValue, exists := c.Get(service.UploadAuditMetadataContextKey)
	if !exists {
		t.Fatal("avatar upload audit metadata was not added to Gin context")
	}
	metadata, ok := metadataValue.(service.UploadAuditMetadata)
	if !ok {
		t.Fatalf("avatar upload audit metadata type = %T", metadataValue)
	}
	if metadata.Purpose != string(uploadsecurity.PurposeAvatar) ||
		metadata.FileName != manager.uploadResult.FileName ||
		metadata.FileSize != manager.uploadResult.FileSize ||
		metadata.DeclaredMIME != "application/octet-stream" ||
		metadata.DetectedMIME != manager.uploadResult.DetectedMIME ||
		metadata.ValidationResult != service.UploadValidationAccepted ||
		metadata.PolicyVersion != manager.uploadResult.PolicyVersion ||
		metadata.ReasonCode != "" {
		t.Fatalf("avatar upload audit metadata = %#v, want controlled accepted result", metadata)
	}
}

func TestUserHandlerUploadAvatarRecordsSanitizedRejectedAuditMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	manager := &recordingAvatarManager{
		err: uploadsecurity.NewError(
			uploadsecurity.CodeImageDecodeInvalid,
			errors.New("decoder detail must not be audited"),
		),
	}
	userHandler := &UserHandler{
		Avatars:              manager,
		AvatarMaxUploadBytes: 1024,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartRequest(t, "/api/user/avatar", []multipartUploadPart{{
		FieldName: "file",
		FileName:  `C:\fakepath\portrait.png`,
		Content:   []byte("broken-image"),
	}})
	c.Set("userID", uint(42))

	userHandler.UploadAvatar(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusUnprocessableEntity,
		uploadsecurity.CodeImageDecodeInvalid,
	)
	metadataValue, exists := c.Get(service.UploadAuditMetadataContextKey)
	if !exists {
		t.Fatal("rejected avatar audit metadata was not added to Gin context")
	}
	metadata, ok := metadataValue.(service.UploadAuditMetadata)
	if !ok {
		t.Fatalf("avatar upload audit metadata type = %T", metadataValue)
	}
	if metadata.Purpose != string(uploadsecurity.PurposeAvatar) ||
		metadata.FileName != "portrait.png" ||
		metadata.FileSize != int64(len("broken-image")) ||
		metadata.DeclaredMIME != "application/octet-stream" ||
		metadata.ValidationResult != service.UploadValidationRejected ||
		metadata.ReasonCode != string(uploadsecurity.CodeImageDecodeInvalid) {
		t.Fatalf("avatar upload audit metadata = %#v, want controlled rejected result", metadata)
	}
}

func TestUserHandlerRestoreDefaultAvatarReturnsControlledDefaultURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	manager := &recordingAvatarManager{
		user: &model.User{
			Username: "alice",
			Avatar:   "https://legacy.example/avatar.png",
		},
	}
	manager.user.ID = 42
	userHandler := &UserHandler{Avatars: manager}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/user/avatar", nil)
	c.Set("userID", uint(42))

	userHandler.RestoreDefaultAvatar(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if manager.restoreCalls != 1 || manager.userID != 42 {
		t.Fatalf("RestoreDefault() calls=%d userID=%d, want 1 and 42",
			manager.restoreCalls, manager.userID)
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			Avatar string `json:"avatar"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusOK ||
		response.Data.Avatar != "/api/avatars/default" {
		t.Fatalf("response = %#v, want default avatar URL", response)
	}
	if strings.Contains(recorder.Body.String(), manager.user.Avatar) {
		t.Fatalf("response leaked historical avatar URL: %s", recorder.Body.String())
	}
}

func TestUserHandlerAvatarMutationsReturnStableErrorsWithoutLeakingDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const internalDetail = "minio secret=private object=avatars/42/internal.png mysql password=hidden"

	tests := []struct {
		name       string
		operation  string
		serviceErr error
		wantHTTP   int
		wantCode   uploadsecurity.Code
	}{
		{
			name:      "upload storage unavailable",
			operation: "upload",
			serviceErr: uploadsecurity.NewError(
				uploadsecurity.CodeStorageUnavailable,
				errors.New(internalDetail),
			),
			wantHTTP: http.StatusServiceUnavailable,
			wantCode: uploadsecurity.CodeStorageUnavailable,
		},
		{
			name:      "upload persistence failed",
			operation: "upload",
			serviceErr: uploadsecurity.NewError(
				uploadsecurity.CodePersistenceFailed,
				errors.New(internalDetail),
			),
			wantHTTP: http.StatusInternalServerError,
			wantCode: uploadsecurity.CodePersistenceFailed,
		},
		{
			name:       "upload unclassified failure",
			operation:  "upload",
			serviceErr: errors.New(internalDetail),
			wantHTTP:   http.StatusInternalServerError,
			wantCode:   uploadsecurity.CodeInternalError,
		},
		{
			name:      "restore persistence failed",
			operation: "restore",
			serviceErr: uploadsecurity.NewError(
				uploadsecurity.CodePersistenceFailed,
				errors.New(internalDetail),
			),
			wantHTTP: http.StatusInternalServerError,
			wantCode: uploadsecurity.CodePersistenceFailed,
		},
		{
			name:      "restore storage unavailable",
			operation: "restore",
			serviceErr: uploadsecurity.NewError(
				uploadsecurity.CodeStorageUnavailable,
				errors.New(internalDetail),
			),
			wantHTTP: http.StatusServiceUnavailable,
			wantCode: uploadsecurity.CodeStorageUnavailable,
		},
		{
			name:       "restore unclassified failure",
			operation:  "restore",
			serviceErr: errors.New(internalDetail),
			wantHTTP:   http.StatusInternalServerError,
			wantCode:   uploadsecurity.CodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &recordingAvatarManager{err: tt.serviceErr}
			userHandler := &UserHandler{
				Avatars:              manager,
				AvatarMaxUploadBytes: 1024,
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("userID", uint(42))

			switch tt.operation {
			case "upload":
				c.Request = newMultipartRequest(
					t,
					"/api/user/avatar",
					[]multipartUploadPart{{
						FieldName: "file",
						FileName:  "portrait.png",
						Content:   []byte("avatar-input"),
					}},
				)
				userHandler.UploadAvatar(c)
				if manager.uploadCalls != 1 {
					t.Fatalf("Upload() calls = %d, want 1", manager.uploadCalls)
				}
			case "restore":
				c.Request = httptest.NewRequest(
					http.MethodDelete,
					"/api/user/avatar",
					nil,
				)
				userHandler.RestoreDefaultAvatar(c)
				if manager.restoreCalls != 1 {
					t.Fatalf("RestoreDefault() calls = %d, want 1", manager.restoreCalls)
				}
			default:
				t.Fatalf("unknown operation %q", tt.operation)
			}

			assertUploadErrorResponse(
				t,
				recorder,
				tt.wantHTTP,
				tt.wantCode,
			)
			for _, forbidden := range []string{
				"minio",
				"secret",
				"private",
				"avatars/42",
				"internal.png",
				"mysql",
				"password",
				"hidden",
			} {
				if strings.Contains(strings.ToLower(recorder.Body.String()), forbidden) {
					t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestUserHandlerGetAvatarStreamsControlledImageWithPublicHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reader := &closingReadCloser{Reader: strings.NewReader("normalized-png")}
	opener := &recordingAvatarContentOpener{
		content: service.AvatarContent{
			ContentType: "image/png",
			Reader:      reader,
		},
	}
	userHandler := &UserHandler{AvatarContents: opener}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/avatars/42", nil)
	c.Params = gin.Params{{Key: "user_id", Value: "42"}}

	userHandler.GetAvatar(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if opener.calls != 1 || opener.userID != 42 {
		t.Fatalf("Open() calls=%d userID=%d, want 1 and 42", opener.calls, opener.userID)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("Cache-Control = %q, want public, max-age=300", got)
	}
	if got := recorder.Body.String(); got != "normalized-png" {
		t.Fatalf("body = %q, want normalized-png", got)
	}
	if !reader.closed {
		t.Fatal("avatar reader was not closed")
	}
}

func TestUserHandlerGetAvatarFallsBackWhenStoredObjectFailsOnFirstRead(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const objectName = "avatars/42/00000000-0000-0000-0000-000000000001.png"
	user := &model.User{
		AvatarObjectName:       objectName,
		AvatarContentType:      "image/png",
		AvatarValidationStatus: model.FileValidationStatusValidated,
	}
	user.ID = 42
	reader := &firstReadErrorCloser{
		err: uploadsecurity.NewError(
			uploadsecurity.CodeStorageUnavailable,
			errors.New("provider unavailable on first avatar read"),
		),
	}
	store := &firstReadErrorStore{
		openReader: reader,
		statResult: objectstorage.ObjectInfo{
			Bucket:      "image",
			Name:        objectName,
			Size:        128,
			ContentType: "image/png",
		},
	}
	avatarContents := service.NewAvatarContentService(
		&avatarContentRepositoryStub{user: user},
		store,
	)
	userHandler := &UserHandler{AvatarContents: avatarContents}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/avatars/42", nil)
	c.Params = gin.Params{{Key: "user_id", Value: "42"}}

	userHandler.GetAvatar(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png fallback", got)
	}
	if _, format, err := image.Decode(bytes.NewReader(recorder.Body.Bytes())); err != nil {
		t.Fatalf(
			"stable repro: trusted avatar passed Stat/Open, its first Read() failed, "+
				"and handler returned a non-decodable body instead of built-in PNG: %v",
			err,
		)
	} else if format != "png" {
		t.Fatalf("fallback avatar format = %q, want png", format)
	}
	if store.openCalls != 1 {
		t.Fatalf("storage Open() calls = %d, want 1", store.openCalls)
	}
	if !reader.closed {
		t.Fatal("failed avatar reader was not closed")
	}
}

func TestUserHandlerGetAvatarUsesIndistinguishableDefaultForInvalidUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	opener := &recordingAvatarContentOpener{}
	userHandler := &UserHandler{AvatarContents: opener}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/avatars/not-a-user", nil)
	c.Params = gin.Params{{Key: "user_id", Value: "not-a-user"}}

	userHandler.GetAvatar(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if opener.calls != 0 {
		t.Fatalf("Open() calls = %d, want 0 for invalid user ID", opener.calls)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("Cache-Control = %q, want public, max-age=300", got)
	}
	if _, format, err := image.Decode(bytes.NewReader(recorder.Body.Bytes())); err != nil {
		t.Fatalf("decode fallback avatar: %v", err)
	} else if format != "png" {
		t.Fatalf("fallback avatar format = %q, want png", format)
	}
}

func TestUserHandlerGetDefaultAvatarReturnsBuiltInPNGWithLongCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userHandler := &UserHandler{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/avatars/default", nil)

	userHandler.GetDefaultAvatar(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=86400" {
		t.Fatalf("Cache-Control = %q, want public, max-age=86400", got)
	}
	if _, format, err := image.Decode(bytes.NewReader(recorder.Body.Bytes())); err != nil {
		t.Fatalf("decode default avatar: %v", err)
	} else if format != "png" {
		t.Fatalf("default avatar format = %q, want png", format)
	}
}

func TestUserHandlerUpdateSelfRejectsAvatarBeforeProfilePersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updater := &recordingUserProfileUpdater{}
	userHandler := &UserHandler{ProfileUpdates: updater}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/user/info",
		strings.NewReader(`{
			"nickname":"after",
			"avatar":"https://attacker.example/avatar.png"
		}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("userID", uint(42))

	userHandler.UpdateSelf(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s",
			recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if updater.calls != 0 {
		t.Fatalf("profile Update() calls = %d, want 0", updater.calls)
	}
	var response struct {
		Code      int                 `json:"code"`
		ErrorCode uploadsecurity.Code `json:"error_code"`
		Msg       string              `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusBadRequest ||
		response.ErrorCode != uploadsecurity.Code("AVATAR_FIELD_NOT_WRITABLE") ||
		response.Msg == "" {
		t.Fatalf("response = %#v, want stable avatar field rejection", response)
	}
	if strings.Contains(recorder.Body.String(), "attacker.example") {
		t.Fatalf("response leaked rejected avatar value: %s", recorder.Body.String())
	}
}

func TestUserHandlerUpdateSelfRejectsNullAvatarBeforeProfilePersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updater := &recordingUserProfileUpdater{}
	userHandler := &UserHandler{ProfileUpdates: updater}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/user/info",
		strings.NewReader(`{"nickname":"after","avatar":null}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("userID", uint(42))

	userHandler.UpdateSelf(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusBadRequest,
		uploadsecurity.CodeAvatarFieldNotWritable,
	)
	if updater.calls != 0 {
		t.Fatalf("profile Update() calls = %d, want 0", updater.calls)
	}
}

func TestUserHandlerUpdateSelfPreservesNicknameAndEmailValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid email",
			body: `{"email":"not-an-email"}`,
		},
		{
			name: "nickname exceeds maximum length",
			body: `{"nickname":"` + strings.Repeat("a", 101) + `"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updater := &recordingUserProfileUpdater{}
			userHandler := &UserHandler{ProfileUpdates: updater}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodPut,
				"/api/user/info",
				strings.NewReader(tt.body),
			)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("userID", uint(42))

			userHandler.UpdateSelf(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s",
					recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if updater.calls != 0 {
				t.Fatalf("profile Update() calls = %d, want 0", updater.calls)
			}
		})
	}
}

func TestUserHandlerUpdateSelfForwardsValidProfileFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updater := &recordingUserProfileUpdater{
		user: &dto.UserInfo{
			ID:       42,
			Nickname: "after",
			Email:    "after@example.com",
			Avatar:   "/api/avatars/default",
		},
	}
	userHandler := &UserHandler{ProfileUpdates: updater}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/user/info",
		strings.NewReader(`{
			"nickname":"after",
			"email":"after@example.com"
		}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("userID", uint(42))

	userHandler.UpdateSelf(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s",
			recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if updater.calls != 1 ||
		updater.userID != 42 ||
		updater.req.Nickname != "after" ||
		updater.req.Email != "after@example.com" {
		t.Fatalf("profile Update() calls=%d userID=%d req=%#v, want valid fields",
			updater.calls, updater.userID, updater.req)
	}
}

func TestUserHandlerUpdateSelfDoesNotExposeUnclassifiedPersistenceError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const internalDetail = "mysql dsn password=private object=avatars/42/internal.jpg"
	updater := &recordingUserProfileUpdater{
		err: errors.New(internalDetail),
	}
	userHandler := &UserHandler{ProfileUpdates: updater}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/user/info",
		strings.NewReader(`{"nickname":"after"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("userID", uint(42))

	userHandler.UpdateSelf(c)

	assertUploadErrorResponse(
		t,
		recorder,
		http.StatusInternalServerError,
		uploadsecurity.CodeInternalError,
	)
	if strings.Contains(recorder.Body.String(), internalDetail) ||
		strings.Contains(recorder.Body.String(), "private") ||
		strings.Contains(recorder.Body.String(), "avatars/42") {
		t.Fatalf("response leaked profile persistence detail: %s", recorder.Body.String())
	}
}
