# Foundry Hooks Lifecycle Contract

**Owner:** TSC-Architect (Foundry IR Design)
**Status:** FROZEN — Compiler must implement these signatures exactly.

This document specifies the exact Go function signatures, context semantics, and return behavior for every hook point in the Foundry application lifecycle. Engineers implement these functions in `hooks/*.go`; the compiler generates the call sites.

---

## Overview

Hooks are the escape hatch. When trusted components cannot satisfy a requirement, custom Go code can be injected at well-defined lifecycle points. The AI declares hook points in `app.foundry.yaml`; engineers implement them in `hooks/*.go`.

**Invariants:**

1. Hook files are copied into the generated project unchanged.
2. The compiler generates a `HookRegistry` in `main.go` that wires each declared hook to its call site.
3. Hooks are called synchronously in the request/event/db lifecycle.
4. Hooks that return a non-nil error abort the current operation and return an error response to the caller.
5. All hook functions receive a `*foundry.HookContext` that carries request state, the logger, and the application tracer.

---

## Shared Types

The compiler generates these types in the `foundry` package (imported by all hook files):

```go
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
}

// Logger is a minimal structured logging interface.
type Logger interface {
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    With(fields ...Field) Logger
}

// Field is a structured log field.
type Field struct{ Key string; Value any }

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
```

---

## Hook Point Specifications

### 1. `pre-handler` — HTTP Request Interceptor

Called before the route handler processes the request. Use for request enrichment, additional authentication checks, or request logging.

```go
// File: hooks/<name>.go
// Package: hooks (must match generated package declaration)
package hooks

import (
    "github.com/jsell-rh/trusted-software-foundry/foundry"
    "net/http"
)

// PreHandlerHook is the function signature for pre-handler hooks.
// Returning a non-nil error causes the compiler-generated call site to write
// an HTTP 400 (or 500 for unexpected errors) response and abort the handler.
// The hook MAY mutate req headers or set values on the context.
//
// Generated call site (in the HTTP middleware chain):
//
//   if err := hooks.<HookName>PreHandler(hctx, w, r); err != nil {
//       http.Error(w, err.Error(), http.StatusBadRequest)
//       return
//   }
func <HookName>PreHandler(hctx *foundry.HookContext, w http.ResponseWriter, r *http.Request) error {
    // Implementation here.
    // - Read and validate request headers, body, query params.
    // - Set values on r.Context() via context.WithValue (use typed keys).
    // - Write partial response via w (rare — prefer returning error).
    return nil
}
```

**Constraints:**
- Must not call `w.WriteHeader()` unless returning a terminal error response.
- Must not consume `r.Body` (the handler needs it).
- Execution order when multiple pre-handler hooks target the same route: alphabetical by hook name.

---

### 2. `post-handler` — HTTP Response Interceptor

Called after the route handler has written the response. Use for response logging, audit trails, or adding response headers. The response has already been sent — the hook cannot change the status code or body.

```go
package hooks

import (
    "github.com/jsell-rh/trusted-software-foundry/foundry"
    "net/http"
)

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

// PostHandlerHook signature.
// Errors are logged but do NOT affect the HTTP response (it is already sent).
//
// Generated call site (deferred in the HTTP middleware):
//
//   defer func() {
//       if err := hooks.<HookName>PostHandler(hctx, postReq); err != nil {
//           hctx.Logger.Error("post-handler hook failed", foundry.Field{"hook", "<HookName>"}, foundry.Field{"err", err})
//       }
//   }()
func <HookName>PostHandler(hctx *foundry.HookContext, req *PostHandlerRequest) error {
    return nil
}
```

**Constraints:**
- Cannot modify the response (already sent).
- Errors are logged but swallowed — do not use for critical path logic.

---

### 3. `pre-db` — Database Operation Interceptor

Called before a CRUD database operation executes. Use for audit logging, field enrichment, or additional validation against the database state.

```go
package hooks

import (
    "github.com/jsell-rh/trusted-software-foundry/foundry"
)

// DBOperation describes the database operation about to execute.
type DBOperation struct {
    // Resource is the resource name, e.g. "Dinosaur".
    Resource string

    // Operation is one of: "create", "read", "update", "delete", "list".
    Operation string

    // ID is the resource primary key (empty for create and list).
    ID string

    // Payload is the mutable record payload for create/update operations.
    // Mutating Payload fields here will affect the values written to the database.
    Payload map[string]any

    // Filters are the query filters for list/read operations (read-only).
    Filters map[string]any
}

// PreDBHook signature.
// Returning a non-nil error aborts the database operation and surfaces the
// error as an HTTP 400 (validation) or 500 (internal) response.
// The hook MAY mutate op.Payload to enrich or override values before the write.
//
// Generated call site (in the DAO before the SQL statement):
//
//   if err := hooks.<HookName>PreDB(hctx, &op); err != nil {
//       return nil, fmt.Errorf("pre-db hook %s: %w", "<HookName>", err)
//   }
func <HookName>PreDB(hctx *foundry.HookContext, op *DBOperation) error {
    return nil
}
```

**Constraints:**
- `op.Payload` mutations are applied before the SQL statement — use this for field enrichment (e.g. setting `data_source_id`).
- `op.Filters` is read-only — mutating it has no effect.
- Long-running operations should respect `hctx.Ctx` cancellation.

---

### 4. `post-db` — Database Operation Result Interceptor

Called after a CRUD database operation completes successfully. Use for cache invalidation, event emission, or derived computations.

```go
package hooks

import (
    "github.com/jsell-rh/trusted-software-foundry/foundry"
)

// DBResult carries the result of a completed database operation.
type DBResult struct {
    // Resource and Operation mirror DBOperation.
    Resource  string
    Operation string

    // ID is the primary key of the affected record (including for create).
    ID string

    // Record is the full record as read back from the database after the operation.
    // For delete, this is the record as it existed before deletion.
    // For list, this is the first record in the result set (use Records for all).
    Record map[string]any

    // Records is the full result set for list operations.
    Records []map[string]any

    // RowsAffected is the number of database rows affected (for update/delete).
    RowsAffected int64
}

// PostDBHook signature.
// Errors are logged. To surface errors to the caller, they must be returned
// as a non-nil error, which will convert a 200/201 response into a 500.
// Use sparingly — prefer fire-and-forget for cache/event side effects.
//
// Generated call site (in the DAO after the SQL statement):
//
//   if err := hooks.<HookName>PostDB(hctx, &result); err != nil {
//       hctx.Logger.Error("post-db hook failed", foundry.Field{"err", err})
//       // Note: error propagation is configurable per hook via IR (future)
//   }
func <HookName>PostDB(hctx *foundry.HookContext, result *DBResult) error {
    return nil
}
```

---

### 5. `pre-publish` — Event Publication Interceptor

Called before an event is published to the event bus. Use for message enrichment, schema validation, or conditional suppression.

```go
package hooks

import (
    "github.com/jsell-rh/trusted-software-foundry/foundry"
)

// EventMessage is the mutable event about to be published.
type EventMessage struct {
    // Topic is the destination topic name.
    Topic string

    // Key is the partition key (may be empty).
    Key string

    // Headers are the event metadata headers. Mutable.
    Headers map[string]string

    // Payload is the event body. Mutable — enrichment goes here.
    // The compiler serializes Payload using the topic's configured schema format.
    Payload map[string]any
}

// PrePublishHook signature.
// Returning a non-nil error suppresses publication of this message.
// The hook MAY mutate msg.Payload and msg.Headers.
//
// Generated call site (in the event producer before serialization):
//
//   if err := hooks.<HookName>PrePublish(hctx, &msg); err != nil {
//       return fmt.Errorf("pre-publish hook %s: %w", "<HookName>", err)
//   }
func <HookName>PrePublish(hctx *foundry.HookContext, msg *EventMessage) error {
    return nil
}
```

---

### 6. `post-consume` — Event Consumption Interceptor

Called after an event consumer handler has successfully processed a message. Use for acknowledgment logging, metrics, or chaining additional effects.

```go
package hooks

import (
    "github.com/jsell-rh/trusted-software-foundry/foundry"
)

// ConsumedEvent describes a successfully processed event.
type ConsumedEvent struct {
    // Topic and Partition identify the source.
    Topic     string
    Partition int32
    Offset    int64

    // Key, Headers, Payload mirror the original message.
    Key     string
    Headers map[string]string
    Payload map[string]any

    // HandlerDuration is the time taken by the consumer handler.
    HandlerDuration time.Duration
}

// PostConsumeHook signature.
// Errors are logged but do NOT trigger a redelivery or DLQ routing.
// To trigger DLQ routing on error, return a non-nil error from the consumer
// handler itself (not this hook).
//
// Generated call site (in the consumer loop after handler returns nil):
//
//   if err := hooks.<HookName>PostConsume(hctx, &event); err != nil {
//       hctx.Logger.Warn("post-consume hook failed", foundry.Field{"err", err})
//   }
func <HookName>PostConsume(hctx *foundry.HookContext, event *ConsumedEvent) error {
    return nil
}
```

---

## Compiler Obligations

For each hook declared in `app.foundry.yaml`, the compiler must:

1. **Copy** `hooks/<name>.go` unchanged into `generated/hooks/<name>.go`.
2. **Generate** a `HookRegistry` struct in `generated/main.go` with one field per hook.
3. **Generate** the shared types (`HookContext`, `Logger`, `DBOperation`, `DBResult`, `EventMessage`, `ConsumedEvent`, `PostHandlerRequest`) in `generated/foundry/types.go`.
4. **Wire** each hook call site at the declared lifecycle point in the appropriate generated middleware/DAO/producer/consumer.
5. **Validate** that the hook file exists and exports the expected function signature at compile time (via `go build`).

### HookRegistry (generated)

```go
// generated/main.go
type HookRegistry struct {
    // One field per declared hook, in IR order.
    // Example for hook named "enrich-request":
    EnrichRequest hooks.EnrichRequestPreHandler
}

var registry = &HookRegistry{
    EnrichRequest: hooks.EnrichRequestPreHandler,
}
```

---

## Example: Dinosaur Registry with Hooks

```yaml
hooks:
  - name: set-data-source
    point: pre-db
    resources: [Dinosaur]
    implementation: hooks/set_data_source.go

  - name: audit-writes
    point: post-db
    resources: [Dinosaur]
    implementation: hooks/audit_writes.go

  - name: enrich-response
    point: post-handler
    routes: ["/api/v1/dinosaurs"]
    implementation: hooks/enrich_response.go
```

```go
// hooks/set_data_source.go
package hooks

import "github.com/jsell-rh/trusted-software-foundry/foundry"

func SetDataSourcePreDB(hctx *foundry.HookContext, op *DBOperation) error {
    if op.Operation == "create" {
        op.Payload["data_source_id"] = hctx.TenantID + ":default"
    }
    return nil
}
```

The compiler generates a call site for `SetDataSourcePreDB` in the Dinosaur DAO before every INSERT.
