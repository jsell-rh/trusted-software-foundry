package migrations_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/foundry/components/postgres/migrations"
)

// ---------------------------------------------------------------------------
// In-process SQL mock — no external deps
// ---------------------------------------------------------------------------

// mockState tracks the state of the in-memory fake database.
type mockState struct {
	mu sync.Mutex

	// applied tracks IDs recorded in the fake _foundry_migrations table.
	applied map[string]time.Time

	// tableExists controls whether the tracking table is already present.
	tableExists bool

	// Injected failures:
	createTableErr error // returned on CREATE TABLE
	queryErr       error // returned on SELECT from tracking table
	execMigErr     error // returned on the user migration SQL exec
	recordErr      error // returned on INSERT into tracking table
	beginErr       error // returned on BeginTx
	commitErr      error // returned on Commit; set per-call via commitErrAfter
	commitErrAfter int   // trigger commitErr after N commits

	commitCount int
}

func newMockState() *mockState {
	return &mockState{applied: make(map[string]time.Time)}
}

// --- database/sql/driver implementation ---

type mockMigrationsDriver struct {
	mu     sync.Mutex
	states map[string]*mockState
}

func (d *mockMigrationsDriver) Open(name string) (driver.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.states[name]
	if !ok {
		return nil, fmt.Errorf("mockMigrationsDriver: unknown DSN %q", name)
	}
	return &mockMigrConn{state: s}, nil
}

type mockMigrConn struct{ state *mockState }

func (c *mockMigrConn) Prepare(query string) (driver.Stmt, error) {
	return &mockMigrStmt{state: c.state, query: query}, nil
}
func (c *mockMigrConn) Close() error { return nil }
func (c *mockMigrConn) Begin() (driver.Tx, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.beginErr != nil {
		return nil, c.state.beginErr
	}
	return &mockMigrTx{state: c.state}, nil
}

type mockMigrTx struct{ state *mockState }

func (t *mockMigrTx) Commit() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.commitCount++
	if t.state.commitErr != nil && t.state.commitCount >= t.state.commitErrAfter {
		return t.state.commitErr
	}
	return nil
}
func (t *mockMigrTx) Rollback() error { return nil }

type mockMigrStmt struct {
	state *mockState
	query string
}

func (s *mockMigrStmt) Close() error  { return nil }
func (s *mockMigrStmt) NumInput() int { return -1 }

func (s *mockMigrStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	switch {
	case contains(s.query, "CREATE TABLE IF NOT EXISTS _foundry_migrations"):
		if s.state.createTableErr != nil {
			return nil, s.state.createTableErr
		}
		s.state.tableExists = true
		return driver.RowsAffected(0), nil

	case contains(s.query, "INSERT INTO _foundry_migrations"):
		if s.state.recordErr != nil {
			return nil, s.state.recordErr
		}
		if len(args) > 0 {
			id, _ := args[0].(string)
			s.state.applied[id] = time.Now()
		}
		return driver.RowsAffected(1), nil

	default:
		// User migration SQL.
		if s.state.execMigErr != nil {
			return nil, s.state.execMigErr
		}
		return driver.RowsAffected(0), nil
	}
}

func (s *mockMigrStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	if contains(s.query, "SELECT id") && contains(s.query, "_foundry_migrations") {
		if s.state.queryErr != nil {
			return nil, s.state.queryErr
		}
		// Build rows from the applied map.
		ids := make([]string, 0, len(s.state.applied))
		times := make([]time.Time, 0, len(s.state.applied))
		for id, at := range s.state.applied {
			ids = append(ids, id)
			times = append(times, at)
		}
		// Detect whether caller wants applied_at column too.
		withTime := contains(s.query, "applied_at")
		return &idRows{ids: ids, times: times, withTime: withTime}, nil
	}
	return &emptyMigrRows{}, nil
}

// --- helper Rows ---

type idRows struct {
	ids      []string
	times    []time.Time
	withTime bool
	pos      int
}

func (r *idRows) Columns() []string {
	if r.withTime {
		return []string{"id", "applied_at"}
	}
	return []string{"id"}
}
func (r *idRows) Close() error { return nil }
func (r *idRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.ids) {
		return io.EOF
	}
	dest[0] = r.ids[r.pos]
	if r.withTime && len(dest) > 1 {
		dest[1] = r.times[r.pos]
	}
	r.pos++
	return nil
}

type emptyMigrRows struct{}

func (r *emptyMigrRows) Columns() []string           { return nil }
func (r *emptyMigrRows) Close() error                { return nil }
func (r *emptyMigrRows) Next(_ []driver.Value) error { return io.EOF }

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) &&
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}()
}

// ---------------------------------------------------------------------------
// Test infrastructure
// ---------------------------------------------------------------------------

var (
	migrDriver     *mockMigrationsDriver
	migrDriverOnce sync.Once
)

func init() {
	migrDriver = &mockMigrationsDriver{states: make(map[string]*mockState)}
	sql.Register("mock-migrations", migrDriver)
}

func newTestRunner(t *testing.T, state *mockState) *migrations.Runner {
	t.Helper()
	dsn := fmt.Sprintf("migr-%s", t.Name())
	migrDriver.mu.Lock()
	migrDriver.states[dsn] = state
	migrDriver.mu.Unlock()
	t.Cleanup(func() {
		migrDriver.mu.Lock()
		delete(migrDriver.states, dsn)
		migrDriver.mu.Unlock()
	})
	db, err := sql.Open("mock-migrations", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return migrations.New(db)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestApply_EmptySlice(t *testing.T) {
	r := newTestRunner(t, newMockState())
	if err := r.Apply(context.Background(), nil); err != nil {
		t.Fatalf("Apply(nil): %v", err)
	}
}

func TestApply_SingleMigration(t *testing.T) {
	state := newMockState()
	r := newTestRunner(t, state)

	migs := []migrations.Migration{
		{ID: "001_init", SQL: "CREATE TABLE foo (id UUID PRIMARY KEY)"},
	}
	if err := r.Apply(context.Background(), migs); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if _, ok := state.applied["001_init"]; !ok {
		t.Error("expected 001_init to be recorded")
	}
}

func TestApply_MultipleMigrationsInOrder(t *testing.T) {
	state := newMockState()
	r := newTestRunner(t, state)

	migs := []migrations.Migration{
		{ID: "001_create_users", SQL: "CREATE TABLE users (id UUID PRIMARY KEY)"},
		{ID: "002_add_email", SQL: "ALTER TABLE users ADD COLUMN email TEXT"},
		{ID: "003_add_index", SQL: "CREATE INDEX idx_users_email ON users (email)"},
	}
	if err := r.Apply(context.Background(), migs); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	for _, m := range migs {
		if _, ok := state.applied[m.ID]; !ok {
			t.Errorf("expected %q to be recorded", m.ID)
		}
	}
}

func TestApply_Idempotent_SkipsAlreadyApplied(t *testing.T) {
	state := newMockState()
	// Pre-populate: 001 already applied.
	state.applied["001_init"] = time.Now()
	r := newTestRunner(t, state)

	migs := []migrations.Migration{
		{ID: "001_init", SQL: "CREATE TABLE foo (id UUID PRIMARY KEY)"},
		{ID: "002_add_col", SQL: "ALTER TABLE foo ADD COLUMN name TEXT"},
	}
	if err := r.Apply(context.Background(), migs); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if _, ok := state.applied["002_add_col"]; !ok {
		t.Error("expected 002_add_col to be recorded")
	}
}

func TestApply_Idempotent_SecondCallNoOp(t *testing.T) {
	state := newMockState()
	r := newTestRunner(t, state)

	migs := []migrations.Migration{
		{ID: "001_init", SQL: "CREATE TABLE baz (id UUID PRIMARY KEY)"},
	}
	if err := r.Apply(context.Background(), migs); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// Verify that second Apply doesn't fail.
	execErrBefore := state.execMigErr
	if err := r.Apply(context.Background(), migs); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	_ = execErrBefore // no exec should happen
}

func TestApply_EmptyID_Error(t *testing.T) {
	state := newMockState()
	r := newTestRunner(t, state)

	migs := []migrations.Migration{
		{ID: "", SQL: "SELECT 1"},
	}
	err := r.Apply(context.Background(), migs)
	if err == nil {
		t.Fatal("expected error for empty migration ID")
	}
}

func TestApply_CreateTableError(t *testing.T) {
	state := newMockState()
	state.createTableErr = errors.New("pg: permission denied")
	r := newTestRunner(t, state)

	err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001", SQL: "CREATE TABLE x (id UUID PRIMARY KEY)"},
	})
	if err == nil {
		t.Fatal("expected error from createTable failure")
	}
}

func TestApply_QueryError(t *testing.T) {
	state := newMockState()
	state.queryErr = errors.New("pg: table not found")
	r := newTestRunner(t, state)

	err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001", SQL: "SELECT 1"},
	})
	if err == nil {
		t.Fatal("expected error from queryErr")
	}
}

func TestApply_MigrationExecError(t *testing.T) {
	state := newMockState()
	state.execMigErr = errors.New("pg: syntax error")
	r := newTestRunner(t, state)

	err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001_bad", SQL: "INVALID SQL"},
	})
	if err == nil {
		t.Fatal("expected error from migration exec failure")
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if _, ok := state.applied["001_bad"]; ok {
		t.Error("failed migration must not be recorded in tracking table")
	}
}

func TestApply_RecordError(t *testing.T) {
	state := newMockState()
	state.recordErr = errors.New("pg: unique violation")
	r := newTestRunner(t, state)

	err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001_rec_fail", SQL: "CREATE TABLE y (id UUID PRIMARY KEY)"},
	})
	if err == nil {
		t.Fatal("expected error from record failure")
	}
}

func TestApply_BeginError(t *testing.T) {
	state := newMockState()
	state.beginErr = errors.New("pg: connection refused")
	r := newTestRunner(t, state)

	err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001", SQL: "SELECT 1"},
	})
	if err == nil {
		t.Fatal("expected error from BeginTx failure")
	}
}

func TestApply_CommitError(t *testing.T) {
	state := newMockState()
	state.commitErr = errors.New("pg: connection lost")
	state.commitErrAfter = 1 // fail on first commit (the migration tx)
	r := newTestRunner(t, state)

	err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001_commit_fail", SQL: "SELECT 1"},
	})
	if err == nil {
		t.Fatal("expected error from commit failure")
	}
}

func TestApply_PartialFailure_SubsequentMigsNotRun(t *testing.T) {
	state := newMockState()
	// First migration succeeds; second fails on exec.
	callCount := 0
	_ = callCount
	state.execMigErr = nil // start with no error
	r := newTestRunner(t, state)

	// Apply first migration successfully.
	if err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001_ok", SQL: "SELECT 1"},
	}); err != nil {
		t.Fatalf("Apply first: %v", err)
	}

	// Now inject failure and apply second.
	state.mu.Lock()
	state.execMigErr = errors.New("pg: disk full")
	state.mu.Unlock()

	err := r.Apply(context.Background(), []migrations.Migration{
		{ID: "001_ok", SQL: "SELECT 1"}, // already applied, skipped
		{ID: "002_fail", SQL: "INSERT INTO x"},
	})
	if err == nil {
		t.Fatal("expected error on second migration failure")
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if _, ok := state.applied["002_fail"]; ok {
		t.Error("failed migration 002_fail must not be recorded")
	}
}

func TestApplied_ReturnsTimestamps(t *testing.T) {
	state := newMockState()
	now := time.Now().UTC().Truncate(time.Second)
	state.applied["001_init"] = now
	r := newTestRunner(t, state)

	result, err := r.Applied(context.Background())
	if err != nil {
		t.Fatalf("Applied: %v", err)
	}
	if _, ok := result["001_init"]; !ok {
		t.Error("expected 001_init in Applied() result")
	}
}

func TestApplied_EmptyDB(t *testing.T) {
	state := newMockState()
	r := newTestRunner(t, state)

	result, err := r.Applied(context.Background())
	if err != nil {
		t.Fatalf("Applied on empty DB: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}
