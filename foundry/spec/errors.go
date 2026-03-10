// Package spec — ServiceError: typed API error hierarchy for the Trusted Software Foundry.
//
// Error codes mirror the rh-trex error taxonomy so that applications built on
// TSF produce consistent, machine-readable error responses.  All helpers are
// stdlib-only; no external dependencies are introduced.
package spec

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorCode is a numeric error code that uniquely identifies an error class.
type ErrorCode int

const (
	// ErrNotFound — requested resource does not exist.  HTTP 404.
	ErrNotFound ErrorCode = 7
	// ErrConflict — a unique-constraint or state conflict.  HTTP 409.
	ErrConflict ErrorCode = 6
	// ErrBadRequest — the request body or parameters are invalid.  HTTP 400.
	ErrBadRequest ErrorCode = 21
	// ErrValidation — field-level validation failure.  HTTP 400.
	ErrValidation ErrorCode = 8
	// ErrMalformedRequest — the request body could not be read or parsed.  HTTP 400.
	ErrMalformedRequest ErrorCode = 17
	// ErrUnauthorized — authenticated but not permitted to perform this action.  HTTP 403.
	ErrUnauthorized ErrorCode = 11
	// ErrForbidden — account is explicitly denied access.  HTTP 403.
	ErrForbidden ErrorCode = 4
	// ErrUnauthenticated — credentials missing or invalid.  HTTP 401.
	ErrUnauthenticated ErrorCode = 15
	// ErrNotImplemented — API method not yet implemented.  HTTP 405.
	ErrNotImplemented ErrorCode = 10
	// ErrInternal — unspecified internal error.  HTTP 500.
	ErrInternal ErrorCode = 9
)

// httpStatus maps each error code to its canonical HTTP status code.
var httpStatus = map[ErrorCode]int{
	ErrNotFound:         http.StatusNotFound,
	ErrConflict:         http.StatusConflict,
	ErrBadRequest:       http.StatusBadRequest,
	ErrValidation:       http.StatusBadRequest,
	ErrMalformedRequest: http.StatusBadRequest,
	ErrUnauthorized:     http.StatusForbidden,
	ErrForbidden:        http.StatusForbidden,
	ErrUnauthenticated:  http.StatusUnauthorized,
	ErrNotImplemented:   http.StatusMethodNotAllowed,
	ErrInternal:         http.StatusInternalServerError,
}

// defaultReason maps each error code to its default human-readable reason.
var defaultReason = map[ErrorCode]string{
	ErrNotFound:         "Resource not found",
	ErrConflict:         "An entity with the specified unique values already exists",
	ErrBadRequest:       "Bad request",
	ErrValidation:       "General validation failure",
	ErrMalformedRequest: "Unable to read request body",
	ErrUnauthorized:     "Account is unauthorized to perform this action",
	ErrForbidden:        "Forbidden to perform this action",
	ErrUnauthenticated:  "Account authentication could not be verified",
	ErrNotImplemented:   "HTTP method not implemented for this endpoint",
	ErrInternal:         "Unspecified internal error",
}

// ServiceError is a typed, structured API error that carries an error code,
// an HTTP status, a human-readable reason, and an optional root cause.
//
// ServiceError implements the error interface and can be serialised directly
// to JSON for API responses via WriteHTTP.
type ServiceError struct {
	// Code is the canonical numeric error code.
	Code ErrorCode `json:"code"`
	// HTTPStatus is the HTTP status code to use in the response.
	HTTPStatus int `json:"http_status"`
	// Reason is the context-specific human-readable explanation.
	Reason string `json:"reason"`
	// Cause is the underlying error, if any.  Not serialised to JSON.
	Cause error `json:"-"`
}

// Error implements the error interface.
func (e *ServiceError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("foundry-%d: %s: %v", e.Code, e.Reason, e.Cause)
	}
	return fmt.Sprintf("foundry-%d: %s", e.Code, e.Reason)
}

// Unwrap returns the underlying cause so errors.Is/errors.As work correctly.
func (e *ServiceError) Unwrap() error {
	return e.Cause
}

// IsNotFound reports whether this error has code ErrNotFound.
func (e *ServiceError) IsNotFound() bool { return e.Code == ErrNotFound }

// IsConflict reports whether this error has code ErrConflict.
func (e *ServiceError) IsConflict() bool { return e.Code == ErrConflict }

// IsBadRequest reports whether this error represents a client error (4xx).
func (e *ServiceError) IsBadRequest() bool { return e.HTTPStatus >= 400 && e.HTTPStatus < 500 }

// WriteHTTP serialises the error as JSON and writes it to w using the
// canonical HTTP status.  The response body has the shape:
//
//	{"code": 7, "http_status": 404, "reason": "Resource not found"}
func (e *ServiceError) WriteHTTP(w ResponseWriter) {
	data, _ := json.Marshal(e)
	h := w.Header()
	h["Content-Type"] = []string{"application/json"}
	w.WriteHeader(e.HTTPStatus)
	w.Write(data) //nolint:errcheck
}

// newServiceError constructs a ServiceError for the given code.
// reason overrides the default; if empty the default reason is used.
// cause may be nil.
func newServiceError(code ErrorCode, reason string, cause error) *ServiceError {
	if reason == "" {
		reason = defaultReason[code]
	}
	status, ok := httpStatus[code]
	if !ok {
		status = http.StatusInternalServerError
	}
	return &ServiceError{Code: code, HTTPStatus: status, Reason: reason, Cause: cause}
}

// --- Constructor helpers ---

// NewNotFoundError returns a 404 ServiceError for the given resource and id.
func NewNotFoundError(resource, id string) *ServiceError {
	return newServiceError(ErrNotFound, fmt.Sprintf("%s with id '%s' not found", resource, id), nil)
}

// NewConflictError returns a 409 ServiceError with the provided message.
func NewConflictError(msg string) *ServiceError {
	return newServiceError(ErrConflict, msg, nil)
}

// NewBadRequestError returns a 400 ServiceError with the provided message.
func NewBadRequestError(msg string) *ServiceError {
	return newServiceError(ErrBadRequest, msg, nil)
}

// NewValidationError returns a 400 ServiceError describing a field validation failure.
func NewValidationError(msg string) *ServiceError {
	return newServiceError(ErrValidation, msg, nil)
}

// NewMalformedRequestError returns a 400 ServiceError wrapping the parse error.
func NewMalformedRequestError(cause error) *ServiceError {
	return newServiceError(ErrMalformedRequest, "Unable to read request body", cause)
}

// NewUnauthorizedError returns a 403 ServiceError with the provided message.
func NewUnauthorizedError(msg string) *ServiceError {
	return newServiceError(ErrUnauthorized, msg, nil)
}

// NewForbiddenError returns a 403 ServiceError with the provided message.
func NewForbiddenError(msg string) *ServiceError {
	return newServiceError(ErrForbidden, msg, nil)
}

// NewUnauthenticatedError returns a 401 ServiceError with the provided message.
func NewUnauthenticatedError(msg string) *ServiceError {
	return newServiceError(ErrUnauthenticated, msg, nil)
}

// NewNotImplementedError returns a 405 ServiceError.
func NewNotImplementedError(method string) *ServiceError {
	return newServiceError(ErrNotImplemented, fmt.Sprintf("method %s is not implemented", method), nil)
}

// NewInternalError returns a 500 ServiceError wrapping an underlying cause.
// The cause is stored on the error but the reason shown to the client is
// generic to avoid leaking internals.
func NewInternalError(cause error) *ServiceError {
	return newServiceError(ErrInternal, "Internal server error", cause)
}

// NewInternalErrorf returns a 500 ServiceError with a formatted reason.
func NewInternalErrorf(format string, args ...any) *ServiceError {
	return newServiceError(ErrInternal, fmt.Sprintf(format, args...), nil)
}

// AsServiceError attempts to cast err to *ServiceError.
// Returns (err, true) if err is a *ServiceError, otherwise
// returns (NewInternalError(err), false).
func AsServiceError(err error) (*ServiceError, bool) {
	if err == nil {
		return nil, false
	}
	if se, ok := err.(*ServiceError); ok {
		return se, true
	}
	return NewInternalError(err), false
}
