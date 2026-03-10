package locks_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/components/postgres/locks"
)

// ---------------------------------------------------------------------------
// Minimal in-process SQL mock — no external deps
// ---------------------------------------------------------------------------

// mockDriver implements database/sql/driver.Driver to register a test DSN.
type mockDriver struct {
	mu    sync.Mutex
	conns map[string]*mockConn
}

func (d *mockDriver) Open(name string) (driver.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	c, ok := d.conns[name]
	if !ok {
		return nil, fmt.Errorf("mockDriver: no conn registered for %q", name)
	}
	return c, nil
}

// mockConn is a single in-memory database connection that records advisory
// lock calls and controls the outcome via configurable fields.
type mockConn struct {
	mu sync.Mutex

	// Controls whether pg_advisory_xact_lock / pg_try_advisory_xact_lock succeed.
	acquireErr error
	// Controls whether BeginTx succeeds.
	beginErr error
	// Controls whether Commit succeeds.
	commitErr error
	// When true, pg_try_advisory_xact_lock returns FALSE (lock contended).
	contended bool

	// Tracks which lock key was last acquired via the advisory lock call.
	lastKey int64
	// Number of times pg_advisory_xact_lock was called.
	acquireCalls int
	// Number of times pg_try_advisory_xact_lock was called.
	tryCalls int
	// Number of times Commit was called.
	commitCalls int
	// Number of times Rollback was called.
	rollbackCalls int
}

// --- driver.Conn ---

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{conn: c, query: query}, nil
}

func (c *mockConn) Close() error { return nil }

func (c *mockConn) Begin() (driver.Tx, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.beginErr != nil {
		return nil, c.beginErr
	}
	return &mockTx{conn: c}, nil
}

// --- driver.Stmt ---

type mockStmt struct {
	conn  *mockConn
	query string
}

func (s *mockStmt) Close() error { return nil }

func (s *mockStmt) NumInput() int { return -1 }

func (s *mockStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.conn.mu.Lock()
	defer s.conn.mu.Unlock()

	switch {
	case containsAny(s.query, "pg_advisory_xact_lock"):
		s.conn.acquireCalls++
		if len(args) > 0 {
			if k, ok := args[0].(int64); ok {
				s.conn.lastKey = k
			}
		}
		if s.conn.acquireErr != nil {
			return nil, s.conn.acquireErr
		}
		return driver.RowsAffected(0), nil
	}
	return driver.RowsAffected(0), nil
}

func (s *mockStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.conn.mu.Lock()
	defer s.conn.mu.Unlock()

	switch {
	case containsAny(s.query, "pg_try_advisory_xact_lock"):
		s.conn.tryCalls++
		if len(args) > 0 {
			if k, ok := args[0].(int64); ok {
				s.conn.lastKey = k
			}
		}
		if s.conn.acquireErr != nil {
			return nil, s.conn.acquireErr
		}
		acquired := !s.conn.contended
		return &boolRows{val: acquired}, nil
	}
	return &emptyRows{}, nil
}

// --- driver.Tx ---

type mockTx struct {
	conn *mockConn
}

func (t *mockTx) Commit() error {
	t.conn.mu.Lock()
	defer t.conn.mu.Unlock()
	t.conn.commitCalls++
	return t.conn.commitErr
}

func (t *mockTx) Rollback() error {
	t.conn.mu.Lock()
	defer t.conn.mu.Unlock()
	t.conn.rollbackCalls++
	return nil
}

// --- helper Rows types ---

type boolRows struct {
	val  bool
	done bool
}

func (r *boolRows) Columns() []string { return []string{"pg_try_advisory_xact_lock"} }
func (r *boolRows) Close() error      { return nil }
func (r *boolRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.val
	return nil
}

type emptyRows struct{}

func (r *emptyRows) Columns() []string           { return nil }
func (r *emptyRows) Close() error                { return nil }
func (r *emptyRows) Next(_ []driver.Value) error { return io.EOF }

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var (
	driverOnce sync.Once
	drv        *mockDriver
)

func init() {
	drv = &mockDriver{conns: make(map[string]*mockConn)}
	sql.Register("mock-locks", drv)
}

// newTestDB registers a fresh mockConn and returns an *sql.DB backed by it.
func newTestDB(t *testing.T, conn *mockConn) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("test-%s", t.Name())
	drv.mu.Lock()
	drv.conns[dsn] = conn
	drv.mu.Unlock()
	t.Cleanup(func() {
		drv.mu.Lock()
		delete(drv.conns, dsn)
		drv.mu.Unlock()
	})
	db, err := sql.Open("mock-locks", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAcquireLock_Success(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	if err := mgr.AcquireLock(context.Background(), 100); err != nil {
		t.Fatalf("AcquireLock: unexpected error: %v", err)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.acquireCalls != 1 {
		t.Errorf("expected 1 acquire call, got %d", conn.acquireCalls)
	}
	if conn.lastKey != 100 {
		t.Errorf("expected key 100, got %d", conn.lastKey)
	}
}

func TestAcquireLock_HeldKeysTracked(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	if err := mgr.AcquireLock(context.Background(), 42); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	keys := mgr.HeldKeys()
	if len(keys) != 1 || keys[0] != 42 {
		t.Errorf("HeldKeys: expected [42], got %v", keys)
	}
}

func TestAcquireLock_DoubleLockSameKey(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	if err := mgr.AcquireLock(context.Background(), 7); err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	// Second acquire on same key must return an error without calling DB.
	conn.mu.Lock()
	callsBefore := conn.acquireCalls
	conn.mu.Unlock()

	err := mgr.AcquireLock(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error on double-lock, got nil")
	}

	conn.mu.Lock()
	callsAfter := conn.acquireCalls
	conn.mu.Unlock()
	if callsAfter != callsBefore {
		t.Error("double-lock should not reach the DB")
	}
}

func TestAcquireLock_DBError(t *testing.T) {
	conn := &mockConn{acquireErr: errors.New("pg: lock timeout")}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	err := mgr.AcquireLock(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Key must not be tracked after failure.
	if len(mgr.HeldKeys()) != 0 {
		t.Errorf("expected no held keys after failed acquire, got %v", mgr.HeldKeys())
	}
}

func TestAcquireLock_BeginError(t *testing.T) {
	conn := &mockConn{beginErr: errors.New("pg: connection refused")}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	err := mgr.AcquireLock(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from BeginTx failure, got nil")
	}
	if len(mgr.HeldKeys()) != 0 {
		t.Errorf("expected no held keys, got %v", mgr.HeldKeys())
	}
}

func TestReleaseLock_Success(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	if err := mgr.AcquireLock(context.Background(), 99); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if err := mgr.ReleaseLock(context.Background(), 99); err != nil {
		t.Fatalf("ReleaseLock: %v", err)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.commitCalls != 1 {
		t.Errorf("expected 1 commit, got %d", conn.commitCalls)
	}
	if len(mgr.HeldKeys()) != 0 {
		t.Errorf("expected no held keys after release")
	}
}

func TestReleaseLock_NotHeld_NoOp(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	// Release a key that was never acquired — must not error.
	if err := mgr.ReleaseLock(context.Background(), 555); err != nil {
		t.Fatalf("ReleaseLock on unheld key: %v", err)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.commitCalls != 0 {
		t.Errorf("expected 0 commits for unheld key, got %d", conn.commitCalls)
	}
}

func TestReleaseLock_CommitError(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	if err := mgr.AcquireLock(context.Background(), 5); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	// Inject commit failure.
	conn.mu.Lock()
	conn.commitErr = errors.New("pg: connection lost")
	conn.mu.Unlock()

	err := mgr.ReleaseLock(context.Background(), 5)
	if err == nil {
		t.Fatal("expected commit error, got nil")
	}
	// Key is removed from the map even on commit failure (tx is gone).
	if len(mgr.HeldKeys()) != 0 {
		t.Errorf("expected key removed after release attempt, got %v", mgr.HeldKeys())
	}
}

func TestTryLock_Success(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	acquired, err := mgr.TryLock(context.Background(), 200)
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.tryCalls != 1 {
		t.Errorf("expected 1 try call, got %d", conn.tryCalls)
	}
}

func TestTryLock_Contended(t *testing.T) {
	conn := &mockConn{contended: true}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	acquired, err := mgr.TryLock(context.Background(), 300)
	if err != nil {
		t.Fatalf("TryLock: unexpected error: %v", err)
	}
	if acquired {
		t.Fatal("expected lock to NOT be acquired (contended)")
	}
	if len(mgr.HeldKeys()) != 0 {
		t.Errorf("key should not be tracked when lock was not granted")
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.rollbackCalls != 1 {
		t.Errorf("expected transaction rolled back on contention, got %d rollbacks", conn.rollbackCalls)
	}
}

func TestTryLock_DBError(t *testing.T) {
	conn := &mockConn{acquireErr: errors.New("pg: server error")}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	_, err := mgr.TryLock(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from DB, got nil")
	}
	if len(mgr.HeldKeys()) != 0 {
		t.Errorf("expected no held keys after failed TryLock")
	}
}

func TestTryLock_DoubleLockSameKey(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	if _, err := mgr.TryLock(context.Background(), 11); err != nil {
		t.Fatalf("first TryLock: %v", err)
	}
	_, err := mgr.TryLock(context.Background(), 11)
	if err == nil {
		t.Fatal("expected error on double TryLock, got nil")
	}
}

func TestHeldKeys_MultipleKeys(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	keys := []int64{10, 20, 30}
	for _, k := range keys {
		if err := mgr.AcquireLock(context.Background(), k); err != nil {
			t.Fatalf("AcquireLock(%d): %v", k, err)
		}
	}

	held := mgr.HeldKeys()
	if len(held) != 3 {
		t.Fatalf("expected 3 held keys, got %d: %v", len(held), held)
	}

	// Release two.
	if err := mgr.ReleaseLock(context.Background(), 10); err != nil {
		t.Fatalf("ReleaseLock(10): %v", err)
	}
	if err := mgr.ReleaseLock(context.Background(), 30); err != nil {
		t.Fatalf("ReleaseLock(30): %v", err)
	}

	held = mgr.HeldKeys()
	if len(held) != 1 || held[0] != 20 {
		t.Errorf("expected [20] after partial release, got %v", held)
	}
}

func TestAcquireThenReleaseThenReacquire(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	// Acquire → release → acquire again (must succeed).
	if err := mgr.AcquireLock(context.Background(), 77); err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	if err := mgr.ReleaseLock(context.Background(), 77); err != nil {
		t.Fatalf("ReleaseLock: %v", err)
	}
	if err := mgr.AcquireLock(context.Background(), 77); err != nil {
		t.Fatalf("second AcquireLock after release: %v", err)
	}
}

func TestMixedAcquireAndTryLock(t *testing.T) {
	conn := &mockConn{}
	db := newTestDB(t, conn)
	mgr := locks.New(db)

	if err := mgr.AcquireLock(context.Background(), 1); err != nil {
		t.Fatalf("AcquireLock(1): %v", err)
	}
	acquired, err := mgr.TryLock(context.Background(), 2)
	if err != nil || !acquired {
		t.Fatalf("TryLock(2): acquired=%v err=%v", acquired, err)
	}

	held := mgr.HeldKeys()
	if len(held) != 2 {
		t.Errorf("expected 2 held keys, got %v", held)
	}
}
