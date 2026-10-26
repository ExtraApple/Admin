package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jwtadapter "admin/internal/identity/adapters/jwt"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	"github.com/gin-gonic/gin"
)

type browserUsers struct{}

func (browserUsers) FindByUsername(context.Context, string) (domain.User, error) {
	return domain.User{ID: 7, Username: "alice", Status: 1}, nil
}
func (browserUsers) FindByID(context.Context, uint) (domain.User, error) {
	return domain.User{ID: 7, Username: "alice", Status: 1}, nil
}
func (browserUsers) ExistsByUsernameOrEmail(context.Context, string, string) (bool, error) {
	return false, nil
}
func (browserUsers) Create(context.Context, *domain.User) error { return nil }

type browserCaptcha struct{}

func (browserCaptcha) Verify(context.Context, string, string) bool { return true }

type browserPasswords struct{}

func (browserPasswords) Compare(string, string) error { return nil }
func (browserPasswords) Hash(string) (string, error)  { return "fixture-hash", nil }

type browserAttempts struct{}

func (browserAttempts) IsLocked(context.Context, string) (time.Duration, bool, error) {
	return 0, false, nil
}
func (browserAttempts) RecordFailure(context.Context, string) (int, error) { return 1, nil }
func (browserAttempts) Lock(context.Context, string, time.Duration) error  { return nil }
func (browserAttempts) Clear(context.Context, string) error                { return nil }

type browserAuthorization struct{}

func (browserAuthorization) EnsureVersion(context.Context, uint) (int, error) { return 3, nil }
func (browserAuthorization) Snapshot(context.Context, uint) (domain.AccessSnapshot, error) {
	return domain.AccessSnapshot{Version: 3}, nil
}

type browserSessions struct {
	active      map[string]domain.LoginSession
	results     map[string]application.TokenIssue
	rotateErr   error
	revokeErr   error
	creates     int
	rotations   int
	revocations int
}

func (store *browserSessions) Create(_ context.Context, session domain.LoginSession, previous *application.TokenClaims) error {
	store.creates++
	store.active[session.ID] = session
	if previous != nil {
		delete(store.active, previous.SessionID)
	}
	return nil
}
func (store *browserSessions) Read(_ context.Context, id string) (domain.LoginSession, error) {
	value, ok := store.active[id]
	if !ok {
		return domain.LoginSession{}, application.ErrSessionMissing
	}
	return value, nil
}
func (store *browserSessions) Rotate(_ context.Context, request application.RefreshRotation) (application.TokenIssue, error) {
	store.rotations++
	if store.rotateErr != nil {
		return application.TokenIssue{}, store.rotateErr
	}
	session, ok := store.active[request.Credential.SessionID]
	if !ok {
		return application.TokenIssue{}, application.ErrSessionMissing
	}
	key := session.ID + ":" + request.AttemptID
	if result, ok := store.results[key]; ok {
		return result, nil
	}
	if request.Credential.Generation != session.Generation || application.CredentialDigest(request.Credential.RefreshID) != session.RefreshDigest || request.Credential.Expired {
		return application.TokenIssue{}, application.ErrRefreshReuse
	}
	if request.Candidate.SessionID == "" {
		return application.TokenIssue{}, application.ErrRefreshUncommitted
	}
	store.active[session.ID] = application.SessionFromIssue(request.Candidate)
	store.results[key] = request.Candidate
	return request.Candidate, nil
}
func (store *browserSessions) Revoke(_ context.Context, claims application.TokenClaims) error {
	store.revocations++
	if store.revokeErr != nil {
		return store.revokeErr
	}
	delete(store.active, claims.SessionID)
	return nil
}

func browserProtocolFixture(t *testing.T, secure bool) (*gin.Engine, *browserSessions, *application.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := &browserSessions{active: make(map[string]domain.LoginSession), results: make(map[string]application.TokenIssue)}
	tokens := jwtadapter.NewService(jwtadapter.Config{Secret: "browser-protocol-fixture", AccessExpireMins: 15, RefreshExpireMins: 60})
	service := application.NewService(browserUsers{}, browserCaptcha{}, browserPasswords{}, browserAttempts{}, store, browserAuthorization{}, tokens)
	origin := "https://admin.example.test"
	if !secure {
		origin = "http://localhost:5173"
	}
	engine := gin.New()
	for _, route := range Routes(service, nil, nil, nil, nil, 2, BrowserSessionConfig{Origin: origin, Secure: secure}) {
		if route.Path == "/api/login" || route.Path == "/api/refresh" || route.Path == "/api/user/logout" {
			engine.Handle(route.Method, route.Path, route.Handler)
		}
	}
	engine.GET("/protected", AuthMiddleware(service), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return engine, store, service
}

func browserRequest(engine *gin.Engine, origin, path, body string, cookie *http.Cookie, attempt string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://upstream.internal"+path, strings.NewReader(body))
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Admin-Request", "1")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if attempt != "" {
		request.Header.Set("X-Refresh-Attempt-ID", attempt)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func browserEnvelope(t *testing.T, response *httptest.ResponseRecorder, status int, errorCode string) map[string]json.RawMessage {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal("invalid JSON envelope")
	}
	var code int
	var actualError string
	_ = json.Unmarshal(envelope["code"], &code)
	_ = json.Unmarshal(envelope["error_code"], &actualError)
	if response.Code != status || code != status || actualError != errorCode || len(envelope) != 4 || envelope["msg"] == nil || envelope["data"] == nil {
		t.Fatalf("unexpected browser protocol response: status=%d code=%d error=%q", response.Code, code, actualError)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authentication control response was cacheable")
	}
	return envelope
}

func loginBrowser(t *testing.T, engine *gin.Engine, origin string) (*http.Cookie, string, string) {
	t.Helper()
	response := browserRequest(engine, origin, "/api/login", `{"username":"alice","password":"fixture-password","captcha_id":"fixture","captcha_code":"123456"}`, nil, "")
	envelope := browserEnvelope(t, response, http.StatusOK, "")
	var data map[string]json.RawMessage
	if err := json.Unmarshal(envelope["data"], &data); err != nil {
		t.Fatal(err)
	}
	var access, sid string
	_ = json.Unmarshal(data["access_token"], &access)
	_ = json.Unmarshal(data["session_id"], &sid)
	if len(data) != 3 || data["refresh_token"] != nil || data["user"] == nil || access == "" || !application.ValidSessionIdentifier(sid) {
		t.Fatal("login violated Cookie-only response contract")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("login did not set exactly one Cookie")
	}
	return cookies[0], access, sid
}

func TestBrowserProtocolCookieRoundTripAndAccessIndependentLogout(t *testing.T) {
	for _, secure := range []bool{true, false} {
		t.Run(map[bool]string{true: "https", false: "explicit loopback http"}[secure], func(t *testing.T) {
			engine, store, service := browserProtocolFixture(t, secure)
			origin := "https://admin.example.test"
			if !secure {
				origin = "http://localhost:5173"
			}
			cookie, access, sid := loginBrowser(t, engine, origin)
			if cookie.Name != refreshCookieName || cookie.Domain != "" || cookie.Path != "/api" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure != secure || cookie.MaxAge != 0 || !cookie.Expires.IsZero() {
				t.Fatal("session Cookie attributes incorrect")
			}
			response := browserRequest(engine, origin, "/api/refresh", `{}`, cookie, "browser-attempt-000001")
			envelope := browserEnvelope(t, response, http.StatusOK, "")
			var data map[string]json.RawMessage
			if err := json.Unmarshal(envelope["data"], &data); err != nil {
				t.Fatal(err)
			}
			var refreshedSID string
			_ = json.Unmarshal(data["session_id"], &refreshedSID)
			if len(data) != 2 || data["refresh_token"] != nil || data["access_token"] == nil || refreshedSID != sid {
				t.Fatal("refresh violated Cookie-only response contract")
			}
			if _, err := service.Authenticate(context.Background(), access); err != nil {
				t.Fatal("rotation revoked unexpired earlier Access")
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Value == cookie.Value {
				t.Fatal("refresh did not replace credential")
			}
			response = browserRequest(engine, origin, "/api/user/logout", `{}`, cookies[0], "")
			envelope = browserEnvelope(t, response, http.StatusOK, "")
			if string(envelope["data"]) != "null" || store.revocations != 1 {
				t.Fatal("Cookie logout did not revoke without Access")
			}
			deleted := response.Result().Cookies()
			if len(deleted) != 1 || deleted[0].Name != cookie.Name || deleted[0].Path != cookie.Path || deleted[0].Domain != cookie.Domain || deleted[0].Secure != secure || !deleted[0].HttpOnly || deleted[0].SameSite != cookie.SameSite || deleted[0].MaxAge != -1 || !deleted[0].Expires.Before(time.Now()) {
				t.Fatal("logout did not delete the matching Cookie scope")
			}
			if _, err := service.Authenticate(context.Background(), access); err == nil {
				t.Fatal("logout left earlier Access active")
			}
			browserEnvelope(t, browserRequest(engine, origin, "/api/user/logout", `{}`, nil, ""), http.StatusOK, "")
		})
	}
}

func TestBrowserProtectionPrecedesCredentialAndCookieEffects(t *testing.T) {
	engine, store, _ := browserProtocolFixture(t, true)
	for _, path := range []string{"/api/login", "/api/refresh", "/api/user/logout"} {
		for _, failure := range []string{"missing origin", "null origin", "wrong origin", "forwarded origin", "missing guard", "wrong guard", "form content", "missing content", "cross site", "duplicate origin", "duplicate guard"} {
			t.Run(path+"/"+failure, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, "http://upstream.internal"+path, strings.NewReader(`{}`))
				request.Header.Set("Origin", "https://admin.example.test")
				request.Header.Set("X-Admin-Request", "1")
				request.Header.Set("Content-Type", "application/json")
				request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "not-processed"})
				switch failure {
				case "missing origin":
					request.Header.Del("Origin")
				case "null origin":
					request.Header.Set("Origin", "null")
				case "wrong origin":
					request.Header.Set("Origin", "https://admin.example.test.evil.test")
				case "forwarded origin":
					request.Header.Set("Origin", "https://evil.test")
					request.Header.Set("X-Forwarded-Host", "admin.example.test")
					request.Header.Set("X-Forwarded-Proto", "https")
				case "missing guard":
					request.Header.Del("X-Admin-Request")
				case "wrong guard":
					request.Header.Set("X-Admin-Request", "true")
				case "form content":
					request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				case "missing content":
					request.Header.Del("Content-Type")
				case "cross site":
					request.Header.Set("Sec-Fetch-Site", "cross-site")
					request.Header.Set("Origin", "https://evil.test")
				case "duplicate origin":
					request.Header.Add("Origin", "https://admin.example.test")
				case "duplicate guard":
					request.Header.Add("X-Admin-Request", "1")
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				envelope := browserEnvelope(t, response, http.StatusForbidden, "AUTHN_CSRF_INVALID")
				if string(envelope["data"]) != "null" || response.Header().Get("Set-Cookie") != "" || store.creates != 0 || store.rotations != 0 || store.revocations != 0 {
					t.Fatal("CSRF rejection had credential or Cookie effects")
				}
			})
		}
	}
}

func TestSessionControlsRejectOldJSONAndMalformedAttemptsBeforeConsumption(t *testing.T) {
	engine, store, _ := browserProtocolFixture(t, true)
	cookie, _, _ := loginBrowser(t, engine, "https://admin.example.test")
	for _, path := range []string{"/api/refresh", "/api/user/logout"} {
		for _, body := range []string{`{"refresh_token":"obsolete"}`, `{"other":1}`, `null`, `[]`, `{`, `{} {}`, ``} {
			response := browserRequest(engine, "https://admin.example.test", path, body, cookie, "browser-attempt-000001")
			browserEnvelope(t, response, http.StatusBadRequest, "HTTP_REQUEST_INVALID")
			if response.Header().Get("Set-Cookie") != "" || store.rotations != 0 || store.revocations != 0 {
				t.Fatal("bad protocol consumed a credential or changed Cookie")
			}
		}
	}
	for _, attempt := range []string{"", "short", strings.Repeat("x", 129), "not-a-valid attempt-id"} {
		response := browserRequest(engine, "https://admin.example.test", "/api/refresh", `{}`, cookie, attempt)
		browserEnvelope(t, response, http.StatusBadRequest, "HTTP_REQUEST_INVALID")
		if response.Header().Get("Set-Cookie") != "" || store.rotations != 0 {
			t.Fatal("invalid attempt consumed a credential")
		}
	}
}

func TestBrowserErrorMappingPreservesRetryCookiesAndBusinessResponses(t *testing.T) {
	engine, store, _ := browserProtocolFixture(t, true)
	cookie, _, _ := loginBrowser(t, engine, "https://admin.example.test")
	for _, test := range []struct {
		cause   error
		status  int
		code    string
		deletes bool
	}{
		{application.ErrRefreshSuperseded, http.StatusConflict, "AUTHN_REFRESH_SUPERSEDED", false},
		{errors.New("redis internal detail"), http.StatusInternalServerError, "IDENTITY_INTERNAL_ERROR", false},
		{application.ErrSessionMissing, http.StatusUnauthorized, "AUTHN_REFRESH_TOKEN_INVALID", true},
	} {
		store.rotateErr = test.cause
		response := browserRequest(engine, "https://admin.example.test", "/api/refresh", `{}`, cookie, "browser-attempt-000001")
		envelope := browserEnvelope(t, response, test.status, test.code)
		if string(envelope["data"]) != "null" || strings.Contains(response.Body.String(), "redis internal detail") {
			t.Fatal("unsafe error envelope")
		}
		cookies := response.Result().Cookies()
		if test.deletes && (len(cookies) != 1 || cookies[0].MaxAge != -1) || !test.deletes && len(cookies) != 0 {
			t.Fatal("error had incorrect Cookie effects")
		}
	}
	store.revokeErr = errors.New("redis internal detail")
	response := browserRequest(engine, "https://admin.example.test", "/api/user/logout", `{}`, cookie, "")
	browserEnvelope(t, response, http.StatusInternalServerError, "IDENTITY_INTERNAL_ERROR")
	if response.Header().Get("Set-Cookie") != "" {
		t.Fatal("failed logout cleared retry Cookie")
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if response.Code != http.StatusUnauthorized || response.Header().Get("Set-Cookie") != "" {
		t.Fatal("ordinary business rejection modified Cookie")
	}
}
