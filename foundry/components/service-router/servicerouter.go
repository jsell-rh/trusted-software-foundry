// Package servicerouter provides the foundry-service-router trusted component —
// intelligent request routing between services in a multi-service application.
//
// In a multi-service deployment the service-router component registers a
// reverse-proxy middleware that routes API requests to the appropriate
// downstream service binary based on route prefix matching.
//
// Configuration (inferred from spec services block — no explicit config block):
//
//	services:
//	  - name: api-server
//	    role: gateway      # the gateway service receives all inbound traffic
//	    port: 8000
//	  - name: worker
//	    role: worker
//	    port: 8001
package servicerouter

import (
	"context"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-service-router"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000016"
)

// ServiceRouterComponent implements spec.Component for inter-service routing.
type ServiceRouterComponent struct {
	routes map[string]string // path prefix → downstream URL
}

// New returns a ServiceRouterComponent with no configured routes.
func New() *ServiceRouterComponent {
	return &ServiceRouterComponent{
		routes: make(map[string]string),
	}
}

func (c *ServiceRouterComponent) Name() string      { return componentName }
func (c *ServiceRouterComponent) Version() string   { return componentVersion }
func (c *ServiceRouterComponent) AuditHash() string { return auditHash }

// Configure reads service routing rules from the IR spec.
// Routes are inferred from the services block rather than explicit config.
func (c *ServiceRouterComponent) Configure(cfg spec.ComponentConfig) error {
	// Routes are populated by the compiler from the services block.
	// If explicit routes are provided, apply them.
	if routes, ok := cfg["routes"].(map[string]string); ok {
		for prefix, target := range routes {
			c.routes[prefix] = target
		}
	}
	return nil
}

// Register installs the service router middleware.
// In a gateway service, this proxies sub-routes to worker services.
// In a worker service, this component is a no-op.
func (c *ServiceRouterComponent) Register(app *spec.Application) error {
	// TODO: install reverse proxy middleware for each route prefix.
	// Routes point to internal service addresses derived from the services block.
	return nil
}

// Start is a no-op for the service router.
func (c *ServiceRouterComponent) Start(ctx context.Context) error { return nil }

// Stop is a no-op for the service router.
func (c *ServiceRouterComponent) Stop(ctx context.Context) error { return nil }
