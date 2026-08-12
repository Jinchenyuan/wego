package reminder

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

var ErrInvalidReminder = errors.New("invalid reminder")
var ErrNotFound = errors.New("reminder not found")
var ErrAlreadyExists = errors.New("reminder already exists")
var ErrStateConflict = errors.New("reminder state conflict")

type Store struct{ db *bun.DB }
type listCursor struct {
	ScheduleAt time.Time `json:"schedule_at"`
	ID         int64     `json:"id"`
}

func NewStore(db *bun.DB) *Store { return &Store{db: db} }
func (s *Store) Migrate(ctx context.Context) error {
	if s == nil {
		return ErrInvalidReminder
	}
	return Migrate(ctx, s.db)
}

func (s *Store) Create(ctx context.Context, params CreateParams) (*Reminder, error) {
	if s == nil || s.db == nil || strings.TrimSpace(params.Key) == "" || strings.TrimSpace(params.UserID) == "" || strings.TrimSpace(params.Channel) == "" || strings.TrimSpace(params.Payload) == "" || params.ScheduleAt.IsZero() {
		return nil, ErrInvalidReminder
	}
	maxRetry := params.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 3
	}
	now := time.Now()
	item := &Reminder{Key: strings.TrimSpace(params.Key), UserID: strings.TrimSpace(params.UserID), Channel: strings.TrimSpace(params.Channel), Payload: params.Payload, ScheduleAt: params.ScheduleAt, Status: StatusPending, MaxRetry: maxRetry, CreatedAt: now, UpdatedAt: now}
	if _, err := s.db.NewInsert().Model(item).Exec(ctx); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return item, nil
}

func (s *Store) GetByKey(ctx context.Context, key string) (*Reminder, error) {
	if s == nil || s.db == nil || strings.TrimSpace(key) == "" {
		return nil, ErrInvalidReminder
	}
	item := new(Reminder)
	err := s.db.NewSelect().Model(item).Where("key = ?", strings.TrimSpace(key)).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) List(ctx context.Context, params ListParams) (*ListResult, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidReminder
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := s.db.NewSelect().Model((*Reminder)(nil))
	if v := strings.TrimSpace(params.UserID); v != "" {
		query = query.Where("user_id = ?", v)
	}
	if v := strings.TrimSpace(params.Channel); v != "" {
		query = query.Where("channel = ?", v)
	}
	if len(params.Statuses) > 0 {
		for _, status := range params.Statuses {
			if !validStatus(status) {
				return nil, ErrInvalidReminder
			}
		}
		query = query.Where("status IN (?)", bun.In(params.Statuses))
	}
	if !params.ScheduleFrom.IsZero() {
		query = query.Where("schedule_at >= ?", params.ScheduleFrom)
	}
	if !params.ScheduleTo.IsZero() {
		query = query.Where("schedule_at <= ?", params.ScheduleTo)
	}
	if params.Cursor != "" {
		cursor, err := decodeCursor(params.Cursor)
		if err != nil {
			return nil, ErrInvalidReminder
		}
		query = query.Where("(schedule_at, id) > (?, ?)", cursor.ScheduleAt, cursor.ID)
	}
	var items []*Reminder
	if err := query.OrderExpr("schedule_at ASC, id ASC").Limit(limit+1).Scan(ctx, &items); err != nil {
		return nil, err
	}
	result := &ListResult{Items: items}
	if len(items) > limit {
		result.Items = items[:limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor = encodeCursor(listCursor{ScheduleAt: last.ScheduleAt, ID: last.ID})
	}
	return result, nil
}

func (s *Store) CancelByKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if s == nil || s.db == nil || key == "" {
		return ErrInvalidReminder
	}
	item, err := s.GetByKey(ctx, key)
	if err != nil {
		return err
	}
	if item.Status == StatusCanceled {
		return nil
	}
	if item.Status != StatusPending && item.Status != StatusFailed {
		return ErrStateConflict
	}
	now := time.Now()
	result, err := s.db.NewUpdate().Model((*Reminder)(nil)).Set("status = ?", StatusCanceled).Set("canceled_at = ?", now).Set("updated_at = ?", now).Where("key = ?", key).Where("status IN (?)", bun.In([]Status{StatusPending, StatusFailed})).Exec(ctx)
	return mutationResult(result, err)
}

func (s *Store) RescheduleByKey(ctx context.Context, key string, scheduleAt time.Time) error {
	key = strings.TrimSpace(key)
	if s == nil || s.db == nil || key == "" || scheduleAt.IsZero() {
		return ErrInvalidReminder
	}
	item, err := s.GetByKey(ctx, key)
	if err != nil {
		return err
	}
	if item.Status != StatusPending && item.Status != StatusFailed {
		return ErrStateConflict
	}
	result, err := s.db.NewUpdate().Model((*Reminder)(nil)).Set("schedule_at = ?", scheduleAt).Set("status = ?", StatusPending).Set("retry_count = 0").Set("last_error = ''").Set("sent_at = NULL").Set("canceled_at = NULL").Set("updated_at = ?", time.Now()).Where("key = ?", key).Where("status IN (?)", bun.In([]Status{StatusPending, StatusFailed})).Exec(ctx)
	return mutationResult(result, err)
}

func (s *Store) ClaimDue(ctx context.Context, now time.Time, limit int) ([]*Reminder, error) {
	if s == nil || s.db == nil || limit <= 0 {
		return nil, ErrInvalidReminder
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var items []*Reminder
	if err := tx.NewSelect().Model(&items).Where("status = ?", StatusPending).Where("schedule_at <= ?", now).OrderExpr("schedule_at ASC, id ASC").Limit(limit).For("UPDATE SKIP LOCKED").Scan(ctx); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, tx.Commit()
	}
	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	if _, err := tx.NewUpdate().Model((*Reminder)(nil)).Set("status = ?", StatusProcessing).Set("updated_at = ?", now).Where("id IN (?)", bun.In(ids)).Where("status = ?", StatusPending).Exec(ctx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, item := range items {
		item.Status = StatusProcessing
		item.UpdatedAt = now
	}
	return items, nil
}

func (s *Store) RequeueStaleProcessing(ctx context.Context, staleBefore time.Time) (int64, error) {
	if s == nil || s.db == nil || staleBefore.IsZero() {
		return 0, ErrInvalidReminder
	}
	result, err := s.db.NewUpdate().Model((*Reminder)(nil)).Set("status = ?", StatusPending).Set("updated_at = ?", time.Now()).Where("status = ?", StatusProcessing).Where("updated_at <= ?", staleBefore).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) MarkSent(ctx context.Context, id int64, sentAt time.Time) error {
	if s == nil || s.db == nil || id == 0 || sentAt.IsZero() {
		return ErrInvalidReminder
	}
	result, err := s.db.NewUpdate().Model((*Reminder)(nil)).Set("status = ?", StatusSent).Set("sent_at = ?", sentAt).Set("updated_at = ?", sentAt).Where("id = ?", id).Where("status = ?", StatusProcessing).Exec(ctx)
	return mutationResult(result, err)
}
func (s *Store) MarkRetry(ctx context.Context, item *Reminder, next time.Time, lastErr string) error {
	if s == nil || s.db == nil || item == nil || item.ID == 0 || next.IsZero() {
		return ErrInvalidReminder
	}
	result, err := s.db.NewUpdate().Model((*Reminder)(nil)).Set("status = ?", StatusPending).Set("retry_count = ?", item.RetryCount+1).Set("schedule_at = ?", next).Set("last_error = ?", lastErr).Set("updated_at = ?", time.Now()).Where("id = ?", item.ID).Where("status = ?", StatusProcessing).Exec(ctx)
	return mutationResult(result, err)
}
func (s *Store) MarkFailed(ctx context.Context, item *Reminder, lastErr string) error {
	if s == nil || s.db == nil || item == nil || item.ID == 0 {
		return ErrInvalidReminder
	}
	result, err := s.db.NewUpdate().Model((*Reminder)(nil)).Set("status = ?", StatusFailed).Set("retry_count = ?", item.RetryCount+1).Set("last_error = ?", lastErr).Set("updated_at = ?", time.Now()).Where("id = ?", item.ID).Where("status = ?", StatusProcessing).Exec(ctx)
	return mutationResult(result, err)
}

func mutationResult(result sqlResult, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrStateConflict
	}
	return nil
}

type sqlResult interface{ RowsAffected() (int64, error) }

func validStatus(s Status) bool {
	switch s {
	case StatusPending, StatusProcessing, StatusSent, StatusFailed, StatusCanceled:
		return true
	}
	return false
}
func encodeCursor(c listCursor) string {
	data, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(data)
}
func decodeCursor(raw string) (listCursor, error) {
	var c listCursor
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c); err != nil || c.ScheduleAt.IsZero() || c.ID <= 0 {
		return c, ErrInvalidReminder
	}
	return c, nil
}
func isUniqueViolation(err error) bool {
	var pgErr pgdriver.Error
	return errors.As(err, &pgErr) && pgErr.Field('C') == "23505"
}
