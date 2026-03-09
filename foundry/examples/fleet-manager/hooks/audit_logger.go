package hooks

import (
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec/foundry"
)

// AuditLoggerPreDb writes an immutable audit record before every database write.
// Point: pre-db — called for INSERT, UPDATE, DELETE operations.
func AuditLoggerPreDb(hctx *foundry.HookContext, op *foundry.DBOperation) error {
	actor, _ := hctx.Claims["sub"].(string)
	orgID, _ := hctx.Claims["org_id"].(string)
	// In production: write to an append-only audit log stream (e.g. Kafka topic).
	hctx.Logger.Info("audit",
		"actor", actor,
		"org_id", orgID,
		"op", op.Type,
		"resource", op.Resource,
		"resource_id", op.ResourceID,
		"request_id", hctx.RequestID,
		"trace_id", hctx.TraceID,
	)
	_ = fmt.Sprintf // suppress unused import if logger is swapped out
	return nil
}
