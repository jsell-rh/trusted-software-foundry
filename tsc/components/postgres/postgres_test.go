package postgres

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

// badDSN points to an always-refused TCP port so that sql.Open succeeds
// (lazy connection) but any actual network call fails immediately with
// ECONNREFUSED. This lets us exercise non-network code paths.
const badDSN = "host=127.0.0.1 port=60999 dbname=test sslmode=disable"

// --------------------------------------------------------------------------
// Metadata
// --------------------------------------------------------------------------

func TestComponentMetadata(t *testing.T) {
	c := New()
	if c.Name() != componentName {
		t.Errorf("Name() = %q, want %q", c.Name(), componentName)
	}
	if c.Version() != componentVersion {
		t.Errorf("Version() = %q, want %q", c.Version(), componentVersion)
	}
	if c.AuditHash() == "" {
		t.Error("AuditHash() must not be empty")
	}
}

// --------------------------------------------------------------------------
// Configure
// --------------------------------------------------------------------------

func TestConfigure(t *testing.T) {
	c := New()
	cfg := spec.ComponentConfig{
		"dsn":            "host=localhost dbname=test",
		"max_open_conns": 10,
	}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure() error: %v", err)
	}
	if c.cfg.DSN != "host=localhost dbname=test" {
		t.Errorf("DSN = %q, want %q", c.cfg.DSN, "host=localhost dbname=test")
	}
	if c.cfg.MaxOpenConns != 10 {
		t.Errorf("MaxOpenConns = %d, want 10", c.cfg.MaxOpenConns)
	}
}

func TestConfigureDefaults(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure() error: %v", err)
	}
	if c.cfg.MaxOpenConns != 25 {
		t.Errorf("default MaxOpenConns = %d, want 25", c.cfg.MaxOpenConns)
	}
	if c.cfg.MaxIdleConns != 5 {
		t.Errorf("default MaxIdleConns = %d, want 5", c.cfg.MaxIdleConns)
	}
	if c.cfg.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("default ConnMaxLifetime = %v, want 5m", c.cfg.ConnMaxLifetime)
	}
}

func TestConfigure_AllFields(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{
		"dsn":                   "host=db",
		"max_open_conns":        50,
		"max_idle_conns":        10,
		"conn_max_lifetime_sec": 120,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", c.cfg.MaxIdleConns)
	}
	if c.cfg.ConnMaxLifetime != 120*time.Second {
		t.Errorf("ConnMaxLifetime = %v, want 120s", c.cfg.ConnMaxLifetime)
	}
}

func TestConfigure_EmptyDSNIgnored(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": ""}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.DSN != "" {
		t.Errorf("DSN = %q, want empty (not set)", c.cfg.DSN)
	}
}

func TestConfigure_NonIntConnsIgnored(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{
		"max_open_conns": "not-an-int",
		"max_idle_conns": true,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns = %d, want default 25", c.cfg.MaxOpenConns)
	}
}

// --------------------------------------------------------------------------
// Register — sql.Open is lazy so Register works without a real DB.
// --------------------------------------------------------------------------

func TestRegister_SetsDefaultDSN(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatal(err)
	}
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if c.cfg.DSN == "" {
		t.Error("Register should set a default DSN when none provided")
	}
	c.db.db.Close()
}

func TestRegister_PreservesDSN(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if c.cfg.DSN != badDSN {
		t.Errorf("DSN = %q, want preserved %q", c.cfg.DSN, badDSN)
	}
	c.db.db.Close()
}

func TestRegister_BuildsDAOs(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	resources := []spec.ResourceDefinition{
		{Name: "Dinosaur", Plural: "dinosaurs"},
		{Name: "Plant", Plural: "plants"},
	}
	app := spec.NewApplication(resources)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer c.db.db.Close()

	if len(c.daos) != 2 {
		t.Errorf("daos len = %d, want 2", len(c.daos))
	}
	if _, ok := c.daos["Dinosaur"]; !ok {
		t.Error("expected DAO for Dinosaur")
	}
}

func TestRegister_SetsDBOnApplication(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer c.db.db.Close()

	if c.db == nil {
		t.Error("c.db must not be nil after Register")
	}
}

// --------------------------------------------------------------------------
// Start via sqlmock — exercises PingContext + runMigrations
// --------------------------------------------------------------------------

// newMockComponent creates a Component with its db field pre-set to the sqlmock
// database. This lets us test Start without going through Register's sql.Open.
func newMockComponent(t *testing.T, resources []spec.ResourceDefinition) (*Component, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	c := New()
	c.cfg.Resources = resources
	c.db = &sqlDB{db: db}
	return c, mock
}

func TestStart_PingFails(t *testing.T) {
	// Use a real bad DSN so PingContext fails with ECONNREFUSED.
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	if err := c.Register(spec.NewApplication(nil)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer c.db.db.Close()

	err := c.Start(context.Background())
	if err == nil {
		t.Skip("port 60999 appears open; skipping ping-failure assertion")
	}
	if !strings.Contains(err.Error(), "foundry-postgres") {
		t.Errorf("expected 'foundry-postgres' in error, got: %v", err)
	}
}

func TestStart_PingSucceeds_MigrationFails(t *testing.T) {
	c, mock := newMockComponent(t, []spec.ResourceDefinition{
		{Name: "Dinosaur", Plural: "dinosaurs"},
	})
	mock.ExpectPing()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").
		WillReturnError(sql.ErrNoRows)

	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error from migration failure, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-postgres") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_PingSucceeds_MigrationsRun(t *testing.T) {
	resources := []spec.ResourceDefinition{
		{Name: "Dinosaur", Plural: "dinosaurs", Fields: []spec.FieldDefinition{
			{Name: "species", Type: "string"},
		}},
		{Name: "Plant", Plural: "plants"},
	}
	c, mock := newMockComponent(t, resources)
	mock.ExpectPing()
	// Expect CREATE TABLE for each resource.
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS dinosaurs").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS plants").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// --------------------------------------------------------------------------
// Stop
// --------------------------------------------------------------------------

func TestStop_NilDB(t *testing.T) {
	c := New()
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop(nil db): %v", err)
	}
}

func TestStop_ClosesPool(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	if err := c.Register(spec.NewApplication(nil)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// --------------------------------------------------------------------------
// ResourceFor
// --------------------------------------------------------------------------

func TestResourceFor_Found(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	resources := []spec.ResourceDefinition{{Name: "Dinosaur", Plural: "dinosaurs"}}
	if err := c.Register(spec.NewApplication(resources)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer c.db.db.Close()

	if dao := c.ResourceFor("Dinosaur"); dao == nil {
		t.Error("ResourceFor(Dinosaur) = nil, want a DAO")
	}
}

func TestResourceFor_NotFound(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	if err := c.Register(spec.NewApplication(nil)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer c.db.db.Close()

	if dao := c.ResourceFor("Unknown"); dao != nil {
		t.Error("ResourceFor(Unknown) should return nil")
	}
}

// --------------------------------------------------------------------------
// sqlDB.ExecContext / QueryContext
// --------------------------------------------------------------------------

func openBadDB(t *testing.T) *sqlDB {
	t.Helper()
	db, err := sql.Open("postgres", badDSN)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &sqlDB{db: db}
}

func TestSQLDB_ExecContext_PropagatesError(t *testing.T) {
	db := openBadDB(t)
	if err := db.ExecContext(context.Background(), "SELECT 1"); err == nil {
		t.Error("expected error from ExecContext with unreachable DB")
	}
}

func TestSQLDB_QueryContext_PropagatesError(t *testing.T) {
	db := openBadDB(t)
	if _, err := db.QueryContext(context.Background(), "SELECT 1"); err == nil {
		t.Error("expected error from QueryContext with unreachable DB")
	}
}

func TestSQLDB_ExecContext_Success(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()
	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(0, 0))

	db := &sqlDB{db: rawDB}
	if err := db.ExecContext(context.Background(), "SELECT 1"); err != nil {
		t.Errorf("ExecContext: %v", err)
	}
}

func TestSQLDB_QueryContext_Success(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	db := &sqlDB{db: rawDB}
	rows, err := db.QueryContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	rows.Close()
}

// --------------------------------------------------------------------------
// resourceDAO CRUD — tested with sqlmock
// --------------------------------------------------------------------------

func newDAO(t *testing.T, res spec.ResourceDefinition) (*resourceDAO, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &resourceDAO{db: db, resource: res}, mock
}

var dinoResource = spec.ResourceDefinition{
	Name:   "Dinosaur",
	Plural: "dinosaurs",
	Fields: []spec.FieldDefinition{
		{Name: "species", Type: "string"},
	},
}

var dinoEventsResource = spec.ResourceDefinition{
	Name:   "Dinosaur",
	Plural: "dinosaurs",
	Events: true,
	Fields: []spec.FieldDefinition{
		{Name: "species", Type: "string"},
	},
}

var softDeleteResource = spec.ResourceDefinition{
	Name:   "Record",
	Plural: "records",
	Fields: []spec.FieldDefinition{
		{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
	},
}

// Create

func TestCreate_Success(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectQuery("INSERT INTO dinosaurs").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("uuid-123"))

	id, err := dao.Create(context.Background(), map[string]any{"species": "T-Rex"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "uuid-123" {
		t.Errorf("id = %q, want uuid-123", id)
	}
}

func TestCreate_Error(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectQuery("INSERT INTO dinosaurs").
		WillReturnError(sql.ErrConnDone)

	_, err := dao.Create(context.Background(), map[string]any{"species": "T-Rex"})
	if err == nil {
		t.Fatal("expected error from Create, got nil")
	}
	if !strings.Contains(err.Error(), "create Dinosaur") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreate_WithEvents_NotifyCalled(t *testing.T) {
	dao, mock := newDAO(t, dinoEventsResource)
	mock.ExpectQuery("INSERT INTO dinosaurs").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("evt-id"))
	// notify calls SELECT pg_notify(...)
	mock.ExpectExec("SELECT pg_notify").
		WillReturnResult(sqlmock.NewResult(0, 0))

	id, err := dao.Create(context.Background(), map[string]any{"species": "Raptor"})
	if err != nil {
		t.Fatalf("Create with events: %v", err)
	}
	if id != "evt-id" {
		t.Errorf("id = %q, want evt-id", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// Get

func TestGet_Success(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	rows := sqlmock.NewRows([]string{"id", "species"}).
		AddRow("uuid-123", "T-Rex")
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WithArgs("uuid-123").
		WillReturnRows(rows)

	obj, err := dao.Get(context.Background(), "uuid-123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if obj["species"] != "T-Rex" {
		t.Errorf("species = %v, want T-Rex", obj["species"])
	}
}

func TestGet_NotFound(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WithArgs("missing-id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "species"})) // empty

	_, err := dao.Get(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected error for not-found, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestGet_QueryError(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WillReturnError(sql.ErrConnDone)

	_, err := dao.Get(context.Background(), "id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "get Dinosaur") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Update

func TestUpdate_Success(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectExec("UPDATE dinosaurs SET").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := dao.Update(context.Background(), "uuid-1", map[string]any{"species": "Velociraptor"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func TestUpdate_Error(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectExec("UPDATE dinosaurs SET").
		WillReturnError(sql.ErrConnDone)

	err := dao.Update(context.Background(), "uuid-1", map[string]any{"species": "Raptor"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "update Dinosaur") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdate_WithEvents_NotifyCalled(t *testing.T) {
	dao, mock := newDAO(t, dinoEventsResource)
	mock.ExpectExec("UPDATE dinosaurs SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_notify").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := dao.Update(context.Background(), "evt-id", map[string]any{"species": "Rex"}); err != nil {
		t.Fatalf("Update with events: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// Delete

func TestDelete_HardDelete(t *testing.T) {
	dao, mock := newDAO(t, dinoResource) // no soft-delete field
	mock.ExpectExec("DELETE FROM dinosaurs WHERE id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := dao.Delete(context.Background(), "uuid-1"); err != nil {
		t.Fatalf("Delete (hard): %v", err)
	}
}

func TestDelete_SoftDelete(t *testing.T) {
	dao, mock := newDAO(t, softDeleteResource)
	mock.ExpectExec("UPDATE records SET deleted_at").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := dao.Delete(context.Background(), "rec-1"); err != nil {
		t.Fatalf("Delete (soft): %v", err)
	}
}

func TestDelete_Error(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectExec("DELETE FROM dinosaurs WHERE id").
		WillReturnError(sql.ErrConnDone)

	err := dao.Delete(context.Background(), "uuid-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "delete Dinosaur") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDelete_WithEvents_NotifyCalled(t *testing.T) {
	dao, mock := newDAO(t, dinoEventsResource)
	mock.ExpectExec("DELETE FROM dinosaurs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_notify").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := dao.Delete(context.Background(), "del-id"); err != nil {
		t.Fatalf("Delete with events: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// List

func TestList_Success(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	rows := sqlmock.NewRows([]string{"id", "species"}).
		AddRow("id-1", "T-Rex").
		AddRow("id-2", "Raptor")
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnRows(rows)

	results, err := dao.List(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("len = %d, want 2", len(results))
	}
}

func TestList_DefaultSize(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	// size <= 0 → default 100
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if _, err := dao.List(context.Background(), 0, 0); err != nil {
		t.Fatalf("List(size=0): %v", err)
	}
}

func TestList_Error(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnError(sql.ErrConnDone)

	_, err := dao.List(context.Background(), 0, 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "list dinosaurs") {
		t.Errorf("unexpected error: %v", err)
	}
}

// notify

func TestNotify_Success(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectExec("SELECT pg_notify").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := dao.notify(context.Background(), "created", "id-1"); err != nil {
		t.Errorf("notify: %v", err)
	}
}

func TestNotify_Error(t *testing.T) {
	dao, mock := newDAO(t, dinoResource)
	mock.ExpectExec("SELECT pg_notify").
		WillReturnError(sql.ErrConnDone)

	if err := dao.notify(context.Background(), "created", "id-1"); err == nil {
		t.Error("expected error from notify, got nil")
	}
}

// --------------------------------------------------------------------------
// GenerateMigrationSQL
// --------------------------------------------------------------------------

func TestGenerateMigrationSQL(t *testing.T) {
	resources := []spec.ResourceDefinition{
		{
			Name:   "Dinosaur",
			Plural: "dinosaurs",
			Fields: []spec.FieldDefinition{
				{Name: "species", Type: "string", Required: true, MaxLength: 255},
				{Name: "description", Type: "string"},
				{Name: "created_at", Type: "timestamp", Auto: "created"},
				{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
			},
			Operations: []string{"create", "read", "update", "delete", "list"},
			Events:     true,
		},
	}

	got := GenerateMigrationSQL(resources)
	if got == "" {
		t.Fatal("GenerateMigrationSQL() returned empty string")
	}
	for _, substr := range []string{"dinosaurs", "id UUID", "species VARCHAR(255)", "created_at TIMESTAMPTZ", "deleted_at TIMESTAMPTZ"} {
		if !strings.Contains(got, substr) {
			t.Errorf("SQL missing %q\nSQL:\n%s", substr, got)
		}
	}
}

func TestGenerateMigrationSQL_Empty(t *testing.T) {
	if got := GenerateMigrationSQL(nil); got != "" {
		t.Errorf("GenerateMigrationSQL(nil) = %q, want empty", got)
	}
}

func TestGenerateMigrationSQL_MultipleResources(t *testing.T) {
	resources := []spec.ResourceDefinition{
		{Name: "Dinosaur", Plural: "dinosaurs"},
		{Name: "Plant", Plural: "plants"},
	}
	got := GenerateMigrationSQL(resources)
	if !strings.Contains(got, "dinosaurs") {
		t.Error("missing dinosaurs table")
	}
	if !strings.Contains(got, "plants") {
		t.Error("missing plants table")
	}
}

// --------------------------------------------------------------------------
// hasSoftDelete
// --------------------------------------------------------------------------

func TestHasSoftDelete(t *testing.T) {
	dao := &resourceDAO{
		resource: spec.ResourceDefinition{
			Fields: []spec.FieldDefinition{
				{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
			},
		},
	}
	if !dao.hasSoftDelete() {
		t.Error("hasSoftDelete() = false, want true")
	}

	dao2 := &resourceDAO{resource: spec.ResourceDefinition{}}
	if dao2.hasSoftDelete() {
		t.Error("hasSoftDelete() = true, want false for empty resource")
	}
}

// --------------------------------------------------------------------------
// irTypeToSQL
// --------------------------------------------------------------------------

func TestIRTypeToSQL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"string", "TEXT"},
		{"int", "BIGINT"},
		{"float", "DOUBLE PRECISION"},
		{"bool", "BOOLEAN"},
		{"timestamp", "TIMESTAMPTZ"},
		{"uuid", "UUID"},
		{"unknown", "TEXT"},
	}
	for _, tc := range cases {
		got := irTypeToSQL(tc.in)
		if got != tc.want {
			t.Errorf("irTypeToSQL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --------------------------------------------------------------------------
// buildInsert
// --------------------------------------------------------------------------

func TestBuildInsert_SingleField(t *testing.T) {
	cols, placeholders, vals := buildInsert(map[string]any{"name": "rex"})
	if len(cols) != 1 || cols[0] != "name" {
		t.Errorf("cols = %v, want [name]", cols)
	}
	if len(placeholders) != 1 || placeholders[0] != "$1" {
		t.Errorf("placeholders = %v, want [$1]", placeholders)
	}
	if len(vals) != 1 || vals[0] != "rex" {
		t.Errorf("vals = %v, want [rex]", vals)
	}
}

func TestBuildInsert_MultipleFields(t *testing.T) {
	obj := map[string]any{"a": 1, "b": 2, "c": 3}
	cols, placeholders, vals := buildInsert(obj)
	if len(cols) != 3 || len(placeholders) != 3 || len(vals) != 3 {
		t.Errorf("buildInsert: unexpected lengths cols=%d placeholders=%d vals=%d", len(cols), len(placeholders), len(vals))
	}
	sort.Strings(placeholders)
	for i, want := range []string{"$1", "$2", "$3"} {
		if placeholders[i] != want {
			t.Errorf("placeholder[%d] = %q, want %q", i, placeholders[i], want)
		}
	}
}

func TestBuildInsert_Empty(t *testing.T) {
	cols, placeholders, vals := buildInsert(map[string]any{})
	if len(cols) != 0 || len(placeholders) != 0 || len(vals) != 0 {
		t.Error("expected empty slices for empty input")
	}
}

// --------------------------------------------------------------------------
// buildUpdate
// --------------------------------------------------------------------------

func TestBuildUpdate_SkipsID(t *testing.T) {
	sets, vals := buildUpdate(map[string]any{"id": "abc", "name": "rex"})
	if len(sets) != 1 {
		t.Errorf("sets len = %d, want 1 (id must be skipped)", len(sets))
	}
	if sets[0] != "name = $1" {
		t.Errorf("sets[0] = %q, want %q", sets[0], "name = $1")
	}
	if len(vals) != 1 || vals[0] != "rex" {
		t.Errorf("vals = %v, want [rex]", vals)
	}
}

func TestBuildUpdate_MultipleFields(t *testing.T) {
	sets, vals := buildUpdate(map[string]any{"a": 1, "b": 2})
	if len(sets) != 2 || len(vals) != 2 {
		t.Errorf("sets=%d vals=%d, want 2 each", len(sets), len(vals))
	}
}

func TestBuildUpdate_OnlyID(t *testing.T) {
	sets, vals := buildUpdate(map[string]any{"id": "abc"})
	if len(sets) != 0 || len(vals) != 0 {
		t.Error("expected empty slices when only id provided")
	}
}

// --------------------------------------------------------------------------
// fieldToColumn
// --------------------------------------------------------------------------

func TestFieldToColumn_Required(t *testing.T) {
	f := spec.FieldDefinition{Name: "email", Type: "string", Required: true}
	got := fieldToColumn(f)
	if !strings.Contains(got, "NOT NULL") {
		t.Errorf("fieldToColumn(%v) = %q, want NOT NULL", f, got)
	}
}

func TestFieldToColumn_MaxLength(t *testing.T) {
	f := spec.FieldDefinition{Name: "code", Type: "string", MaxLength: 10}
	got := fieldToColumn(f)
	if !strings.Contains(got, "VARCHAR(10)") {
		t.Errorf("fieldToColumn(%v) = %q, want VARCHAR(10)", f, got)
	}
}

func TestFieldToColumn_MaxLengthNonString(t *testing.T) {
	f := spec.FieldDefinition{Name: "count", Type: "int", MaxLength: 10}
	got := fieldToColumn(f)
	if strings.Contains(got, "VARCHAR") {
		t.Errorf("fieldToColumn(%v) = %q, must not contain VARCHAR for non-string", f, got)
	}
}

func TestFieldToColumn_NoConstraints(t *testing.T) {
	f := spec.FieldDefinition{Name: "notes", Type: "string"}
	if got := fieldToColumn(f); got != "notes TEXT" {
		t.Errorf("fieldToColumn(%v) = %q, want %q", f, got, "notes TEXT")
	}
}
