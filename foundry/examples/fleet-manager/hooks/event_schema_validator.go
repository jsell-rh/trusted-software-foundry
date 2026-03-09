package hooks

import (
	"context"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec/foundry"
)

// EventSchemaValidator ensures every outbound event has required envelope fields
// before it is published to Kafka. Prevents malformed events from entering the bus.
// Point: pre-publish — called before publishing to any topic.
func EventSchemaValidator(ctx context.Context, hctx *foundry.HookContext, msg *foundry.EventMessage) error {
	if msg.Topic == "" {
		return fmt.Errorf("event schema: missing topic")
	}
	if msg.Key == "" {
		return fmt.Errorf("event schema: missing partition key for topic %q", msg.Topic)
	}
	if _, ok := msg.Headers["event_type"]; !ok {
		return fmt.Errorf("event schema: missing event_type header for topic %q", msg.Topic)
	}
	return nil
}
