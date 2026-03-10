# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Project Overview

This is the **Trusted Software Foundry** — an IR-first application platform where AI agents write declarative YAML specs (`app.foundry.yaml`) and a deterministic compiler assembles working Go applications from pre-audited, version-pinned trusted components.

**AI agents write specs, not code.** The compiler does the rest.

## Key Directories

```
foundry/spec/           Core interfaces: Component, Application, Registrar, JSON Schema
foundry/components/     Trusted component library (7 components)
foundry/compiler/       Foundry compiler: parse → resolve → generate
foundry/examples/       Reference applications
cmd/forge/            forge CLI entrypoint
FOUNDRY-ARCHITECTURE.md Full architecture reference
```

## Development Commands

### Build the Foundry compiler

```bash
go build -o /tmp/forge ./cmd/forge
```

### Test

```bash
go test ./foundry/...
```

### Compile an example spec

```bash
/tmp/forge compile foundry/examples/dinosaur-registry/app.foundry.yaml \
  --trusted-software-foundry $(pwd) \
  -o /tmp/dinosaur-out

cd /tmp/dinosaur-out && go build -o app .
```

### Validate a spec

```bash
/tmp/forge validate foundry/examples/dinosaur-registry/app.foundry.yaml
```

## IR Spec Format

All application specs follow `foundry/spec/schema.json`. Validate with `forge validate`.

Key sections:
- `components` — SBOM: pinned trusted component versions
- `resources` — data entities (fields, operations, events)
- `api` — REST and gRPC configuration
- `auth` — JWT or none
- `database` — postgres with auto-migrations
- `observability` — health check and metrics ports

## Trusted Components

Components implement `foundry/spec.Component`. Each component:
- Has an `AuditHash()` that the compiler verifies against the registry
- Implements `Configure(ComponentConfig)`, `Register(*Application)`, `Start(ctx)`, `Stop(ctx)`
- Is immutable after audit — bug fixes create new versions

To add a new component: implement `spec.Component`, add an audit record to the registry, and add the component name to the JSON Schema's `components.propertyNames.enum`.

## What NOT to do

- Do not write business logic code — it belongs in components or in the IR spec
- Do not modify component implementations after they've been audited (create a new version)
- Do not add fields to `app.foundry.yaml` that aren't in `foundry/spec/schema.json` — the compiler validates strictly
- Do not use `additionalProperties: true` in the schema — strictness is a trust property

## Legacy Code

The `cmd/trex/`, `pkg/`, `plugins/`, `scripts/`, and `templates/` directories contain the original rh-trex template code that predates this project. They are kept as historical reference only. **Do not extend or use them for new work.** All new development goes in `foundry/`.

## Module

```
github.com/jsell-rh/trusted-software-foundry
```
