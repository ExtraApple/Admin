package model

import (
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestExitVersionUserModelDoesNotDeclareLegacyTokenVersion(t *testing.T) {
	userType := reflect.TypeOf(User{})
	if _, ok := userType.FieldByName("TokenVersion"); ok {
		t.Fatal("exit-version User model still declares TokenVersion")
	}

	parsed, err := schema.Parse(
		&User{},
		&syncMapForUserExitTest,
		schema.NamingStrategy{},
	)
	if err != nil {
		t.Fatalf("parse exit-version User schema: %v", err)
	}
	if field := parsed.LookUpField("token_version"); field != nil {
		t.Fatalf(
			"exit-version User GORM schema still declares legacy column %q",
			field.DBName,
		)
	}
}

var syncMapForUserExitTest = sync.Map{}
