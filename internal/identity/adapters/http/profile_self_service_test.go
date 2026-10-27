package httpadapter_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "admin/internal/identity/adapters/http"
	passwordadapter "admin/internal/identity/adapters/password"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"

	"github.com/gin-gonic/gin"
)

type profileRepository struct {
	application.UserManagementRepository
	user           domain.User
	readErr        error
	lockErr        error
	postWriteErr   error
	writes         int
	passwordWrites int
	creates        int
}

func (repository *profileRepository) FindByID(context.Context, uint) (domain.User, error) {
	return repository.user, repository.readErr
}
func (repository *profileRepository) FindByIDForUpdate(ctx context.Context, id uint) (domain.User, error) {
	if repository.lockErr != nil {
		return domain.User{}, repository.lockErr
	}
	return repository.FindByID(ctx, id)
}
func (repository *profileRepository) Update(_ context.Context, _ uint, changes application.UserChanges) error {
	repository.writes++
	if changes.Nickname != nil {
		repository.user.Nickname = *changes.Nickname
	}
	if changes.Status != nil {
		repository.user.Status = *changes.Status
	}
	repository.readErr = repository.postWriteErr
	return nil
}

func (repository *profileRepository) ExistsByUsernameOrEmail(context.Context, string, string) (bool, error) {
	return false, nil
}
func (repository *profileRepository) Create(_ context.Context, user *domain.User) error {
	repository.creates++
	user.ID = 42
	repository.user = *user
	return nil
}
func (repository *profileRepository) UpdatePassword(_ context.Context, _ uint, password string) error {
	repository.passwordWrites++
	repository.user.Password = password
	return nil
}

type profileTransactions struct{}

func (profileTransactions) Run(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type profileAccess struct {
	application.AccessManager
	increments int
}

func (access *profileAccess) IsProtectedUser(context.Context, uint, uint) (bool, error) {
	return false, nil
}
func (access *profileAccess) ManagementUserScope(context.Context, uint) (domain.UserScope, error) {
	return domain.UserScope{All: true}, nil
}
func (access *profileAccess) IncrementVersions(context.Context, []uint) error {
	access.increments++
	return nil
}

func serveProfileRequest(t *testing.T, users *application.UserService, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveProfileServices(t, nil, users, method, path, body)
}

func serveProfileServices(t *testing.T, service *application.Service, users *application.UserService, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userID", uint(7)); c.Next() })
	for _, route := range httpadapter.Routes(service, users, nil, nil, nil, 5, httpadapter.BrowserSessionConfig{}) {
		router.Handle(route.Method, route.Path, route.Handler)
	}
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertProfileError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var gotCode string
	if err := json.Unmarshal(envelope["error_code"], &gotCode); err != nil {
		t.Fatal(err)
	}
	if response.Code != status || gotCode != code || string(envelope["data"]) != "null" || len(envelope) != 4 {
		t.Fatalf("response = %d %s; want safe four-field %d/%s/null", response.Code, response.Body, status, code)
	}
	if strings.Contains(response.Body.String(), "private-driver-fault") {
		t.Fatal("response leaked repository fault")
	}
	if len(response.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("profile failure changed session cookies")
	}
}

func TestProfileUserReadFaultClassification(t *testing.T) {
	for _, endpoint := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/user/info", ""},
		{http.MethodPut, "/api/user/info", `{"nickname":"changed"}`},
		{http.MethodPut, "/api/user/password", `{"old_password":"OldPass123!","new_password":"NewPass123!","confirm_password":"NewPass123!"}`},
	} {
		for _, failure := range []struct {
			name   string
			err    error
			status int
			code   string
		}{
			{"missing", application.ErrUserNotFound, 401, "AUTHN_TOKEN_INVALID"},
			{"wrapped-missing", fmt.Errorf("lookup: %w", application.ErrUserNotFound), 401, "AUTHN_TOKEN_INVALID"},
			{"storage-fault", errors.New("private-driver-fault"), 500, "IDENTITY_INTERNAL_ERROR"},
		} {
			t.Run(endpoint.method+endpoint.path+"/"+failure.name, func(t *testing.T) {
				repository := &profileRepository{readErr: failure.err}
				access := &profileAccess{}
				users := application.NewUserService(repository, nil, profileTransactions{}, access)
				assertProfileError(t, serveProfileRequest(t, users, endpoint.method, endpoint.path, endpoint.body), failure.status, failure.code)
				if repository.writes != 0 || access.increments != 0 {
					t.Fatal("read failure mutated identity")
				}
			})
		}
	}
}

func TestProfileCommittedUpdateReadFault(t *testing.T) {
	for _, endpoint := range []struct {
		name, path, body string
		increments       int
	}{
		{"self", "/api/user/info", `{"nickname":"committed"}`, 0},
		{"admin-nickname", "/api/admin/users/42", `{"nickname":"committed"}`, 0},
		{"admin-status", "/api/admin/users/42", `{"nickname":"committed","status":0}`, 1},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			repository := &profileRepository{user: domain.User{ID: 42, Nickname: "before", Status: 1}, postWriteErr: errors.New("private-driver-fault")}
			access := &profileAccess{}
			users := application.NewUserService(repository, nil, profileTransactions{}, access)
			assertProfileError(t, serveProfileRequest(t, users, http.MethodPut, endpoint.path, endpoint.body), 500, "IDENTITY_INTERNAL_ERROR")
			if repository.writes != 1 || repository.user.Nickname != "committed" || access.increments != endpoint.increments {
				t.Fatalf("committed state changed: writes=%d nickname=%q increments=%d", repository.writes, repository.user.Nickname, access.increments)
			}
		})
	}
}

type profileCaptcha struct{}

func (profileCaptcha) Verify(context.Context, string, string) bool { return true }

type profilePasswords struct {
	passwordadapter.Bcrypt
	hashCalls int
}

func (passwords *profilePasswords) Hash(value string) (string, error) {
	passwords.hashCalls++
	return passwords.Bcrypt.Hash(value)
}

func TestProfileNewPasswordByteLimit(t *testing.T) {
	for _, password := range []struct{ name, value string }{
		{"ascii-73", "Aa1!" + strings.Repeat("x", 69)},
		{"unicode-73", "Aa1!" + strings.Repeat("界", 23)},
		{"ascii-above-old-transport-bound", "Aa1!" + strings.Repeat("x", 252)},
	} {
		for _, registration := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/register=%t", password.name, registration), func(t *testing.T) {
				passwords := &profilePasswords{Bcrypt: passwordadapter.Bcrypt{Cost: 4}}
				oldHash, err := passwords.Bcrypt.Hash("OldPass123!")
				if err != nil {
					t.Fatal(err)
				}
				repository := &profileRepository{user: domain.User{ID: 7, Password: oldHash, Status: 1}}
				access := &profileAccess{}
				users := application.NewUserService(repository, passwords, profileTransactions{}, access)
				service := application.NewService(repository, profileCaptcha{}, passwords, nil, nil, nil, nil)
				method, path := http.MethodPut, "/api/user/password"
				payload := map[string]string{"old_password": "OldPass123!", "new_password": password.value, "confirm_password": password.value}
				if registration {
					method, path = http.MethodPost, "/api/register"
					payload = map[string]string{"username": "new-user", "email": "new@example.test", "password": password.value, "captcha_id": "captcha", "captcha_code": "123456"}
				}
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				response := serveProfileServices(t, service, users, method, path, string(body))
				var envelope struct {
					ErrorCode string `json:"error_code"`
					Data      struct {
						Fields []struct {
							Field     string `json:"field"`
							ErrorCode string `json:"error_code"`
						} `json:"fields"`
					} `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if response.Code != 422 || envelope.ErrorCode != "IDENTITY_VALIDATION_INVALID" || len(envelope.Data.Fields) != 1 || envelope.Data.Fields[0].Field != "password" || envelope.Data.Fields[0].ErrorCode != "IDENTITY_PASSWORD_INVALID" {
					t.Errorf("new password response = %d %s, want 422 password validation detail", response.Code, response.Body)
				}
				if passwords.hashCalls != 0 || repository.creates != 0 || repository.passwordWrites != 0 || access.increments != 0 || repository.user.Password != oldHash {
					t.Errorf("oversized password reached mutation: hashes=%d creates=%d passwordWrites=%d increments=%d", passwords.hashCalls, repository.creates, repository.passwordWrites, access.increments)
				}
			})
		}
	}
}

func TestProfileNewPasswordAccepts72Bytes(t *testing.T) {
	for _, value := range []string{"Aa1!" + strings.Repeat("x", 68), "Aa1!" + strings.Repeat("界", 22) + "xy"} {
		for _, registration := range []bool{true, false} {
			t.Run(fmt.Sprintf("bytes=%d/register=%t/unicode=%t", len(value), registration, strings.Contains(value, "界")), func(t *testing.T) {
				passwords := &profilePasswords{Bcrypt: passwordadapter.Bcrypt{Cost: 4}}
				oldHash, err := passwords.Bcrypt.Hash("OldPass123!")
				if err != nil {
					t.Fatal(err)
				}
				repository := &profileRepository{user: domain.User{ID: 7, Password: oldHash, Status: 1}}
				access := &profileAccess{}
				users := application.NewUserService(repository, passwords, profileTransactions{}, access)
				service := application.NewService(repository, profileCaptcha{}, passwords, nil, nil, nil, nil)
				method, path := http.MethodPut, "/api/user/password"
				payload := map[string]string{"old_password": "OldPass123!", "new_password": value, "confirm_password": value}
				wantCreates, wantPasswordWrites, wantIncrements := 0, 1, 1
				if registration {
					method, path = http.MethodPost, "/api/register"
					payload = map[string]string{"username": "new-user", "email": "new@example.test", "password": value, "captcha_id": "captcha", "captcha_code": "123456"}
					wantCreates, wantPasswordWrites, wantIncrements = 1, 0, 0
				}
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				response := serveProfileServices(t, service, users, method, path, string(body))
				if response.Code != 200 || passwords.hashCalls != 1 || repository.creates != wantCreates || repository.passwordWrites != wantPasswordWrites || access.increments != wantIncrements {
					t.Fatalf("72-byte setup = %d %s hashes=%d creates=%d updates=%d versions=%d", response.Code, response.Body, passwords.hashCalls, repository.creates, repository.passwordWrites, access.increments)
				}
				if err := passwords.Compare(repository.user.Password, value); err != nil {
					t.Fatal("stored hash does not match the complete password")
				}
				if strings.Contains(response.Body.String(), repository.user.Password) {
					t.Fatal("response exposed password hash")
				}
			})
		}
	}
}

func TestProfilePasswordValidationLayersAndOrder(t *testing.T) {
	oversized := "Aa1!" + strings.Repeat("x", 69)
	for _, test := range []struct {
		name, old, password, confirmation string
		status                            int
		field, code                       string
	}{
		{"required", "OldPass123!", "", "", 400, "", ""},
		{"minimum", "OldPass123!", "Aa1!", "Aa1!", 400, "", ""},
		{"unicode-minimum", "OldPass123!", "A1!界", "A1!界", 400, "", ""},
		{"complexity", "OldPass123!", "abcdef", "abcdef", 422, "password", "IDENTITY_PASSWORD_INVALID"},
		{"confirmation-before-policy", "OldPass123!", oversized, "different-password", 422, "confirm_password", "IDENTITY_PASSWORD_CONFIRMATION_INVALID"},
		{"policy-before-reuse", oversized, oversized, oversized, 422, "password", "IDENTITY_PASSWORD_INVALID"},
		{"reuse-before-read", "SamePass123!", "SamePass123!", "SamePass123!", 422, "new_password", "IDENTITY_PASSWORD_REUSE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &profileRepository{readErr: errors.New("private-driver-fault")}
			users := application.NewUserService(repository, nil, profileTransactions{}, &profileAccess{})
			body, err := json.Marshal(map[string]string{"old_password": test.old, "new_password": test.password, "confirm_password": test.confirmation})
			if err != nil {
				t.Fatal(err)
			}
			response := serveProfileRequest(t, users, http.MethodPut, "/api/user/password", string(body))
			var envelope struct {
				Data struct {
					Fields []struct {
						Field     string `json:"field"`
						ErrorCode string `json:"error_code"`
					} `json:"fields"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if response.Code != test.status {
				t.Fatalf("validation status = %d %s", response.Code, response.Body)
			}
			if test.field != "" && (len(envelope.Data.Fields) != 1 || envelope.Data.Fields[0].Field != test.field || envelope.Data.Fields[0].ErrorCode != test.code) {
				t.Fatalf("validation priority = %s", response.Body)
			}
		})
	}
}

func TestProfileAdminPrerequisiteReadFault(t *testing.T) {
	for _, locked := range []bool{false, true} {
		t.Run(fmt.Sprintf("locked=%t", locked), func(t *testing.T) {
			repository := &profileRepository{user: domain.User{ID: 42, Status: 1}}
			if locked {
				repository.lockErr = errors.New("private-driver-fault")
			} else {
				repository.readErr = errors.New("private-driver-fault")
			}
			access := &profileAccess{}
			users := application.NewUserService(repository, nil, profileTransactions{}, access)
			response := serveProfileRequest(t, users, http.MethodPut, "/api/admin/users/42", `{"nickname":"must-not-commit","status":0}`)
			assertProfileError(t, response, 500, "IDENTITY_INTERNAL_ERROR")
			if repository.writes != 0 || access.increments != 0 {
				t.Fatal("admin read fault committed mutation")
			}
		})
	}
}
