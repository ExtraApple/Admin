package application_test

import (
	"context"
	"errors"
	"testing"

	"admin/internal/messaging/application"
)

type refreshSinkFake struct {
	events []application.RefreshEvent
	err    error
}

func (sink *refreshSinkFake) Send(_ context.Context, event application.RefreshEvent) error {
	sink.events = append(sink.events, event)
	return sink.err
}

func TestRefreshHubFansOutOnlyToConnectionsForTheTargetUser(t *testing.T) {
	hub := application.NewRefreshHub()
	first := &refreshSinkFake{}
	second := &refreshSinkFake{}
	other := &refreshSinkFake{}
	releaseFirst := hub.Register(7, first)
	hub.Register(7, second)
	hub.Register(8, other)
	event := application.RefreshEvent{EventID: "event-1", Cursor: "cursor-1", MessageCopyID: 41, UserID: 7, AggregateVersion: 2}
	hub.Publish(context.Background(), event)
	if len(first.events) != 1 || len(second.events) != 1 || len(other.events) != 0 {
		t.Fatalf("fanout first=%#v second=%#v other=%#v", first.events, second.events, other.events)
	}
	releaseFirst()
	hub.Publish(context.Background(), event)
	if len(first.events) != 1 || len(second.events) != 2 {
		t.Fatalf("release fanout first=%#v second=%#v", first.events, second.events)
	}
	second.err = errors.New("connection closed")
	hub.Publish(context.Background(), event)
	hub.Publish(context.Background(), event)
	if len(second.events) != 3 {
		t.Fatalf("failed connection not evicted: %#v", second.events)
	}
}
