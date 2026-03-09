package redis

import (
	"testing"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

// --- Component interface tests ---

func TestComponent_ImplementsSpecComponent(t *testing.T) {
	var _ spec.Component = New()
}

func TestComponent_Identity(t *testing.T) {
	c := New()
	if c.Name() != "foundry-redis" {
		t.Errorf("Name() = %q, want foundry-redis", c.Name())
	}
	if c.Version() != "v1.0.0" {
		t.Errorf("Version() = %q, want v1.0.0", c.Version())
	}
	if c.AuditHash() == "" {
		t.Error("AuditHash() must not be empty")
	}
}

func TestComponent_Configure_Defaults(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure with empty config: %v", err)
	}
	if c.cfg.defaultURL != "redis://localhost:6379" {
		t.Errorf("default URL = %q, want redis://localhost:6379", c.cfg.defaultURL)
	}
	if c.cfg.defaultTTL != 300*time.Second {
		t.Errorf("default TTL = %v, want 300s", c.cfg.defaultTTL)
	}
	if c.cfg.keyPrefix != "foundry" {
		t.Errorf("default keyPrefix = %q, want foundry", c.cfg.keyPrefix)
	}
}

func TestComponent_Configure_Custom(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"url":           "redis://myhost:6380",
		"cache_url":     "redis://cache-host:6379",
		"ratelimit_url": "redis://rl-host:6379",
		"lock_url":      "redis://lock-host:6379",
		"default_ttl":   60,
		"key_prefix":    "myapp",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.cacheURL != "redis://cache-host:6379" {
		t.Errorf("cacheURL = %q, want redis://cache-host:6379", c.cfg.cacheURL)
	}
	if c.cfg.ratelimitURL != "redis://rl-host:6379" {
		t.Errorf("ratelimitURL = %q", c.cfg.ratelimitURL)
	}
	if c.cfg.lockURL != "redis://lock-host:6379" {
		t.Errorf("lockURL = %q", c.cfg.lockURL)
	}
	if c.cfg.defaultTTL != 60*time.Second {
		t.Errorf("defaultTTL = %v, want 60s", c.cfg.defaultTTL)
	}
	if c.cfg.keyPrefix != "myapp" {
		t.Errorf("keyPrefix = %q, want myapp", c.cfg.keyPrefix)
	}
}

func TestComponent_Register_NoOp(t *testing.T) {
	c := New()
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register should be a no-op, got: %v", err)
	}
	if len(app.HTTPHandlers()) != 0 {
		t.Error("Register should not mount any HTTP handlers")
	}
}

func TestComponent_CacheKey_Format(t *testing.T) {
	c := New()
	c.cfg.keyPrefix = "myapp"

	got := c.cacheKey("dinosaur:123")
	want := "myapp:cache:dinosaur:123"
	if got != want {
		t.Errorf("cacheKey() = %q, want %q", got, want)
	}
}

func TestErrRateLimited_IsNotNil(t *testing.T) {
	if ErrRateLimited == nil {
		t.Error("ErrRateLimited must not be nil")
	}
}

func TestErrLockNotAcquired_IsNotNil(t *testing.T) {
	if ErrLockNotAcquired == nil {
		t.Error("ErrLockNotAcquired must not be nil")
	}
}

// TestComponent_Start_SkippedWithoutRedis verifies that Start fails gracefully
// when no Redis server is available — it does NOT panic or hang.
func TestComponent_Start_SkippedWithoutRedis(t *testing.T) {
	c := New()
	c.Configure(spec.ComponentConfig{
		"url": "redis://127.0.0.1:19999", // port that is almost certainly not listening
	})

	// Start should return an error, not panic or block.
	ctx := t.Context()
	err := c.Start(ctx)
	if err == nil {
		// If a Redis server happens to be on this port, skip.
		t.Skip("Redis server found on :19999 — skipping failure test")
	}
	// Error message should mention connection.
	t.Logf("expected error: %v", err)
}
