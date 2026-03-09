package metrics

// metrics_test.go provides coverage for the foundry-metrics component:
//   New, Name, Version, AuditHash, Configure, Register, Start, Stop.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// Constructor and accessors
// --------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.cfg.bind != defaultBind {
		t.Errorf("default bind = %q, want %q", c.cfg.bind, defaultBind)
	}
	if c.cfg.path != defaultPath {
		t.Errorf("default path = %q, want %q", c.cfg.path, defaultPath)
	}
}

func TestName(t *testing.T) {
	if got := New().Name(); got != componentName {
		t.Errorf("Name() = %q, want %q", got, componentName)
	}
}

func TestVersion(t *testing.T) {
	if got := New().Version(); got != componentVersion {
		t.Errorf("Version() = %q, want %q", got, componentVersion)
	}
}

func TestAuditHash(t *testing.T) {
	if got := New().AuditHash(); got != auditHash {
		t.Errorf("AuditHash() = %q, want %q", got, auditHash)
	}
}

// --------------------------------------------------------------------------
// Configure
// --------------------------------------------------------------------------

func TestConfigure_Empty(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure({}): %v", err)
	}
	if c.cfg.bind != defaultBind {
		t.Errorf("bind = %q, want default %q", c.cfg.bind, defaultBind)
	}
}

func TestConfigure_SetBind(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"bind": ":9191"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.bind != ":9191" {
		t.Errorf("bind = %q, want :9191", c.cfg.bind)
	}
}

func TestConfigure_SetPath(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"path": "/prom"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.path != "/prom" {
		t.Errorf("path = %q, want /prom", c.cfg.path)
	}
}

func TestConfigure_NonStringIgnored(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"bind": 8080, "path": false}); err != nil {
		t.Fatalf("Configure with wrong types: %v", err)
	}
	if c.cfg.bind != defaultBind {
		t.Errorf("bind = %q, want default", c.cfg.bind)
	}
}

func TestConfigure_EmptyStringIgnored(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"bind": "", "path": ""}); err != nil {
		t.Fatalf("Configure with empty strings: %v", err)
	}
	if c.cfg.bind != defaultBind {
		t.Errorf("bind = %q, want default", c.cfg.bind)
	}
}

// --------------------------------------------------------------------------
// Register (no-op)
// --------------------------------------------------------------------------

func TestRegister_NoOp(t *testing.T) {
	c := New()
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Errorf("Register: %v", err)
	}
}

// --------------------------------------------------------------------------
// Start / Stop — lifecycle
// --------------------------------------------------------------------------

func randomPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestStart_HappyPath(t *testing.T) {
	c := New()
	port := randomPort(t)
	c.cfg.bind = fmt.Sprintf(":%d", port)

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = c.Stop(context.Background()) }()

	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, defaultPath))
	if err != nil {
		t.Fatalf("GET %s: %v", defaultPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain (Prometheus format)", ct)
	}
}

func TestStop_NilServer(t *testing.T) {
	c := New()
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop(nil server): %v", err)
	}
}

func TestStop_LiveServer(t *testing.T) {
	c := New()
	port := randomPort(t)
	c.cfg.bind = fmt.Sprintf(":%d", port)

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}
