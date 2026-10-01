package httpresponse_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	apimetahttp "admin/internal/apimetadata/adapters/http"
	identityhttp "admin/internal/identity/adapters/http"
	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

type testValidationData struct {
	Fields []httpresponse.FieldError `json:"fields"`
}

func TestWriteSuccessUsesTheFourFieldEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/empty", func(c *gin.Context) {
		httpresponse.WriteSuccess(c, http.StatusOK, nil)
	})
	engine.GET("/list", func(c *gin.Context) {
		var values []string
		httpresponse.WriteSuccess(c, http.StatusOK, values)
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
	definition := httpresponse.ErrorDefinition{
		Owner:      "identity",
		Code:       "IDENTITY_INVALID",
		Status:     http.StatusUnprocessableEntity,
		Message:    "request is invalid",
		DataSchema: reflect.TypeOf(testValidationData{}),
		Fields:     []httpresponse.FieldErrorDefinition{{Field: "username", Code: "IDENTITY_USERNAME_INVALID", Message: "username is invalid"}},
	}
	cause := errors.New("database password=secret")
	data := testValidationData{Fields: []httpresponse.FieldError{{Field: "username", ErrorCode: "IDENTITY_USERNAME_INVALID", Message: "username is invalid"}}}
	engine := gin.New()
	engine.POST("/error", func(c *gin.Context) {
		httpresponse.WriteError(c, definition, cause, data)
		stored, ok := httpresponse.ErrorContextOf(c)
		if !ok || stored.Cause != cause || stored.Definition.Code != definition.Code {
			t.Fatalf("error context = %#v, want definition and cause", stored)
		}
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/error", bytes.NewReader(nil)))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	var envelope httpresponse.Envelope[testValidationData]
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
		def  httpresponse.ErrorDefinition
	}{
		{name: "missing owner", def: httpresponse.ErrorDefinition{Code: "HTTP_INVALID", Status: 400, Message: "bad request"}},
		{name: "invalid code", def: httpresponse.ErrorDefinition{Owner: "http", Code: "bad-code", Status: 400, Message: "bad request"}},
		{name: "invalid status", def: httpresponse.ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 200, Message: "bad request"}},
		{name: "missing message", def: httpresponse.ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 400}},
		{name: "duplicate field code", def: httpresponse.ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 422, Message: "bad request", Fields: []httpresponse.FieldErrorDefinition{{Field: "name", Code: "HTTP_NAME_INVALID", Message: "invalid"}, {Field: "name", Code: "HTTP_NAME_INVALID", Message: "invalid"}}}},
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
	definition := httpresponse.ErrorDefinition{Owner: "http", Code: "HTTP_INVALID", Status: 400, Message: "bad request", DataSchema: reflect.TypeOf(testValidationData{})}
	engine := gin.New()
	engine.GET("/error", func(c *gin.Context) {
		httpresponse.WriteError(c, definition, nil, map[string]string{"secret": "value"})
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/error", nil))
	if got := recorder.Body.String(); got != `{"code":400,"error_code":"HTTP_INVALID","msg":"bad request","data":null}` {
		t.Fatalf("body = %s", got)
	}
}

func TestSharedAPIResponseContractFixture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve response test path")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "testsupport", "testdata", "api-response-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name     string                                  `json:"name"`
			Status   int                                     `json:"status"`
			Envelope *httpresponse.Envelope[json.RawMessage] `json:"envelope"`
			Native   json.RawMessage                         `json:"native"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	definitions := map[string]httpresponse.ErrorDefinition{
		"HTTP_INTERNAL_ERROR": httpresponse.InternalErrorDefinition(),
	}
	for _, route := range identityhttp.Routes(nil, nil, nil, nil, nil, 5) {
		for _, response := range route.OpenAPI.Responses {
			for _, definition := range response.Errors {
				definitions[definition.Code] = definition
			}
		}
	}
	for _, definition := range apimetahttp.PermissionErrorDefinitions() {
		definitions[definition.Code] = definition
	}
	for _, test := range fixture.Cases {
		// Native success belongs to real handlers; an unknown top-level code only tests client localization.
		if len(test.Native) != 0 || test.Name == "future-localization-conflict" {
			continue
		}
		t.Run(test.Name, func(t *testing.T) {
			if test.Envelope == nil {
				t.Fatal("business fixture must declare an envelope")
			}
			expected := test.Envelope
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			if test.Status < 400 {
				switch test.Name {
				case "success-object":
					httpresponse.WriteSuccess(context, http.StatusOK, struct {
						ID int `json:"id"`
					}{ID: 7})
				case "success-null":
					httpresponse.WriteSuccess(context, http.StatusOK, httpresponse.NoDataValue())
				case "success-empty-array":
					var values []string
					httpresponse.WriteSuccess(context, http.StatusOK, values)
				default:
					t.Fatalf("success fixture %q needs a concrete writer input", test.Name)
				}
			} else {
				definition, exists := definitions[expected.ErrorCode]
				if !exists {
					t.Fatalf("no runtime definition for %s", expected.ErrorCode)
				}
				if test.Name == "future-localization-multiple-errors-same-field" {
					// Extend a copy only for the named future-localization case, not the runtime catalog.
					definition.Fields = append(append([]httpresponse.FieldErrorDefinition(nil), definition.Fields...), httpresponse.FieldErrorDefinition{
						Field: "password", Code: "FUTURE_PASSWORD_RULE", Message: "Password cannot contain the account name.",
					})
				}
				var data any
				if !bytes.Equal(expected.Data, []byte("null")) {
					if definition.DataSchema == nil {
						t.Fatalf("%s declares no public data schema", definition.Code)
					}
					value := reflect.New(definition.DataSchema)
					if err := json.Unmarshal(expected.Data, value.Interface()); err != nil {
						t.Fatal(err)
					}
					data = value.Elem().Interface()
					if validation, ok := data.(httpresponse.ValidationErrorData); ok {
						// Supply reverse order to prove deterministic public field ordering.
						for left, right := 0, len(validation.Fields)-1; left < right; left, right = left+1, right-1 {
							validation.Fields[left], validation.Fields[right] = validation.Fields[right], validation.Fields[left]
						}
						data = validation
					}
				}
				cause := errors.New("database password=secret")
				httpresponse.WriteError(context, definition, cause, data)
				stored, ok := httpresponse.ErrorContextOf(context)
				if !ok || stored.Cause != cause || stored.Definition.Code != expected.ErrorCode {
					t.Fatalf("internal context = %#v, want cause and public definition", stored)
				}

				unsafeRecorder := httptest.NewRecorder()
				unsafeContext, _ := gin.CreateTestContext(unsafeRecorder)
				httpresponse.WriteError(unsafeContext, definition, cause, map[string]any{"password": "secret", "username_exists": true})
				var unsafe httpresponse.Envelope[any]
				if err := json.Unmarshal(unsafeRecorder.Body.Bytes(), &unsafe); err != nil {
					t.Fatal(err)
				}
				if unsafeRecorder.Code != test.Status || unsafe.ErrorCode != expected.ErrorCode || unsafe.Data != nil || bytes.Contains(unsafeRecorder.Body.Bytes(), []byte("secret")) {
					t.Fatalf("unapproved data or cause escaped the schema: %s", unsafeRecorder.Body.String())
				}
			}
			if recorder.Code != test.Status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.Status)
			}
			var got map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			expectedJSON, err := json.Marshal(expected)
			if err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := json.Unmarshal(expectedJSON, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("public response = %s, want %s", recorder.Body.String(), expectedJSON)
			}
		})
	}
}
