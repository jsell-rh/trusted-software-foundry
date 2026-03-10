package compiler

import "fmt"

// knownComponents is the development stub registry.
// These module paths and versions match the architecture plan.
// In production, these entries are signed and come from the trusted catalog.
var knownComponents = map[string]*RegistryEntry{
	"foundry-http": {
		Name:      "foundry-http",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/http",
		AuditHash: "stub-not-verified",
	},
	"foundry-postgres": {
		Name:      "foundry-postgres",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/postgres",
		AuditHash: "stub-not-verified",
	},
	"foundry-auth-jwt": {
		Name:      "foundry-auth-jwt",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/auth/jwt",
		AuditHash: "stub-not-verified",
	},
	"foundry-grpc": {
		Name:      "foundry-grpc",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/grpc",
		AuditHash: "stub-not-verified",
	},
	"foundry-health": {
		Name:      "foundry-health",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/health",
		AuditHash: "stub-not-verified",
	},
	"foundry-metrics": {
		Name:      "foundry-metrics",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/metrics",
		AuditHash: "stub-not-verified",
	},
	"foundry-events": {
		Name:      "foundry-events",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/events",
		AuditHash: "stub-not-verified",
	},
	"foundry-auth-spicedb": {
		Name:      "foundry-auth-spicedb",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/spicedb",
		AuditHash: "stub-not-verified",
	},
	"foundry-graph-age": {
		Name:      "foundry-graph-age",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/graph/age",
		AuditHash: "stub-not-verified",
	},
	"foundry-kafka": {
		Name:      "foundry-kafka",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/kafka",
		AuditHash: "stub-not-verified",
	},
	"foundry-nats": {
		Name:      "foundry-nats",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/nats",
		AuditHash: "stub-not-verified",
	},
	"foundry-redis": {
		Name:      "foundry-redis",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/redis",
		AuditHash: "stub-not-verified",
	},
	"foundry-redis-streams": {
		Name:      "foundry-redis-streams",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/redis-streams",
		AuditHash: "stub-not-verified",
	},
	"foundry-temporal": {
		Name:      "foundry-temporal",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/temporal",
		AuditHash: "stub-not-verified",
	},
	"foundry-tenancy": {
		Name:      "foundry-tenancy",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/tenancy",
		AuditHash: "stub-not-verified",
	},
	"foundry-service-router": {
		Name:      "foundry-service-router",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/foundry/components/service-router",
		AuditHash: "stub-not-verified",
	},
}

// StubRegistry is an in-memory registry for development and testing.
// It supports all components defined in the architecture plan.
// Audit hash verification is skipped (hashes are placeholder values).
type StubRegistry struct{}

// NewStubRegistry creates a development stub registry.
func NewStubRegistry() *StubRegistry {
	return &StubRegistry{}
}

// Lookup returns the stub registry entry for a component.
func (s *StubRegistry) Lookup(name, version string) (*RegistryEntry, error) {
	entry, ok := knownComponents[name]
	if !ok {
		known := make([]string, 0, len(knownComponents))
		for k := range knownComponents {
			known = append(known, k)
		}
		return nil, fmt.Errorf("unknown component %q — not in stub registry (known: %v)", name, known)
	}
	if entry.Version != version {
		return nil, fmt.Errorf("component %q: requested version %q but stub registry only has %q", name, version, entry.Version)
	}
	return entry, nil
}
