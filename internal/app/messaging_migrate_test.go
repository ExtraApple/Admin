package app_test

import (
	"testing"

	"admin/internal/app"
	identitymodule "admin/internal/identity"
	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/testsupport/testutil"
)

func TestMigrateCollectsMessagingModelsAndConstraints(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate application database: %v", err)
	}

	for _, model := range messaginggorm.Models() {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("migration did not create table for %T", model)
		}
	}
	if !db.Migrator().HasTable("email_verification_credentials") || !db.Migrator().HasColumn(&identitymodule.User{}, "pending_email") || !db.Migrator().HasColumn(&identitymodule.User{}, "email_verified_at") {
		t.Fatal("migration did not create Identity email verification state")
	}
	for _, constraint := range []struct {
		model any
		name  string
	}{
		{messaginggorm.MessageCategory{}, "ux_message_category_org_code"},
		{messaginggorm.MessageAudience{}, "ux_message_audience_rule"},
		{messaginggorm.MessageRecipient{}, "ux_message_recipient"},
		{messaginggorm.MessageUserState{}, "ux_message_user_state"},
		{messaginggorm.MessageOutbox{}, "ux_message_outbox_copy_version"},
		{messaginggorm.MessageEventConsumption{}, "ux_message_consumption_consumer_event"},
		{messaginggorm.MessageEventConsumerCursor{}, "ux_message_cursor_consumer_copy"},
		{messaginggorm.MessageEventDelivery{}, "ux_message_delivery_consumer_event_user"},
		{messaginggorm.MessageConsumerDeadLetter{}, "ux_message_dead_letter_consumer_event"},
	} {
		if !db.Migrator().HasIndex(constraint.model, constraint.name) {
			t.Fatalf("migration did not create index %s for %T", constraint.name, constraint.model)
		}
	}
}
