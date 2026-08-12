//go:build integration

package reminder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestStoreLifecycleIntegration(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	item, err := store.Create(ctx, CreateParams{Key: "order-1", UserID: "user-1", Channel: "email", Payload: `{}`, ScheduleAt: now})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err = store.Create(ctx, CreateParams{Key: item.Key, UserID: "user-1", Channel: "email", Payload: `{}`, ScheduleAt: now}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate error = %v", err)
	}
	got, err := store.GetByKey(ctx, item.Key)
	if err != nil || got.ID != item.ID {
		t.Fatalf("GetByKey = %#v, %v", got, err)
	}
	if err = store.CancelByKey(ctx, item.Key); err != nil {
		t.Fatalf("CancelByKey: %v", err)
	}
	if err = store.CancelByKey(ctx, item.Key); err != nil {
		t.Fatalf("idempotent CancelByKey: %v", err)
	}
	if err = store.RescheduleByKey(ctx, item.Key, now.Add(time.Hour)); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("reschedule canceled error = %v", err)
	}
	if _, err = store.GetByKey(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestStoreListCursorIntegration(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 3; i++ {
		if _, err := store.Create(ctx, CreateParams{Key: fmt.Sprintf("item-%d", i), UserID: "user-1", Channel: "sms", Payload: `{}`, ScheduleAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.List(ctx, ListParams{UserID: "user-1", Statuses: []Status{StatusPending}, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	second, err := store.List(ctx, ListParams{UserID: "user-1", Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID == first.Items[1].ID {
		t.Fatalf("second page = %#v", second)
	}
}

func TestClaimDueSkipsClaimedRowsIntegration(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := store.Create(ctx, CreateParams{Key: "due", UserID: "u", Channel: "email", Payload: `{}`, ScheduleAt: now.Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	first, err := store.ClaimDue(ctx, now, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim = %d, %v", len(first), err)
	}
	second, err := store.ClaimDue(ctx, now, 1)
	if err != nil || len(second) != 0 {
		t.Fatalf("second claim = %d, %v", len(second), err)
	}
}

func TestRescheduleFailedResetsDeliveryStateIntegration(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	item, err := store.Create(ctx, CreateParams{Key: "failed", UserID: "u", Channel: "email", Payload: `{}`, ScheduleAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.NewUpdate().Model((*Reminder)(nil)).Set("status = ?", StatusFailed).Set("retry_count = 2").Set("last_error = 'boom'").Where("id = ?", item.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.RescheduleByKey(ctx, item.Key, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByKey(ctx, item.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPending || got.RetryCount != 0 || got.LastError != "" {
		t.Fatalf("rescheduled reminder = %#v", got)
	}
}

func newIntegrationStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("WEGO_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set WEGO_POSTGRES_TEST_DSN to run PostgreSQL integration tests")
	}
	schema := fmt.Sprintf("reminder_test_%d", time.Now().UnixNano())
	if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(schema) {
		t.Fatal("invalid schema")
	}
	adminSQL := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	admin := bun.NewDB(adminSQL, pgdialect.New())
	if err := admin.PingContext(context.Background()); err != nil {
		_ = admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	if _, err := admin.ExecContext(context.Background(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		_ = admin.Close()
	})
	conn := pgdriver.NewConnector(pgdriver.WithDSN(dsn), pgdriver.WithConnParams(map[string]interface{}{"search_path": schema}))
	db := bun.NewDB(sql.OpenDB(conn), pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewStore(db)
}
