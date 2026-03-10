// Package postgres implements the foundry-postgres trusted component.
//
// foundry-postgres provides:
//   - A PostgreSQL connection pool (implements spec.DB)
//   - Per-resource CRUD DAOs generated from IRSpec ResourceDefinitions
//   - SQL migration generation from IR resource declarations
//   - PostgreSQL LISTEN/NOTIFY event emission on mutations (when Events: true)
//
// Audit record is frozen at audit time. Bug fixes create new audited versions.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-postgres"
	componentVersion = "v1.0.0"
	auditHash        = "sha256:a3f8c2e1b4d7f9a0c5e2b8d4f1a6c3e0b7d2f5a8c1e4b7d0f3a6c9e2b5d8f1a4"
)

// Component is the foundry-postgres trusted component implementation.
// It satisfies spec.Component and spec.ResourceProvider.
type Component struct {
	cfg  Config
	db   *sqlDB
	daos map[string]*resourceDAO
	app  *spec.Application // held to read TenantField() at migration time
}

// Config holds the postgres component configuration derived from the IR spec.
type Config struct {
	// DSN is the PostgreSQL data source name. Resolved from env if ${VAR} syntax used.
	DSN string
	// MaxOpenConns is the maximum number of open connections (default: 25).
	MaxOpenConns int
	// MaxIdleConns is the maximum number of idle connections (default: 5).
	MaxIdleConns int
	// ConnMaxLifetime is the max connection lifetime (default: 5 minutes).
	ConnMaxLifetime time.Duration
	// Resources are the IR resource definitions to create DAOs for.
	Resources []spec.ResourceDefinition
}

// New returns a new foundry-postgres Component. Use in generated main.go.
func New() *Component {
	return &Component{
		daos: make(map[string]*resourceDAO),
	}
}

// --- spec.Component interface ---

func (c *Component) Name() string { return componentName }

func (c *Component) Version() string { return componentVersion }

func (c *Component) AuditHash() string { return auditHash }

// Configure reads the postgres config from the IR spec section.
// Expected keys in cfg (ComponentConfig map[string]any):
//   - dsn: PostgreSQL DSN string (or ${ENV_VAR})
//   - max_open_conns: int (optional, default 25)
//   - max_idle_conns: int (optional, default 5)
//   - conn_max_lifetime_sec: int (optional, default 300)
func (c *Component) Configure(cfg spec.ComponentConfig) error {
	c.cfg = Config{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}

	if dsn, ok := cfg["dsn"].(string); ok && dsn != "" {
		c.cfg.DSN = dsn
	}
	if v, ok := cfg["max_open_conns"].(int); ok {
		c.cfg.MaxOpenConns = v
	}
	if v, ok := cfg["max_idle_conns"].(int); ok {
		c.cfg.MaxIdleConns = v
	}
	if v, ok := cfg["conn_max_lifetime_sec"].(int); ok {
		c.cfg.ConnMaxLifetime = time.Duration(v) * time.Second
	}

	return nil
}

// Register connects to PostgreSQL, runs migrations, creates DAOs, and sets
// the DB on the application registrar so other components can use it.
func (c *Component) Register(app *spec.Application) error {
	c.app = app
	c.cfg.Resources = app.Resources()

	if c.cfg.DSN == "" {
		c.cfg.DSN = "host=localhost user=postgres dbname=postgres sslmode=disable"
	}

	sqldb, err := sql.Open("postgres", c.cfg.DSN)
	if err != nil {
		return fmt.Errorf("foundry-postgres: open db: %w", err)
	}
	sqldb.SetMaxOpenConns(c.cfg.MaxOpenConns)
	sqldb.SetMaxIdleConns(c.cfg.MaxIdleConns)
	sqldb.SetConnMaxLifetime(c.cfg.ConnMaxLifetime)

	c.db = &sqlDB{db: sqldb}

	// Expose DB to other components (e.g. foundry-events).
	app.SetDB(c.db)

	// Build one DAO per resource.
	for _, res := range c.cfg.Resources {
		c.daos[res.Name] = &resourceDAO{db: sqldb, resource: res, app: app}
	}

	// Register REST CRUD handlers for each resource.
	c.registerCRUDHandlers(app)

	return nil
}

// Start pings the database and runs migrations.
func (c *Component) Start(ctx context.Context) error {
	if err := c.db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("foundry-postgres: ping: %w", err)
	}
	if err := c.runMigrations(ctx); err != nil {
		return fmt.Errorf("foundry-postgres: migrations: %w", err)
	}
	return nil
}

// Stop closes the connection pool.
func (c *Component) Stop(ctx context.Context) error {
	if c.db != nil {
		return c.db.db.Close()
	}
	return nil
}

// ResourceFor returns the DAO for the named resource, or nil.
// Implements spec.ResourceProvider.
func (c *Component) ResourceFor(resourceName string) *resourceDAO {
	return c.daos[resourceName]
}

// --- Migration generation ---

// runMigrations creates tables for each IR resource if they do not exist.
// When foundry-tenancy is registered, the tenant isolation column is injected
// automatically into the DDL (and added via ALTER TABLE for existing tables).
func (c *Component) runMigrations(ctx context.Context) error {
	tenantField := ""
	if c.app != nil {
		tenantField = c.app.TenantField()
	}
	for _, res := range c.cfg.Resources {
		ddl := generateCreateTable(res, tenantField)
		if _, err := c.db.db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("migrate %s: %w", res.Name, err)
		}
		// Idempotent column addition for tables that predate tenancy being enabled.
		if tenantField != "" {
			table := strings.ToLower(res.Plural)
			alterSQL := fmt.Sprintf(
				"ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s TEXT NOT NULL DEFAULT ''",
				table, tenantField,
			)
			if _, err := c.db.db.ExecContext(ctx, alterSQL); err != nil {
				return fmt.Errorf("add tenant column to %s: %w", res.Name, err)
			}
		}
	}
	return nil
}

// GenerateMigrationSQL returns the SQL DDL for all resources.
// tenantField is the column name to inject for row-level tenancy (e.g. "org_id").
// Pass an empty string to omit tenant isolation columns.
// Called by the compiler to write migrations/ files.
func GenerateMigrationSQL(resources []spec.ResourceDefinition, tenantField string) string {
	var sb strings.Builder
	for _, res := range resources {
		sb.WriteString(generateCreateTable(res, tenantField))
		sb.WriteString("\n")
	}
	return sb.String()
}

// generateCreateTable produces a CREATE TABLE IF NOT EXISTS statement for res.
// If tenantField is non-empty and not already declared in res.Fields, the column
// is injected right after the primary key column so every table is tenant-scoped.
func generateCreateTable(res spec.ResourceDefinition, tenantField string) string {
	table := strings.ToLower(res.Plural)
	var cols []string
	cols = append(cols, "  id UUID PRIMARY KEY DEFAULT gen_random_uuid()")

	// Inject tenant isolation column unless the resource already declares it.
	if tenantField != "" && !resourceHasField(res, tenantField) {
		cols = append(cols, "  "+tenantField+" TEXT NOT NULL DEFAULT ''")
	}

	for _, f := range res.Fields {
		cols = append(cols, "  "+fieldToColumn(f))
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n%s\n);\n",
		table, strings.Join(cols, ",\n"))
}

// resourceHasField returns true if res.Fields already contains a field with fieldName
// (case-insensitive comparison).
func resourceHasField(res spec.ResourceDefinition, fieldName string) bool {
	lower := strings.ToLower(fieldName)
	for _, f := range res.Fields {
		if strings.ToLower(f.Name) == lower {
			return true
		}
	}
	return false
}

func fieldToColumn(f spec.FieldDefinition) string {
	colType := irTypeToSQL(f.Type)
	var constraints []string

	if f.Required {
		constraints = append(constraints, "NOT NULL")
	}
	if f.MaxLength > 0 && f.Type == "string" {
		colType = fmt.Sprintf("VARCHAR(%d)", f.MaxLength)
	}

	col := f.Name + " " + colType
	if len(constraints) > 0 {
		col += " " + strings.Join(constraints, " ")
	}
	return col
}

func irTypeToSQL(t string) string {
	switch t {
	case "string":
		return "TEXT"
	case "int":
		return "BIGINT"
	case "float":
		return "DOUBLE PRECISION"
	case "bool":
		return "BOOLEAN"
	case "timestamp":
		return "TIMESTAMPTZ"
	case "uuid":
		return "UUID"
	default:
		return "TEXT"
	}
}

// --- spec.DB implementation ---

type sqlDB struct {
	db *sql.DB
}

func (s *sqlDB) ExecContext(ctx context.Context, query string, args ...any) error {
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *sqlDB) QueryContext(ctx context.Context, query string, args ...any) (spec.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

// --- ResourceDAO ---

// resourceDAO is a CRUD data access object for a single IR resource.
// The table name and column list are derived from the ResourceDefinition.
// app is held to read the tenant field lazily at query time (foundry-tenancy
// may register after foundry-postgres, so we cannot cache the field at creation).
type resourceDAO struct {
	db       *sql.DB
	resource spec.ResourceDefinition
	app      *spec.Application
}

// tenantClause returns an additional WHERE clause fragment and the tenant value
// when a tenant ID is present in ctx. The placeholder index p is the next
// available parameter index (e.g. $3). Returns ("", nil) when no tenant context.
func (d *resourceDAO) tenantClause(ctx context.Context, p int) (clause string, val []any) {
	if d.app == nil {
		return "", nil
	}
	field := d.app.TenantField()
	if field == "" {
		return "", nil
	}
	tenantID, ok := spec.TenantIDFromContext(ctx)
	if !ok {
		return "", nil
	}
	return fmt.Sprintf(" AND %s = $%d", field, p), []any{tenantID}
}

// writableFieldSet returns the set of field names that users are allowed to write.
// Auto-managed fields (created_at, updated_at) and soft-delete sentinel columns
// are excluded; only columns explicitly declared in the IR resource definition
// with no Auto tag and not marked as soft-delete are writable.
// System base columns (id, created_at, updated_at, deleted_at) are never writable.
func (d *resourceDAO) writableFieldSet() map[string]bool {
	protected := map[string]bool{"id": true, "created_at": true, "updated_at": true, "deleted_at": true}
	allowed := make(map[string]bool, len(d.resource.Fields))
	for _, f := range d.resource.Fields {
		name := strings.ToLower(f.Name)
		if !protected[name] && f.Auto == "" && !f.SoftDelete {
			allowed[name] = true
		}
	}
	return allowed
}

// filterWritable returns a copy of obj containing only keys that are writable fields
// for this resource. This prevents SQL injection via user-controlled column names
// and rejects attempts to overwrite auto-managed columns.
func (d *resourceDAO) filterWritable(obj map[string]any) map[string]any {
	allowed := d.writableFieldSet()
	filtered := make(map[string]any, len(obj))
	for k, v := range obj {
		if allowed[strings.ToLower(k)] {
			filtered[strings.ToLower(k)] = v
		}
	}
	return filtered
}

// Create inserts obj (map[string]any) into the resource table and returns the generated ID.
func (d *resourceDAO) Create(ctx context.Context, obj map[string]any) (string, error) {
	table := strings.ToLower(d.resource.Plural)
	cols, placeholders, vals := buildInsert(d.filterWritable(obj))

	var query string
	if len(cols) == 0 {
		// No user-supplied fields — insert with all database defaults.
		query = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES RETURNING id", table)
	} else {
		query = fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s) RETURNING id",
			table, strings.Join(cols, ", "), strings.Join(placeholders, ", "),
		)
	}

	var id string
	err := d.db.QueryRowContext(ctx, query, vals...).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", d.resource.Name, err)
	}

	// Emit event if requested.
	if d.resource.Events {
		_ = d.notify(ctx, "created", id)
	}
	return id, nil
}

// Get retrieves a single row by ID and returns map[string]any.
func (d *resourceDAO) Get(ctx context.Context, id string) (map[string]any, error) {
	table := strings.ToLower(d.resource.Plural)
	tenantSQL, tenantVals := d.tenantClause(ctx, 2)
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 AND deleted_at IS NULL%s", table, tenantSQL)
	args := append([]any{id}, tenantVals...)
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get %s %s: %w", d.resource.Name, id, err)
	}
	defer rows.Close()

	results, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%s %s: not found", d.resource.Name, id)
	}
	return results[0], nil
}

// Update replaces the row with id. obj must include the "id" field.
func (d *resourceDAO) Update(ctx context.Context, id string, obj map[string]any) error {
	table := strings.ToLower(d.resource.Plural)
	sets, vals := buildUpdate(d.filterWritable(obj))
	if len(sets) == 0 {
		// Nothing to update — all fields were filtered or only id was provided.
		return fmt.Errorf("update %s: no writable fields provided", d.resource.Name)
	}
	vals = append(vals, id)
	tenantSQL, tenantVals := d.tenantClause(ctx, len(vals)+1)
	vals = append(vals, tenantVals...)

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $%d AND deleted_at IS NULL%s",
		table, strings.Join(sets, ", "), len(vals)-len(tenantVals), tenantSQL,
	)
	if _, err := d.db.ExecContext(ctx, query, vals...); err != nil {
		return fmt.Errorf("update %s %s: %w", d.resource.Name, id, err)
	}
	if d.resource.Events {
		_ = d.notify(ctx, "updated", id)
	}
	return nil
}

// Delete soft-deletes a row by setting deleted_at (if the resource has soft_delete field),
// otherwise hard-deletes.
func (d *resourceDAO) Delete(ctx context.Context, id string) error {
	table := strings.ToLower(d.resource.Plural)
	tenantSQL, tenantVals := d.tenantClause(ctx, 2)
	args := append([]any{id}, tenantVals...)
	var query string
	if d.hasSoftDelete() {
		query = fmt.Sprintf("UPDATE %s SET deleted_at = now() WHERE id = $1%s", table, tenantSQL)
	} else {
		query = fmt.Sprintf("DELETE FROM %s WHERE id = $1%s", table, tenantSQL)
	}
	if _, err := d.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete %s %s: %w", d.resource.Name, id, err)
	}
	if d.resource.Events {
		_ = d.notify(ctx, "deleted", id)
	}
	return nil
}

// Count returns the total number of non-deleted rows (tenant-scoped when applicable).
// Used by List to populate the total field in paginated responses.
func (d *resourceDAO) Count(ctx context.Context) (int64, error) {
	table := strings.ToLower(d.resource.Plural)
	tenantSQL, tenantVals := d.tenantClause(ctx, 1)
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL%s", table, tenantSQL)
	var total int64
	if err := d.db.QueryRowContext(ctx, query, tenantVals...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count %s: %w", d.resource.Plural, err)
	}
	return total, nil
}

// List returns paginated non-deleted rows. Page is 1-based.
func (d *resourceDAO) List(ctx context.Context, page, size int) ([]map[string]any, error) {
	table := strings.ToLower(d.resource.Plural)
	if size <= 0 {
		size = 100
	}
	offset := (page - 1) * size

	tenantSQL, tenantVals := d.tenantClause(ctx, 3)
	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE deleted_at IS NULL%s ORDER BY id LIMIT $1 OFFSET $2",
		table, tenantSQL,
	)
	args := append([]any{size, offset}, tenantVals...)
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", d.resource.Plural, err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// notify emits a PostgreSQL NOTIFY event for this resource mutation.
// The payload is built with encoding/json to correctly escape IDs that may
// contain special characters (e.g. quotes from URL path extraction).
func (d *resourceDAO) notify(ctx context.Context, action, id string) error {
	channel := strings.ToLower(d.resource.Name) + "_events"
	payloadBytes, err := json.Marshal(map[string]string{"action": action, "id": id})
	if err != nil {
		return fmt.Errorf("notify marshal: %w", err)
	}
	_, err = d.db.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, string(payloadBytes))
	return err
}

func (d *resourceDAO) hasSoftDelete() bool {
	for _, f := range d.resource.Fields {
		if f.SoftDelete {
			return true
		}
	}
	return false
}

// --- SQL helpers ---

// buildInsert builds the column, placeholder, and value slices for an INSERT statement.
// Keys are sorted alphabetically to produce deterministic SQL regardless of map iteration order.
func buildInsert(obj map[string]any) (cols, placeholders []string, vals []any) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		cols = append(cols, k)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
		vals = append(vals, obj[k])
	}
	return
}

// buildUpdate builds the SET clause and value slice for an UPDATE statement.
// Keys are sorted alphabetically to produce deterministic SQL regardless of map iteration order.
func buildUpdate(obj map[string]any) (sets []string, vals []any) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		if k != "id" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for i, k := range keys {
		sets = append(sets, fmt.Sprintf("%s = $%d", k, i+1))
		vals = append(vals, obj[k])
	}
	return
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var results []map[string]any
	for rows.Next() {
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = dest[i]
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
