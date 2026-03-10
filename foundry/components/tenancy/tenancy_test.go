package tenancy

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
	// Register with nil app is a no-op.
	if err := c.Register(nil); err != nil {
		t.Errorf("Register(nil): unexpected error: %v", err)
	}
	// Register with a real app installs middleware and sets tenant field.
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Errorf("Register(app): unexpected error: %v", err)
	}
	if app.TenantField() != "org_id" {
		t.Errorf("TenantField() = %q, want org_id", app.TenantField())
	}
	if len(app.Middlewares()) != 1 {
		t.Errorf("Middlewares count = %d, want 1", len(app.Middlewares()))
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

func TestMiddleware_ExtractsTenantFromHeader(t *testing.T) {
	c := New()
	app := spec.NewApplication(nil)
	_ = c.Register(app)

	called := false
	var capturedTenantID string
	mws := app.Middlewares()
	if len(mws) == 0 {
		t.Fatal("no middleware registered")
	}
	mw := mws[0]
	handler := mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		called = true
		capturedTenantID, _ = spec.TenantIDFromContext(r.Context)
	}))

	w := &nopResponseWriter{}
	r := &spec.Request{
		Method:  "GET",
		URL:     "/things",
		Headers: map[string][]string{"X-Organization-Id": {"org-abc"}},
		Context: context.Background(),
	}
	handler.ServeHTTP(w, r)

	if !called {
		t.Error("next handler not called")
	}
	if capturedTenantID != "org-abc" {
		t.Errorf("tenantID = %q, want org-abc", capturedTenantID)
	}
}

func TestMiddleware_RejectsMissingHeader(t *testing.T) {
	c := New()
	app := spec.NewApplication(nil)
	_ = c.Register(app)

	called := false
	mws := app.Middlewares()
	mw := mws[0]
	handler := mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		called = true
	}))

	w := &nopResponseWriter{}
	r := &spec.Request{
		Method:  "GET",
		URL:     "/things",
		Headers: map[string][]string{},
		Context: context.Background(),
	}
	handler.ServeHTTP(w, r)

	if called {
		t.Error("next handler should NOT be called when tenant header is missing")
	}
	if w.code != 400 {
		t.Errorf("status = %d, want 400", w.code)
	}
}

// handlerFunc adapts a func to spec.HTTPHandler.
type handlerFunc func(w spec.ResponseWriter, r *spec.Request)

func (f handlerFunc) ServeHTTP(w spec.ResponseWriter, r *spec.Request) { f(w, r) }

// nopResponseWriter captures the status code for assertions.
type nopResponseWriter struct {
	code    int
	headers map[string][]string
	written []byte
}

func (n *nopResponseWriter) Header() map[string][]string {
	if n.headers == nil {
		n.headers = make(map[string][]string)
	}
	return n.headers
}
func (n *nopResponseWriter) Write(b []byte) (int, error) {
	n.written = append(n.written, b...)
	return len(b), nil
}
func (n *nopResponseWriter) WriteHeader(code int) { n.code = code }

func TestTenantIDFromContext_NoTenant(t *testing.T) {
	id, ok := spec.TenantIDFromContext(context.Background())
	if ok {
		t.Errorf("expected ok=false for context without tenant, got %q", id)
	}
}

func TestTenantIDFromContext_WithTenant(t *testing.T) {
	ctx := spec.WithTenantID(context.Background(), "tenant-42")
	id, ok := spec.TenantIDFromContext(ctx)
	if !ok {
		t.Error("expected ok=true for context with tenant")
	}
	if id != "tenant-42" {
		t.Errorf("id = %q, want tenant-42", id)
	}
}

func TestTenantIDFromContext_NilContext(t *testing.T) {
	id, ok := spec.TenantIDFromContext(nil)
	if ok || id != "" {
		t.Errorf("expected ('', false) for nil context, got (%q, %v)", id, ok)
	}
}
