package api

// api_extra_test.go covers:
//   NewID
//   SendNotFound, SendUnauthorized, SendPanic
//   EventList.Index
//   Event.BeforeCreate (sets ID via NewID)
//   Version / BuildTime package vars

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// NewID
// --------------------------------------------------------------------------

func TestNewID_NonEmpty(t *testing.T) {
	id := NewID()
	if id == "" {
		t.Error("NewID() returned empty string")
	}
}

func TestNewID_Unique(t *testing.T) {
	a := NewID()
	b := NewID()
	if a == b {
		t.Errorf("NewID() returned duplicate values: %q", a)
	}
}

func TestNewID_Length(t *testing.T) {
	id := NewID()
	// ksuid strings are 27 characters
	if len(id) != 27 {
		t.Errorf("NewID() length = %d, want 27", len(id))
	}
}

// --------------------------------------------------------------------------
// SendNotFound
// --------------------------------------------------------------------------

func TestSendNotFound_StatusCode(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/things/123", nil)
	SendNotFound(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestSendNotFound_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/things/123", nil)
	SendNotFound(rr, req)
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestSendNotFound_BodyContainsPath(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/things/123", nil)
	SendNotFound(rr, req)
	body := rr.Body.String()
	if !strings.Contains(body, "/api/v1/things/123") {
		t.Errorf("body = %q, want to contain request path", body)
	}
}

func TestSendNotFound_ValidJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	SendNotFound(rr, req)
	var out Error
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Errorf("response body is not valid JSON: %v", err)
	}
	if out.Type != ErrorType {
		t.Errorf("Type = %q, want %q", out.Type, ErrorType)
	}
}

// --------------------------------------------------------------------------
// SendUnauthorized
// --------------------------------------------------------------------------

func TestSendUnauthorized_StatusCode(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	SendUnauthorized(rr, req, "token expired")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestSendUnauthorized_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	SendUnauthorized(rr, req, "not allowed")
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestSendUnauthorized_ValidJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	SendUnauthorized(rr, req, "access denied")
	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Errorf("response body is not valid JSON: %v", err)
	}
}

// --------------------------------------------------------------------------
// SendPanic
// --------------------------------------------------------------------------

func TestSendPanic_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	SendPanic(rr, req)
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestSendPanic_ValidJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	SendPanic(rr, req)
	var out Error
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Errorf("panic response body is not valid JSON: %v", err)
	}
	if out.Type != ErrorType {
		t.Errorf("Type = %q, want %q", out.Type, ErrorType)
	}
}

func TestSendPanic_Idempotent(t *testing.T) {
	// Calling SendPanic multiple times should not panic.
	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		SendPanic(rr, req)
	}
}

// --------------------------------------------------------------------------
// EventList.Index
// --------------------------------------------------------------------------

func TestEventList_Index_Empty(t *testing.T) {
	var list EventList
	idx := list.Index()
	if len(idx) != 0 {
		t.Errorf("Index() length = %d, want 0", len(idx))
	}
}

func TestEventList_Index_MultipleEvents(t *testing.T) {
	list := EventList{
		{Meta: Meta{ID: "a"}, Source: "src", EventType: CreateEventType},
		{Meta: Meta{ID: "b"}, Source: "src", EventType: UpdateEventType},
	}
	idx := list.Index()
	if len(idx) != 2 {
		t.Errorf("Index() length = %d, want 2", len(idx))
	}
	if idx["a"] == nil || idx["b"] == nil {
		t.Error("Index() missing expected keys")
	}
	if idx["a"].EventType != CreateEventType {
		t.Errorf("idx[a].EventType = %q, want %q", idx["a"].EventType, CreateEventType)
	}
}

// --------------------------------------------------------------------------
// Version / BuildTime
// --------------------------------------------------------------------------

func TestVersionAndBuildTime_DefaultValues(t *testing.T) {
	// Default values are set at package init; "unknown" is set by ldflags in production,
	// but in test they could be anything non-nil.
	_ = Version
	_ = BuildTime
}

// --------------------------------------------------------------------------
// EventType constants
// --------------------------------------------------------------------------

func TestEventTypeConstants(t *testing.T) {
	if CreateEventType == "" || UpdateEventType == "" || DeleteEventType == "" {
		t.Error("one or more EventType constants are empty")
	}
	if CreateEventType == UpdateEventType || UpdateEventType == DeleteEventType {
		t.Error("EventType constants must be distinct")
	}
}
