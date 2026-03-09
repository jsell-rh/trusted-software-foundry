package redisstreams

import (
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

func TestComponent_ImplementsSpec(t *testing.T) {
	var _ spec.Component = New()
}

func TestComponent_Identity(t *testing.T) {
	c := New()
	if c.Name() != "foundry-redis-streams" {
		t.Errorf("Name() = %q, want foundry-redis-streams", c.Name())
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
	if c.groupName != "foundry-consumers" {
		t.Errorf("default groupName = %q, want foundry-consumers", c.groupName)
	}
}

func TestComponent_Configure_Custom(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"broker_url":     "redis://redis.internal:6379",
		"consumer_group": "my-group",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.url != "redis://redis.internal:6379" {
		t.Errorf("url = %q, want redis://redis.internal:6379", c.url)
	}
	if c.groupName != "my-group" {
		t.Errorf("groupName = %q, want my-group", c.groupName)
	}
}

func TestComponent_Register_EmptyURL(t *testing.T) {
	c := New()
	c.url = "" // force empty
	if err := c.Register(nil); err == nil {
		t.Error("Register: expected error when url is empty, got nil")
	}
}

func TestComponent_Register_WithURL(t *testing.T) {
	c := New()
	if err := c.Register(nil); err != nil {
		t.Errorf("Register with default URL: unexpected error: %v", err)
	}
}
