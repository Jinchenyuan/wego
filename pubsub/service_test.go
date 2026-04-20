package pubsub

import (
	"context"
	"errors"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func TestMessageValidate(t *testing.T) {
	if err := (Message{}).Validate(); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}

	msg := Message{Type: "order.created", Data: []byte(`{"id":1}`)}
	if err := msg.Validate(); err != nil {
		t.Fatalf("expected message to validate, got %v", err)
	}
}

func TestSubscriptionValidate(t *testing.T) {
	sub := Subscription{Topic: "orders", Group: "billing", Handler: HandlerFunc(func(context.Context, *Delivery) error { return nil })}
	if err := sub.Validate(); err != nil {
		t.Fatalf("expected subscription to validate, got %v", err)
	}

	if err := (Subscription{}).Validate(); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("expected ErrInvalidSubscription, got %v", err)
	}
}

func TestRouterRejectsDuplicateSubscription(t *testing.T) {
	router := NewRouter()
	sub := Subscription{Topic: "orders", Group: "billing", Handler: HandlerFunc(func(context.Context, *Delivery) error { return nil })}
	if err := router.Add(sub); err != nil {
		t.Fatalf("expected first add to succeed, got %v", err)
	}
	if err := router.Add(sub); !errors.Is(err, ErrDuplicateSubscription) {
		t.Fatalf("expected ErrDuplicateSubscription, got %v", err)
	}
}

func TestDecodeEntry(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	delivery, err := decodeEntry("orders", "billing", "worker-1", Entry{ID: "1-0", Values: map[string]string{
		fieldType:        "order.created",
		fieldData:        `{"id":1}`,
		fieldHeaders:     `{"trace_id":"abc"}`,
		fieldPublishedAt: now,
		fieldMaxDeliver:  "3",
	}})
	if err != nil {
		t.Fatalf("expected decode to succeed, got %v", err)
	}
	if delivery.Message.Type != "order.created" {
		t.Fatalf("expected type order.created, got %q", delivery.Message.Type)
	}
	if delivery.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", delivery.Attempt)
	}
}

func TestRetryDelay(t *testing.T) {
	policy := RetryPolicy{Backoff: []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}}
	if got := retryDelay(policy, 2); got != time.Second {
		t.Fatalf("expected first backoff bucket, got %s", got)
	}
	if got := retryDelay(policy, 4); got != 3*time.Second {
		t.Fatalf("expected last backoff bucket, got %s", got)
	}
	if got := retryDelay(policy, 1); got != 0 {
		t.Fatalf("expected zero delay for first attempt, got %s", got)
	}
}

func TestServiceRejectsSubscribeAfterStart(t *testing.T) {
	svc := NewService(&redis.Client{}, WithConsumerNamer(func() string { return "worker" }))
	svc.started.Store(true)
	err := svc.Subscribe(Subscription{Topic: "orders", Group: "billing", Handler: HandlerFunc(func(context.Context, *Delivery) error { return nil })})
	if !errors.Is(err, ErrServiceStarted) {
		t.Fatalf("expected ErrServiceStarted, got %v", err)
	}
}
