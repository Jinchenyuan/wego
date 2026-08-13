package wego

import (
	"context"
	"errors"
	"testing"

	"github.com/Jinchenyuan/wego/reminder"
)

func TestConfigValidateReportsField(t *testing.T) {
	cfg := Config{
		HTTP: HTTPConfig{Port: 70000},
		Components: ComponentsConfig{
			PubSub: PubSubComponentConfig{Enabled: true},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var configErr *ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected ConfigError, got %T", err)
	}
}

func TestConfigOptions(t *testing.T) {
	cfg := Config{
		HTTP:        HTTPConfig{Port: 8080},
		PostgresDSN: "postgres://localhost/example",
		Redis:       RedisConfig{Addr: "127.0.0.1:6379"},
		Components: ComponentsConfig{Reminder: ReminderComponentConfig{
			Enabled:  true,
			Notifier: reminder.NotifierFunc(func(_ context.Context, _ *reminder.Reminder) error { return nil }),
		}},
	}
	opts, err := cfg.Options()
	if err != nil {
		t.Fatalf("Options() error = %v", err)
	}
	var applied options
	for _, opt := range opts {
		opt(&applied)
	}
	if applied.HttpPort != 8080 || applied.RedisConfig.Addr != cfg.Redis.Addr || applied.dsn != cfg.PostgresDSN {
		t.Fatalf("options not applied: %+v", applied)
	}
}
