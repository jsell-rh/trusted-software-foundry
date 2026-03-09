package health

// health_test.go provides coverage for the foundry-health component:
//   New, Name, Version, AuditHash, Configure, Register, Start, Stop, healthHandler.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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
	// Defaults unchanged when config is empty.
	if c.cfg.bind != defaultBind {
		t.Errorf("bind = %q, want default %q", c.cfg.bind, defaultBind)
	}
}

func TestConfigure_SetBind(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"bind": ":9999"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.bind != ":9999" {
		t.Errorf("bind = %q, want :9999", c.cfg.bind)
	}
}

func TestConfigure_SetPath(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"path": "/health"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.path != "/health" {
		t.Errorf("path = %q, want /health", c.cfg.path)
	}
}

func TestConfigure_NonStringIgnored(t *testing.T) {
	// Non-string values for bind/path should be silently ignored (no panic).
	c := New()
	if err := c.Configure(spec.ComponentConfig{"bind": 8083, "path": true}); err != nil {
		t.Fatalf("Configure with wrong types: %v", err)
	}
	// Defaults unchanged.
	if c.cfg.bind != defaultBind {
		t.Errorf("bind = %q, want default", c.cfg.bind)
	}
}

func TestConfigure_EmptyStringIgnored(t *testing.T) {
	// Empty string values should be ignored (condition: ok && bind != "").
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
// Start / Stop — lifecycle with a real HTTP server
// --------------------------------------------------------------------------

// randomPort returns a free TCP port for test use.
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
	defer func() {
		_ = c.Stop(context.Background())
	}()

	// Give the server a moment to be ready.
	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, defaultPath))
	if err != nil {
		t.Fatalf("GET %s: %v", defaultPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestStart_ReadyzAlias(t *testing.T) {
	c := New()
	port := randomPort(t)
	c.cfg.bind = fmt.Sprintf(":%d", port)

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		_ = c.Stop(context.Background())
	}()

	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/readyz", port))
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/readyz status = %d, want 200", resp.StatusCode)
	}
}

func TestStop_NilServer(t *testing.T) {
	c := New()
	// Stop without Start should return nil (server is nil).
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

// --------------------------------------------------------------------------
// healthHandler — tested via httptest
// --------------------------------------------------------------------------

func TestHealthHandler_ResponseJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}
