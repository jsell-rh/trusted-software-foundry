package temporal

import (
	"context"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

func TestComponent_ImplementsSpec(t *testing.T) {
	var _ spec.Component = New()
}

func TestComponent_Identity(t *testing.T) {
	c := New()
	if c.Name() != "foundry-temporal" {
		t.Errorf("Name() = %q, want foundry-temporal", c.Name())
	}
	if c.Version() != "v1.0.0" {
		t.Errorf("Version() = %q, want v1.0.0", c.Version())
	}
	if len(c.AuditHash()) != 64 {
		t.Errorf("AuditHash() length = %d, want 64 hex chars", len(c.AuditHash()))
	}
}

func TestComponent_Configure_Defaults(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure with empty config: %v", err)
	}
	if c.host != defaultHost {
		t.Errorf("default host = %q, want %q", c.host, defaultHost)
	}
	if c.namespace != "default" {
		t.Errorf("default namespace = %q, want default", c.namespace)
	}
	if c.workerQueue != "foundry-workers" {
		t.Errorf("default workerQueue = %q, want foundry-workers", c.workerQueue)
	}
}

func TestComponent_Configure_Custom(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"host":         "temporal.internal:7233",
		"namespace":    "fleet-manager",
		"worker_queue": "fleet-provisioning",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.host != "temporal.internal:7233" {
		t.Errorf("host = %q, want temporal.internal:7233", c.host)
	}
	if c.namespace != "fleet-manager" {
		t.Errorf("namespace = %q, want fleet-manager", c.namespace)
	}
	if c.workerQueue != "fleet-provisioning" {
		t.Errorf("workerQueue = %q, want fleet-provisioning", c.workerQueue)
	}
}

func TestComponent_Register_EmptyHost(t *testing.T) {
	c := New()
	c.host = "" // force empty
	if err := c.Register(nil); err == nil {
		t.Error("Register: expected error when host is empty, got nil")
	}
}

func TestComponent_Register_WithHost(t *testing.T) {
	c := New()
	if err := c.Register(nil); err != nil {
		t.Errorf("Register with default host: unexpected error: %v", err)
	}
}

func TestComponent_Start(t *testing.T) {
	c := New()
	if err := c.Start(context.Background()); err != nil {
		t.Errorf("Start: %v", err)
	}
}

func TestComponent_Stop(t *testing.T) {
	c := New()
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}
