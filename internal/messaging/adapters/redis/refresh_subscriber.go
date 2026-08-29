package redisadapter

import (
	"context"
	"encoding/json"
	"fmt"

	"admin/internal/messaging/application"
	goredis "github.com/redis/go-redis/v9"
)

// RefreshSubscriber receives the cross-process refresh notices emitted after a
// durable per-user stream entry has been written.
type RefreshSubscriber struct{ client *goredis.Client }

func NewRefreshSubscriber(client *goredis.Client) *RefreshSubscriber {
	return &RefreshSubscriber{client: client}
}

func (subscriber *RefreshSubscriber) Run(ctx context.Context, hub *application.RefreshHub) error {
	if subscriber == nil || subscriber.client == nil || hub == nil {
		return fmt.Errorf("Redis refresh subscriber is unavailable")
	}
	pubsub := subscriber.client.Subscribe(ctx, refreshPubSubChannel)
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	for {
		message, err := pubsub.ReceiveMessage(ctx)
		if err != nil {
			return err
		}
		event, err := decodeRefreshNotice(message.Payload)
		if err != nil {
			continue
		}
		hub.Publish(ctx, event)
	}
}

func decodeRefreshNotice(payload string) (application.RefreshEvent, error) {
	var notice refreshNotice
	if err := json.Unmarshal([]byte(payload), &notice); err != nil {
		return application.RefreshEvent{}, fmt.Errorf("decode Redis refresh notice: %w", err)
	}
	if notice.EventID == "" || notice.Cursor == "" || notice.MessageCopyID == 0 || notice.UserID == 0 || notice.AggregateVersion == 0 {
		return application.RefreshEvent{}, fmt.Errorf("invalid Redis refresh notice")
	}
	return application.RefreshEvent{EventID: notice.EventID, Cursor: notice.Cursor, MessageCopyID: notice.MessageCopyID, UserID: notice.UserID, AggregateVersion: notice.AggregateVersion}, nil
}
