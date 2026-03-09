package logging

// logging_extra_test.go covers:
//   NewJSONLogFormatter
//   JSONLogFormatter.FormatRequestLog
//   JSONLogFormatter.FormatResponseLog
//   NewLoggingWriter
//   LoggingWriter.Write, WriteHeader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// NewJSONLogFormatter
// --------------------------------------------------------------------------

func TestNewJSONLogFormatter_NotNil(t *testing.T) {
	f := NewJSONLogFormatter()
	if f == nil {
		t.Error("NewJSONLogFormatter() returned nil")
	}
}

func TestNewJSONLogFormatter_ImplementsInterface(t *testing.T) {
	var _ LogFormatter = NewJSONLogFormatter()
}

// --------------------------------------------------------------------------
// JSONLogFormatter.FormatRequestLog
// --------------------------------------------------------------------------

func TestFormatRequestLog_ValidJSON(t *testing.T) {
	f := NewJSONLogFormatter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	log, err := f.FormatRequestLog(req)
	if err != nil {
		t.Fatalf("FormatRequestLog: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(log), &out); err != nil {
		t.Errorf("FormatRequestLog output is not valid JSON: %v", err)
	}
}

func TestFormatRequestLog_ContainsMethod(t *testing.T) {
	f := NewJSONLogFormatter()
	req := httptest.NewRequest(http.MethodPost, "/items", nil)
	log, err := f.FormatRequestLog(req)
	if err != nil {
		t.Fatalf("FormatRequestLog: %v", err)
	}
	if !strings.Contains(log, "POST") {
		t.Errorf("log = %q, expected to contain POST", log)
	}
}

// --------------------------------------------------------------------------
// JSONLogFormatter.FormatResponseLog
// --------------------------------------------------------------------------

func TestFormatResponseLog_ValidJSON(t *testing.T) {
	f := NewJSONLogFormatter()
	info := &ResponseInfo{
		Status:  http.StatusOK,
		Elapsed: "10ms",
	}
	log, err := f.FormatResponseLog(info)
	if err != nil {
		t.Fatalf("FormatResponseLog: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(log), &out); err != nil {
		t.Errorf("FormatResponseLog output is not valid JSON: %v", err)
	}
}

func TestFormatResponseLog_ContainsStatus(t *testing.T) {
	f := NewJSONLogFormatter()
	info := &ResponseInfo{
		Status:  http.StatusNotFound,
		Elapsed: "5ms",
	}
	log, err := f.FormatResponseLog(info)
	if err != nil {
		t.Fatalf("FormatResponseLog: %v", err)
	}
	if !strings.Contains(log, "404") {
		t.Errorf("log = %q, expected to contain status 404", log)
	}
}

// --------------------------------------------------------------------------
// NewLoggingWriter
// --------------------------------------------------------------------------

func TestNewLoggingWriter_NotNil(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	f := NewJSONLogFormatter()
	w := NewLoggingWriter(rr, req, f)
	if w == nil {
		t.Error("NewLoggingWriter returned nil")
	}
}

// --------------------------------------------------------------------------
// LoggingWriter.Write / WriteHeader
// --------------------------------------------------------------------------

func TestLoggingWriter_Write(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	f := NewJSONLogFormatter()
	w := NewLoggingWriter(rr, req, f)

	body := []byte(`{"key":"value"}`)
	n, err := w.Write(body)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(body) {
		t.Errorf("Write n = %d, want %d", n, len(body))
	}
	if rr.Body.String() != string(body) {
		t.Errorf("body = %q, want %q", rr.Body.String(), string(body))
	}
}

func TestLoggingWriter_WriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	f := NewJSONLogFormatter()
	w := NewLoggingWriter(rr, req, f)

	w.WriteHeader(http.StatusCreated)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
}
