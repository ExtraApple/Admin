package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"admin/internal/identity/application"
	"github.com/gin-gonic/gin"
)

func TestWriteIdentityErrorEmitsStructuredLoginLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	writeIdentityError(context, application.NewLoginLockedError(37, nil), false)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Retry-After"); got != "37" {
		t.Fatalf("Retry-After = %q, want 37", got)
	}
	want := `{"code":429,"error_code":"AUTHN_LOGIN_LOCKED","msg":"login is temporarily locked","data":{"retry_after_seconds":37}}`
	if recorder.Body.String() != want {
		t.Fatalf("body = %s, want %s", recorder.Body.String(), want)
	}
}

func TestWriteIdentityErrorEmitsSortedFieldDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	err := application.NewValidationError([]application.FieldError{
		{Field: "password", ErrorCode: "IDENTITY_PASSWORD_INVALID", Message: "password does not meet the security requirements"},
		{Field: "confirm_password", ErrorCode: "IDENTITY_PASSWORD_CONFIRMATION_INVALID", Message: "password confirmation does not match"},
	}, nil)
	writeIdentityError(context, err, false)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	want := `{"code":422,"error_code":"IDENTITY_VALIDATION_INVALID","msg":"request validation failed","data":{"fields":[{"field":"confirm_password","error_code":"IDENTITY_PASSWORD_CONFIRMATION_INVALID","message":"password confirmation does not match"},{"field":"password","error_code":"IDENTITY_PASSWORD_INVALID","message":"password does not meet the security requirements"}]}}`
	if recorder.Body.String() != want {
		t.Fatalf("body = %s, want %s", recorder.Body.String(), want)
	}
}
