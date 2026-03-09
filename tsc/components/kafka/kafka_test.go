package kafka

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
	if c.Name() != "foundry-kafka" {
		t.Errorf("Name() = %q, want foundry-kafka", c.Name())
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
	if c.brokerURL != defaultBroker {
		t.Errorf("default brokerURL = %q, want %q", c.brokerURL, defaultBroker)
	}
}

func TestComponent_Configure_CustomBroker(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"broker_url": "kafka.internal:9092",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.brokerURL != "kafka.internal:9092" {
		t.Errorf("brokerURL = %q, want kafka.internal:9092", c.brokerURL)
	}
}

func TestComponent_Register_EmptyBroker(t *testing.T) {
	c := New()
	c.brokerURL = "" // force empty
	err := c.Register(nil)
	if err == nil {
		t.Error("Register: expected error when brokerURL is empty, got nil")
	}
}

func TestComponent_Register_WithBroker(t *testing.T) {
	c := New()
	_ = c.Configure(spec.ComponentConfig{"broker_url": "localhost:9092"})
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
