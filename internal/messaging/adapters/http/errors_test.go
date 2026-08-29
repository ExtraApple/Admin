package httpadapter

import (
	"errors"
	"net/http"
	"testing"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

func TestClassifyMessageErrorUsesStableMSGDefinitions(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		code   string
		status int
	}{
		{name: "request", err: application.ErrInboxRequestInvalid, code: CodeRequestInvalid, status: http.StatusBadRequest},
		{name: "permission", err: application.ErrPermissionDenied, code: CodePermissionDenied, status: http.StatusForbidden},
		{name: "not found", err: application.ErrNotFound, code: CodeNotFound, status: http.StatusNotFound},
		{name: "conflict", err: application.ErrMessageImmutable, code: CodeStateConflict, status: http.StatusConflict},
		{name: "content", err: domain.ErrMessageContentUnsafe, code: CodeContentUnsafe, status: http.StatusUnprocessableEntity},
		{name: "internal", err: errors.New("database details must not escape"), code: CodeInternalError, status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := classifyMessageError(test.err)
			if definition.Code != test.code || definition.Status != test.status || definition.Owner != "messaging" {
				t.Fatalf("definition = %#v, want %s/%d", definition, test.code, test.status)
			}
		})
	}
}

func TestMessagingErrorDefinitionsAreValidAndMSGOwned(t *testing.T) {
	definitions := MessageErrorDefinitions()
	if len(definitions) == 0 {
		t.Fatal("MessageErrorDefinitions() returned no definitions")
	}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil || len(definition.Code) < 4 || definition.Code[:4] != "MSG_" {
			t.Fatalf("invalid messaging definition = %#v, err=%v", definition, err)
		}
	}
}
