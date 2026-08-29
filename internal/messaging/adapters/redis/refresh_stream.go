package redisadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"admin/internal/messaging/application"
	goredis "github.com/redis/go-redis/v9"
)

const (
	refreshStreamRetention = 24 * time.Hour
	refreshPubSubChannel   = "admin:messaging:refresh"
)
const publishRefreshLua = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return false
end
local entryID = redis.call('XADD', KEYS[1], '*', 'event_id', ARGV[3], 'message_copy_id', ARGV[4], 'aggregate_version', ARGV[5], 'event_cursor', ARGV[6])
redis.call('SET', KEYS[2], entryID, 'EX', ARGV[1])
redis.call('XTRIM', KEYS[1], 'MINID', '~', ARGV[2])
return entryID
`

type scriptEvaluator interface {
	Eval(context.Context, string, []string, ...any) (any, error)
}

type redisScriptEvaluator struct{ client *goredis.Client }

func (executor redisScriptEvaluator) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	if executor.client == nil {
		return nil, fmt.Errorf("Redis client is unavailable")
	}
	return executor.client.Eval(ctx, script, keys, args...).Result()
}

type refreshNotifier interface {
	Publish(context.Context, string, string) error
}

type redisRefreshNotifier struct{ client *goredis.Client }

func (notifier redisRefreshNotifier) Publish(ctx context.Context, channel, payload string) error {
	if notifier.client == nil {
		return fmt.Errorf("Redis client is unavailable")
	}
	return notifier.client.Publish(ctx, channel, payload).Err()
}

type RefreshStreamPublisher struct {
	evaluator scriptEvaluator
	notifier  refreshNotifier
	clock     application.Clock
}

func NewRefreshStreamPublisher(client *goredis.Client) *RefreshStreamPublisher {
	return newRefreshStreamPublisherWithNotifier(redisScriptEvaluator{client: client}, redisRefreshNotifier{client: client}, application.ClockFunc(time.Now))
}

func newRefreshStreamPublisher(evaluator scriptEvaluator, clock application.Clock) *RefreshStreamPublisher {
	return newRefreshStreamPublisherWithNotifier(evaluator, nil, clock)
}

func newRefreshStreamPublisherWithNotifier(evaluator scriptEvaluator, notifier refreshNotifier, clock application.Clock) *RefreshStreamPublisher {
	if clock == nil {
		clock = application.ClockFunc(time.Now)
	}
	return &RefreshStreamPublisher{evaluator: evaluator, notifier: notifier, clock: clock}
}

func (publisher *RefreshStreamPublisher) PublishRefresh(ctx context.Context, event application.RefreshEvent) error {
	if publisher == nil || publisher.evaluator == nil || event.EventID == "" || event.Cursor == "" || event.MessageCopyID == 0 || event.UserID == 0 || event.AggregateVersion == 0 {
		return fmt.Errorf("invalid Redis refresh event")
	}
	minimumID := strconv.FormatInt(publisher.clock.Now().UTC().Add(-refreshStreamRetention).UnixMilli(), 10) + "-0"
	result, err := publisher.evaluator.Eval(ctx, publishRefreshLua, []string{refreshStreamKey(event.UserID), refreshDedupeKey(event.UserID, event.EventID)}, int64(refreshStreamRetention/time.Second), minimumID, event.EventID, event.MessageCopyID, event.AggregateVersion, event.Cursor)
	if err != nil {
		return err
	}
	streamID, created, err := refreshStreamEntryID(result)
	if err != nil {
		return err
	}
	if !created || publisher.notifier == nil {
		return nil
	}
	payload, err := json.Marshal(refreshNotice{EventID: event.EventID, Cursor: streamID, MessageCopyID: event.MessageCopyID, UserID: event.UserID, AggregateVersion: event.AggregateVersion})
	if err != nil {
		return fmt.Errorf("encode Redis refresh notice: %w", err)
	}
	return publisher.notifier.Publish(ctx, refreshPubSubChannel, string(payload))
}

func refreshStreamEntryID(result any) (string, bool, error) {
	switch value := result.(type) {
	case nil:
		return "", false, nil
	case bool:
		if !value {
			return "", false, nil
		}
	case string:
		if value != "" {
			return value, true, nil
		}
	case []byte:
		if len(value) != 0 {
			return string(value), true, nil
		}
	}
	return "", false, fmt.Errorf("invalid Redis refresh stream entry ID")
}

type refreshNotice struct {
	EventID          string `json:"event_id"`
	Cursor           string `json:"cursor"`
	MessageCopyID    uint   `json:"message_copy_id"`
	UserID           uint   `json:"user_id"`
	AggregateVersion uint64 `json:"aggregate_version"`
}

func refreshStreamKey(userID uint) string {
	return "admin:messaging:refresh:" + strconv.FormatUint(uint64(userID), 10)
}

func refreshDedupeKey(userID uint, eventID string) string {
	return "admin:messaging:refresh-dedupe:" + strconv.FormatUint(uint64(userID), 10) + ":" + eventID
}

var _ application.RefreshStream = (*RefreshStreamPublisher)(nil)
