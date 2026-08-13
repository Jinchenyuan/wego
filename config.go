package wego

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Jinchenyuan/wego/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Config is the validated, typed configuration surface for a Mesa runtime.
// Loading files, environment variables, and secrets remains the application's responsibility.
type Config struct {
	Profile     Profile
	LogLevel    string
	HTTP        HTTPConfig
	PostgresDSN string
	Redis       RedisConfig
	Etcd        clientv3.Config
	Components  ComponentsConfig
}

// ConfigError identifies an invalid configuration field.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string { return fmt.Sprintf("config %s: %s", e.Field, e.Message) }

func (c Config) Validate() error {
	var result error
	invalid := func(field, message string) {
		result = errors.Join(result, &ConfigError{Field: field, Message: message})
	}
	if c.HTTP.Port < 0 || c.HTTP.Port > 65535 {
		invalid("http.port", "must be between 0 and 65535")
	}
	for field, value := range map[string]time.Duration{
		"http.read_header_timeout": c.HTTP.ReadHeaderTimeout,
		"http.read_timeout":        c.HTTP.ReadTimeout,
		"http.write_timeout":       c.HTTP.WriteTimeout,
		"http.idle_timeout":        c.HTTP.IdleTimeout,
		"http.request_timeout":     c.HTTP.RequestTimeout,
	} {
		if value < 0 {
			invalid(field, "must not be negative")
		}
	}
	if c.HTTP.MaxHeaderBytes < 0 {
		invalid("http.max_header_bytes", "must not be negative")
	}
	if c.HTTP.MaxBodyBytes < 0 {
		invalid("http.max_body_bytes", "must not be negative")
	}
	if c.Redis.Addr != "" {
		if _, _, err := net.SplitHostPort(c.Redis.Addr); err != nil {
			invalid("redis.addr", "must use host:port format")
		}
		if c.Redis.DB < 0 {
			invalid("redis.db", "must not be negative")
		}
	}
	for i, endpoint := range c.Etcd.Endpoints {
		if strings.TrimSpace(endpoint) == "" {
			invalid(fmt.Sprintf("etcd.endpoints[%d]", i), "must not be empty")
		}
	}
	if (c.Etcd.Username == "") != (c.Etcd.Password == "") {
		invalid("etcd.auth", "username and password must be configured together")
	}
	if c.Components.PubSub.Enabled && c.Redis.Addr == "" {
		invalid("components.pubsub", "requires redis.addr")
	}
	if c.Components.Reminder.Enabled && strings.TrimSpace(c.PostgresDSN) == "" {
		invalid("components.reminder", "requires postgres_dsn")
	}
	if c.Components.Reminder.Enabled && c.Components.Reminder.Notifier == nil {
		invalid("components.reminder.notifier", "is required")
	}
	return result
}

// Options validates c and converts it into Mesa options.
func (c Config) Options() ([]Options, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return []Options{
		WithProfile(c.Profile),
		WithLogLevel(logger.ParseLevel(c.LogLevel)),
		WithHTTPConfig(c.HTTP),
		WithDSN(c.PostgresDSN),
		WithRedisConfig(c.Redis),
		WithEtcdConfig(c.Etcd),
		WithComponents(c.Components),
	}, nil
}
