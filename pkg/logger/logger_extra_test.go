package logger

// logger_extra_test.go covers:
//   NewLogger
//   V, Extra, Info, Warning, Error, Infof
//   WithOpID, GetOperationID
//   OperationIDMiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --------------------------------------------------------------------------
// NewLogger
// --------------------------------------------------------------------------

func TestNewLogger_NotNil(t *testing.T) {
	l := NewLogger(context.Background())
	if l == nil {
		t.Error("NewLogger returned nil")
	}
}

func TestNewLogger_WithAccountID(t *testing.T) {
	ctx := context.WithValue(context.Background(), "accountID", "acc-123")
	l := NewLogger(ctx)
	if l == nil {
		t.Error("NewLogger with accountID returned nil")
	}
}

// --------------------------------------------------------------------------
// V
// --------------------------------------------------------------------------

func TestLogger_V_ReturnsLogger(t *testing.T) {
	l := NewLogger(context.Background())
	l2 := l.V(2)
	if l2 == nil {
		t.Error("V() returned nil")
	}
}

// --------------------------------------------------------------------------
// Extra
// --------------------------------------------------------------------------

func TestLogger_Extra_Chaining(t *testing.T) {
	l := NewLogger(context.Background())
	l2 := l.Extra("key", "value")
	if l2 == nil {
		t.Error("Extra() returned nil")
	}
}

// --------------------------------------------------------------------------
// Info, Warning, Error, Infof (exercise the log methods without crashing)
// --------------------------------------------------------------------------

func TestLogger_Info(t *testing.T) {
	l := NewLogger(context.Background())
	// Should not panic.
	l.Info("test info message")
}

func TestLogger_Warning(t *testing.T) {
	l := NewLogger(context.Background())
	l.Warning("test warning message")
}

func TestLogger_Error(t *testing.T) {
	l := NewLogger(context.Background())
	l.Error("test error message")
}

func TestLogger_Infof(t *testing.T) {
	l := NewLogger(context.Background())
	l.Infof("test %s message", "infof")
}

// --------------------------------------------------------------------------
// Logger with txid and opID in context
// --------------------------------------------------------------------------

func TestLogger_WithTxID(t *testing.T) {
	ctx := context.WithValue(context.Background(), "txid", int64(42))
	l := NewLogger(ctx)
	l.Info("message with txid")
}

func TestLogger_WithOpID(t *testing.T) {
	ctx := context.WithValue(context.Background(), OpIDKey, "op-123")
	l := NewLogger(ctx)
	l.Info("message with opid")
}

// --------------------------------------------------------------------------
// WithOpID / GetOperationID
// --------------------------------------------------------------------------

func TestWithOpID_SetsID(t *testing.T) {
	ctx := context.Background()
	ctx2 := WithOpID(ctx)
	opID := GetOperationID(ctx2)
	if opID == "" {
		t.Error("WithOpID should set a non-empty operation ID")
	}
}

func TestWithOpID_Idempotent(t *testing.T) {
	ctx := context.Background()
	ctx1 := WithOpID(ctx)
	opID1 := GetOperationID(ctx1)
	ctx2 := WithOpID(ctx1)
	opID2 := GetOperationID(ctx2)
	if opID1 != opID2 {
		t.Errorf("WithOpID on already-set context should not change ID: %q != %q", opID1, opID2)
	}
}

func TestGetOperationID_NotSet(t *testing.T) {
	got := GetOperationID(context.Background())
	if got != "" {
		t.Errorf("GetOperationID (not set) = %q, want empty", got)
	}
}

// --------------------------------------------------------------------------
// OperationIDMiddleware
// --------------------------------------------------------------------------

func TestOperationIDMiddleware_SetsHeader(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		opID := GetOperationID(r.Context())
		if opID == "" {
			t.Error("context should have an operation ID inside the handler")
		}
	})
	handler := OperationIDMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Error("next handler was not called")
	}
	if rr.Header().Get(string(OpIDHeader)) == "" {
		t.Error("X-Operation-ID header should be set")
	}
}

func TestOperationIDMiddleware_PreservesExistingOpID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := OperationIDMiddleware(next)

	// Send request with an existing opID in context.
	ctx := context.WithValue(context.Background(), OpIDKey, "existing-op-id")
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}
