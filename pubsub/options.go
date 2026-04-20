package pubsub

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jinchenyuan/wego/logger"
)

type Option func(*options)

type RetryPolicy struct {
	MaxDeliver   int
	Backoff      []time.Duration
	IdleTimeout  time.Duration
	ClaimInterval time.Duration
}

type StreamPolicy struct {
	Prefix       string
	MaxLen       int64
	ApproxMaxLen bool
	Block        time.Duration
	ReadCount    int64
}

type DeadLetterPolicy struct {
	Enabled bool
	Suffix  string
}

type options struct {
	name          string
	logger        *logger.Logger
	retry         RetryPolicy
	stream        StreamPolicy
	dlq           DeadLetterPolicy
	consumerNamer func() string
	now           func() time.Time
}

func defaultOptions() options {
	return options{
		name:   "pubsub",
		retry:  RetryPolicy{MaxDeliver: 3, Backoff: []time.Duration{time.Second, 5 * time.Second, 30 * time.Second}, IdleTimeout: 30 * time.Second, ClaimInterval: time.Second},
		stream: StreamPolicy{Prefix: "wego:pubsub", MaxLen: 10000, ApproxMaxLen: true, Block: time.Second, ReadCount: 16},
		dlq:    DeadLetterPolicy{Enabled: true, Suffix: ":dlq"},
		consumerNamer: func() string {
			return fmt.Sprintf("consumer-%d", time.Now().UnixNano())
		},
		now: time.Now,
	}
}

func WithName(name string) Option {
	return func(o *options) {
		if strings.TrimSpace(name) != "" {
			o.name = strings.TrimSpace(name)
		}
	}
}

func WithLogger(l *logger.Logger) Option {
	return func(o *options) {
		if l != nil {
			o.logger = l
		}
	}
}

func WithRetryPolicy(policy RetryPolicy) Option {
	return func(o *options) {
		if policy.MaxDeliver > 0 {
			o.retry.MaxDeliver = policy.MaxDeliver
		}
		if policy.IdleTimeout > 0 {
			o.retry.IdleTimeout = policy.IdleTimeout
		}
		if policy.ClaimInterval > 0 {
			o.retry.ClaimInterval = policy.ClaimInterval
		}
		if len(policy.Backoff) > 0 {
			o.retry.Backoff = append([]time.Duration(nil), policy.Backoff...)
		}
	}
}

func WithStreamPolicy(policy StreamPolicy) Option {
	return func(o *options) {
		if strings.TrimSpace(policy.Prefix) != "" {
			o.stream.Prefix = strings.TrimSpace(policy.Prefix)
		}
		if policy.MaxLen > 0 {
			o.stream.MaxLen = policy.MaxLen
		}
		o.stream.ApproxMaxLen = policy.ApproxMaxLen
		if policy.Block > 0 {
			o.stream.Block = policy.Block
		}
		if policy.ReadCount > 0 {
			o.stream.ReadCount = policy.ReadCount
		}
	}
}

func WithDeadLetterPolicy(policy DeadLetterPolicy) Option {
	return func(o *options) {
		o.dlq.Enabled = policy.Enabled
		if policy.Suffix != "" {
			o.dlq.Suffix = policy.Suffix
		}
	}
}

func WithConsumerNamer(fn func() string) Option {
	return func(o *options) {
		if fn != nil {
			o.consumerNamer = fn
		}
	}
}

func WithNow(fn func() time.Time) Option {
	return func(o *options) {
		if fn != nil {
			o.now = fn
		}
	}
}
