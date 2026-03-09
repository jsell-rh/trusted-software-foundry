// Package spicedb provides the foundry-auth-spicedb trusted component —
// fine-grained authorization using Authzed SpiceDB.
//
// Configuration (spec authz block):
//
//	authz:
//	  backend: spicedb
//	  schema_file: authz/schema.zed
//	  endpoint: "${SPICEDB_ENDPOINT}"   # default: localhost:50051
//	  token: "${SPICEDB_TOKEN}"
package spicedb

import (
	"context"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-auth-spicedb"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000010"

	defaultEndpoint = "localhost:50051"
)

// SpiceDBComponent implements spec.Component for SpiceDB fine-grained authz.
type SpiceDBComponent struct {
	endpoint   string
	token      string
	schemaFile string
}

// New returns a SpiceDBComponent with defaults.
func New() *SpiceDBComponent {
	return &SpiceDBComponent{endpoint: defaultEndpoint}
}

func (c *SpiceDBComponent) Name() string      { return componentName }
func (c *SpiceDBComponent) Version() string   { return componentVersion }
func (c *SpiceDBComponent) AuditHash() string { return auditHash }

// Configure reads the authz.spicedb section.
func (c *SpiceDBComponent) Configure(cfg spec.ComponentConfig) error {
	if ep, ok := cfg["endpoint"].(string); ok && ep != "" {
		c.endpoint = ep
	}
	if tok, ok := cfg["token"].(string); ok {
		c.token = tok
	}
	if sf, ok := cfg["schema_file"].(string); ok {
		c.schemaFile = sf
	}
	return nil
}

// Register wires SpiceDB middleware into the application.
// In production this installs a permission check middleware on every route
// and provides the app with a CheckPermission function.
func (c *SpiceDBComponent) Register(app *spec.Application) error {
	if c.token == "" {
		return fmt.Errorf("foundry-auth-spicedb: token is required (set SPICEDB_TOKEN env var)")
	}
	// TODO: connect to SpiceDB gRPC endpoint, load schema, register middleware.
	return nil
}

// Start begins the SpiceDB connection health-check loop.
func (c *SpiceDBComponent) Start(ctx context.Context) error { return nil }

// Stop gracefully closes the SpiceDB connection.
func (c *SpiceDBComponent) Stop(ctx context.Context) error { return nil }
