package hooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec/foundry"
)

// --------------------------------------------------------------------------
// Test logger stub
// --------------------------------------------------------------------------

type testLogger struct{}

func (testLogger) Info(msg string, keyvals ...string)  {}
func (testLogger) Warn(msg string, keyvals ...string)  {}
func (testLogger) Error(msg string, keyvals ...string) {}
func (testLogger) Debug(msg string, keyvals ...string) {}

func newHookCtx(claims map[string]any) *foundry.HookContext {
	return &foundry.HookContext{
		Ctx:       context.Background(),
		Logger:    testLogger{},
		Claims:    claims,
		RequestID: "req-1",
		TraceID:   "trace-1",
	}
}

// --------------------------------------------------------------------------
// AuditLoggerPreDb
// --------------------------------------------------------------------------

func TestAuditLogger_NoActor(t *testing.T) {
	hctx := newHookCtx(map[string]any{})
	op := &foundry.DBOperation{Type: "create", Resource: "Cluster", ResourceID: ""}
	if err := AuditLoggerPreDb(hctx, op); err != nil {
		t.Errorf("AuditLogger: %v", err)
	}
}

func TestAuditLogger_WithClaims(t *testing.T) {
	hctx := newHookCtx(map[string]any{
		"sub":    "user@example.com",
		"org_id": "org-abc",
	})
	op := &foundry.DBOperation{Type: "delete", Resource: "Cluster", ResourceID: "clus-1"}
	if err := AuditLoggerPreDb(hctx, op); err != nil {
		t.Errorf("AuditLogger: %v", err)
	}
}

// --------------------------------------------------------------------------
// ClusterStatusEnricher
// --------------------------------------------------------------------------

func TestClusterStatusEnricher_NonOK(t *testing.T) {
	hctx := newHookCtx(nil)
	req := &foundry.PostHandlerRequest{StatusCode: http.StatusNotFound}
	if err := ClusterStatusEnricherPostHandler(hctx, req); err != nil {
		t.Errorf("ClusterStatusEnricher non-200: %v", err)
	}
}

func TestClusterStatusEnricher_OK(t *testing.T) {
	hctx := newHookCtx(nil)
	req := &foundry.PostHandlerRequest{StatusCode: http.StatusOK}
	if err := ClusterStatusEnricherPostHandler(hctx, req); err != nil {
		t.Errorf("ClusterStatusEnricher 200: %v", err)
	}
}

// --------------------------------------------------------------------------
// TenantIsolationCheck
// --------------------------------------------------------------------------

func TestTenantIsolationCheck_MissingOrgID(t *testing.T) {
	hctx := newHookCtx(map[string]any{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	if err := TenantIsolationCheckPreHandler(hctx, w, r); err == nil {
		t.Error("expected error for missing org_id claim, got nil")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestTenantIsolationCheck_OrgMismatch(t *testing.T) {
	hctx := newHookCtx(map[string]any{"org_id": "org-abc"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	r.Header.Set("X-Organization-Id", "org-other")
	if err := TenantIsolationCheckPreHandler(hctx, w, r); err == nil {
		t.Error("expected error for org mismatch, got nil")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestTenantIsolationCheck_Success_NoHeader(t *testing.T) {
	hctx := newHookCtx(map[string]any{"org_id": "org-abc"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	if err := TenantIsolationCheckPreHandler(hctx, w, r); err != nil {
		t.Errorf("TenantIsolationCheck no header: %v", err)
	}
}

func TestTenantIsolationCheck_Success_MatchingHeader(t *testing.T) {
	hctx := newHookCtx(map[string]any{"org_id": "org-abc"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	r.Header.Set("X-Organization-Id", "org-abc")
	if err := TenantIsolationCheckPreHandler(hctx, w, r); err != nil {
		t.Errorf("TenantIsolationCheck matching header: %v", err)
	}
}
