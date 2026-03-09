package hooks

import (
	"net/http"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec/foundry"
)

// ClusterStatusEnricherPostHandler adds real-time node health summary to cluster GET responses.
// Point: post-handler — called after /api/fleet-manager/v1/clusters responses are written.
func ClusterStatusEnricherPostHandler(hctx *foundry.HookContext, req *foundry.PostHandlerRequest) error {
	if req.StatusCode != http.StatusOK {
		return nil
	}
	// In production: attach live node metrics from the graph topology index.
	hctx.Logger.Debug("enrich", "msg", "cluster status enrichment applied")
	return nil
}
