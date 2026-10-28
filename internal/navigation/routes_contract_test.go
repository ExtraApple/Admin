package navigation_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin/internal/navigation"
	"admin/internal/platform/configimpact"
	"github.com/gin-gonic/gin"
)

func TestMenuImpactPreviewIsReadOnlyAndRejectsInvalidSyncAndBinding(t *testing.T) {
	service, authorization, apis := newNavigationServiceFixtureWithAPIs(t)
	router := navigationTestRouter(service)
	beforeInvalidations := len(authorization.incremented)

	response := navigationHTTPJSON(t, router, http.MethodPost, "/api/admin/menus/impact-preview", `{"operation":"create","candidate":{"name":"Preview page","path":"/preview-page","type":2}}`, "")
	if response.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Fingerprint string `json:"preview_fingerprint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode preview response: %v", err)
	}
	if !configimpact.ValidFingerprint(envelope.Data.Fingerprint) {
		t.Fatalf("preview fingerprint=%q", envelope.Data.Fingerprint)
	}
	menus, err := service.MenuTree(t.Context())
	if err != nil || len(menus) != 0 || len(authorization.incremented) != beforeInvalidations {
		t.Fatalf("preview changed facts: menus=%#v error=%v invalidations=%#v", menus, err, authorization.incremented)
	}

	unknownParent := `{"routes":[{"name":"Unknown","path":"/unknown","parent_path":"/missing","type":2}],"expected_fingerprint":"` + strings.Repeat("0", 64) + `"}`
	response = navigationHTTPJSON(t, router, http.MethodPost, "/api/admin/menus/sync", unknownParent, "")
	if response.Code != http.StatusConflict {
		t.Fatalf("unknown parent status=%d body=%s", response.Code, response.Body.String())
	}

	directory, err := service.CreateMenu(t.Context(), navigation.CreateInput{Name: "Directory", Type: 1})
	if err != nil {
		t.Fatalf("create directory fixture: %v", err)
	}
	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Read", Method: "GET", Path: "/api/read", Status: 1, NeedAuth: 1}
	response = navigationHTTPJSON(t, router, http.MethodPost, "/api/admin/menus/"+itoa(directory.ID)+"/apis", `{"api_ids":[9],"expected_fingerprint":"`+strings.Repeat("0", 64)+`"}`, "")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("directory binding status=%d body=%s", response.Code, response.Body.String())
	}
}

func navigationTestRouter(service *navigation.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	for _, descriptor := range navigation.Routes(service) {
		router.Handle(descriptor.Method, descriptor.Path, descriptor.Handler)
	}
	return router
}

func navigationHTTPJSON(t *testing.T, router http.Handler, method, path, body, fingerprint string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if fingerprint != "" {
		request.Header.Set("X-Expected-Fingerprint", fingerprint)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func itoa(value uint) string {
	return fmt.Sprintf("%d", value)
}
