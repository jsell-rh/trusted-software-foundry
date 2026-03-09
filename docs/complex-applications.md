# TSC IR Extensions for Complex Applications

**Author:** TSC-Architect
**Branch:** feat/foundry-complex-ir
**Status:** Draft — for CTO review
**Context:** Extends `foundry/spec/schema.json` (v1) to support complex platform applications beyond simple CRUD services. Informed by studying [kartograph](https://github.com/openshift-hyperfleet/kartograph) (bi-temporal property graph platform) as the canonical complex-case target.

---

## Problem Statement

TSC v1 targets CRUD services (trex parity). Real enterprise platforms (kartograph, OCM, ACS) require:

1. **Property graphs** — nodes, edges, bi-directional relationships, Cypher queries (Apache AGE)
2. **Multi-service** — cooperating services with distinct deployment profiles and APIs within one logical application
3. **Event-driven** beyond pg_notify — Kafka, NATS, Redis Pub/Sub, dead-letter queues, schema-versioned events
4. **External authorization** — SpiceDB (ReBAC / Zanzibar-model), OPA, attribute-based access control
5. **External state** — Redis (cache, rate limiting, distributed locking, ephemeral queues)
6. **Bi-temporality** — valid time + transaction time tracking for audit and point-in-time queries
7. **Multi-tenancy** — tenant-scoped data, isolated namespaces, per-tenant config overrides

The AI should still write only the IR. TSC v2 components handle all implementation complexity.

---

## Design Principles

1. **IR stays declarative.** No procedural logic enters the spec file — ever.
2. **Backward compatible.** All v1 specs remain valid. Extensions are opt-in blocks.
3. **Component-driven.** Each extension maps to one or more new trusted components. No ad-hoc integrations.
4. **Escape hatch is explicit.** Custom code hooks are declared in the IR, not implicit.

---

## IR Extension Blocks

### 1. Graph Data (`graph:`)

For applications that store and query property graphs (nodes, edges, Cypher).

```yaml
graph:
  backend: age          # "age" (Apache AGE / PostgreSQL) | "neo4j" (future)

  node_types:
    - name: Resource
      labels: [resource]
      properties:
        - name: slug
          type: string
          required: true
          indexed: true
        - name: kind
          type: string
          required: true
        - name: data_source_id
          type: string
          system: true          # managed by platform, not user-editable

    - name: DataSource
      labels: [data_source]
      properties:
        - name: name
          type: string
          required: true

  edge_types:
    - name: Owns
      from: DataSource
      to: Resource
      properties:
        - name: weight
          type: float

    - name: RelatesTo
      from: Resource
      to: Resource
      directed: false           # undirected edge

  mutations:
    # Allowed operation types on graph entities
    operations: [DEFINE, CREATE, UPDATE, DELETE]
    bulk_loading: true          # enable bulk JSONL mutation loading (like kartograph)
    mutation_format: jsonl      # "jsonl" | "json"

  queries:
    language: cypher            # "cypher" (AGE) | "gremlin" (future)
    max_depth: 10               # max traversal depth (compiler enforces in generated code)
    expose_api: true            # generate REST+gRPC query endpoints
```

**New trusted component:** `foundry-graph-age` v1.x

---

### 2. Multi-Service (`services:`)

For applications composed of cooperating services (e.g. API + background worker + event consumer).

```yaml
services:
  - name: api
    role: rest-api
    port: 8000
    components:
      - foundry-http
      - foundry-auth-jwt
      - foundry-postgres
      - foundry-graph-age
    resources: [Resource, DataSource]    # which resources this service owns

  - name: mutation-worker
    role: worker
    components:
      - foundry-postgres
      - foundry-graph-age
      - foundry-kafka-consumer
    triggers:
      - event: graph.mutation.requested
        handler: apply_mutation           # compiler generates dispatch to foundry-graph-age

  - name: metrics-server
    role: metrics
    port: 8080
    components:
      - foundry-metrics
```

The compiler generates a separate `main_<service>.go` per service. A single `docker-compose.yaml` (dev) and `deploy/` tree (prod) is also generated.

**New trusted component:** `foundry-service-router` v1.x (handles inter-service discovery)

---

### 3. Advanced Events (`events:`)

Replaces the v1 `resources[].events: true` (pg_notify only) with a full event bus declaration.

```yaml
events:
  backend: kafka              # "kafka" | "nats" | "redis-streams" | "pg-notify" (v1 default)

  broker:
    url: "${KAFKA_BOOTSTRAP_SERVERS}"

  schema_registry:
    url: "${SCHEMA_REGISTRY_URL}"
    format: avro              # "avro" | "json" | "protobuf"

  topics:
    - name: graph.mutation.requested
      partitions: 12
      replication: 3
      retention_hours: 168
      schema: MutationRequest   # references a resource or inline schema

    - name: graph.mutation.applied
      partitions: 12
      replication: 3
      schema: MutationResult

    - name: graph.mutation.dlq
      role: dead-letter-queue
      source: graph.mutation.requested

  producers:
    - service: api
      topics: [graph.mutation.requested]

  consumers:
    - service: mutation-worker
      topics: [graph.mutation.requested]
      group_id: "${APP_NAME}-mutation-worker"
      error_topic: graph.mutation.dlq
```

**New trusted components:** `foundry-kafka` v1.x, `foundry-nats` v1.x, `foundry-redis-streams` v1.x

---

### 4. External Authorization (`authz:`)

For applications requiring fine-grained, relationship-based access control (SpiceDB / Zanzibar).

```yaml
authz:
  backend: spicedb            # "spicedb" | "opa" | "casbin"

  spicedb:
    endpoint: "${SPICEDB_ENDPOINT}"
    token: "${SPICEDB_TOKEN}"
    tls: true

  schema_file: authz/schema.zed   # path to SpiceDB schema within the repo (not generated)

  enforcement:
    default: deny             # "deny" | "allow" — default when no rule matches

  policies:
    # Binds resource operations to SpiceDB permission checks
    - resource: Resource
      operations:
        read:   "can_view"
        create: "can_create"
        update: "can_edit"
        delete: "can_delete"
      subject_type: user
      object_type: resource
```

**New trusted component:** `foundry-auth-spicedb` v1.x

---

### 5. External State (`state:`)

For Redis-backed caching, rate limiting, distributed locking, and ephemeral queues.

```yaml
state:
  backends:
    - name: cache
      type: redis
      url: "${REDIS_CACHE_URL}"
      default_ttl: 300        # seconds

    - name: ratelimit
      type: redis
      url: "${REDIS_RATELIMIT_URL}"

    - name: locks
      type: redis
      url: "${REDIS_LOCKS_URL}"

  uses:
    - cache: cache
      resources: [Resource]   # cache GET responses for these resources

    - rate_limit: ratelimit
      routes: ["/api/v1/graph/mutations"]
      requests_per_second: 100
      burst: 200

    - distributed_lock: locks
      resources: [Resource]
      operations: [update, delete]   # acquire lock before mutating
```

**New trusted component:** `foundry-redis` v1.x

---

### 6. Bi-Temporality (`temporal:`)

For applications that require point-in-time queries and full audit history.

```yaml
temporal:
  enabled: true

  valid_time:
    field: valid_from         # timestamp field marking when data is "true in the world"

  transaction_time:
    auto: true                # system-managed; compiler generates trigger/middleware

  resources: [Resource]       # which resources get bi-temporal tracking

  query_api:
    as_of_param: "as_of"      # ?as_of=2025-01-01T00:00:00Z
    between_param: "between"  # ?between=2024-01-01,2025-01-01
```

**New trusted component:** `foundry-temporal` v1.x

---

### 7. Multi-Tenancy (`tenancy:`)

For SaaS platforms where each tenant's data is isolated.

```yaml
tenancy:
  model: schema               # "schema" (pg schema per tenant) | "row" (tenant_id column) | "database"

  tenant_identifier:
    source: jwt_claim         # "jwt_claim" | "header" | "subdomain"
    claim: "tenant_id"

  resources: all              # "all" | list of resource names

  admin_bypass:
    role: "platform-admin"    # JWT role that bypasses tenant filtering
```

**New trusted component:** `foundry-tenancy` v1.x

---

### 8. Custom Code Hooks (`hooks:`)

The escape hatch. When a trusted component cannot satisfy a requirement, custom Go code can be registered at well-defined hook points. The AI declares the hook in the IR; a human engineer implements it.

```yaml
hooks:
  # Hook into the request lifecycle
  - name: enrich-mutation-context
    point: pre-handler           # "pre-handler" | "post-handler" | "pre-db" | "post-db"
    service: api
    routes: ["/api/v1/graph/mutations"]
    implementation: hooks/enrich_mutation.go   # path to custom Go file (human-written)

  # Hook into the event pipeline
  - name: validate-mutation-schema
    point: pre-publish
    topic: graph.mutation.requested
    implementation: hooks/validate_mutation_schema.go
```

The compiler copies `hooks/*.go` into the generated project without modification. This is the ONLY way custom code enters a TSC application.

---

## Extended Schema Design (foundry/spec/schema.json)

New top-level optional blocks added to the v1 JSON Schema:

```json
{
  "graph":   { "$ref": "#/$defs/graphConfig" },
  "services":{ "type": "array", "items": { "$ref": "#/$defs/service" } },
  "events":  { "$ref": "#/$defs/eventsConfig" },
  "authz":   { "$ref": "#/$defs/authzConfig" },
  "state":   { "$ref": "#/$defs/stateConfig" },
  "temporal":{ "$ref": "#/$defs/temporalConfig" },
  "tenancy": { "$ref": "#/$defs/tenancyConfig" },
  "hooks":   { "type": "array", "items": { "$ref": "#/$defs/hook" } }
}
```

All blocks are `additionalProperties: false` (same discipline as v1).

New component registry entries (all require audit before use):

| Component | Provides | Dependencies |
|-----------|----------|--------------|
| `foundry-graph-age` | Apache AGE graph CRUD, Cypher query API | `foundry-postgres` |
| `foundry-kafka` | Kafka producer/consumer, schema registry | — |
| `foundry-nats` | NATS JetStream producer/consumer | — |
| `foundry-redis-streams` | Redis Streams producer/consumer | `foundry-redis` |
| `foundry-redis` | Redis cache, rate limiting, distributed locks | — |
| `foundry-auth-spicedb` | SpiceDB ReBAC enforcement middleware | `foundry-auth-jwt` |
| `foundry-temporal` | Bi-temporal table management, AS OF queries | `foundry-postgres` |
| `foundry-tenancy` | Tenant isolation (schema/row/database model) | `foundry-postgres` |
| `foundry-service-router` | Inter-service discovery and gRPC routing | — |

---

## Kartograph in TSC IR

Using the v2 IR, the kartograph application would be described as:

```yaml
apiVersion: foundry/v1
kind: Application
metadata:
  name: kartograph
  version: 1.0.0

components:
  foundry-http:         v2.0.0
  foundry-postgres:     v1.0.0
  foundry-auth-jwt:     v1.0.0
  foundry-auth-spicedb: v1.0.0
  foundry-graph-age:    v1.0.0
  foundry-kafka:        v1.0.0
  foundry-health:       v1.0.0
  foundry-metrics:      v1.0.0
  foundry-redis:        v1.0.0
  foundry-tenancy:      v1.0.0

graph:
  backend: age
  node_types:
    - name: Resource
      labels: [resource]
      properties:
        - { name: slug,           type: string,  required: true, indexed: true }
        - { name: kind,           type: string,  required: true }
        - { name: data_source_id, type: string,  system: true }
        - { name: source_path,    type: string,  system: true }
    - name: DataSource
      labels: [data_source]
      properties:
        - { name: name, type: string, required: true }
  edge_types:
    - name: Owns
      from: DataSource
      to:   Resource
    - name: RelatesTo
      from: Resource
      to:   Resource
      directed: false
  mutations:
    operations: [DEFINE, CREATE, UPDATE, DELETE]
    bulk_loading: true
    mutation_format: jsonl
  queries:
    language: cypher
    expose_api: true

services:
  - name: api
    role: rest-api
    port: 8000
    components: [foundry-http, foundry-auth-jwt, foundry-auth-spicedb, foundry-postgres, foundry-graph-age, foundry-redis]
  - name: mutation-worker
    role: worker
    components: [foundry-postgres, foundry-graph-age, foundry-kafka]
    triggers:
      - event: graph.mutation.requested
        handler: apply_mutation

events:
  backend: kafka
  broker:
    url: "${KAFKA_BOOTSTRAP_SERVERS}"
  topics:
    - { name: graph.mutation.requested, partitions: 12, replication: 3 }
    - { name: graph.mutation.applied,   partitions: 12, replication: 3 }
    - { name: graph.mutation.dlq, role: dead-letter-queue, source: graph.mutation.requested }
  producers:
    - { service: api,             topics: [graph.mutation.requested] }
  consumers:
    - { service: mutation-worker, topics: [graph.mutation.requested], group_id: "${APP_NAME}-worker" }

authz:
  backend: spicedb
  spicedb:
    endpoint: "${SPICEDB_ENDPOINT}"
    token: "${SPICEDB_TOKEN}"
    tls: true
  schema_file: authz/schema.zed
  enforcement:
    default: deny

state:
  backends:
    - { name: cache, type: redis, url: "${REDIS_URL}", default_ttl: 300 }
  uses:
    - rate_limit: cache
      routes: ["/api/v1/graph/mutations"]
      requests_per_second: 100

tenancy:
  model: row
  tenant_identifier:
    source: jwt_claim
    claim: "tenant_id"
  resources: all

auth:
  type: jwt
  jwk_url: "${JWK_CERT_URL}"
  required: true

database:
  type: postgres
  migrations: auto

observability:
  health_check: { port: 8083 }
  metrics: { port: 8080, path: /metrics }
```

This single YAML file describes kartograph's entire architecture. The AI writes it; TSC compiles it.

---

## Implementation Roadmap

| Phase | Deliverable | Owner | Blocks |
|-------|-------------|-------|--------|
| 1 | `schema-v2.json` — all extension blocks formalized | TSC-Architect | TSC-Compiler v2 |
| 2 | `foundry-graph-age` component | TSC-Library | TSC-Compiler v2 |
| 3 | `foundry-kafka`, `foundry-redis` components | TSC-Library | Phase 4 |
| 4 | `foundry-auth-spicedb`, `foundry-tenancy` components | TSC-Library | Phase 5 |
| 5 | TSC-Compiler v2: multi-service codegen, kafka wiring | TSC-Compiler | Phase 6 |
| 6 | kartograph parity: compile kartograph spec end-to-end | All | Done |

---

## Open Questions for CTO

1. **[?BOSS]** Should `schema-v2.json` extend v1 via `$ref` inheritance or replace it with a unified schema? Extending is cleaner for backward compat but adds schema complexity.
2. **[?BOSS]** Should `hooks:` be a v2 feature or a v1 hotfix? Some teams need it immediately.
3. **[?BOSS]** Should `foundry-temporal` use PostgreSQL temporal tables (range types) or a dedicated table-pair pattern (current/history tables)?
4. **[?BOSS]** Is Neo4j a v2 target or future? Impacts whether `graph.backend` is an enum or extensible.
