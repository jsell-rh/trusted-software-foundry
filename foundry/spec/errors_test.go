package spec_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --- Error code → HTTP status mapping ---

func TestServiceError_HTTPStatus(t *testing.T) {
	cases := []struct {
		name       string
		err        *spec.ServiceError
		wantStatus int
	}{
		{"NotFound", spec.NewNotFoundError("Dinosaur", "abc"), http.StatusNotFound},
		{"Conflict", spec.NewConflictError("already exists"), http.StatusConflict},
		{"BadRequest", spec.NewBadRequestError("bad param"), http.StatusBadRequest},
		{"Validation", spec.NewValidationError("field x required"), http.StatusBadRequest},
		{"MalformedRequest", spec.NewMalformedRequestError(errors.New("eof")), http.StatusBadRequest},
		{"Unauthorized", spec.NewUnauthorizedError("not allowed"), http.StatusForbidden},
		{"Forbidden", spec.NewForbiddenError("blocked"), http.StatusForbidden},
		{"Unauthenticated", spec.NewUnauthenticatedError("no token"), http.StatusUnauthorized},
		{"NotImplemented", spec.NewNotImplementedError("PATCH"), http.StatusMethodNotAllowed},
		{"Internal", spec.NewInternalError(errors.New("db down")), http.StatusInternalServerError},
		{"Internalf", spec.NewInternalErrorf("sql: %s", "fail"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.HTTPStatus != tc.wantStatus {
				t.Errorf("HTTPStatus = %d, want %d", tc.err.HTTPStatus, tc.wantStatus)
			}
		})
	}
}

// --- Error codes ---

func TestServiceError_Codes(t *testing.T) {
	if spec.NewNotFoundError("X", "1").Code != spec.ErrNotFound {
		t.Error("NotFound code mismatch")
	}
	if spec.NewConflictError("c").Code != spec.ErrConflict {
		t.Error("Conflict code mismatch")
	}
	if spec.NewInternalError(nil).Code != spec.ErrInternal {
		t.Error("Internal code mismatch")
	}
}

// --- Error interface ---

func TestServiceError_Error(t *testing.T) {
	t.Run("without cause", func(t *testing.T) {
		se := spec.NewNotFoundError("Dinosaur", "42")
		msg := se.Error()
		if !strings.Contains(msg, "42") {
			t.Errorf("Error() = %q, want id '42' present", msg)
		}
		if !strings.Contains(msg, "Dinosaur") {
			t.Errorf("Error() = %q, want 'Dinosaur' present", msg)
		}
	})
	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("connection reset")
		se := spec.NewInternalError(cause)
		msg := se.Error()
		if !strings.Contains(msg, "connection reset") {
			t.Errorf("Error() = %q, want cause text present", msg)
		}
	})
}

// --- Unwrap ---

func TestServiceError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	se := spec.NewInternalError(cause)
	if !errors.Is(se, cause) {
		t.Error("errors.Is should find root cause via Unwrap")
	}
	if !errors.As(se, new(*spec.ServiceError)) {
		t.Error("errors.As should match *ServiceError")
	}
}

func TestServiceError_Unwrap_NilCause(t *testing.T) {
	se := spec.NewNotFoundError("X", "1")
	if se.Unwrap() != nil {
		t.Error("Unwrap should be nil when no cause")
	}
}

// --- Predicates ---

func TestServiceError_IsNotFound(t *testing.T) {
	if !spec.NewNotFoundError("X", "1").IsNotFound() {
		t.Error("IsNotFound should be true")
	}
	if spec.NewConflictError("c").IsNotFound() {
		t.Error("IsNotFound should be false for conflict")
	}
}

func TestServiceError_IsConflict(t *testing.T) {
	if !spec.NewConflictError("c").IsConflict() {
		t.Error("IsConflict should be true")
	}
	if spec.NewNotFoundError("X", "1").IsConflict() {
		t.Error("IsConflict should be false for not-found")
	}
}

func TestServiceError_IsBadRequest(t *testing.T) {
	if !spec.NewBadRequestError("bad").IsBadRequest() {
		t.Error("IsBadRequest should be true for 400")
	}
	if !spec.NewValidationError("v").IsBadRequest() {
		t.Error("IsBadRequest should be true for 400 validation")
	}
	if spec.NewInternalError(nil).IsBadRequest() {
		t.Error("IsBadRequest should be false for 500")
	}
}

// --- Reason formatting ---

func TestNewNotFoundError_Reason(t *testing.T) {
	se := spec.NewNotFoundError("Fleet", "ship-7")
	if !strings.Contains(se.Reason, "Fleet") || !strings.Contains(se.Reason, "ship-7") {
		t.Errorf("Reason = %q, want resource and id", se.Reason)
	}
}

func TestNewNotImplementedError_Reason(t *testing.T) {
	se := spec.NewNotImplementedError("PATCH")
	if !strings.Contains(se.Reason, "PATCH") {
		t.Errorf("Reason = %q, want method name", se.Reason)
	}
}

func TestNewMalformedRequestError_Cause(t *testing.T) {
	cause := errors.New("unexpected EOF")
	se := spec.NewMalformedRequestError(cause)
	if se.Cause != cause {
		t.Error("Cause should be preserved")
	}
}

func TestNewInternalErrorf(t *testing.T) {
	se := spec.NewInternalErrorf("db query %s failed", "SELECT")
	if !strings.Contains(se.Reason, "SELECT") {
		t.Errorf("Reason = %q, want format applied", se.Reason)
	}
}

// --- WriteHTTP ---

type fakeResponseWriter struct {
	status  int
	headers map[string][]string
	body    []byte
}

func newFakeRW() *fakeResponseWriter {
	return &fakeResponseWriter{headers: make(map[string][]string)}
}

func (f *fakeResponseWriter) Header() map[string][]string { return f.headers }
func (f *fakeResponseWriter) WriteHeader(s int)           { f.status = s }
func (f *fakeResponseWriter) Write(b []byte) (int, error) {
	f.body = append(f.body, b...)
	return len(b), nil
}

func TestServiceError_WriteHTTP(t *testing.T) {
	cases := []struct {
		name       string
		se         *spec.ServiceError
		wantStatus int
		wantJSON   string
	}{
		{
			name:       "NotFound",
			se:         spec.NewNotFoundError("Ship", "xyz"),
			wantStatus: http.StatusNotFound,
			wantJSON:   `"code":7`,
		},
		{
			name:       "Conflict",
			se:         spec.NewConflictError("duplicate name"),
			wantStatus: http.StatusConflict,
			wantJSON:   `"code":6`,
		},
		{
			name:       "Internal",
			se:         spec.NewInternalError(errors.New("boom")),
			wantStatus: http.StatusInternalServerError,
			wantJSON:   `"code":9`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newFakeRW()
			tc.se.WriteHTTP(w)
			if w.status != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.status, tc.wantStatus)
			}
			ct := strings.Join(w.headers["Content-Type"], "")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			body := string(w.body)
			if !strings.Contains(body, tc.wantJSON) {
				t.Errorf("body = %q, want %q present", body, tc.wantJSON)
			}
			if !strings.Contains(body, "http_status") {
				t.Errorf("body = %q, want 'http_status' field", body)
			}
			if !strings.Contains(body, "reason") {
				t.Errorf("body = %q, want 'reason' field", body)
			}
		})
	}
}

// --- AsServiceError ---

func TestAsServiceError_AlreadyServiceError(t *testing.T) {
	orig := spec.NewNotFoundError("X", "1")
	got, ok := spec.AsServiceError(orig)
	if !ok {
		t.Error("expected ok=true for *ServiceError input")
	}
	if got != orig {
		t.Error("expected same pointer returned")
	}
}

func TestAsServiceError_PlainError(t *testing.T) {
	plain := errors.New("something broke")
	got, ok := spec.AsServiceError(plain)
	if ok {
		t.Error("expected ok=false for plain error")
	}
	if got.Code != spec.ErrInternal {
		t.Errorf("code = %d, want ErrInternal", got.Code)
	}
	if got.Cause != plain {
		t.Error("cause should be the original plain error")
	}
}

func TestAsServiceError_Nil(t *testing.T) {
	got, ok := spec.AsServiceError(nil)
	if ok || got != nil {
		t.Error("nil input should return (nil, false)")
	}
}

// --- Default reasons ---

func TestServiceError_DefaultReasons(t *testing.T) {
	cases := []struct {
		name string
		err  *spec.ServiceError
	}{
		{"Conflict", spec.NewConflictError("")},
		{"Unauthorized", spec.NewUnauthorizedError("")},
		{"Forbidden", spec.NewForbiddenError("")},
		{"Unauthenticated", spec.NewUnauthenticatedError("")},
		{"BadRequest", spec.NewBadRequestError("")},
		{"NotImplemented", spec.NewNotImplementedError("")},
		{"Internal", spec.NewInternalError(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Reason == "" {
				t.Error("Reason should not be empty when no reason provided")
			}
		})
	}
}
