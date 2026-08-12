package reminder

import (
	"context"
	"embed"
	"fmt"

	"github.com/uptrace/bun"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Migrate(ctx context.Context, db *bun.DB) error {
	if db == nil {
		return ErrInvalidReminder
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS wego_schema_migrations (component TEXT NOT NULL, version TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY (component, version))`); err != nil {
		return err
	}
	const version = "001_init"
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM wego_schema_migrations WHERE component = 'reminder' AND version = $1)`, version).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		sql, readErr := migrations.ReadFile("migrations/001_init.sql")
		if readErr != nil {
			return readErr
		}
		if _, err = tx.ExecContext(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply reminder migration %s: %w", version, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO wego_schema_migrations (component, version) VALUES ('reminder', $1)`, version); err != nil {
			return err
		}
	}
	return tx.Commit()
}
