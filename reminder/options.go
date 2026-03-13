package reminder

import (
	"time"

	"github.com/Jinchenyuan/wego/logger"
)

type Option func(*options)

type options struct {
	name            string
	pollInterval    time.Duration
	batchSize       int
	processingTTL   time.Duration
	autoCreateTable bool
	logger          *logger.Logger
	retryDelays     []time.Duration
	now             func() time.Time
}

func defaultOptions() options {
	return options{
		name:            "reminder",
		pollInterval:    time.Second,
		batchSize:       32,
		processingTTL:   2 * time.Minute,
		autoCreateTable: true,
		retryDelays:     []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute},
		now:             time.Now,
	}
}

func WithName(name string) Option {
	return func(o *options) {
		if name != "" {
			o.name = name
		}
	}
}

func WithPollInterval(interval time.Duration) Option {
	return func(o *options) {
		if interval > 0 {
			o.pollInterval = interval
		}
	}
}

func WithBatchSize(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.batchSize = size
		}
	}
}

func WithProcessingTTL(ttl time.Duration) Option {
	return func(o *options) {
		if ttl > 0 {
			o.processingTTL = ttl
		}
	}
}

func WithAutoCreateTable(enabled bool) Option {
	return func(o *options) {
		o.autoCreateTable = enabled
	}
}

func WithLogger(l *logger.Logger) Option {
	return func(o *options) {
		o.logger = l
	}
}

func WithRetryDelays(delays ...time.Duration) Option {
	return func(o *options) {
		if len(delays) == 0 {
			return
		}

		filtered := make([]time.Duration, 0, len(delays))
		for _, delay := range delays {
			if delay > 0 {
				filtered = append(filtered, delay)
			}
		}
		if len(filtered) > 0 {
			o.retryDelays = filtered
		}
	}
}
