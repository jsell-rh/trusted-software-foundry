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
// AuditLogger
// --------------------------------------------------------------------------

func TestAuditLogger_NoActor(t *testing.T) {
	hctx := newHookCtx(map[string]any{})
	op := &foundry.DBOperation{Type: "create", Resource: "Cluster", ResourceID: ""}
	if err := AuditLogger(context.Background(), hctx, op); err != nil {
		t.Errorf("AuditLogger: %v", err)
	}
}

func TestAuditLogger_WithClaims(t *testing.T) {
	hctx := newHookCtx(map[string]any{
		"sub":    "user@example.com",
		"org_id": "org-abc",
	})
	op := &foundry.DBOperation{Type: "delete", Resource: "Cluster", ResourceID: "clus-1"}
	if err := AuditLogger(context.Background(), hctx, op); err != nil {
		t.Errorf("AuditLogger: %v", err)
	}
}

// --------------------------------------------------------------------------
// ClusterStatusEnricher
// --------------------------------------------------------------------------

func TestClusterStatusEnricher_NonOK(t *testing.T) {
	hctx := newHookCtx(nil)
	req := &foundry.PostHandlerRequest{StatusCode: http.StatusNotFound}
	if err := ClusterStatusEnricher(context.Background(), hctx, req); err != nil {
		t.Errorf("ClusterStatusEnricher non-200: %v", err)
	}
}

func TestClusterStatusEnricher_OK(t *testing.T) {
	hctx := newHookCtx(nil)
	req := &foundry.PostHandlerRequest{StatusCode: http.StatusOK}
	if err := ClusterStatusEnricher(context.Background(), hctx, req); err != nil {
		t.Errorf("ClusterStatusEnricher 200: %v", err)
	}
}

// --------------------------------------------------------------------------
// EventSchemaValidator
// --------------------------------------------------------------------------

func TestEventSchemaValidator_MissingTopic(t *testing.T) {
	hctx := newHookCtx(nil)
	msg := &foundry.EventMessage{Topic: "", Key: "k", Headers: map[string]string{"event_type": "x"}}
	if err := EventSchemaValidator(context.Background(), hctx, msg); err == nil {
		t.Error("expected error for missing topic, got nil")
	}
}

func TestEventSchemaValidator_MissingKey(t *testing.T) {
	hctx := newHookCtx(nil)
	msg := &foundry.EventMessage{Topic: "t", Key: "", Headers: map[string]string{"event_type": "x"}}
	if err := EventSchemaValidator(context.Background(), hctx, msg); err == nil {
		t.Error("expected error for missing key, got nil")
	}
}

func TestEventSchemaValidator_MissingEventType(t *testing.T) {
	hctx := newHookCtx(nil)
	msg := &foundry.EventMessage{Topic: "t", Key: "k", Headers: map[string]string{}}
	if err := EventSchemaValidator(context.Background(), hctx, msg); err == nil {
		t.Error("expected error for missing event_type header, got nil")
	}
}

func TestEventSchemaValidator_Valid(t *testing.T) {
	hctx := newHookCtx(nil)
	msg := &foundry.EventMessage{
		Topic:   "fleet.events",
		Key:     "cluster-1",
		Headers: map[string]string{"event_type": "created"},
	}
	if err := EventSchemaValidator(context.Background(), hctx, msg); err != nil {
		t.Errorf("EventSchemaValidator valid: %v", err)
	}
}

// --------------------------------------------------------------------------
// GraphSyncConsumer
// --------------------------------------------------------------------------

func TestGraphSyncConsumer_WrongTopic(t *testing.T) {
	hctx := newHookCtx(nil)
	event := &foundry.ConsumedEvent{Topic: "other.topic", Headers: map[string]string{}, Payload: map[string]any{}}
	if err := GraphSyncConsumer(context.Background(), hctx, event); err != nil {
		t.Errorf("GraphSyncConsumer wrong topic: %v", err)
	}
}

func TestGraphSyncConsumer_MissingClusterID(t *testing.T) {
	hctx := newHookCtx(nil)
	event := &foundry.ConsumedEvent{
		Topic:   "fleet.cluster.lifecycle",
		Headers: map[string]string{"event_type": "created"},
		Payload: map[string]any{},
	}
	if err := GraphSyncConsumer(context.Background(), hctx, event); err == nil {
		t.Error("expected error for missing cluster id, got nil")
	}
}

func TestGraphSyncConsumer_Created(t *testing.T) {
	hctx := newHookCtx(nil)
	event := &foundry.ConsumedEvent{
		Topic:   "fleet.cluster.lifecycle",
		Headers: map[string]string{"event_type": "created"},
		Payload: map[string]any{"id": "clus-1"},
	}
	if err := GraphSyncConsumer(context.Background(), hctx, event); err != nil {
		t.Errorf("GraphSyncConsumer created: %v", err)
	}
}

func TestGraphSyncConsumer_Deleted(t *testing.T) {
	hctx := newHookCtx(nil)
	event := &foundry.ConsumedEvent{
		Topic:   "fleet.cluster.lifecycle",
		Headers: map[string]string{"event_type": "deleted"},
		Payload: map[string]any{"id": "clus-2"},
	}
	if err := GraphSyncConsumer(context.Background(), hctx, event); err != nil {
		t.Errorf("GraphSyncConsumer deleted: %v", err)
	}
}

func TestGraphSyncConsumer_DefaultEvent(t *testing.T) {
	hctx := newHookCtx(nil)
	event := &foundry.ConsumedEvent{
		Topic:   "fleet.cluster.lifecycle",
		Headers: map[string]string{"event_type": "updated"},
		Payload: map[string]any{"id": "clus-3"},
	}
	if err := GraphSyncConsumer(context.Background(), hctx, event); err != nil {
		t.Errorf("GraphSyncConsumer default: %v", err)
	}
}

// --------------------------------------------------------------------------
// TenantIsolationCheck
// --------------------------------------------------------------------------

func TestTenantIsolationCheck_MissingOrgID(t *testing.T) {
	hctx := newHookCtx(map[string]any{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	if err := TenantIsolationCheck(context.Background(), hctx, w, r); err == nil {
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
	if err := TenantIsolationCheck(context.Background(), hctx, w, r); err == nil {
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
	if err := TenantIsolationCheck(context.Background(), hctx, w, r); err != nil {
		t.Errorf("TenantIsolationCheck no header: %v", err)
	}
}

func TestTenantIsolationCheck_Success_MatchingHeader(t *testing.T) {
	hctx := newHookCtx(map[string]any{"org_id": "org-abc"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	r.Header.Set("X-Organization-Id", "org-abc")
	if err := TenantIsolationCheck(context.Background(), hctx, w, r); err != nil {
		t.Errorf("TenantIsolationCheck matching header: %v", err)
	}
}
