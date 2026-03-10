// Package locks implements a PostgreSQL advisory lock manager for the
// Trusted Software Foundry.
//
// Advisory locks are coordinated at the PostgreSQL session level via
// transaction-scoped advisory functions (pg_advisory_xact_lock /
// pg_try_advisory_xact_lock). The lock is held for the lifetime of the
// database transaction and is automatically released when the transaction
// commits or rolls back — no explicit unlock SQL is needed.
//
// Usage:
//
//	mgr := locks.New(sqlDB)
//
//	// Blocking acquire — waits until lock is available.
//	if err := mgr.AcquireLock(ctx, 42); err != nil { ... }
//	defer mgr.ReleaseLock(ctx, 42)
//
//	// Non-blocking try — returns false immediately if lock is held.
//	acquired, err := mgr.TryLock(ctx, 42)
//
// The int64 key is the PostgreSQL bigint advisory lock key. Callers
// typically hash a domain string (resource name, migration tag, etc.)
// to an int64 using hash/fnv or a similar stable hash.
//
// Zero external dependencies — stdlib + database/sql only.
package locks

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// Connector is the minimal interface Manager requires. *sql.DB satisfies it.
// Keeping it narrow makes it easy to mock in tests.
type Connector interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Manager holds open lock transactions keyed by their advisory lock key.
// Each held lock is a live database transaction; the transaction keeps the
// advisory lock alive and is committed by ReleaseLock.
type Manager struct {
	db  Connector
	mu  sync.Mutex
	txs map[int64]*sql.Tx
}

// New creates a Manager backed by the provided Connector.
func New(db Connector) *Manager {
	return &Manager{
		db:  db,
		txs: make(map[int64]*sql.Tx),
	}
}

// AcquireLock obtains a blocking PostgreSQL transaction-scoped advisory lock
// for key. It blocks until the lock is available or ctx is cancelled.
//
// The lock is released by calling ReleaseLock with the same key, or
// automatically when the underlying transaction is rolled back on error.
//
// Returns an error if key is already held by this Manager in the current
// process (prevents double-locking the same key from the same process).
func (m *Manager) AcquireLock(ctx context.Context, key int64) error {
	m.mu.Lock()
	if _, held := m.txs[key]; held {
		m.mu.Unlock()
		return fmt.Errorf("advisory lock %d: already held by this manager", key)
	}
	m.mu.Unlock()

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("advisory lock %d: begin tx: %w", key, err)
	}

	// pg_advisory_xact_lock blocks until the lock is granted or the session
	// ends. It is scoped to the transaction — released on commit/rollback.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", key); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("advisory lock %d: acquire: %w", key, err)
	}

	m.mu.Lock()
	m.txs[key] = tx
	m.mu.Unlock()
	return nil
}

// TryLock attempts a non-blocking PostgreSQL transaction-scoped advisory lock
// for key. Returns (true, nil) when the lock is acquired, (false, nil) when
// another session holds the lock, and (false, err) on database error.
//
// If acquired, the lock is released by calling ReleaseLock with the same key.
func (m *Manager) TryLock(ctx context.Context, key int64) (bool, error) {
	m.mu.Lock()
	if _, held := m.txs[key]; held {
		m.mu.Unlock()
		return false, fmt.Errorf("advisory lock %d: already held by this manager", key)
	}
	m.mu.Unlock()

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("advisory lock %d: begin tx: %w", key, err)
	}

	// pg_try_advisory_xact_lock returns TRUE if the lock was granted,
	// FALSE if it is held by another session. Never blocks.
	var acquired bool
	row := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1)", key)
	if err := row.Scan(&acquired); err != nil {
		_ = tx.Rollback()
		return false, fmt.Errorf("advisory lock %d: try-acquire: %w", key, err)
	}

	if !acquired {
		// Lock was not granted — roll back and report gracefully.
		_ = tx.Rollback()
		return false, nil
	}

	m.mu.Lock()
	m.txs[key] = tx
	m.mu.Unlock()
	return true, nil
}

// ReleaseLock commits the transaction that holds the advisory lock for key,
// which causes PostgreSQL to release the lock. It is a no-op (no error) when
// key is not currently held by this Manager.
func (m *Manager) ReleaseLock(ctx context.Context, key int64) error {
	m.mu.Lock()
	tx, held := m.txs[key]
	if !held {
		m.mu.Unlock()
		return nil
	}
	delete(m.txs, key)
	m.mu.Unlock()

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("advisory lock %d: release (commit): %w", key, err)
	}
	return nil
}

// HeldKeys returns the set of advisory lock keys currently held by this Manager.
// Intended for diagnostics and testing.
func (m *Manager) HeldKeys() []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]int64, 0, len(m.txs))
	for k := range m.txs {
		keys = append(keys, k)
	}
	return keys
}
