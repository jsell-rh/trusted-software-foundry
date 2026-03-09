// Package foundry provides the shared types used by all hook implementations.
// Hook files import this package; forge compile copies it into the generated
// project so hooks can build without modification.
//
// Hook function signatures are frozen by the hooks contract in docs/HOOKS_CONTRACT.md.
// Do not change existing signatures — add new hook points instead.
package foundry

import (
	"context"
	"net/http"
	"time"
)

// HookContext is passed to every hook function. It carries operation context,
// a structured logger, and request-scoped metadata.
type HookContext struct {
	// Ctx is the request or event context. Respect cancellation.
	Ctx context.Context

	// Logger is a structured logger scoped to this operation.
	Logger Logger

	// Tracer is an OpenTelemetry tracer for creating child spans.
	Tracer Tracer

	// AppName is the application name from metadata.name.
	AppName string

	// TenantID is the resolved tenant identifier, or "" for single-tenant apps.
	TenantID string

	// RequestID is the per-request correlation identifier.
	RequestID string

	// TraceID is the distributed trace identifier (OpenTelemetry).
	TraceID string

	// Claims are the validated JWT claims for the current request.
	// Empty map for event-driven hook points (pre-publish, post-consume).
	Claims map[string]any
}

// Logger is a minimal structured logging interface.
type Logger interface {
	Info(msg string, keyvals ...string)
	Warn(msg string, keyvals ...string)
	Error(msg string, keyvals ...string)
	Debug(msg string, keyvals ...string)
}

// Tracer wraps OpenTelemetry tracing.
type Tracer interface {
	Start(ctx context.Context, spanName string) (context.Context, Span)
}

// Span is an OpenTelemetry span.
type Span interface {
	End()
	SetAttribute(key string, value any)
	RecordError(err error)
}

// PostHandlerRequest carries the response details available after the handler runs.
type PostHandlerRequest struct {
	// Method and Path of the handled request.
	Method string
	Path   string

	// StatusCode is the HTTP status code that was written.
	StatusCode int

	// ResponseBytes is the number of bytes written to the response body.
	ResponseBytes int64

	// Duration is the time taken by the handler (excluding this hook).
	Duration time.Duration

	// RequestHeaders are the incoming request headers.
	RequestHeaders http.Header
}

// DBOperation describes a database operation about to execute.
type DBOperation struct {
	// Resource is the resource name, e.g. "Dinosaur".
	Resource string

	// Type is one of: "create", "read", "update", "delete", "list".
	Type string

	// ResourceID is the primary key (empty for create and list).
	ResourceID string

	// Payload is the mutable record payload for create/update operations.
	// Mutating Payload affects the values written to the database.
	Payload map[string]any

	// Filter is the query filter for list/read operations.
	Filter map[string]any
}

// DBResult carries the outcome of a completed database operation.
type DBResult struct {
	// Resource and Operation mirror the DBOperation fields.
	Resource  string
	Operation string

	// ID is the primary key of the affected record.
	ID string

	// RowsAffected is the number of rows modified (create/update/delete).
	RowsAffected int64

	// Error is the database error, if any. Non-nil errors abort the operation.
	Error error
}

// EventMessage is the message being published to the event bus.
type EventMessage struct {
	// Topic is the destination topic or subject.
	Topic string

	// Key is the optional partition/routing key.
	Key string

	// Payload is the mutable message body. Mutations affect what is published.
	Payload []byte

	// Headers are message-level metadata.
	Headers map[string]string
}

// ConsumedEvent is the event received from the event bus.
type ConsumedEvent struct {
	// Topic is the source topic or subject.
	Topic string

	// Key is the message partition key.
	Key string

	// Payload is the raw message body (JSON).
	Payload map[string]any

	// Headers are message-level metadata.
	Headers map[string]string

	// Offset is the message position (Kafka) or sequence (NATS).
	Offset int64
}
