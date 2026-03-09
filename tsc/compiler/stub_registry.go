package compiler

import "fmt"

// knownComponents is the development stub registry.
// These module paths and versions match the architecture plan.
// In production, these entries are signed and come from the trusted catalog.
var knownComponents = map[string]*RegistryEntry{
	"foundry-http": {
		Name:      "foundry-http",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/tsc/components/http",
		AuditHash: "stub-not-verified",
	},
	"foundry-postgres": {
		Name:      "foundry-postgres",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/tsc/components/postgres",
		AuditHash: "stub-not-verified",
	},
	"foundry-auth-jwt": {
		Name:      "foundry-auth-jwt",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/tsc/components/auth/jwt",
		AuditHash: "stub-not-verified",
	},
	"foundry-grpc": {
		Name:      "foundry-grpc",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/tsc/components/grpc",
		AuditHash: "stub-not-verified",
	},
	"foundry-health": {
		Name:      "foundry-health",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/tsc/components/health",
		AuditHash: "stub-not-verified",
	},
	"foundry-metrics": {
		Name:      "foundry-metrics",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/tsc/components/metrics",
		AuditHash: "stub-not-verified",
	},
	"foundry-events": {
		Name:      "foundry-events",
		Version:   "v1.0.0",
		Module:    "github.com/jsell-rh/trusted-software-foundry/tsc/components/events",
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
		return nil, fmt.Errorf("unknown component %q — not in stub registry (known: foundry-http, foundry-postgres, foundry-auth-jwt, foundry-grpc, foundry-health, foundry-metrics, foundry-events)", name)
	}
	if entry.Version != version {
		return nil, fmt.Errorf("component %q: requested version %q but stub registry only has %q", name, version, entry.Version)
	}
	return entry, nil
}
