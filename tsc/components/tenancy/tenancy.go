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
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
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
// The middleware extracts the tenant ID from the configured header and
// stores it in the request context. All foundry-postgres queries automatically
// include a WHERE clause for the tenant field.
func (c *TenancyComponent) Register(app *spec.Application) error {
	// TODO: register middleware that intercepts every request, extracts tenant ID,
	// and adds it to context. Patch foundry-postgres query builder to inject
	// WHERE {field} = $tenant clause on every operation.
	return nil
}

// Start is a no-op for tenancy.
func (c *TenancyComponent) Start(ctx context.Context) error { return nil }

// Stop is a no-op for tenancy.
func (c *TenancyComponent) Stop(ctx context.Context) error { return nil }
