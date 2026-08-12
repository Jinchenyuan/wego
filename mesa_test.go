package wego

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jinchenyuan/wego/pubsub"
	"github.com/Jinchenyuan/wego/reminder"
	"github.com/Jinchenyuan/wego/transport"
	httptransport "github.com/Jinchenyuan/wego/transport/http"
	redis "github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
)

type lifecycleServer struct {
	startErr error
	started  atomic.Bool
	stopped  atomic.Int32
}

func (s *lifecycleServer) Start(context.Context) error { s.started.Store(true); return s.startErr }
func (s *lifecycleServer) Stop(context.Context) error  { s.stopped.Add(1); return nil }
func (s *lifecycleServer) GetType() transport.NetType  { return transport.TCP }

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
	mesa := &Mesa{state: StateRunning, componentIndex: make(map[string]Component)}

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

func TestNewWithoutInfrastructure(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.DB != nil || m.Redis != nil || m.etcdCtl != nil {
		t.Fatal("expected no infrastructure clients")
	}
	if got := m.State(); got != StateNew {
		t.Fatalf("state = %q", got)
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	srv := &lifecycleServer{}
	m, err := New(WithServers(srv))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	for deadline := time.Now().Add(time.Second); m.State() != StateRunning && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := srv.stopped.Load(); got != 1 {
		t.Fatalf("Stop calls = %d", got)
	}
	if got := m.State(); got != StateStopped {
		t.Fatalf("state = %q", got)
	}
}

func TestRunRollsBackStartedServers(t *testing.T) {
	started := &lifecycleServer{}
	failing := &lifecycleServer{startErr: fmt.Errorf("bind failed")}
	m, err := New(WithServers(started, failing))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = m.Run(context.Background())
	if err == nil {
		t.Fatal("expected startup error")
	}
	if got := started.stopped.Load(); got != 1 {
		t.Fatalf("rollback Stop calls = %d", got)
	}
	if got := m.State(); got != StateStopped {
		t.Fatalf("state = %q", got)
	}
}

func TestHealthRoutesBeforeRun(t *testing.T) {
	m, err := New(WithHttpPort(8080))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Shutdown(context.Background()) })
	srv := m.GetServerByType(transport.HTTP).(*httptransport.Server)

	live := httptest.NewRecorder()
	srv.GetEngine().ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("live status = %d", live.Code)
	}

	ready := httptest.NewRecorder()
	srv.GetEngine().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status = %d", ready.Code)
	}
}
