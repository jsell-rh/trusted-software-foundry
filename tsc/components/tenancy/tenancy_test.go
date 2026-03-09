package tenancy

import (
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

func TestComponent_ImplementsSpec(t *testing.T) {
	var _ spec.Component = New()
}

func TestComponent_Identity(t *testing.T) {
	c := New()
	if c.Name() != "foundry-tenancy" {
		t.Errorf("Name() = %q, want foundry-tenancy", c.Name())
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
	if c.field != "org_id" {
		t.Errorf("default field = %q, want org_id", c.field)
	}
	if c.strategy != "row" {
		t.Errorf("default strategy = %q, want row", c.strategy)
	}
	if c.header != "X-Organization-Id" {
		t.Errorf("default header = %q, want X-Organization-Id", c.header)
	}
}

func TestComponent_Configure_Custom(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"field":    "tenant_id",
		"strategy": "row",
		"header":   "X-Tenant-Id",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.field != "tenant_id" {
		t.Errorf("field = %q, want tenant_id", c.field)
	}
	if c.header != "X-Tenant-Id" {
		t.Errorf("header = %q, want X-Tenant-Id", c.header)
	}
}

func TestComponent_Configure_UnsupportedStrategy(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{"strategy": "schema"})
	if err == nil {
		t.Error("Configure: expected error for unsupported strategy 'schema', got nil")
	}
}

func TestComponent_Register(t *testing.T) {
	c := New()
	_ = c.Configure(spec.ComponentConfig{})
	if err := c.Register(nil); err != nil {
		t.Errorf("Register: unexpected error: %v", err)
	}
}
