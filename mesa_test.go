package wego

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jinchenyuan/wego/pubsub"
	"github.com/Jinchenyuan/wego/reminder"
	redis "github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
)

type testComponent struct {
	started atomic.Bool
}

func (t *testComponent) Start(context.Context) error {
	t.started.Store(true)
	return nil
}

func (t *testComponent) Name() string {
	return "test-component"
}

func TestRegisterComponent(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}
	component := &testComponent{}

	if err := mesa.RegisterComponent(component); err != nil {
		t.Fatalf("expected component registration to succeed, got %v", err)
	}

	if len(mesa.components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(mesa.components))
	}
	if mesa.components[0] != component {
		t.Fatalf("expected stored component to match input")
	}
}

func TestRegisterComponentAfterRuntimeStarted(t *testing.T) {
	mesa := &Mesa{runtimeStarted: true, componentIndex: make(map[string]Component)}

	err := mesa.RegisterComponent(&testComponent{})
	if err == nil {
		t.Fatalf("expected registration to fail after runtime start")
	}
	if err != ErrRuntimeStarted {
		t.Fatalf("expected ErrRuntimeStarted, got %v", err)
	}
}

func TestGetComponent(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}
	component := &testComponent{}

	if err := mesa.RegisterComponent(component); err != nil {
		t.Fatalf("expected component registration to succeed, got %v", err)
	}

	got, ok := mesa.GetComponent("test-component")
	if !ok {
		t.Fatalf("expected component lookup to succeed")
	}
	if got != component {
		t.Fatalf("expected looked up component to match input")
	}
}

func TestRegisterComponentRejectsDuplicateName(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}

	if err := mesa.RegisterComponent(&testComponent{}); err != nil {
		t.Fatalf("expected first registration to succeed, got %v", err)
	}

	err := mesa.RegisterComponent(&testComponent{})
	if err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}
	if !errors.Is(err, ErrComponentExists) {
		t.Fatalf("expected ErrComponentExists, got %v", err)
	}
}

func TestGetComponentAs(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}
	component := &testComponent{}

	if err := mesa.RegisterComponent(component); err != nil {
		t.Fatalf("expected component registration to succeed, got %v", err)
	}

	typed, ok := GetComponentAs[*testComponent](mesa, "test-component")
	if !ok {
		t.Fatalf("expected typed lookup to succeed")
	}
	if typed != component {
		t.Fatalf("expected typed component to match input")
	}
}

func TestRegisterDefaultComponentsAddsPubSubWhenEnabled(t *testing.T) {
	mesa := &Mesa{
		Redis:          redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}),
		componentIndex: make(map[string]Component),
		opts: options{
			components: ComponentsConfig{
				PubSub: PubSubComponentConfig{
					Enabled: true,
					Name:    "custom-pubsub",
				},
			},
		},
	}
	t.Cleanup(func() {
		_ = mesa.Redis.Close()
	})

	if err := mesa.registerDefaultComponents(); err != nil {
		t.Fatalf("expected registerDefaultComponents to succeed, got %v", err)
	}

	svc, ok := GetComponentAs[*pubsub.Service](mesa, "custom-pubsub")
	if !ok {
		t.Fatalf("expected pubsub component to be registered")
	}
	if svc == nil {
		t.Fatalf("expected pubsub service to be non-nil")
	}
}

func TestRegisterDefaultComponentsAddsReminderWhenEnabled(t *testing.T) {
	mesa := &Mesa{
		DB:             &bun.DB{},
		componentIndex: make(map[string]Component),
		opts: options{
			components: ComponentsConfig{
				Reminder: ReminderComponentConfig{
					Enabled:      true,
					Notifier:     reminder.NotifierFunc(func(context.Context, *reminder.Reminder) error { return nil }),
					Name:         "custom-reminder",
					PollInterval: 2 * time.Second,
				},
			},
		},
	}

	if err := mesa.registerDefaultComponents(); err != nil {
		t.Fatalf("expected registerDefaultComponents to succeed, got %v", err)
	}

	svc, ok := GetComponentAs[*reminder.Service](mesa, "custom-reminder")
	if !ok || svc == nil {
		t.Fatalf("expected reminder component to be registered")
	}
}

func TestRegisterDefaultComponentsRequiresRedisForPubSub(t *testing.T) {
	mesa := &Mesa{
		componentIndex: make(map[string]Component),
		opts: options{
			components: ComponentsConfig{
				PubSub: PubSubComponentConfig{Enabled: true},
			},
		},
	}

	err := mesa.registerDefaultComponents()
	if !errors.Is(err, pubsub.ErrRedisNotConfigured) {
		t.Fatalf("expected ErrRedisNotConfigured, got %v", err)
	}
}

func TestRegisterDefaultComponentsRequiresReminderNotifier(t *testing.T) {
	mesa := &Mesa{
		DB:             &bun.DB{},
		componentIndex: make(map[string]Component),
		opts: options{
			components: ComponentsConfig{
				Reminder: ReminderComponentConfig{Enabled: true},
			},
		},
	}

	err := mesa.registerDefaultComponents()
	if !errors.Is(err, ErrReminderNotifierNotConfigured) {
		t.Fatalf("expected ErrReminderNotifierNotConfigured, got %v", err)
	}
}

func TestRegisterDefaultComponentsRequiresReminderDB(t *testing.T) {
	mesa := &Mesa{
		componentIndex: make(map[string]Component),
		opts: options{
			components: ComponentsConfig{
				Reminder: ReminderComponentConfig{
					Enabled:  true,
					Notifier: reminder.NotifierFunc(func(context.Context, *reminder.Reminder) error { return nil }),
				},
			},
		},
	}

	err := mesa.registerDefaultComponents()
	if !errors.Is(err, ErrReminderDBNotConfigured) {
		t.Fatalf("expected ErrReminderDBNotConfigured, got %v", err)
	}
}

func TestWithComponentsEnablesPubSub(t *testing.T) {
	var opts options
	approx := false
	dlqEnabled := false
	WithComponents(ComponentsConfig{
		PubSub: PubSubComponentConfig{
			Enabled:            true,
			Name:               "custom-pubsub",
			StreamPrefix:       "test:pubsub",
			StreamApproxMaxLen: &approx,
			DeadLetterEnabled:  &dlqEnabled,
		},
		Reminder: ReminderComponentConfig{
			Enabled:      true,
			Notifier:     reminder.NotifierFunc(func(context.Context, *reminder.Reminder) error { return nil }),
			Name:         "custom-reminder",
			PollInterval: time.Second,
		},
	})(&opts)

	if !opts.components.PubSub.Enabled {
		t.Fatalf("expected pubsub to be enabled")
	}
	if opts.components.PubSub.Name != "custom-pubsub" {
		t.Fatalf("expected custom pubsub name, got %q", opts.components.PubSub.Name)
	}
	if !opts.components.Reminder.Enabled {
		t.Fatalf("expected reminder to be enabled")
	}
	if opts.components.Reminder.Name != "custom-reminder" {
		t.Fatalf("expected custom reminder name, got %q", opts.components.Reminder.Name)
	}
}

func TestDefaultComponentRegistrarsIncludeBuiltins(t *testing.T) {
	if len(defaultComponentRegistrars) < 2 {
		t.Fatalf("expected built-in default component registrars to be present")
	}

	foundPubSub := false
	foundReminder := false
	for _, registrar := range defaultComponentRegistrars {
		switch registrar.name {
		case "pubsub":
			foundPubSub = true
		case "reminder":
			foundReminder = true
		}
	}

	if !foundPubSub {
		t.Fatalf("expected pubsub registrar to be present")
	}
	if !foundReminder {
		t.Fatalf("expected reminder registrar to be present")
	}
}
