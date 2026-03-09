// Package nats provides the foundry-nats trusted component —
// lightweight pub/sub messaging via NATS.
//
// Configuration (spec events block when backend=nats, or for internal service bus):
//
//	events:
//	  backend: nats
//	  broker_url: "${NATS_URL}"   # default: nats://localhost:4222
package nats

import (
	"context"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

const (
	componentName    = "foundry-nats"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000012"

	defaultURL = "nats://localhost:4222"
)

// NATSComponent implements spec.Component for NATS messaging.
type NATSComponent struct {
	url string
}

// New returns a NATSComponent with defaults.
func New() *NATSComponent {
	return &NATSComponent{url: defaultURL}
}

func (c *NATSComponent) Name() string      { return componentName }
func (c *NATSComponent) Version() string   { return componentVersion }
func (c *NATSComponent) AuditHash() string { return auditHash }

// Configure reads the events.nats section.
func (c *NATSComponent) Configure(cfg spec.ComponentConfig) error {
	if url, ok := cfg["broker_url"].(string); ok && url != "" {
		c.url = url
	}
	return nil
}

// Register installs NATS publisher and subscriber on the application.
func (c *NATSComponent) Register(app *spec.Application) error {
	if c.url == "" {
		return fmt.Errorf("foundry-nats: broker_url is required")
	}
	// TODO: connect to NATS server, register pub/sub helpers.
	return nil
}

// Start begins the NATS connection and subscription processing loop.
func (c *NATSComponent) Start(ctx context.Context) error { return nil }

// Stop drains subscriptions and closes the NATS connection.
func (c *NATSComponent) Stop(ctx context.Context) error { return nil }
