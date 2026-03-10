# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Project Overview

This is the **Trusted Software Foundry** — an IR-first application platform where AI agents write
declarative YAML specs (`app.foundry.yaml`) and a deterministic compiler assembles working Go
applications from pre-audited, version-pinned trusted components.

**AI agents write specs, not code.** The compiler does the rest.

## Key Directories

```
foundry/spec/           Core interfaces: Component, Application, Registrar, JSON Schema
foundry/components/     Trusted component library (15 components, 60+ tests)
foundry/compiler/       Foundry compiler: parse → validate → resolve → generate
foundry/examples/       Reference applications (kartograph, dinosaur-registry, fleet-manager)
cmd/forge/              forge CLI (compile, init, scaffold, lint, explain, diff, sbom, verify, deploy)
docs/                   Complex IR extension design docs
FOUNDRY-ARCHITECTURE.md Full architecture reference
```

## Development Commands

### Build the forge compiler

```bash
go build -o /tmp/forge ./cmd/forge
```

### Test

```bash
go test ./...                  # full suite
go test ./foundry/...          # foundry packages only
go test ./cmd/forge/...        # CLI tests only
```

### Initialize a new project

```bash
/tmp/forge init my-service --resource Widget
cd my-service/
# Edit app.foundry.yaml, then:
```

### Compile an example spec

```bash
/tmp/forge compile foundry/examples/kartograph/app.foundry.yaml \
  --foundry-path $(pwd) \
  -o /tmp/kartograph-out

cd /tmp/kartograph-out && go build -o app .
```

### Validate a spec

```bash
/tmp/forge lint foundry/examples/dinosaur-registry/app.foundry.yaml
```

### Explain what a spec builds

```bash
/tmp/forge explain foundry/examples/kartograph/app.foundry.yaml
```

## IR Spec Format

All application specs follow `foundry/spec/schema.json`. Validate with `forge lint`.

Key sections:
- `components` — SBOM: pinned trusted component versions
- `resources` — data entities (fields, operations, events)
- `api` — REST and gRPC configuration
- `auth` — JWT or none
- `database` — postgres with auto-migrations
- `observability` — health check and metrics ports
- `hooks` — lifecycle injection points (pre-handler, post-db, etc.)
- `graph` — Apache AGE property graph configuration
- `events` — Kafka/NATS event topics
- `tenancy` — multi-tenant row isolation
- `authz` — SpiceDB fine-grained authorization
- `workflows` — Temporal workflow definitions
- `state` — Redis cache/lock configuration

## Trusted Components (15 total)

| Component | Purpose |
|-----------|---------|
| `foundry-http` | HTTP server, routing, middleware |
| `foundry-postgres` | DB pool, CRUD DAOs, migrations, soft delete |
| `foundry-auth-jwt` | JWT validation, RBAC, JWK integration |
| `foundry-auth-spicedb` | SpiceDB fine-grained authz |
| `foundry-grpc` | gRPC server and interceptors |
| `foundry-health` | Liveness/readiness endpoints |
| `foundry-metrics` | Prometheus metrics |
| `foundry-events` | PostgreSQL LISTEN/NOTIFY |
| `foundry-kafka` | Kafka producer/consumer |
| `foundry-nats` | NATS messaging |
| `foundry-redis` | Cache, rate limiting, distributed locking |
| `foundry-redis-streams` | Redis Streams consumer groups |
| `foundry-graph-age` | Apache AGE graph database |
| `foundry-temporal` | Temporal workflow orchestration |
| `foundry-tenancy` | Row-level multi-tenant isolation |
| `foundry-service-router` | Service mesh routing |

Each component implements `foundry/spec.Component`:
- `AuditHash()` — compiler verifies against registry
- `Configure(ComponentConfig)`, `Register(*Application)`, `Start(ctx)`, `Stop(ctx)`
- Immutable after audit — bug fixes create new versions

## forge CLI Commands

| Command | Flags | Notes |
|---------|-------|-------|
| `forge init <name>` | `--resource`, `--version` | Creates full project dir |
| `forge scaffold` | `--name`, `--resource`, `--output` | Outputs spec YAML |
| `forge lint <spec>` | — | JSON Schema + semantic validation |
| `forge explain <spec>` | — | Human-readable summary |
| `forge compile <spec>` | `--output` (required), `--foundry-path`, `--registry`, `--source` | Main command |
| `forge diff <old> <new>` | — | Structural spec diff |
| `forge sbom <spec>` | `--format` | CycloneDX 1.5 SBOM |
| `forge verify <spec>` | `--source` | Audit hash check |
| `forge deploy <spec>` | `--output`, `--helm` | K8s manifests + Helm chart |

## What NOT to do

- Do not write business logic — it belongs in components or in hooks
- Do not modify component implementations after audit (create a new version)
- Do not add fields to `app.foundry.yaml` not in `foundry/spec/schema.json`
- Do not use `additionalProperties: true` in the schema — strictness is a trust property

## Module

```
github.com/jsell-rh/trusted-software-foundry
```
