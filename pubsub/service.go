package pubsub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jinchenyuan/wego/logger"
	redis "github.com/redis/go-redis/v9"
)

type Service struct {
	rdb     *redis.Client
	store   *Store
	router  *Router
	opts    options
	started atomic.Bool
	mu      sync.RWMutex
}

func NewService(rdb *redis.Client, opts ...Option) *Service {
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}
	if options.logger == nil {
		options.logger = logger.GetLogger(options.name)
	}
	return &Service{
		rdb:    rdb,
		router: NewRouter(),
		opts:   options,
		store:  NewStore(rdb, options),
	}
}

func (s *Service) Name() string {
	return s.opts.name
}

func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.rdb == nil {
		return ErrRedisNotConfigured
	}
	if !s.started.CompareAndSwap(false, true) {
		return ErrServiceStarted
	}
	s.router.Freeze()

	subs := s.router.List()
	for _, sub := range subs {
		if err := s.store.EnsureGroup(ctx, sub.Topic, sub.Group); err != nil {
			return err
		}
		consumer := strings.TrimSpace(sub.Consumer)
		if consumer == "" {
			consumer = s.opts.consumerNamer()
			sub.Consumer = consumer
		}
		for index := 0; index < sub.Concurrency; index++ {
			workerName := consumer
			if sub.Concurrency > 1 {
				workerName = fmt.Sprintf("%s-%d", consumer, index+1)
			}
			go s.consumeLoop(ctx, sub, workerName)
			go s.claimLoop(ctx, sub, workerName)
		}
		go s.retryLoop(ctx, sub)
	}
	return nil
}

func (s *Service) Publish(ctx context.Context, topic string, msg Message) (string, error) {
	if normalizeTopic(topic) == "" {
		return "", ErrInvalidTopic
	}
	return s.store.Publish(ctx, topic, msg)
}

func (s *Service) Subscribe(sub Subscription) error {
	if s.started.Load() {
		return ErrServiceStarted
	}
	return s.router.Add(sub)
}

func (s *Service) MustSubscribe(sub Subscription) {
	if err := s.Subscribe(sub); err != nil {
		panic(err)
	}
}

func (s *Service) Subscriptions() []Subscription {
	return s.router.List()
}

func (s *Service) consumeLoop(ctx context.Context, sub Subscription, consumer string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		entries, err := s.store.ReadGroup(ctx, sub, consumer)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			s.opts.logger.Error("pubsub read failed:", err)
			continue
		}
		for _, entry := range entries {
			s.handleEntry(ctx, sub, consumer, entry)
		}
	}
}

func (s *Service) claimLoop(ctx context.Context, sub Subscription, consumer string) {
	ticker := time.NewTicker(effectiveRetry(sub, s.opts.retry).ClaimInterval)
	defer ticker.Stop()
	start := "0-0"
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			entries, next, err := s.store.AutoClaim(ctx, sub, consumer, start, s.opts.stream.ReadCount)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				s.opts.logger.Error("pubsub claim failed:", err)
				continue
			}
			start = next
			if len(entries) == 0 {
				start = "0-0"
				continue
			}
			for _, entry := range entries {
				s.handleEntry(ctx, sub, consumer, entry)
			}
		}
	}
}

func (s *Service) retryLoop(ctx context.Context, sub Subscription) {
	ticker := time.NewTicker(effectiveRetry(sub, s.opts.retry).ClaimInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			items, err := s.store.ClaimDueRetries(ctx, sub.Topic, s.opts.stream.ReadCount, s.opts.now())
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				s.opts.logger.Error("pubsub retry claim failed:", err)
				continue
			}
			for _, item := range items {
				if _, err := s.store.PublishRetry(ctx, item); err != nil {
					_ = s.store.ScheduleRetry(ctx, item.Topic, &Delivery{
						Topic: item.Topic,
						Message: Message{
							Key:        item.Key,
							Type:       item.Type,
							Data:       item.Data,
							Headers:    item.Headers,
							MaxDeliver: item.MaxDeliver,
						},
						Attempt:     item.Attempt,
						PublishedAt: item.PublishedAt,
					}, s.opts.now().Add(time.Second), item.Attempt)
					s.opts.logger.Error("pubsub retry publish failed:", err)
				}
			}
		}
	}
}

func (s *Service) handleEntry(ctx context.Context, sub Subscription, consumer string, entry Entry) {
	delivery, err := decodeEntry(sub.Topic, sub.Group, consumer, entry)
	if err != nil {
		s.opts.logger.Error("pubsub decode failed:", err)
		_ = s.store.Ack(ctx, sub.Topic, sub.Group, entry.ID)
		return
	}

	retry := effectiveRetry(sub, s.opts.retry)
	if delivery.Message.MaxDeliver <= 0 {
		delivery.Message.MaxDeliver = retry.MaxDeliver
	}
	if delivery.Attempt > delivery.Message.MaxDeliver && s.opts.dlq.Enabled {
		_, _ = s.store.MoveToDLQ(ctx, sub.Topic, delivery, "max delivery exceeded")
		_ = s.store.Ack(ctx, sub.Topic, sub.Group, delivery.ID)
		return
	}

	if err := sub.Handler.Handle(ctx, delivery); err != nil {
		s.opts.logger.Warn("pubsub handler failed:", err)
		nextAttempt := delivery.Attempt + 1
		if nextAttempt > delivery.Message.MaxDeliver {
			if s.opts.dlq.Enabled {
				_, _ = s.store.MoveToDLQ(ctx, sub.Topic, delivery, "max delivery exceeded")
			}
			_ = s.store.Ack(ctx, sub.Topic, sub.Group, delivery.ID)
			return
		}

		delay := retryDelay(retry, nextAttempt)
		if err := s.store.ScheduleRetry(ctx, sub.Topic, delivery, s.opts.now().Add(delay), nextAttempt); err != nil {
			s.opts.logger.Error("pubsub retry scheduling failed:", err)
			return
		}
		if err := s.store.Ack(ctx, sub.Topic, sub.Group, delivery.ID); err != nil {
			s.opts.logger.Error("pubsub ack after retry scheduling failed:", err)
		}
		return
	}

	if err := s.store.Ack(ctx, sub.Topic, sub.Group, delivery.ID); err != nil {
		s.opts.logger.Error("pubsub ack failed:", err)
	}
}

func retryDelay(policy RetryPolicy, attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	if len(policy.Backoff) == 0 {
		return 0
	}
	index := attempt - 2
	if index < 0 {
		index = 0
	}
	if index >= len(policy.Backoff) {
		return policy.Backoff[len(policy.Backoff)-1]
	}
	return policy.Backoff[index]
}
