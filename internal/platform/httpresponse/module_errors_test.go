package httpresponse_test

import (
	"errors"
	"testing"

	apimetaapp "admin/internal/apimetadata/application"
	auditapp "admin/internal/audit"
	authapp "admin/internal/authorization/application"
	dictapp "admin/internal/dictionary"
	identityapp "admin/internal/identity/application"
	navapp "admin/internal/navigation"
	orgapp "admin/internal/organization"
)

func TestModuleErrorsPreserveCodeAndCause(t *testing.T) {
	cause := errors.New("database unavailable")
	tests := []struct {
		name string
		make func() (string, error)
	}{
		{"identity", func() (string, error) {
			err := identityapp.NewError(identityapp.CodeInternalError, cause)
			code, _ := identityapp.CodeOf(err)
			return string(code), err
		}},
		{"authorization", func() (string, error) {
			err := authapp.NewError(authapp.CodeInternalError, cause)
			code, _ := authapp.CodeOf(err)
			return string(code), err
		}},
		{"navigation", func() (string, error) {
			err := navapp.NewError(navapp.CodeInternalError, cause)
			code, _ := navapp.CodeOf(err)
			return string(code), err
		}},
		{"api metadata", func() (string, error) {
			err := apimetaapp.NewError(apimetaapp.CodeInternalError, cause)
			code, _ := apimetaapp.CodeOf(err)
			return string(code), err
		}},
		{"organization", func() (string, error) {
			err := orgapp.NewError(orgapp.CodeInternalError, cause)
			code, _ := orgapp.CodeOf(err)
			return string(code), err
		}},
		{"dictionary", func() (string, error) {
			err := dictapp.NewError(dictapp.CodeInternalError, cause)
			code, _ := dictapp.CodeOf(err)
			return string(code), err
		}},
		{"audit", func() (string, error) {
			err := auditapp.NewError(auditapp.CodeInternalError, cause)
			code, _ := auditapp.CodeOf(err)
			return string(code), err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, err := test.make()
			if code == "" || !errors.Is(err, cause) {
				t.Fatalf("code=%q error=%v, want code and preserved cause", code, err)
			}
		})
	}
}
