package redisadapter

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"admin/internal/messaging/application"
	goredis "github.com/redis/go-redis/v9"
)

const refreshReplayLimit = 500

// RefreshReplayStore reads the durable, per-user Redis Stream written before a
// best-effort PubSub notification is emitted.
type RefreshReplayStore struct{ client *goredis.Client }

func NewRefreshReplayStore(client *goredis.Client) *RefreshReplayStore {
	return &RefreshReplayStore{client: client}
}

func (store *RefreshReplayStore) ReplayRefresh(ctx context.Context, userID uint, cursor string) (application.RefreshRecovery, error) {
	if store == nil || store.client == nil || userID == 0 {
		return application.RefreshRecovery{}, fmt.Errorf("Redis refresh replay store is unavailable")
	}
	key := refreshStreamKey(userID)
	oldest, err := store.client.XRangeN(ctx, key, "-", "+", 1).Result()
	if err != nil {
		return application.RefreshRecovery{}, err
	}
	if len(oldest) == 0 {
		return application.RefreshRecovery{}, nil
	}
	start := "-"
	if cursor != "" {
		comparison, valid := compareRefreshStreamIDs(cursor, oldest[0].ID)
		if !valid || comparison < 0 {
			return application.RefreshRecovery{FullRefresh: true}, nil
		}
		start = "(" + cursor
	}
	messages, err := store.client.XRangeN(ctx, key, start, "+", refreshReplayLimit).Result()
	if err != nil {
		return application.RefreshRecovery{}, err
	}
	recovery := application.RefreshRecovery{Events: make([]application.RefreshEvent, 0, len(messages))}
	for _, message := range messages {
		event, parseErr := parseRefreshStreamMessage(message, userID)
		if parseErr != nil {
			continue
		}
		recovery.Events = append(recovery.Events, event)
	}
	return recovery, nil
}

func parseRefreshStreamMessage(message goredis.XMessage, userID uint) (application.RefreshEvent, error) {
	eventID, ok := refreshStreamField(message.Values, "event_id")
	if !ok || eventID == "" || message.ID == "" || userID == 0 {
		return application.RefreshEvent{}, fmt.Errorf("invalid Redis refresh stream event")
	}
	messageCopy, ok := refreshStreamField(message.Values, "message_copy_id")
	if !ok {
		return application.RefreshEvent{}, fmt.Errorf("invalid Redis refresh stream message copy")
	}
	messageCopyID, err := strconv.ParseUint(messageCopy, 10, 0)
	if err != nil || messageCopyID == 0 {
		return application.RefreshEvent{}, fmt.Errorf("invalid Redis refresh stream message copy")
	}
	version, ok := refreshStreamField(message.Values, "aggregate_version")
	if !ok {
		return application.RefreshEvent{}, fmt.Errorf("invalid Redis refresh stream aggregate version")
	}
	aggregateVersion, err := strconv.ParseUint(version, 10, 64)
	if err != nil || aggregateVersion == 0 {
		return application.RefreshEvent{}, fmt.Errorf("invalid Redis refresh stream aggregate version")
	}
	return application.RefreshEvent{EventID: eventID, Cursor: message.ID, MessageCopyID: uint(messageCopyID), UserID: userID, AggregateVersion: aggregateVersion}, nil
}

func refreshStreamField(values map[string]any, field string) (string, bool) {
	value, found := values[field]
	if !found {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	default:
		return "", false
	}
}

func compareRefreshStreamIDs(left, right string) (int, bool) {
	leftParts, leftOK := parseRefreshStreamID(left)
	rightParts, rightOK := parseRefreshStreamID(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	if leftParts[0] != rightParts[0] {
		if leftParts[0] < rightParts[0] {
			return -1, true
		}
		return 1, true
	}
	if leftParts[1] < rightParts[1] {
		return -1, true
	}
	if leftParts[1] > rightParts[1] {
		return 1, true
	}
	return 0, true
}

func parseRefreshStreamID(value string) ([2]uint64, bool) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return [2]uint64{}, false
	}
	milliseconds, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return [2]uint64{}, false
	}
	sequence, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return [2]uint64{}, false
	}
	return [2]uint64{milliseconds, sequence}, true
}

var _ application.RefreshRecoveryStore = (*RefreshReplayStore)(nil)
