package reminder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jinchenyuan/wego/logger"
	"github.com/uptrace/bun"
)

type Service struct {
	store    *Store
	notifier Notifier
	opts     options
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
	if s.opts.autoCreateTable {
		if err := s.store.EnsureSchema(ctx); err != nil {
			return err
		}
	}
	if err := s.recoverStaleProcessing(ctx); err != nil {
		return err
	}

	go s.loop(ctx)
	return nil
}

func (s *Service) Create(ctx context.Context, params CreateParams) (*Reminder, error) {
	return s.store.Create(ctx, params)
}

func (s *Service) CancelByKey(ctx context.Context, key string) error {
	return s.store.CancelByKey(ctx, key)
}

func (s *Service) RescheduleByKey(ctx context.Context, key string, scheduleAt time.Time) error {
	return s.store.RescheduleByKey(ctx, key, scheduleAt)
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	return s.store.EnsureSchema(ctx)
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

	for _, item := range reminders {
		if err := s.dispatch(ctx, item); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) dispatch(ctx context.Context, item *Reminder) error {
	err := s.notifier.Notify(ctx, item)
	if err == nil {
		return s.store.MarkSent(ctx, item.ID, s.opts.now())
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "notify failed"
	}

	if item.RetryCount+1 >= item.MaxRetry {
		if markErr := s.store.MarkFailed(ctx, item, message); markErr != nil {
			return fmt.Errorf("notify: %w; mark failed: %v", err, markErr)
		}
		s.opts.logger.Warn("reminder permanently failed:", item.Key, message)
		return nil
	}

	nextScheduleAt := s.opts.now().Add(s.retryDelay(item.RetryCount + 1))
	if markErr := s.store.MarkRetry(ctx, item, nextScheduleAt, message); markErr != nil {
		return fmt.Errorf("notify: %w; mark retry: %v", err, markErr)
	}

	s.opts.logger.Warn("reminder rescheduled:", item.Key, message)
	return nil
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
