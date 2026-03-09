package hooks

import (
	"context"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec/foundry"
)

// GraphSyncConsumer updates the Apache AGE graph topology whenever a cluster
// lifecycle event arrives on fleet.cluster.lifecycle.
// Point: post-consume — called after an event is consumed from the topic.
func GraphSyncConsumer(ctx context.Context, hctx *foundry.HookContext, event *foundry.ConsumedEvent) error {
	if event.Topic != "fleet.cluster.lifecycle" {
		return nil
	}
	eventType, _ := event.Headers["event_type"]
	clusterID, _ := event.Payload["id"].(string)
	if clusterID == "" {
		return fmt.Errorf("graph sync: missing cluster id in event payload")
	}

	switch eventType {
	case "created":
		hctx.Logger.Info("graph", "action", "upsert", "node", "Cluster", "id", clusterID)
	case "deleted":
		hctx.Logger.Info("graph", "action", "remove", "node", "Cluster", "id", clusterID)
	default:
		hctx.Logger.Debug("graph", "action", "update", "node", "Cluster", "id", clusterID, "event", eventType)
	}
	return nil
}
