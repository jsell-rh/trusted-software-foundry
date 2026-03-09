package hooks

import (
	"fmt"
	"net/http"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec/foundry"
)

// TenantIsolationCheckPreHandler enforces that every request carries a valid org_id claim
// and that the org_id in the JWT matches the X-Organization-Id header.
// Point: pre-handler — called before any route handler runs.
func TenantIsolationCheckPreHandler(hctx *foundry.HookContext, w http.ResponseWriter, r *http.Request) error {
	claimOrg, _ := hctx.Claims["org_id"].(string)
	if claimOrg == "" {
		http.Error(w, "missing org_id claim", http.StatusForbidden)
		return fmt.Errorf("tenant isolation: missing org_id claim")
	}

	headerOrg := r.Header.Get("X-Organization-Id")
	if headerOrg != "" && headerOrg != claimOrg {
		http.Error(w, "org_id mismatch", http.StatusForbidden)
		return fmt.Errorf("tenant isolation: header org %q != claim org %q", headerOrg, claimOrg)
	}

	return nil
}
