// Package spec defines the core interfaces and types for the Trusted Software
// Trusted Software Foundry (TSF) platform. All trusted components must implement these
// interfaces. The component interface contract is frozen — bug fixes create
// new audited versions rather than modifying existing ones.
package spec

import "context"

// ComponentConfig carries the IR spec section for a single component,
// as parsed from the app.foundry.yaml spec file.
type ComponentConfig map[string]any

// Component is implemented by every trusted component in the Foundry registry.
// All methods must be safe for concurrent use.
type Component interface {
	// Name returns the registry name, e.g. "foundry-http".
	Name() string

	// Version returns the semver string, e.g. "v1.0.0".
	Version() string

	// AuditHash returns the SHA-256 hex digest of the component source tree
	// at audit time. The compiler verifies this matches the registry record
	// before generating output — a mismatch causes a fatal compile error.
	AuditHash() string

	// Configure applies the IR spec section for this component.
	// Called once, before Register. Returns an error if the config is invalid.
	Configure(cfg ComponentConfig) error

	// Register wires this component into the application.
	// Called after Configure on all components. The component should attach
	// HTTP routes, gRPC services, middleware, or background goroutines to app.
	Register(app *Application) error

	// Start is called after all components have been registered.
	// Long-running work (servers, listeners) should begin here.
	// The component must respect ctx cancellation for clean shutdown.
	Start(ctx context.Context) error

	// Stop is called when the application is shutting down.
	// The component must release resources and return promptly.
	Stop(ctx context.Context) error
}

// ResourceDefinition describes a data resource declared in the IR spec.
// The compiler passes these to components (e.g. foundry-postgres) that need
// to generate migrations and CRUD handlers.
type ResourceDefinition struct {
	// Name is the singular resource name, e.g. "Dinosaur".
	Name string

	// Plural is the pluralised form used in API paths, e.g. "dinosaurs".
	Plural string

	// Fields describes the resource's data fields.
	Fields []FieldDefinition

	// Operations lists the CRUD operations to expose: create, read, update, delete, list.
	Operations []string

	// Events, when true, means the component must emit PostgreSQL LISTEN/NOTIFY
	// events on every mutation.
	Events bool
}

// FieldDefinition describes a single field in a ResourceDefinition.
type FieldDefinition struct {
	// Name is the field name in snake_case.
	Name string

	// Type is one of: string, int, float, bool, timestamp, uuid.
	Type string

	// Required, when true, disallows null/empty values.
	Required bool

	// MaxLength constrains string fields (0 = unlimited).
	MaxLength int

	// Auto, when non-empty, auto-populates the field:
	//   "created" — set on insert
	//   "updated" — set on insert and update
	Auto string

	// SoftDelete, when true, marks this field as the soft-delete timestamp.
	// The component must filter deleted records from list queries.
	SoftDelete bool
}

// Registrar is the write side of Application — exposed to components during
// Register so they can attach capabilities without accessing runtime state.
type Registrar interface {
	// AddHTTPHandler registers an HTTP handler at the given pattern.
	AddHTTPHandler(pattern string, handler HTTPHandler)

	// AddMiddleware appends an HTTP middleware to the global chain.
	AddMiddleware(mw HTTPMiddleware)

	// AddGRPCService registers a gRPC service descriptor and its implementation.
	AddGRPCService(desc GRPCServiceDesc, impl any)

	// SetDB provides a database connection pool to any component that needs it.
	// Only foundry-postgres calls this; other components receive it via DB().
	SetDB(db DB)

	// DB returns the shared database connection pool, or nil if not yet set.
	DB() DB

	// Resources returns the resource definitions declared in the IR spec.
	Resources() []ResourceDefinition
}

// HTTPHandler is a minimal abstraction over net/http.Handler.
type HTTPHandler interface {
	ServeHTTP(w ResponseWriter, r *Request)
}

// HTTPMiddleware wraps an HTTPHandler.
type HTTPMiddleware func(next HTTPHandler) HTTPHandler

// ResponseWriter mirrors net/http.ResponseWriter.
type ResponseWriter interface {
	Header() map[string][]string
	Write([]byte) (int, error)
	WriteHeader(statusCode int)
}

// Request mirrors net/http.Request (subset used by components).
type Request struct {
	Method  string
	URL     string
	Headers map[string][]string
	Body    []byte
	Context context.Context
}

// GRPCServiceDesc is an opaque handle to a gRPC service descriptor.
type GRPCServiceDesc any

// DB is a minimal interface over a SQL connection pool.
// foundry-postgres implements this; other components depend on it.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) error
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
}

// Rows is returned by DB.QueryContext.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// tenantIDKeyType is an unexported type for the tenant context key to avoid
// collisions with other packages using context.WithValue.
type tenantIDKeyType struct{}

// TenantIDKey is the context key used by foundry-tenancy to store the
// tenant identifier extracted from the request. foundry-postgres reads this
// key from the request context to scope all queries to the correct tenant.
var TenantIDKey = tenantIDKeyType{}

// TenantIDFromContext extracts the tenant identifier from ctx.
// Returns ("", false) when no tenant is in context.
func TenantIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(TenantIDKey).(string)
	return id, ok && id != ""
}

// WithTenantID returns a copy of ctx with the tenant identifier attached.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// ---------------------------------------------------------------------------
// Error tracking
// ---------------------------------------------------------------------------

// ErrorTracker is the interface for reporting unexpected errors to an external
// tracking service (e.g. Sentry, Rollbar). The platform core uses this interface
// so it stays decoupled from any specific service — teams plug in their preferred
// tracker via Application.SetErrorTracker.
//
// All methods must be safe for concurrent use.
type ErrorTracker interface {
	// ReportError sends an error event with optional structured tags.
	// tags is a flat map of key=value strings for additional context
	// (e.g. "component", "foundry-postgres", "operation", "db.query").
	// Implementations must not panic on nil err.
	ReportError(ctx context.Context, err error, tags map[string]string)

	// Flush blocks until all pending error events have been delivered,
	// or until the context is cancelled.
	// Call this during graceful shutdown so events are not lost.
	Flush(ctx context.Context)
}

// NoopErrorTracker silently discards all errors. It is the default tracker
// used when no external tracker is configured.
type NoopErrorTracker struct{}

func (NoopErrorTracker) ReportError(_ context.Context, _ error, _ map[string]string) {}
func (NoopErrorTracker) Flush(_ context.Context)                                     {}
