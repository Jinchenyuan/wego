package reminder

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

var ErrInvalidReminder = errors.New("invalid reminder")

type Store struct {
	db *bun.DB
}

func NewStore(db *bun.DB) *Store {
	return &Store{db: db}
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrInvalidReminder
	}

	_, err := s.db.NewCreateTable().Model((*Reminder)(nil)).IfNotExists().Exec(ctx)
	return err
}

func (s *Store) Create(ctx context.Context, params CreateParams) (*Reminder, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidReminder
	}
	if strings.TrimSpace(params.Key) == "" || strings.TrimSpace(params.UserID) == "" || strings.TrimSpace(params.Channel) == "" || strings.TrimSpace(params.Payload) == "" || params.ScheduleAt.IsZero() {
		return nil, ErrInvalidReminder
	}

	maxRetry := params.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 3
	}

	now := time.Now()
	reminder := &Reminder{
		Key:        params.Key,
		UserID:     params.UserID,
		Channel:    params.Channel,
		Payload:    params.Payload,
		ScheduleAt: params.ScheduleAt,
		Status:     StatusPending,
		MaxRetry:   maxRetry,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if _, err := s.db.NewInsert().Model(reminder).Exec(ctx); err != nil {
		return nil, err
	}

	return reminder, nil
}

func (s *Store) CancelByKey(ctx context.Context, key string) error {
	if s == nil || s.db == nil || strings.TrimSpace(key) == "" {
		return ErrInvalidReminder
	}

	now := time.Now()
	_, err := s.db.NewUpdate().Model((*Reminder)(nil)).
		Set("status = ?", StatusCanceled).
		Set("canceled_at = ?", now).
		Set("updated_at = ?", now).
		Where("key = ?", key).
		Where("status IN (?)", bun.In([]Status{StatusPending, StatusFailed})).
		Exec(ctx)
	return err
}

func (s *Store) RescheduleByKey(ctx context.Context, key string, scheduleAt time.Time) error {
	if s == nil || s.db == nil || strings.TrimSpace(key) == "" || scheduleAt.IsZero() {
		return ErrInvalidReminder
	}

	_, err := s.db.NewUpdate().Model((*Reminder)(nil)).
		Set("schedule_at = ?", scheduleAt).
		Set("status = ?", StatusPending).
		Set("last_error = ''").
		Set("updated_at = ?", time.Now()).
		Where("key = ?", key).
		Where("status IN (?)", bun.In([]Status{StatusPending, StatusFailed})).
		Exec(ctx)
	return err
}

func (s *Store) ClaimDue(ctx context.Context, now time.Time, limit int) ([]*Reminder, error) {
	if s == nil || s.db == nil || limit <= 0 {
		return nil, ErrInvalidReminder
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var reminders []*Reminder
	if err := tx.NewSelect().
		Model(&reminders).
		Where("status = ?", StatusPending).
		Where("schedule_at <= ?", now).
		OrderExpr("schedule_at ASC").
		Limit(limit).
		For("UPDATE SKIP LOCKED").
		Scan(ctx); err != nil {
		return nil, err
	}

	if len(reminders) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	ids := make([]int64, 0, len(reminders))
	for _, item := range reminders {
		ids = append(ids, item.ID)
	}

	if _, err := tx.NewUpdate().Model((*Reminder)(nil)).
		Set("status = ?", StatusProcessing).
		Set("updated_at = ?", now).
		Where("id IN (?)", bun.In(ids)).
		Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	for _, item := range reminders {
		item.Status = StatusProcessing
		item.UpdatedAt = now
	}

	return reminders, nil
}

func (s *Store) RequeueStaleProcessing(ctx context.Context, staleBefore time.Time) (int64, error) {
	if s == nil || s.db == nil || staleBefore.IsZero() {
		return 0, ErrInvalidReminder
	}

	result, err := s.db.NewUpdate().Model((*Reminder)(nil)).
		Set("status = ?", StatusPending).
		Set("updated_at = ?", time.Now()).
		Where("status = ?", StatusProcessing).
		Where("updated_at <= ?", staleBefore).
		Exec(ctx)
	if err != nil {
		return 0, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rows, nil
}

func (s *Store) MarkSent(ctx context.Context, id int64, sentAt time.Time) error {
	if s == nil || s.db == nil || id == 0 || sentAt.IsZero() {
		return ErrInvalidReminder
	}

	_, err := s.db.NewUpdate().Model((*Reminder)(nil)).
		Set("status = ?", StatusSent).
		Set("sent_at = ?", sentAt).
		Set("updated_at = ?", sentAt).
		Where("id = ?", id).
		Where("status = ?", StatusProcessing).
		Exec(ctx)
	return err
}

func (s *Store) MarkRetry(ctx context.Context, reminder *Reminder, nextScheduleAt time.Time, lastErr string) error {
	if s == nil || s.db == nil || reminder == nil || reminder.ID == 0 || nextScheduleAt.IsZero() {
		return ErrInvalidReminder
	}

	nextRetryCount := reminder.RetryCount + 1
	_, err := s.db.NewUpdate().Model((*Reminder)(nil)).
		Set("status = ?", StatusPending).
		Set("retry_count = ?", nextRetryCount).
		Set("schedule_at = ?", nextScheduleAt).
		Set("last_error = ?", lastErr).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", reminder.ID).
		Where("status = ?", StatusProcessing).
		Exec(ctx)
	return err
}

func (s *Store) MarkFailed(ctx context.Context, reminder *Reminder, lastErr string) error {
	if s == nil || s.db == nil || reminder == nil || reminder.ID == 0 {
		return ErrInvalidReminder
	}

	nextRetryCount := reminder.RetryCount + 1
	_, err := s.db.NewUpdate().Model((*Reminder)(nil)).
		Set("status = ?", StatusFailed).
		Set("retry_count = ?", nextRetryCount).
		Set("last_error = ?", lastErr).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", reminder.ID).
		Where("status = ?", StatusProcessing).
		Exec(ctx)
	return err
}
