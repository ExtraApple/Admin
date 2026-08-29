package domain_test

import (
	"errors"
	"testing"

	"admin/internal/messaging/domain"
)

func TestMessageLifecycleAllowsOnlyDeclaredTransitions(t *testing.T) {
	tests := []struct {
		name   string
		kind   domain.MessageKind
		from   domain.MessageStatus
		to     domain.MessageStatus
		wantOK bool
	}{
		{name: "announcement draft schedules", kind: domain.MessageKindAnnouncement, from: domain.MessageStatusDraft, to: domain.MessageStatusScheduled, wantOK: true},
		{name: "announcement draft publishes", kind: domain.MessageKindAnnouncement, from: domain.MessageStatusDraft, to: domain.MessageStatusPublished, wantOK: true},
		{name: "announcement scheduled publishes", kind: domain.MessageKindAnnouncement, from: domain.MessageStatusScheduled, to: domain.MessageStatusPublished, wantOK: true},
		{name: "published announcement expires", kind: domain.MessageKindAnnouncement, from: domain.MessageStatusPublished, to: domain.MessageStatusExpired, wantOK: true},
		{name: "published announcement revokes", kind: domain.MessageKindAnnouncement, from: domain.MessageStatusPublished, to: domain.MessageStatusRevoked, wantOK: true},
		{name: "draft announcement cannot expire", kind: domain.MessageKindAnnouncement, from: domain.MessageStatusDraft, to: domain.MessageStatusExpired, wantOK: false},
		{name: "expired announcement is terminal", kind: domain.MessageKindAnnouncement, from: domain.MessageStatusExpired, to: domain.MessageStatusPublished, wantOK: false},
		{name: "private message is published at creation", kind: domain.MessageKindPrivate, from: domain.MessageStatusPublished, to: domain.MessageStatusRevoked, wantOK: true},
		{name: "private message cannot be edited", kind: domain.MessageKindPrivate, from: domain.MessageStatusPublished, to: domain.MessageStatusDraft, wantOK: false},
		{name: "broadcast message is published at creation", kind: domain.MessageKindBroadcast, from: domain.MessageStatusPublished, to: domain.MessageStatusRevoked, wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := domain.CanTransitionMessage(test.kind, test.from, test.to); got != test.wantOK {
				t.Fatalf("CanTransitionMessage(%q, %q, %q) = %v, want %v", test.kind, test.from, test.to, got, test.wantOK)
			}
		})
	}
}

func TestMessageLifecycleRejectsInvalidStateAndKind(t *testing.T) {
	if _, err := domain.TransitionMessage(domain.MessageKindAnnouncement, domain.MessageStatusDraft, domain.MessageStatusExpired); !errors.Is(err, domain.ErrInvalidMessageTransition) {
		t.Fatalf("invalid transition error = %v, want ErrInvalidMessageTransition", err)
	}
	if _, err := domain.TransitionMessage(domain.MessageKind("unknown"), domain.MessageStatusDraft, domain.MessageStatusPublished); !errors.Is(err, domain.ErrInvalidMessageKind) {
		t.Fatalf("invalid kind error = %v, want ErrInvalidMessageKind", err)
	}
	if _, err := domain.TransitionMessage(domain.MessageKindAnnouncement, domain.MessageStatus("unknown"), domain.MessageStatusPublished); !errors.Is(err, domain.ErrInvalidMessageStatus) {
		t.Fatalf("invalid status error = %v, want ErrInvalidMessageStatus", err)
	}
}

func TestInitialMessageStatusMatchesMessageKind(t *testing.T) {
	tests := []struct {
		kind domain.MessageKind
		want domain.MessageStatus
	}{
		{kind: domain.MessageKindPrivate, want: domain.MessageStatusPublished},
		{kind: domain.MessageKindBroadcast, want: domain.MessageStatusPublished},
		{kind: domain.MessageKindAnnouncement, want: domain.MessageStatusDraft},
	}
	for _, test := range tests {
		got, err := domain.InitialMessageStatus(test.kind)
		if err != nil || got != test.want {
			t.Fatalf("InitialMessageStatus(%q) = %q, %v; want %q", test.kind, got, err, test.want)
		}
	}
}
