package redisadapter

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type evalFake struct {
	script string
	keys   []string
	args   []any
	result any
	err    error
}

func (executor *evalFake) Eval(_ context.Context, script string, keys []string, args ...any) (any, error) {
	executor.script = script
	executor.keys = append([]string(nil), keys...)
	executor.args = append([]any(nil), args...)
	return executor.result, executor.err
}

type publishFake struct {
	channel string
	payload string
	err     error
}

func (publisher *publishFake) Publish(_ context.Context, channel, payload string) error {
	publisher.channel = channel
	publisher.payload = payload
	return publisher.err
}

func TestRefreshStreamPublisherStoresMinimalEventWithAtomicLuaDedupe(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	executor := &evalFake{result: "1724451723000-0"}
	publisher := newRefreshStreamPublisher(executor, application.ClockFunc(func() time.Time { return now }))
	event := application.RefreshEvent{EventID: "event-1", Cursor: "event-1", MessageCopyID: 41, UserID: 2, AggregateVersion: 3}
	if err := publisher.PublishRefresh(context.Background(), event); err != nil {
		t.Fatalf("PublishRefresh() = %v", err)
	}
	expectedMinID := strconv.FormatInt(now.Add(-24*time.Hour).UnixMilli(), 10) + "-0"
	if len(executor.keys) != 2 || executor.keys[0] != "admin:messaging:refresh:2" || executor.keys[1] != "admin:messaging:refresh-dedupe:2:event-1" || len(executor.args) != 6 || executor.args[0] != int64(86400) || executor.args[1] != expectedMinID || executor.args[2] != "event-1" || executor.args[3] != uint(41) || executor.args[4] != uint64(3) || executor.args[5] != "event-1" {
		t.Fatalf("script keys=%#v args=%#v", executor.keys, executor.args)
	}
	for _, required := range []string{"XADD", "SET", "XTRIM"} {
		if !strings.Contains(executor.script, required) {
			t.Fatalf("Lua script missing %s: %s", required, executor.script)
		}
	}
	for _, forbidden := range []string{"markdown", "html", "token", "image", "url"} {
		if strings.Contains(executor.script, forbidden) {
			t.Fatalf("Lua script exposes forbidden %q: %s", forbidden, executor.script)
		}
	}
}

func TestRefreshStreamPublisherPublishesBatchThroughOneAtomicLuaCall(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	executor := &evalFake{result: []any{"1724451723000-0", "1724451723000-1"}}
	notifier := &publishFake{}
	publisher := newRefreshStreamPublisherWithNotifier(executor, notifier, application.ClockFunc(func() time.Time { return now }))
	events := []application.RefreshEvent{
		{EventID: "event-1", Cursor: "event-1", MessageCopyID: 41, UserID: 2, AggregateVersion: 3},
		{EventID: "event-2", Cursor: "event-2", MessageCopyID: 41, UserID: 3, AggregateVersion: 3},
	}
	if err := publisher.PublishRefreshBatch(context.Background(), events); err != nil {
		t.Fatalf("PublishRefreshBatch() = %v", err)
	}
	if len(executor.keys) != 4 || len(executor.args) != 11 || !strings.Contains(executor.script, "for") || !strings.Contains(notifier.payload, `"event_id":"event-2"`) {
		t.Fatalf("batch script keys=%#v args=%#v script=%q notice=%q", executor.keys, executor.args, executor.script, notifier.payload)
	}
}

func TestRefreshStreamPublisherPublishesMinimalOnlineRefreshAfterStreamWrite(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	executor := &evalFake{result: "1724461324000-0"}
	notifier := &publishFake{}
	publisher := newRefreshStreamPublisherWithNotifier(executor, notifier, application.ClockFunc(func() time.Time { return now }))
	event := application.RefreshEvent{EventID: "event-2", Cursor: "1724461323000-0", MessageCopyID: 42, UserID: 7, AggregateVersion: 4}
	if err := publisher.PublishRefresh(context.Background(), event); err != nil {
		t.Fatalf("PublishRefresh() = %v", err)
	}
	if notifier.channel != "admin:messaging:refresh" || !strings.Contains(notifier.payload, `"user_id":7`) || !strings.Contains(notifier.payload, `"event_id":"event-2"`) || !strings.Contains(notifier.payload, `"cursor":"1724461324000-0"`) {
		t.Fatalf("online refresh = channel %q payload %q", notifier.channel, notifier.payload)
	}
	for _, forbidden := range []string{"markdown", "html", "token", "image", "url"} {
		if strings.Contains(notifier.payload, forbidden) {
			t.Fatalf("online refresh exposes forbidden %q: %q", forbidden, notifier.payload)
		}
	}
}
