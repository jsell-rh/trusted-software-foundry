package spicedb

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
	if c.Name() != "foundry-auth-spicedb" {
		t.Errorf("Name() = %q, want foundry-auth-spicedb", c.Name())
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
	if c.endpoint != defaultEndpoint {
		t.Errorf("default endpoint = %q, want %q", c.endpoint, defaultEndpoint)
	}
}

func TestComponent_Configure_CustomEndpoint(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"endpoint": "spicedb.internal:50051",
		"token":    "secret-token",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.endpoint != "spicedb.internal:50051" {
		t.Errorf("endpoint = %q, want spicedb.internal:50051", c.endpoint)
	}
	if c.token != "secret-token" {
		t.Errorf("token = %q, want secret-token", c.token)
	}
}

func TestComponent_Register_RequiresToken(t *testing.T) {
	c := New()
	_ = c.Configure(spec.ComponentConfig{})
	err := c.Register(nil)
	if err == nil {
		t.Error("Register: expected error when token is empty, got nil")
	}
}

func TestComponent_Register_WithToken(t *testing.T) {
	c := New()
	_ = c.Configure(spec.ComponentConfig{"token": "my-token"})
	// Register with nil app — stub implementation should not dereference app.
	if err := c.Register(nil); err != nil {
		t.Errorf("Register with token: unexpected error: %v", err)
	}
}

func TestComponent_Configure_SchemaFile(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"schema_file": "authz/schema.zed"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.schemaFile != "authz/schema.zed" {
		t.Errorf("schemaFile = %q, want authz/schema.zed", c.schemaFile)
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
