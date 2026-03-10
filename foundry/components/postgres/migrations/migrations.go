// Package migrations implements a versioned SQL migration runner for the
// Trusted Software Foundry.
//
// Migration tracking is stored in a _foundry_migrations table in the target
// database. Each migration is applied exactly once, in the order provided.
// Re-running Apply is safe and idempotent — already-applied migrations are
// skipped.
//
// # Usage
//
//	runner := migrations.New(sqlDB)
//
//	err := runner.Apply(ctx, []migrations.Migration{
//	    {ID: "001_create_users",   SQL: "CREATE TABLE users (id UUID PRIMARY KEY, ...);"},
//	    {ID: "002_add_email_idx",  SQL: "CREATE UNIQUE INDEX users_email_idx ON users (email);"},
//	})
//
// Migration IDs must be stable — they are the primary key of the tracking
// table. A good convention is a zero-padded sequence number followed by a
// short description: "001_init", "002_add_index", "20240101_01_rename_col".
//
// Zero external dependencies — stdlib database/sql only.
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const trackingTable = "_foundry_migrations"

// Migration is a single versioned schema change.
type Migration struct {
	// ID uniquely identifies this migration. It must be stable across
	// deployments — it is the primary key of the tracking table.
	// Recommended format: "NNN_description" e.g. "001_create_users".
	ID string

	// SQL is the DDL/DML to execute. It is run inside a transaction.
	// The SQL may contain multiple statements separated by semicolons.
	SQL string
}

// DB is the minimal interface Runner requires. *sql.DB satisfies it.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Runner applies versioned migrations against a database.
type Runner struct {
	db DB
}

// New creates a Runner backed by the given DB.
func New(db DB) *Runner {
	return &Runner{db: db}
}

// Apply ensures all provided migrations are applied, in order, exactly once.
//
// It first bootstraps the _foundry_migrations tracking table (if absent),
// then iterates through migrations in the provided slice order and executes
// each one that has not yet been recorded. Each migration runs inside its
// own transaction so a failure rolls back only that step.
//
// Apply is idempotent — calling it multiple times with the same slice is safe.
func (r *Runner) Apply(ctx context.Context, migs []Migration) error {
	if err := r.ensureTrackingTable(ctx); err != nil {
		return fmt.Errorf("migrations: bootstrap tracking table: %w", err)
	}

	applied, err := r.appliedSet(ctx)
	if err != nil {
		return fmt.Errorf("migrations: read applied set: %w", err)
	}

	for _, m := range migs {
		if m.ID == "" {
			return fmt.Errorf("migrations: migration has empty ID (SQL: %.40s...)", m.SQL)
		}
		if applied[m.ID] {
			continue // already done
		}
		if err := r.run(ctx, m); err != nil {
			return fmt.Errorf("migrations: apply %q: %w", m.ID, err)
		}
	}
	return nil
}

// Applied returns the set of migration IDs that have already been applied.
// The returned map may be used by callers to inspect migration state.
func (r *Runner) Applied(ctx context.Context) (map[string]time.Time, error) {
	if err := r.ensureTrackingTable(ctx); err != nil {
		return nil, fmt.Errorf("migrations: bootstrap tracking table: %w", err)
	}
	return r.appliedWithTime(ctx)
}

// ensureTrackingTable creates _foundry_migrations if it does not exist.
func (r *Runner) ensureTrackingTable(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    id          TEXT        PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
)`, trackingTable))
	return err
}

// appliedSet returns the set of already-applied migration IDs as a string set.
func (r *Runner) appliedSet(ctx context.Context) (map[string]bool, error) {
	applied := make(map[string]bool)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s", trackingTable))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		applied[id] = true
	}
	return applied, rows.Err()
}

// appliedWithTime returns migration IDs with their applied timestamps.
func (r *Runner) appliedWithTime(ctx context.Context) (map[string]time.Time, error) {
	result := make(map[string]time.Time)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("SELECT id, applied_at FROM %s", trackingTable))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		result[id] = at
	}
	return result, rows.Err()
}

// run executes a single migration inside a transaction and records it in the
// tracking table.
func (r *Runner) run(ctx context.Context, m Migration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("exec: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (id) VALUES ($1)", trackingTable), m.ID,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
