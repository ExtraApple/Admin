package redisadapter

import (
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func TestParseRefreshStreamMessageUsesDurableEntryIDAsCursor(t *testing.T) {
	event, err := parseRefreshStreamMessage(goredis.XMessage{ID: "1724461324000-1", Values: map[string]any{"event_id": "event-2", "message_copy_id": "42", "aggregate_version": "4"}}, 7)
	if err != nil {
		t.Fatalf("parseRefreshStreamMessage() = %v", err)
	}
	if event.EventID != "event-2" || event.Cursor != "1724461324000-1" || event.MessageCopyID != 42 || event.UserID != 7 || event.AggregateVersion != 4 {
		t.Fatalf("event = %#v", event)
	}
}

func TestRefreshCursorOrderingRejectsInvalidCursorAndDetectsTrimmedHistory(t *testing.T) {
	if _, ok := compareRefreshStreamIDs("not-a-stream-id", "1724461324000-0"); ok {
		t.Fatal("invalid cursor accepted")
	}
	if comparison, ok := compareRefreshStreamIDs("1724461323999-9", "1724461324000-0"); !ok || comparison >= 0 {
		t.Fatalf("comparison=%d ok=%t", comparison, ok)
	}
}
