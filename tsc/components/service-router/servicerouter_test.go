package servicerouter

import (
	"context"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

func TestComponent_ImplementsSpec(t *testing.T) {
	var _ spec.Component = New()
}

func TestComponent_Identity(t *testing.T) {
	c := New()
	if c.Name() != "foundry-service-router" {
		t.Errorf("Name() = %q, want foundry-service-router", c.Name())
	}
	if c.Version() != "v1.0.0" {
		t.Errorf("Version() = %q, want v1.0.0", c.Version())
	}
	if len(c.AuditHash()) != 64 {
		t.Errorf("AuditHash() length = %d, want 64 hex chars", len(c.AuditHash()))
	}
}

func TestComponent_Configure_Empty(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure with empty config: %v", err)
	}
	if len(c.routes) != 0 {
		t.Errorf("routes should be empty with no config, got %d", len(c.routes))
	}
}

func TestComponent_Configure_Routes(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"routes": map[string]string{
			"/api/workers": "http://worker:8001",
		},
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(c.routes) != 1 {
		t.Errorf("routes = %d, want 1", len(c.routes))
	}
	if c.routes["/api/workers"] != "http://worker:8001" {
		t.Errorf("route /api/workers = %q, want http://worker:8001", c.routes["/api/workers"])
	}
}

func TestComponent_Register(t *testing.T) {
	c := New()
	if err := c.Register(nil); err != nil {
		t.Errorf("Register: unexpected error: %v", err)
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
