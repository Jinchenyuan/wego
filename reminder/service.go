package reminder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Jinchenyuan/wego/logger"
	"github.com/uptrace/bun"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Service struct {
	store    *Store
	notifier Notifier
	opts     options
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	started  bool
}

func NewService(db *bun.DB, notifier Notifier, opts ...Option) *Service {
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}
	if options.logger == nil {
		options.logger = logger.GetLogger(options.name)
	}

	return &Service{
		store:    NewStore(db),
		notifier: notifier,
		opts:     options,
	}
}

func (s *Service) Name() string {
	return s.opts.name
}

func (s *Service) Start(ctx context.Context) error {
	if s.store == nil || s.store.db == nil {
		return errors.New("reminder store is not configured")
	}
	if s.notifier == nil {
		return errors.New("reminder notifier is not configured")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("reminder service already started")
	}
	s.started = true
	s.mu.Unlock()
	if err := s.recoverStaleProcessing(ctx); err != nil {
		s.mu.Lock()
		s.started = false
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	workerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	done := s.done
	s.mu.Unlock()
	go func() { defer close(done); s.loop(workerCtx) }()
	return nil
}

func (s *Service) Create(ctx context.Context, params CreateParams) (*Reminder, error) {
	return s.store.Create(ctx, params)
}

func (s *Service) GetByKey(ctx context.Context, key string) (*Reminder, error) {
	return s.store.GetByKey(ctx, key)
}

func (s *Service) List(ctx context.Context, params ListParams) (*ListResult, error) {
	return s.store.List(ctx, params)
}

func (s *Service) CancelByKey(ctx context.Context, key string) error {
	return s.store.CancelByKey(ctx, key)
}

func (s *Service) RescheduleByKey(ctx context.Context, key string, scheduleAt time.Time) error {
	return s.store.RescheduleByKey(ctx, key, scheduleAt)
}

func (s *Service) Migrate(ctx context.Context) error {
	return s.store.Migrate(ctx)
}

func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) loop(ctx context.Context) {
	ticker := time.NewTicker(s.opts.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.recoverStaleProcessing(ctx); err != nil && !errors.Is(err, context.Canceled) {
				s.opts.logger.Error("reminder recovery failed:", err)
				continue
			}
			if err := s.processBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
				s.opts.logger.Error("reminder batch failed:", err)
			}
		}
	}
}

func (s *Service) processBatch(ctx context.Context) error {
	reminders, err := s.store.ClaimDue(ctx, s.opts.now(), s.opts.batchSize)
	if err != nil || len(reminders) == 0 {
		return err
	}

	var result error
	for _, item := range reminders {
		if err := s.dispatch(ctx, item); err != nil {
			result = errors.Join(result, fmt.Errorf("dispatch reminder %s: %w", item.Key, err))
			s.opts.logger.Error("reminder dispatch failed:", item.Key, err)
		}
	}
	return result
}

func (s *Service) dispatch(ctx context.Context, item *Reminder) error {
	ctx, span := s.opts.telemetry.Tracer("wego/reminder").Start(ctx, "reminder notify",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("wego.reminder.channel", item.Channel), attribute.Int("wego.reminder.retry_count", item.RetryCount)),
	)
	defer span.End()
	start := time.Now()
	err := s.notifier.Notify(ctx, item)
	if err == nil {
		err = s.store.MarkSent(ctx, item.ID, s.opts.now())
		result := "sent"
		if err != nil {
			result = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		s.recordDispatch(item.Channel, result, start)
		return err
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "notify failed"
	}

	if item.RetryCount+1 >= item.MaxRetry {
		if markErr := s.store.MarkFailed(ctx, item, message); markErr != nil {
			return fmt.Errorf("notify: %w; mark failed: %v", err, markErr)
		}
		s.opts.logger.Warn("reminder permanently failed:", item.Key, message)
		s.recordDispatch(item.Channel, "failed", start)
		return nil
	}

	nextScheduleAt := s.opts.now().Add(s.retryDelay(item.RetryCount + 1))
	if markErr := s.store.MarkRetry(ctx, item, nextScheduleAt, message); markErr != nil {
		return fmt.Errorf("notify: %w; mark retry: %v", err, markErr)
	}

	s.opts.logger.Warn("reminder rescheduled:", item.Key, message)
	s.recordDispatch(item.Channel, "retry", start)
	return nil
}

func (s *Service) recordDispatch(channel string, result string, start time.Time) {
	labels := map[string]string{"channel": channel, "result": result}
	s.opts.metrics.Inc("wego_reminder_dispatches_total", labels)
	s.opts.metrics.Observe("wego_reminder_dispatch_duration_seconds", labels, time.Since(start).Seconds())
}

func (s *Service) retryDelay(attempt int) time.Duration {
	if attempt <= 0 || len(s.opts.retryDelays) == 0 {
		return time.Minute
	}
	if attempt > len(s.opts.retryDelays) {
		return s.opts.retryDelays[len(s.opts.retryDelays)-1]
	}
	return s.opts.retryDelays[attempt-1]
}

func (s *Service) recoverStaleProcessing(ctx context.Context) error {
	staleBefore := s.opts.now().Add(-s.opts.processingTTL)
	recovered, err := s.store.RequeueStaleProcessing(ctx, staleBefore)
	if err != nil {
		return err
	}
	if recovered > 0 {
		s.opts.logger.Warn("requeued stale processing reminders:", recovered)
	}
	return nil
}
