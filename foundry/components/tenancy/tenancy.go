// Package tenancy provides the foundry-tenancy trusted component —
// multi-tenant row-level isolation for all database resources.
//
// Configuration (spec tenancy block):
//
//	tenancy:
//	  field: org_id          # column name in every table
//	  strategy: row          # row-level isolation (only option today)
//	  header: X-Organization-Id   # HTTP header carrying the tenant ID
package tenancy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-tenancy"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000015"
)

// TenancyComponent implements spec.Component for multi-tenant isolation.
// It installs middleware that extracts the tenant identifier from every request
// and enforces that all database queries are scoped to that tenant.
type TenancyComponent struct {
	field    string // database column name
	strategy string // "row" (the only supported strategy)
	header   string // HTTP header name
}

// New returns a TenancyComponent with defaults.
func New() *TenancyComponent {
	return &TenancyComponent{
		field:    "org_id",
		strategy: "row",
		header:   "X-Organization-Id",
	}
}

func (c *TenancyComponent) Name() string      { return componentName }
func (c *TenancyComponent) Version() string   { return componentVersion }
func (c *TenancyComponent) AuditHash() string { return auditHash }

// Configure reads the tenancy section.
func (c *TenancyComponent) Configure(cfg spec.ComponentConfig) error {
	if field, ok := cfg["field"].(string); ok && field != "" {
		c.field = field
	}
	if strategy, ok := cfg["strategy"].(string); ok && strategy != "" {
		if strategy != "row" {
			return fmt.Errorf("foundry-tenancy: unsupported strategy %q (only 'row' is supported)", strategy)
		}
		c.strategy = strategy
	}
	if header, ok := cfg["header"].(string); ok && header != "" {
		c.header = header
	}
	return nil
}

// Register installs tenancy middleware on the application.
// The middleware extracts the tenant ID from the configured HTTP header and
// stores it in the request context via spec.WithTenantID. foundry-postgres
// reads this value and appends WHERE {field} = $N to all queries, providing
// transparent row-level multi-tenant isolation with zero application code.
func (c *TenancyComponent) Register(app *spec.Application) error {
	if app == nil {
		return nil
	}
	// Register the tenant field name so foundry-postgres can scope queries.
	app.SetTenantField(c.field)

	header := c.header
	mw := spec.HTTPMiddleware(func(next spec.HTTPHandler) spec.HTTPHandler {
		return &tenancyHandler{next: next, header: header}
	})
	app.AddMiddleware(mw)
	return nil
}

// Start is a no-op for tenancy — isolation is enforced at request time.
func (c *TenancyComponent) Start(ctx context.Context) error { return nil }

// Stop is a no-op for tenancy.
func (c *TenancyComponent) Stop(ctx context.Context) error { return nil }

// TenantField returns the database column name for tenant isolation.
// foundry-postgres reads this to know which column to filter on.
func (c *TenancyComponent) TenantField() string { return c.field }

// --- middleware ---

type tenancyHandler struct {
	next   spec.HTTPHandler
	header string
}

func (h *tenancyHandler) ServeHTTP(w spec.ResponseWriter, r *spec.Request) {
	tenantID := ""
	if vals, ok := r.Headers[h.header]; ok && len(vals) > 0 {
		tenantID = vals[0]
	}
	if tenantID == "" {
		writeTenancyError(w, 400, "missing required header: "+h.header)
		return
	}
	// Attach tenant ID to context; downstream components (foundry-postgres) read it.
	r = &spec.Request{
		Method:  r.Method,
		URL:     r.URL,
		Headers: r.Headers,
		Body:    r.Body,
		Context: spec.WithTenantID(r.Context, tenantID),
	}
	h.next.ServeHTTP(w, r)
}

func writeTenancyError(w spec.ResponseWriter, status int, msg string) {
	data, _ := json.Marshal(map[string]any{"error": msg, "status": status})
	w.Header()["Content-Type"] = []string{"application/json"}
	w.WriteHeader(status)
	w.Write(data) //nolint:errcheck
}
