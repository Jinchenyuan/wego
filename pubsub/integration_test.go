//go:build integration

package pubsub

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func TestStorePublishReadAckIntegration(t *testing.T) {
	rdb := newIntegrationRedisClient(t)
	ctx := context.Background()
	prefix := "wego:test:pubsub:store:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	store := NewStore(rdb, testOptions(prefix))

	topic := "orders.created"
	group := "billing"
	consumer := "billing-1"

	if err := store.EnsureGroup(ctx, topic, group); err != nil {
		t.Fatalf("expected EnsureGroup to succeed, got %v", err)
	}

	_, err := store.Publish(ctx, topic, Message{
		Key:  "order-1",
		Type: "order.created",
		Data: []byte(`{"id":"1"}`),
	})
	if err != nil {
		t.Fatalf("expected Publish to succeed, got %v", err)
	}

	entries, err := store.ReadGroup(ctx, Subscription{Topic: topic, Group: group}, consumer)
	if err != nil {
		t.Fatalf("expected ReadGroup to succeed, got %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	delivery, err := decodeEntry(topic, group, consumer, entries[0])
	if err != nil {
		t.Fatalf("expected decodeEntry to succeed, got %v", err)
	}
	if delivery.Message.Type != "order.created" {
		t.Fatalf("expected delivery type order.created, got %q", delivery.Message.Type)
	}

	if err := store.Ack(ctx, topic, group, entries[0].ID); err != nil {
		t.Fatalf("expected Ack to succeed, got %v", err)
	}

	pending, err := rdb.XPending(ctx, store.streamKey(topic), group).Result()
	if err != nil {
		t.Fatalf("expected XPending to succeed, got %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected pending count 0 after ack, got %d", pending.Count)
	}
}

func TestServiceRetryToDLQIntegration(t *testing.T) {
	rdb := newIntegrationRedisClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prefix := "wego:test:pubsub:service:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	attempts := make(chan int, 4)
	svc := NewService(
		rdb,
		WithName("pubsub-integration"),
		WithStreamPolicy(StreamPolicy{Prefix: prefix, Block: 10 * time.Millisecond, ReadCount: 1}),
		WithRetryPolicy(RetryPolicy{MaxDeliver: 2, Backoff: []time.Duration{40 * time.Millisecond}, IdleTimeout: 500 * time.Millisecond, ClaimInterval: 20 * time.Millisecond}),
		WithConsumerNamer(func() string { return "worker" }),
	)

	err := svc.Subscribe(Subscription{
		Topic: "payments.failed",
		Group: "notifier",
		Handler: HandlerFunc(func(context.Context, *Delivery) error {
			attempts <- 1
			return errors.New("boom")
		}),
	})
	if err != nil {
		t.Fatalf("expected Subscribe to succeed, got %v", err)
	}

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("expected Start to succeed, got %v", err)
	}

	_, err = svc.Publish(ctx, "payments.failed", Message{
		Key:        "payment-1",
		Type:       "payment.failed",
		Data:       []byte(`{"id":"p1"}`),
		MaxDeliver: 2,
	})
	if err != nil {
		t.Fatalf("expected Publish to succeed, got %v", err)
	}

	waitFor(t, time.Second, func() bool {
		length, err := rdb.ZCard(ctx, svc.store.retryKey("payments.failed")).Result()
		return err == nil && length == 1
	})

	waitFor(t, 3*time.Second, func() bool {
		return len(attempts) >= 2
	})

	waitFor(t, 3*time.Second, func() bool {
		length, err := rdb.XLen(ctx, svc.store.dlqKey("payments.failed")).Result()
		return err == nil && length == 1
	})

	msgs, err := rdb.XRangeN(ctx, svc.store.dlqKey("payments.failed"), "-", "+", 1).Result()
	if err != nil {
		t.Fatalf("expected XRangeN to succeed, got %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected one DLQ message, got %d", len(msgs))
	}
	if msgs[0].Values["reason"] != "max delivery exceeded" {
		t.Fatalf("expected DLQ reason max delivery exceeded, got %v", msgs[0].Values["reason"])
	}
}

func newIntegrationRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("WEGO_REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set WEGO_REDIS_TEST_ADDR to run Redis integration tests")
	}
	db := 15
	if rawDB := os.Getenv("WEGO_REDIS_TEST_DB"); rawDB != "" {
		parsed, err := strconv.Atoi(rawDB)
		if err != nil {
			t.Fatalf("invalid WEGO_REDIS_TEST_DB: %v", err)
		}
		db = parsed
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("WEGO_REDIS_TEST_PASSWORD"),
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = rdb.FlushDB(cleanupCtx).Err()
		_ = rdb.Close()
	})
	return rdb
}

func testOptions(prefix string) options {
	opt := defaultOptions()
	opt.stream.Prefix = prefix
	opt.stream.Block = 10 * time.Millisecond
	opt.stream.ReadCount = 4
	opt.retry.IdleTimeout = 20 * time.Millisecond
	opt.retry.ClaimInterval = 20 * time.Millisecond
	return opt
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}