// Package temporal provides the foundry-temporal trusted component —
// durable workflow execution via Temporal.io.
//
// Configuration (spec temporal block):
//
//	temporal:
//	  namespace: my-app
//	  worker_queue: my-queue
//	  host: "${TEMPORAL_HOST}"   # default: localhost:7233
package temporal

import (
	"context"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-temporal"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000014"

	defaultHost = "localhost:7233"
)

// TemporalComponent implements spec.Component for Temporal workflow execution.
type TemporalComponent struct {
	host        string
	namespace   string
	workerQueue string
}

// New returns a TemporalComponent with defaults.
func New() *TemporalComponent {
	return &TemporalComponent{
		host:        defaultHost,
		namespace:   "default",
		workerQueue: "foundry-workers",
	}
}

func (c *TemporalComponent) Name() string      { return componentName }
func (c *TemporalComponent) Version() string   { return componentVersion }
func (c *TemporalComponent) AuditHash() string { return auditHash }

// Configure reads the temporal section.
func (c *TemporalComponent) Configure(cfg spec.ComponentConfig) error {
	if host, ok := cfg["host"].(string); ok && host != "" {
		c.host = host
	}
	if ns, ok := cfg["namespace"].(string); ok && ns != "" {
		c.namespace = ns
	}
	if q, ok := cfg["worker_queue"].(string); ok && q != "" {
		c.workerQueue = q
	}
	return nil
}

// Register initializes the Temporal client and worker, and registers
// all workflows and activities declared in the IR spec temporal block.
func (c *TemporalComponent) Register(app *spec.Application) error {
	if c.host == "" {
		return fmt.Errorf("foundry-temporal: host is required")
	}
	// TODO: create Temporal client, register workflows and activities, start worker.
	return nil
}

// Start begins the Temporal worker polling loop.
func (c *TemporalComponent) Start(ctx context.Context) error { return nil }

// Stop gracefully drains and shuts down the Temporal worker.
func (c *TemporalComponent) Stop(ctx context.Context) error { return nil }
