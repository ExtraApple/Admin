package httpresponse

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

type testValidationData struct {
	Fields []FieldError `json:"fields"`
}

func TestWriteSuccessUsesTheFourFieldEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/empty", func(c *gin.Context) {
		WriteSuccess(c, http.StatusOK, nil)
	})
	engine.GET("/list", func(c *gin.Context) {
		var values []string
		WriteSuccess(c, http.StatusOK, values)
	})

	t.Run("no data is explicit null", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/empty", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if got := recorder.Body.String(); got != `{"code":200,"error_code":"","msg":"success","data":null}` {
			t.Fatalf("body = %s", got)
		}
	})

	t.Run("nil collection is an empty array", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/list", nil))
		if got := recorder.Body.String(); got != `{"code":200,"error_code":"","msg":"success","data":[]}` {
			t.Fatalf("body = %s", got)
		}
	})
}

func TestWriteErrorKeepsCauseOutOfTheResponseAndStoresContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definition := ErrorDefinition{
		Owner:      "identity",
		Code:       "IDENTITY_INVALID",
		Status:     http.StatusUnprocessableEntity,
		Message:    "request is invalid",
		DataSchema: reflect.TypeOf(testValidationData{}),
		Fields:     []FieldErrorDefinition{{Field: "username", Code: "IDENTITY_USERNAME_INVALID", Message: "username is invalid"}},
	}
	cause := errors.New("database password=secret")
	data := testValidationData{Fields: []FieldError{{Field: "username", ErrorCode: "IDENTITY_USERNAME_INVALID", Message: "username is invalid"}}}
	engine := gin.New()
	engine.POST("/error", func(c *gin.Context) {
		WriteError(c, definition, cause, data)
		stored, ok := ErrorContextOf(c)
		if !ok || stored.Cause != cause || stored.Definition.Code != definition.Code {
			t.Fatalf("error context = %#v, want definition and cause", stored)
		}
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/error", bytes.NewReader(nil)))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	var envelope Envelope[testValidationData]
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != definition.Status || envelope.ErrorCode != definition.Code || envelope.Msg != definition.Message {
		t.Fatalf("envelope = %#v, want definition values", envelope)
	}
	if strings := recorder.Body.String(); bytes.Contains([]byte(strings), []byte("database password")) {
		t.Fatalf("response leaked internal cause: %s", strings)
	}
}

func TestErrorDefinitionValidateRejectsInvalidPublicDefinitions(t *testing.T) {
	tests := []struct {
		name string
		def  ErrorDefinition
	}{
		{name: "missing owner", def: ErrorDefinition{Code: "HTTP_INVALID", Status: 400, Message: "bad request"}},
		{name: "invalid code", def: ErrorDefinition{Owner: "http", Code: "bad-code", Status: 400, Message: "bad request"}},
		{name: "invalid status", def: ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 200, Message: "bad request"}},
		{name: "missing message", def: ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 400}},
		{name: "duplicate field code", def: ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 422, Message: "bad request", Fields: []FieldErrorDefinition{{Field: "name", Code: "HTTP_NAME_INVALID", Message: "invalid"}, {Field: "name", Code: "HTTP_NAME_INVALID", Message: "invalid"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.def.Validate(); err == nil {
				t.Fatal("Validate() = nil, want error")
			}
		})
	}
}

func TestWriteErrorDropsDataOutsideTheDefinitionSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definition := ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 400, Message: "bad request", DataSchema: reflect.TypeOf(testValidationData{})}
	engine := gin.New()
	engine.GET("/error", func(c *gin.Context) {
		WriteError(c, definition, nil, map[string]string{"secret": "value"})
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/error", nil))
	if got := recorder.Body.String(); got != `{"code":400,"error_code":"HTTP_INVALID","msg":"bad request","data":null}` {
		t.Fatalf("body = %s", got)
	}
}
