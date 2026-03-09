package nats

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
	if c.Name() != "foundry-nats" {
		t.Errorf("Name() = %q, want foundry-nats", c.Name())
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
	if c.url != defaultURL {
		t.Errorf("default url = %q, want %q", c.url, defaultURL)
	}
}

func TestComponent_Configure_CustomURL(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"broker_url": "nats://nats.internal:4222",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.url != "nats://nats.internal:4222" {
		t.Errorf("url = %q, want nats://nats.internal:4222", c.url)
	}
}

func TestComponent_Register_EmptyURL(t *testing.T) {
	c := New()
	c.url = "" // force empty
	err := c.Register(nil)
	if err == nil {
		t.Error("Register: expected error when url is empty, got nil")
	}
}

func TestComponent_Register_WithURL(t *testing.T) {
	c := New()
	if err := c.Register(nil); err != nil {
		t.Errorf("Register with default URL: unexpected error: %v", err)
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
