# Trusted Software Foundry

An IR-first application platform where **AI agents write declarative specs, not code**.

Enterprises adopting AI-driven development face a fundamental trust problem: AI agents produce
non-deterministic code that cannot be formally audited. Trusted Software Foundry (TSF) solves this
by inverting the model — AI writes a structured Intermediate Representation (IR), and a
deterministic compiler assembles working applications from pre-audited, version-pinned trusted
components.

```
AI Agent
  │  edits only:
  ▼
app.foundry.yaml          ← declarative IR spec (schema-validated)
  │
  │  forge compile app.foundry.yaml -o ./out
  ▼
Compiler
  │  resolves trusted components (audited, version-pinned)
  │  generates minimal wiring code (main.go, go.mod, migrations/)
  │  generates stub hooks for custom logic injection points
  ▼
go build ./out            ← working, auditable binary
```

The AI never touches source code. Every line of generated code comes from audited, immutable
trusted components. New features are YAML fields, not code diffs.

---

## Demo: Fleet Manager in 3 Commands

Fleet Manager is a multi-tenant SaaS control plane for managing OpenShift cluster lifecycles.
Its entire infrastructure is described in
[`foundry/examples/fleet-manager/app.foundry.yaml`](./foundry/examples/fleet-manager/app.foundry.yaml) —
PostgreSQL, SpiceDB fine-grained authz, JWT auth, Kafka events, Redis distributed state,
multi-tenancy, and 5 lifecycle hooks.

```bash
# 1. Build the forge compiler
go build -o /usr/local/bin/forge ./cmd/forge

# 2. Compile the Fleet Manager spec into a runnable Go project
forge compile foundry/examples/fleet-manager/app.foundry.yaml \
  --foundry-path $(pwd) \
  -o /tmp/fleet-manager

# 3. Build the generated project — exits 0
cd /tmp/fleet-manager && go build -o fleet-manager .
```

That's it. A production-ready binary with REST API, graph database, auth, events, health checks,
and metrics — from a YAML file. No code written by a human or AI.

**What was generated:**

```
/tmp/fleet-manager/
  main.go                  ← component wiring (DO NOT EDIT — generated)
  hook_registry.go         ← hook call sites (DO NOT EDIT — generated)
  hooks/
    stubs_generated.go     ← stub hooks (replace with your real logic)
  migrations/
    0001_clusters.sql
    0002_cluster_upgrades.sql
  go.mod / go.sum
```

---

## Quick Start: Your First Service

### 1. Scaffold a new spec

```bash
forge scaffold --name my-service --resource Widget -o app.foundry.yaml
```

### 2. Lint and validate

```bash
forge lint app.foundry.yaml
```

### 3. Understand what will be built

```bash
forge explain app.foundry.yaml
```

### 4. Compile

```bash
forge compile app.foundry.yaml \
  --foundry-path /path/to/trusted-software-foundry \
  -o ./out
```

### 5. Build and run

```bash
cd ./out
go build -o app .

export JWK_CERT_URL=http://localhost:8080/auth/realms/myrealm/protocol/openid-connect/certs
export OCM_MOCK_ENABLED=true   # skip real auth in dev
./app
```

REST API: `:8000` | Health: `:8083` | Metrics: `:8080`

### 6. Add a resource — zero code

```yaml
# app.foundry.yaml — add this block under resources:
  - name: Gadget
    plural: gadgets
    fields:
      - name: id
        type: uuid
        required: true
        auto: created
      - name: name
        type: string
        required: true
        max_length: 255
      - name: created_at
        type: timestamp
        auto: created
    operations: [create, read, update, delete, list]
```

```bash
forge compile app.foundry.yaml --foundry-path . -o ./out
cd ./out && go build -o app .
# /api/my-service/v1/gadgets is now live. No code written.
```

---

## Custom Logic via Hooks

Hooks let you inject custom Go code at well-defined points without modifying trusted components.

**Declare in spec:**

```yaml
hooks:
  - name: audit-logger
    point: pre-handler
    implementation: hooks/audit_logger.go

  - name: data-enricher
    point: post-db
    implementation: hooks/enricher.go
```

**Implement in Go:**

```go
// hooks/audit_logger.go
package hooks

import (
    "net/http"
    "github.com/jsell-rh/trusted-software-foundry/foundry/spec/foundry"
)

func AuditLoggerPreHandler(hctx *foundry.HookContext, w http.ResponseWriter, r *http.Request) error {
    hctx.Logger.Info("request", "method", r.Method, "path", r.URL.Path, "tenant", hctx.TenantID)
    return nil
}
```

When you run `forge compile`, the compiler copies your hook files into the generated project and
generates typed call sites in `hook_registry.go`. If a hook file doesn't exist yet, a stub is
generated — replace it with real logic and recompile.

**Hook injection points:**

| Point | Signature | Use case |
|-------|-----------|----------|
| `pre-handler` | `(hctx, w, r)` | Auth, rate limiting, logging |
| `post-handler` | `(hctx, req)` | Response transformation, metrics |
| `pre-db` | `(hctx, op)` | Validation, enrichment before write |
| `post-db` | `(hctx, result)` | Side effects after write (sync AGE graph) |
| `pre-publish` | `(hctx, msg)` | Event enrichment before Kafka publish |
| `post-consume` | `(hctx, event)` | Event handling after Kafka consume |

See [`foundry/examples/fleet-manager/hooks/`](./foundry/examples/fleet-manager/hooks/) for
working hook examples including bi-temporal validation, graph sync, and tenant isolation.

---

## The forge Command Suite

| Command | Description |
|---------|-------------|
| `forge scaffold` | Generate a starter `app.foundry.yaml` |
| `forge lint` | Validate spec (JSON Schema + semantic rules) |
| `forge explain` | Human-readable spec summary (resources, components, API surface) |
| `forge compile` | Compile spec → buildable Go project |
| `forge diff old.yaml new.yaml` | Structured diff of two specs |
| `forge sbom` | Generate CycloneDX 1.5 Software Bill of Materials |
| `forge verify` | Verify component audit hashes |
| `forge deploy` | Generate Kubernetes manifests + Helm chart |

---

## Trusted Component Catalog

All components are pre-audited, version-pinned, and immutable after release.
Bug fixes and new features create new component versions — the old version is never modified.

| Component | Purpose |
|-----------|---------|
| `foundry-http` | HTTP server, routing, CORS, middleware registration |
| `foundry-postgres` | DB pool, CRUD DAOs, auto-migrations, soft delete, pg_notify |
| `foundry-auth-jwt` | JWT validation, RBAC middleware, JWK endpoint integration |
| `foundry-auth-spicedb` | Fine-grained authz via SpiceDB/Authzed |
| `foundry-grpc` | gRPC server, interceptors |
| `foundry-health` | Liveness + readiness endpoints |
| `foundry-metrics` | Prometheus metrics server |
| `foundry-events` | PostgreSQL LISTEN/NOTIFY event loop |
| `foundry-kafka` | Kafka producer/consumer with topic management |
| `foundry-nats` | NATS messaging |
| `foundry-redis` | Cache, rate limiting, distributed locking |
| `foundry-redis-streams` | Redis Streams consumer groups |
| `foundry-graph-age` | Apache AGE graph database (Cypher, bi-temporal, bulk mutations) |
| `foundry-temporal` | Temporal workflow orchestration |
| `foundry-tenancy` | Row-level multi-tenancy isolation |
| `foundry-service-router` | Service mesh routing |

---

## Why TSF?

| Traditional AI-Generated Code | Trusted Software Foundry |
|-------------------------------|--------------------------|
| AI writes arbitrary source code | AI writes a validated IR spec |
| No formal audit trail | Every component has audit record + hash |
| Non-deterministic output | Same spec + versions = identical binary |
| Hard to generate SBOM | Component list IS the SBOM |
| New resource = code review | New resource = YAML field, zero code |
| AI mistakes reach production | Schema validation blocks malformed specs |
| Custom logic scattered in generated code | Hooks declared in spec, injected at compile time |

---

## Repository Structure

```
foundry/
  spec/               IR type system — ComponentInterface, Application, JSON Schema
  components/         Trusted component library (15 components, 60+ tests)
  compiler/           Forge compiler: parse → validate → resolve → generate
  examples/
    dinosaur-registry/ Simple CRUD service (quick-start reference)
    fleet-manager/    Multi-service with hooks and events (hero demo)
cmd/
  forge/              forge CLI (compile, scaffold, lint, explain, diff, sbom, verify, deploy)
docs/                 Hook contract, IR field reference
FOUNDRY-ARCHITECTURE.md  Architecture decisions and component design
```

## Testing

```bash
# All foundry + compiler tests
go test ./foundry/...

# Key packages
go test ./foundry/compiler/...    # compiler pipeline (60+ tests)
go test ./foundry/components/...  # trusted component library (60+ tests)
go test ./cmd/forge/...           # forge CLI commands
```

## IR Spec Reference

Full JSON Schema: `foundry/spec/schema.json`

```bash
forge lint app.foundry.yaml   # validates against schema + semantic rules
```

See [FOUNDRY-ARCHITECTURE.md](./FOUNDRY-ARCHITECTURE.md) for full architecture decisions,
component interface contracts, the compiler design, and hook injection point specifications.

---

## Contributing

Trusted components live in `foundry/components/` — changes require a new version and audit record.
The compiler output surface is frozen: changes to generated code shape need architecture review.

```bash
go test ./foundry/... ./cmd/...   # must pass before submitting a PR
```
