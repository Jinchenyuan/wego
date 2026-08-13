package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type Store struct {
	rdb  *redis.Client
	opts options
}

type retryEnvelope struct {
	Topic       string            `json:"topic"`
	Group       string            `json:"group"`
	Key         string            `json:"key"`
	Type        string            `json:"type"`
	Data        string            `json:"data"`
	Headers     map[string]string `json:"headers"`
	MaxDeliver  int               `json:"max_deliver"`
	Attempt     int               `json:"attempt"`
	PublishedAt time.Time         `json:"published_at"`
}

func NewStore(rdb *redis.Client, opts options) *Store {
	return &Store{rdb: rdb, opts: opts}
}

func (s *Store) EnsureGroup(ctx context.Context, topic string, group string) error {
	if s == nil || s.rdb == nil {
		return ErrRedisNotConfigured
	}
	err := s.rdb.XGroupCreateMkStream(ctx, s.streamKey(topic), group, "0").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

func (s *Store) Publish(ctx context.Context, topic string, msg Message) (string, error) {
	return s.publishAttempt(ctx, topic, msg, 1)
}

func (s *Store) publishAttempt(ctx context.Context, topic string, msg Message, attempt int) (string, error) {
	if s == nil || s.rdb == nil {
		return "", ErrRedisNotConfigured
	}
	fields, err := messageFieldsWithAttempt(msg, s.opts.now(), s.opts.retry.MaxDeliver, attempt)
	if err != nil {
		return "", err
	}
	args := &redis.XAddArgs{
		Stream: s.streamKey(topic),
		ID:     "*",
		Values: fields,
	}
	if s.opts.stream.MaxLen > 0 {
		args.MaxLen = s.opts.stream.MaxLen
		args.Approx = s.opts.stream.ApproxMaxLen
	}
	return s.rdb.XAdd(ctx, args).Result()
}

func (s *Store) ReadGroup(ctx context.Context, sub Subscription, consumer string) ([]Entry, error) {
	if s == nil || s.rdb == nil {
		return nil, ErrRedisNotConfigured
	}
	streams, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    sub.Group,
		Consumer: consumer,
		Streams:  []string{s.streamKey(sub.Topic), ">"},
		Count:    s.opts.stream.ReadCount,
		Block:    s.opts.stream.Block,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	return flattenEntries(streams), nil
}

func (s *Store) Ack(ctx context.Context, topic string, group string, ids ...string) error {
	if s == nil || s.rdb == nil {
		return ErrRedisNotConfigured
	}
	if len(ids) == 0 {
		return nil
	}
	return s.rdb.XAck(ctx, s.streamKey(topic), group, ids...).Err()
}

func (s *Store) AutoClaim(ctx context.Context, sub Subscription, consumer string, start string, count int64) ([]Entry, string, error) {
	if s == nil || s.rdb == nil {
		return nil, start, ErrRedisNotConfigured
	}
	if start == "" {
		start = "0-0"
	}
	msgs, next, err := s.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   s.streamKey(sub.Topic),
		Group:    sub.Group,
		Consumer: consumer,
		MinIdle:  effectiveRetry(sub, s.opts.retry).IdleTimeout,
		Start:    start,
		Count:    count,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, start, nil
		}
		return nil, start, err
	}
	entries := make([]Entry, 0, len(msgs))
	for _, msg := range msgs {
		values := toStringMap(msg.Values)
		if retryCount, retryErr := s.pendingRetryCount(ctx, sub.Topic, sub.Group, msg.ID); retryErr == nil && retryCount > 0 {
			values[fieldAttempt] = fmt.Sprint(retryCount)
		}
		entries = append(entries, Entry{ID: msg.ID, Values: values})
	}
	return entries, next, nil
}

func (s *Store) MoveToDLQ(ctx context.Context, topic string, delivery *Delivery, reason string) (string, error) {
	if s == nil || s.rdb == nil {
		return "", ErrRedisNotConfigured
	}
	if delivery == nil {
		return "", ErrInvalidMessage
	}
	values := map[string]any{
		fieldKey:         delivery.Message.Key,
		fieldType:        delivery.Message.Type,
		fieldData:        string(delivery.Message.Data),
		fieldHeaders:     delivery.Raw[fieldHeaders],
		fieldPublishedAt: delivery.PublishedAt.UTC().Format(time.RFC3339Nano),
		fieldMaxDeliver:  delivery.Message.MaxDeliver,
		fieldAttempt:     delivery.Attempt,
		"reason":         reason,
		"source_id":      delivery.ID,
		"source_group":   delivery.Group,
	}
	args := []any{s.dlqKey(topic), s.streamKey(topic), delivery.Group, delivery.ID}
	for key, value := range values {
		args = append(args, key, value)
	}
	return dlqAndAckScript.Run(ctx, s.rdb, nil, args...).Text()
}

func (s *Store) ScheduleRetry(ctx context.Context, topic string, delivery *Delivery, dueAt time.Time, nextAttempt int) error {
	if s == nil || s.rdb == nil {
		return ErrRedisNotConfigured
	}
	if delivery == nil || nextAttempt <= 0 || dueAt.IsZero() {
		return ErrInvalidMessage
	}
	payload, err := json.Marshal(retryEnvelope{
		Topic:       normalizeTopic(topic),
		Group:       normalizeGroup(delivery.Group),
		Key:         delivery.Message.Key,
		Type:        delivery.Message.Type,
		Data:        string(delivery.Message.Data),
		Headers:     delivery.Message.Headers,
		MaxDeliver:  delivery.Message.MaxDeliver,
		Attempt:     nextAttempt,
		PublishedAt: delivery.PublishedAt,
	})
	if err != nil {
		return err
	}
	return retryAndAckScript.Run(ctx, s.rdb, []string{s.retryKey(topic, delivery.Group), s.streamKey(topic)}, dueAt.UnixMilli(), string(payload), delivery.Group, delivery.ID).Err()
}

func (s *Store) PromoteDueRetries(ctx context.Context, topic string, group string, limit int64, now time.Time) (int64, error) {
	if s == nil || s.rdb == nil {
		return 0, ErrRedisNotConfigured
	}
	if limit <= 0 {
		limit = 1
	}
	result, err := retryPromoteScript.Run(ctx, s.rdb, []string{s.retryKey(topic, group), s.streamKey(topic)}, now.UnixMilli(), limit).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return result, nil
}

func (s *Store) PublishRetry(ctx context.Context, envelope retryEnvelope) (string, error) {
	message := Message{
		Key:        envelope.Key,
		Type:       envelope.Type,
		Data:       []byte(envelope.Data),
		Headers:    envelope.Headers,
		MaxDeliver: envelope.MaxDeliver,
	}
	return s.publishAttempt(ctx, envelope.Topic, message, envelope.Attempt)
}

func (s *Store) streamKey(topic string) string {
	return fmt.Sprintf("%s:%s", s.opts.stream.Prefix, normalizeTopic(topic))
}

func (s *Store) dlqKey(topic string) string {
	return s.streamKey(topic) + s.opts.dlq.Suffix
}

func (s *Store) retryKey(topic string, group string) string {
	return s.streamKey(topic) + ":retry:" + normalizeGroup(group)
}

func flattenEntries(streams []redis.XStream) []Entry {
	entries := make([]Entry, 0)
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			entries = append(entries, Entry{ID: msg.ID, Values: toStringMap(msg.Values)})
		}
	}
	return entries
}

func toStringMap(values map[string]any) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = fmt.Sprint(value)
	}
	return result
}

func effectiveRetry(sub Subscription, defaults RetryPolicy) RetryPolicy {
	retry := defaults
	if sub.Retry.MaxDeliver > 0 {
		retry.MaxDeliver = sub.Retry.MaxDeliver
	}
	if sub.Retry.IdleTimeout > 0 {
		retry.IdleTimeout = sub.Retry.IdleTimeout
	}
	if sub.Retry.ClaimInterval > 0 {
		retry.ClaimInterval = sub.Retry.ClaimInterval
	}
	if len(sub.Retry.Backoff) > 0 {
		retry.Backoff = append([]time.Duration(nil), sub.Retry.Backoff...)
	}
	return retry
}

func (s *Store) pendingRetryCount(ctx context.Context, topic string, group string, id string) (int64, error) {
	items, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: s.streamKey(topic),
		Group:  group,
		Start:  id,
		End:    id,
		Count:  1,
	}).Result()
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	return items[0].RetryCount, nil
}

var retryAndAckScript = redis.NewScript(`
redis.call("ZADD", KEYS[1], ARGV[1], ARGV[2])
return redis.call("XACK", KEYS[2], ARGV[3], ARGV[4])
`)

var dlqAndAckScript = redis.NewScript(`
local id = redis.call("XADD", ARGV[1], "*", unpack(ARGV, 5))
redis.call("XACK", ARGV[2], ARGV[3], ARGV[4])
return id
`)

var retryPromoteScript = redis.NewScript(`
local items = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1], "LIMIT", 0, ARGV[2])
local promoted = 0
for _, raw in ipairs(items) do
  local item = cjson.decode(raw)
  local headers = cjson.encode(item.headers or {})
  redis.call("XADD", KEYS[2], "*",
    "key", item.key or "",
    "type", item.type or "",
    "data", item.data or "",
    "headers", headers,
    "published_at", item.published_at,
    "max_deliver", item.max_deliver,
    "attempt", item.attempt)
  redis.call("ZREM", KEYS[1], raw)
  promoted = promoted + 1
end
return promoted
`)
