package reminder

import (
	"context"
	"errors"
	"testing"
)

func TestDispatcherNotifierPrefersExactRoute(t *testing.T) {
	dispatcher := NewDispatcherNotifier()
	var calls []string

	mustRegisterDispatcher(t, dispatcher.RegisterChannel("email", NotifierFunc(func(context.Context, *Reminder) error {
		calls = append(calls, "channel")
		return nil
	})))
	mustRegisterDispatcher(t, dispatcher.RegisterType("payment_due", NotifierFunc(func(context.Context, *Reminder) error {
		calls = append(calls, "type")
		return nil
	})))
	mustRegisterDispatcher(t, dispatcher.Register("email", "payment_due", NotifierFunc(func(context.Context, *Reminder) error {
		calls = append(calls, "route")
		return nil
	})))

	err := dispatcher.Notify(context.Background(), &Reminder{
		Channel: "email",
		Payload: `{"type":"payment_due"}`,
	})
	if err != nil {
		t.Fatalf("expected notify to succeed, got %v", err)
	}
	if len(calls) != 1 || calls[0] != "route" {
		t.Fatalf("expected exact route handler to run, got %#v", calls)
	}
}

func TestDispatcherNotifierFallsBackToChannelWhenPayloadHasNoType(t *testing.T) {
	dispatcher := NewDispatcherNotifier()
	called := false

	mustRegisterDispatcher(t, dispatcher.RegisterChannel("in_app", NotifierFunc(func(context.Context, *Reminder) error {
		called = true
		return nil
	})))

	err := dispatcher.Notify(context.Background(), &Reminder{
		Channel: "in_app",
		Payload: `{"title":"hello"}`,
	})
	if err != nil {
		t.Fatalf("expected notify to succeed, got %v", err)
	}
	if !called {
		t.Fatalf("expected channel handler to run")
	}
}

func TestDispatcherNotifierUsesFallbackForInvalidPayload(t *testing.T) {
	dispatcher := NewDispatcherNotifier()
	called := false

	mustRegisterDispatcher(t, dispatcher.SetFallback(NotifierFunc(func(context.Context, *Reminder) error {
		called = true
		return nil
	})))

	err := dispatcher.Notify(context.Background(), &Reminder{
		Channel: "webhook",
		Payload: `{"type":`,
	})
	if err != nil {
		t.Fatalf("expected fallback handler to succeed, got %v", err)
	}
	if !called {
		t.Fatalf("expected fallback handler to run")
	}
}

func TestDispatcherNotifierReturnsDecodeErrorWithoutFallback(t *testing.T) {
	dispatcher := NewDispatcherNotifier()

	err := dispatcher.Notify(context.Background(), &Reminder{
		Channel: "webhook",
		Payload: `{"type":`,
	})
	if err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestDispatcherNotifierReturnsNoHandlerError(t *testing.T) {
	dispatcher := NewDispatcherNotifier()

	err := dispatcher.Notify(context.Background(), &Reminder{
		Channel: "sms",
		Payload: `{"type":"renewal"}`,
	})
	if !errors.Is(err, ErrNoNotifierHandler) {
		t.Fatalf("expected ErrNoNotifierHandler, got %v", err)
	}
}

func mustRegisterDispatcher(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected registration to succeed, got %v", err)
	}
}
