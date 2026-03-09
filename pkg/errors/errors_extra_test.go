package errors

// errors_extra_test.go covers branches missed by errors_test.go:
//   ErrorCodePrefix, ErrorHrefBase, SetErrorCodePrefix, SetErrorHref,
//   New (undefined code path), Error, AsError,
//   Is404, IsConflict, IsForbidden, AsOpenapiError,
//   CodeStr, Href,
//   all constructor helpers: NotFound, GeneralError, Unauthorized,
//   Unauthenticated, Forbidden, NotImplemented, Conflict, Validation,
//   MalformedRequest, BadRequest, FailedToParseSearch, DatabaseAdvisoryLock.

import (
	"fmt"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// Package-level getters and setters
// --------------------------------------------------------------------------

func TestErrorCodePrefix_Default(t *testing.T) {
	got := ErrorCodePrefix()
	if got == "" {
		t.Error("ErrorCodePrefix() returned empty string")
	}
}

func TestErrorHrefBase_Default(t *testing.T) {
	got := ErrorHrefBase()
	if got == "" {
		t.Error("ErrorHrefBase() returned empty string")
	}
}

func TestSetErrorCodePrefix(t *testing.T) {
	original := ErrorCodePrefix()
	t.Cleanup(func() { SetErrorCodePrefix(original) })

	SetErrorCodePrefix("my-service")
	if got := ErrorCodePrefix(); got != "my-service" {
		t.Errorf("ErrorCodePrefix() = %q after SetErrorCodePrefix, want %q", got, "my-service")
	}
}

func TestSetErrorHref(t *testing.T) {
	original := ErrorHrefBase()
	t.Cleanup(func() { SetErrorHref(original) })

	SetErrorHref("/api/v2/errors/")
	if got := ErrorHrefBase(); got != "/api/v2/errors/" {
		t.Errorf("ErrorHrefBase() = %q after SetErrorHref, want %q", got, "/api/v2/errors/")
	}
}

// --------------------------------------------------------------------------
// New — undefined code falls back to general error
// --------------------------------------------------------------------------

func TestNew_UndefinedCode_FallsBackToGeneral(t *testing.T) {
	// A code that doesn't exist in Errors() list.
	const bogusCode ServiceErrorCode = 999999
	err := New(bogusCode, "")
	if err == nil {
		t.Fatal("New() returned nil for undefined code")
	}
	if err.Code != ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral (%d)", err.Code, ErrorGeneral)
	}
}

func TestNew_EmptyReason_UsesDefault(t *testing.T) {
	err := New(ErrorNotFound, "")
	if err == nil {
		t.Fatal("New() returned nil")
	}
	// Default reason from Errors() table should be used.
	if err.Reason == "" {
		t.Error("Reason should not be empty when empty string passed to New")
	}
}

func TestNew_WithFormatReason(t *testing.T) {
	err := New(ErrorNotFound, "resource %s not found", "widget")
	if err == nil {
		t.Fatal("New() returned nil")
	}
	if !strings.Contains(err.Reason, "widget") {
		t.Errorf("Reason = %q, want to contain 'widget'", err.Reason)
	}
}

// --------------------------------------------------------------------------
// Error and AsError
// --------------------------------------------------------------------------

func TestServiceError_Error(t *testing.T) {
	err := NotFound("the thing is gone")
	s := err.Error()
	if s == "" {
		t.Error("Error() returned empty string")
	}
	// Should contain the reason.
	if !strings.Contains(s, "the thing is gone") {
		t.Errorf("Error() = %q, want to contain reason", s)
	}
}

func TestServiceError_AsError(t *testing.T) {
	svcErr := GeneralError("something went wrong")
	err := svcErr.AsError()
	if err == nil {
		t.Fatal("AsError() returned nil")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("AsError() = %q, want to contain reason", err.Error())
	}
}

// --------------------------------------------------------------------------
// Is404 / IsConflict / IsForbidden
// --------------------------------------------------------------------------

func TestIs404_True(t *testing.T) {
	err := NotFound("gone")
	if !err.Is404() {
		t.Error("Is404() = false for NotFound error")
	}
}

func TestIs404_False(t *testing.T) {
	err := GeneralError("oops")
	if err.Is404() {
		t.Error("Is404() = true for GeneralError")
	}
}

func TestIsConflict_True(t *testing.T) {
	err := Conflict("duplicate")
	if !err.IsConflict() {
		t.Error("IsConflict() = false for Conflict error")
	}
}

func TestIsConflict_False(t *testing.T) {
	err := NotFound("")
	if err.IsConflict() {
		t.Error("IsConflict() = true for NotFound error")
	}
}

func TestIsForbidden_True(t *testing.T) {
	err := Forbidden("not allowed")
	if !err.IsForbidden() {
		t.Error("IsForbidden() = false for Forbidden error")
	}
}

func TestIsForbidden_False(t *testing.T) {
	err := NotFound("")
	if err.IsForbidden() {
		t.Error("IsForbidden() = true for NotFound error")
	}
}

// --------------------------------------------------------------------------
// AsOpenapiError
// --------------------------------------------------------------------------

func TestAsOpenapiError(t *testing.T) {
	err := NotFound("widget not found")
	oe := err.AsOpenapiError("GetWidget")
	if oe.Kind == nil || *oe.Kind != "Error" {
		t.Errorf("Kind = %v, want 'Error'", oe.Kind)
	}
	if oe.Id == nil || *oe.Id == "" {
		t.Error("Id should be non-empty")
	}
	if oe.Href == nil || *oe.Href == "" {
		t.Error("Href should be non-empty")
	}
	if oe.Code == nil || *oe.Code == "" {
		t.Error("Code should be non-empty")
	}
	if oe.Reason == nil || !strings.Contains(*oe.Reason, "widget not found") {
		t.Errorf("Reason = %v, want to contain 'widget not found'", oe.Reason)
	}
	if oe.OperationId == nil || *oe.OperationId != "GetWidget" {
		t.Errorf("OperationId = %v, want 'GetWidget'", oe.OperationId)
	}
}

// --------------------------------------------------------------------------
// CodeStr and Href
// --------------------------------------------------------------------------

func TestCodeStr(t *testing.T) {
	prefix := ErrorCodePrefix()
	s := CodeStr(ErrorNotFound)
	if s == nil || *s == "" {
		t.Fatal("CodeStr() returned nil or empty")
	}
	want := fmt.Sprintf("%s-%d", prefix, ErrorNotFound)
	if *s != want {
		t.Errorf("CodeStr() = %q, want %q", *s, want)
	}
}

func TestHref(t *testing.T) {
	base := ErrorHrefBase()
	h := Href(ErrorNotFound)
	if h == nil || *h == "" {
		t.Fatal("Href() returned nil or empty")
	}
	want := fmt.Sprintf("%s%d", base, ErrorNotFound)
	if *h != want {
		t.Errorf("Href() = %q, want %q", *h, want)
	}
}

// --------------------------------------------------------------------------
// All constructor helpers
// --------------------------------------------------------------------------

func TestNotFound(t *testing.T) {
	err := NotFound("widget %d", 42)
	if err.Code != ErrorNotFound {
		t.Errorf("Code = %d, want ErrorNotFound", err.Code)
	}
	if !strings.Contains(err.Reason, "widget 42") {
		t.Errorf("Reason = %q", err.Reason)
	}
}

func TestGeneralError(t *testing.T) {
	err := GeneralError("catch-all")
	if err.Code != ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral", err.Code)
	}
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized("no token")
	if err.Code != ErrorUnauthorized {
		t.Errorf("Code = %d, want ErrorUnauthorized", err.Code)
	}
}

func TestUnauthenticated(t *testing.T) {
	err := Unauthenticated("bad creds")
	if err.Code != ErrorUnauthenticated {
		t.Errorf("Code = %d, want ErrorUnauthenticated", err.Code)
	}
}

func TestForbidden(t *testing.T) {
	err := Forbidden("blacklisted")
	if err.Code != ErrorForbidden {
		t.Errorf("Code = %d, want ErrorForbidden", err.Code)
	}
}

func TestNotImplemented(t *testing.T) {
	err := NotImplemented("PATCH not supported")
	if err.Code != ErrorNotImplemented {
		t.Errorf("Code = %d, want ErrorNotImplemented", err.Code)
	}
}

func TestConflict(t *testing.T) {
	err := Conflict("already exists")
	if err.Code != ErrorConflict {
		t.Errorf("Code = %d, want ErrorConflict", err.Code)
	}
}

func TestValidation(t *testing.T) {
	err := Validation("field required")
	if err.Code != ErrorValidation {
		t.Errorf("Code = %d, want ErrorValidation", err.Code)
	}
}

func TestMalformedRequest(t *testing.T) {
	err := MalformedRequest("bad json")
	if err.Code != ErrorMalformedRequest {
		t.Errorf("Code = %d, want ErrorMalformedRequest", err.Code)
	}
}

func TestBadRequest(t *testing.T) {
	err := BadRequest("missing param")
	if err.Code != ErrorBadRequest {
		t.Errorf("Code = %d, want ErrorBadRequest", err.Code)
	}
}

func TestFailedToParseSearch(t *testing.T) {
	err := FailedToParseSearch("unexpected token")
	if err.Code != ErrorFailedToParseSearch {
		t.Errorf("Code = %d, want ErrorFailedToParseSearch", err.Code)
	}
	if !strings.Contains(err.Reason, "unexpected token") {
		t.Errorf("Reason = %q, want to contain query", err.Reason)
	}
}

func TestDatabaseAdvisoryLock(t *testing.T) {
	underlying := fmt.Errorf("lock timeout")
	err := DatabaseAdvisoryLock(underlying)
	if err.Code != ErrorDatabaseAdvisoryLock {
		t.Errorf("Code = %d, want ErrorDatabaseAdvisoryLock", err.Code)
	}
	if !strings.Contains(err.Reason, "lock timeout") {
		t.Errorf("Reason = %q, want to contain underlying error", err.Reason)
	}
}
