package compiler

// validation_test.go covers the semantic cross-check validation rules in
// tsc/spec/validate.go that are not covered by parser_test.go.
// Rules tested here:
//   - bi_temporal block component and database requirements
//   - workflows block component and field requirements
//   - Component cross-references for every advanced feature block
//   - Hook validation (point, implementation pattern, routes/topic scope, duplicates)
//   - Graph edge cross-reference against declared node_type labels

import (
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// Helper: minimal valid base spec to build on
// --------------------------------------------------------------------------

// baseMinimal is the smallest valid spec — just metadata + one component.
// Tests extend it by appending YAML for the block under test.
const baseMinimal = `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`

// baseWithPostgres adds foundry-postgres and a database block + one resource
// to baseMinimal; needed whenever testing blocks that require persistence.
const baseWithPostgres = `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
`

// --------------------------------------------------------------------------
// bi_temporal block
// --------------------------------------------------------------------------

func TestParse_BiTemporalRequiresTemporal(t *testing.T) {
	// bi_temporal.enabled=true but foundry-temporal not declared → error
	spec := writeTempSpec(t, baseWithPostgres+`bi_temporal:
  enabled: true
  valid_time:
    field: valid_from
  transaction_time:
    auto: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error when bi_temporal.enabled=true without foundry-temporal, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-temporal") {
		t.Errorf("expected error to mention 'foundry-temporal', got: %v", err)
	}
}

func TestParse_BiTemporalRequiresDatabase(t *testing.T) {
	// bi_temporal.enabled=true but no database block → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-temporal: v1.0.0
bi_temporal:
  enabled: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error when bi_temporal enabled without database block, got nil")
	}
	if !strings.Contains(err.Error(), "bi_temporal") {
		t.Errorf("expected error to mention 'bi_temporal', got: %v", err)
	}
}

func TestParse_BiTemporalDisabled_NoRequirements(t *testing.T) {
	// bi_temporal.enabled=false does not require foundry-temporal or database
	spec := writeTempSpec(t, baseMinimal+`bi_temporal:
  enabled: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err != nil {
		t.Fatalf("expected no error for disabled bi_temporal, got: %v", err)
	}
}

func TestParse_BiTemporalValid(t *testing.T) {
	// Full bi_temporal config with all required components and database block
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-temporal: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Record
    plural: records
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
bi_temporal:
  enabled: true
  valid_time:
    field: valid_from
  transaction_time:
    auto: true
  resources: all
  query_api:
    as_of_param: as_of
    between_param: between
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err != nil {
		t.Fatalf("expected valid bi_temporal spec to parse without error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// workflows block
// --------------------------------------------------------------------------

func TestParse_WorkflowsRequiresTemporal(t *testing.T) {
	// workflows block without foundry-temporal declared → error
	spec := writeTempSpec(t, baseWithPostgres+`workflows:
  namespace: app-ns
  worker_queue: app-queue
  workflows:
    - name: ProcessItem
      trigger: create
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for workflows without foundry-temporal, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-temporal") {
		t.Errorf("expected error to mention 'foundry-temporal', got: %v", err)
	}
}

func TestParse_WorkflowsMissingNamespace(t *testing.T) {
	// workflows block missing namespace → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-temporal: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
workflows:
  worker_queue: app-queue
  workflows:
    - name: ProcessItem
      trigger: create
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for workflows.namespace missing, got nil")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Errorf("expected error to mention 'namespace', got: %v", err)
	}
}

func TestParse_WorkflowsMissingWorkerQueue(t *testing.T) {
	// workflows block missing worker_queue → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-temporal: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
workflows:
  namespace: app-ns
  workflows:
    - name: ProcessItem
      trigger: create
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for workflows.worker_queue missing, got nil")
	}
	if !strings.Contains(err.Error(), "worker_queue") {
		t.Errorf("expected error to mention 'worker_queue', got: %v", err)
	}
}

func TestParse_WorkflowsValid(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-temporal: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
workflows:
  namespace: app-ns
  worker_queue: app-queue
  workflows:
    - name: ProcessItem
      trigger: create
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err != nil {
		t.Fatalf("expected valid workflows spec to parse without error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Component cross-reference: advanced feature blocks
// --------------------------------------------------------------------------

func TestParse_ResourceEventsRequireFoundryEvents(t *testing.T) {
	// A resource with events:true requires foundry-events in components
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for events:true without foundry-events, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-events") {
		t.Errorf("expected error to mention 'foundry-events', got: %v", err)
	}
}

func TestParse_DatabaseRequiresFoundryPostgres(t *testing.T) {
	// database block without foundry-postgres in components → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for database block without foundry-postgres, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-postgres") {
		t.Errorf("expected error to mention 'foundry-postgres', got: %v", err)
	}
}

func TestParse_ResourcesWithoutDatabaseBlock(t *testing.T) {
	// resources declared but no database block → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for resources without database block, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error to mention 'database', got: %v", err)
	}
}

func TestParse_GRPCRequiresFoundryGRPC(t *testing.T) {
	// api.grpc.enabled=true without foundry-grpc → error
	spec := writeTempSpec(t, baseMinimal+`api:
  grpc:
    enabled: true
    port: 9000
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for grpc.enabled without foundry-grpc, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-grpc") {
		t.Errorf("expected error to mention 'foundry-grpc', got: %v", err)
	}
}

func TestParse_GraphBackendRequiresFoundryGraphAge(t *testing.T) {
	// graph.backend=age without foundry-graph-age → error
	spec := writeTempSpec(t, baseWithPostgres+`graph:
  backend: age
  graph_name: test_graph
  node_types:
    - label: Item
      id_field: id
      properties: [id]
  edge_types: []
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for graph.backend=age without foundry-graph-age, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-graph-age") {
		t.Errorf("expected error to mention 'foundry-graph-age', got: %v", err)
	}
}

func TestParse_EventsKafkaRequiresFoundryKafka(t *testing.T) {
	// events.backend=kafka without foundry-kafka → error
	spec := writeTempSpec(t, baseWithPostgres+`events:
  backend: kafka
  topics:
    - name: app.items
      partitions: 3
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for events.backend=kafka without foundry-kafka, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-kafka") {
		t.Errorf("expected error to mention 'foundry-kafka', got: %v", err)
	}
}

func TestParse_EventsNATSRequiresFoundryNATS(t *testing.T) {
	// events.backend=nats without foundry-nats → error
	spec := writeTempSpec(t, baseWithPostgres+`events:
  backend: nats
  topics:
    - name: app.items
      partitions: 1
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for events.backend=nats without foundry-nats, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-nats") {
		t.Errorf("expected error to mention 'foundry-nats', got: %v", err)
	}
}

func TestParse_EventsRedisStreamsRequiresFoundryRedis(t *testing.T) {
	// events.backend=redis-streams without foundry-redis-streams → error
	spec := writeTempSpec(t, baseWithPostgres+`events:
  backend: redis-streams
  topics:
    - name: app.items
      partitions: 1
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for events.backend=redis-streams without foundry-redis-streams/foundry-redis, got nil")
	}
	// Error message should mention one of the required components
	if !strings.Contains(err.Error(), "foundry-redis") {
		t.Errorf("expected error to mention 'foundry-redis', got: %v", err)
	}
}

func TestParse_AuthzSpiceDBRequiresFoundryAuthSpiceDB(t *testing.T) {
	// authz.backend=spicedb without foundry-auth-spicedb → error
	spec := writeTempSpec(t, baseWithPostgres+`authz:
  backend: spicedb
  relations:
    - resource: Item
      relation: owner
      subject: User
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for authz.backend=spicedb without foundry-auth-spicedb, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-auth-spicedb") {
		t.Errorf("expected error to mention 'foundry-auth-spicedb', got: %v", err)
	}
}

func TestParse_StateRequiresFoundryRedis(t *testing.T) {
	// state block without foundry-redis → error
	spec := writeTempSpec(t, baseWithPostgres+`state:
  backend: redis
  keys:
    - name: item_lock
      strategy: distributed_lock
      ttl_seconds: 60
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for state block without foundry-redis, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-redis") {
		t.Errorf("expected error to mention 'foundry-redis', got: %v", err)
	}
}

func TestParse_TenancyRequiresFoundryTenancy(t *testing.T) {
	// tenancy block without foundry-tenancy → error
	spec := writeTempSpec(t, baseWithPostgres+`tenancy:
  field: org_id
  strategy: row
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for tenancy block without foundry-tenancy, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-tenancy") {
		t.Errorf("expected error to mention 'foundry-tenancy', got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Hook validation
// --------------------------------------------------------------------------

func TestParse_HookUnknownPoint(t *testing.T) {
	// Hook with an unknown lifecycle point → error
	spec := writeTempSpec(t, baseWithPostgres+`hooks:
  - name: my-hook
    point: invalid-point
    implementation: hooks/my_hook.go
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for hook with unknown point, got nil")
	}
	if !strings.Contains(err.Error(), "point") {
		t.Errorf("expected error to mention 'point', got: %v", err)
	}
}

func TestParse_HookInvalidImplementationPattern(t *testing.T) {
	// Hook implementation not matching hooks/*.go → error
	spec := writeTempSpec(t, baseWithPostgres+`hooks:
  - name: my-hook
    point: pre-db
    implementation: src/my_hook.go
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for hook implementation not in hooks/*.go, got nil")
	}
	if !strings.Contains(err.Error(), "implementation") {
		t.Errorf("expected error to mention 'implementation', got: %v", err)
	}
}

func TestParse_HookDuplicateName(t *testing.T) {
	// Two hooks with the same name → error
	spec := writeTempSpec(t, baseWithPostgres+`hooks:
  - name: audit
    point: pre-db
    implementation: hooks/audit.go
  - name: audit
    point: post-db
    implementation: hooks/audit2.go
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for duplicate hook name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected error to mention 'duplicate', got: %v", err)
	}
}

func TestParse_HookRoutesOnlyValidForHandlerHooks(t *testing.T) {
	// routes field on a pre-db hook (not a handler hook) → error
	spec := writeTempSpec(t, baseWithPostgres+`hooks:
  - name: my-hook
    point: pre-db
    routes: ["/api/v1/items"]
    implementation: hooks/my_hook.go
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for routes on a non-handler hook, got nil")
	}
	if !strings.Contains(err.Error(), "routes") {
		t.Errorf("expected error to mention 'routes', got: %v", err)
	}
}

func TestParse_HookTopicOnlyValidForEventHooks(t *testing.T) {
	// topic field on a pre-db hook (not an event hook) → error
	spec := writeTempSpec(t, baseWithPostgres+`hooks:
  - name: my-hook
    point: pre-db
    topic: app.items
    implementation: hooks/my_hook.go
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for topic on a non-event hook, got nil")
	}
	if !strings.Contains(err.Error(), "topic") {
		t.Errorf("expected error to mention 'topic', got: %v", err)
	}
}

func TestParse_HookPreDBRequiresDatabaseBlock(t *testing.T) {
	// pre-db hook without a database block → error
	spec := writeTempSpec(t, baseMinimal+`hooks:
  - name: my-hook
    point: pre-db
    implementation: hooks/my_hook.go
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for pre-db hook without database block, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error to mention 'database', got: %v", err)
	}
}

func TestParse_HookPostDBRequiresDatabaseBlock(t *testing.T) {
	// post-db hook without a database block → error
	spec := writeTempSpec(t, baseMinimal+`hooks:
  - name: my-hook
    point: post-db
    implementation: hooks/my_hook.go
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for post-db hook without database block, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error to mention 'database', got: %v", err)
	}
}

func TestParse_HookValidPoints(t *testing.T) {
	// All 6 valid lifecycle points should be accepted.
	// pre-publish and post-consume also need foundry-events.
	handlerPoints := []string{"pre-handler", "post-handler"}
	for _, point := range handlerPoints {
		t.Run(point, func(t *testing.T) {
			spec := writeTempSpec(t, baseMinimal+`hooks:
  - name: my-hook
    point: `+point+`
    implementation: hooks/my_hook.go
`)
			_, err := ParseWithSchema(spec, schemaPathForTest())
			if err != nil {
				t.Errorf("hook point %q should be valid, got error: %v", point, err)
			}
		})
	}

	// DB hooks need database + postgres
	dbPoints := []string{"pre-db", "post-db"}
	for _, point := range dbPoints {
		t.Run(point, func(t *testing.T) {
			spec := writeTempSpec(t, baseWithPostgres+`hooks:
  - name: my-hook
    point: `+point+`
    implementation: hooks/my_hook.go
`)
			_, err := ParseWithSchema(spec, schemaPathForTest())
			if err != nil {
				t.Errorf("hook point %q should be valid with database, got error: %v", point, err)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Graph edge cross-reference validation
// --------------------------------------------------------------------------

func TestParse_GraphEdgeFromNotDeclaredNodeType(t *testing.T) {
	// edge.from references a node label not in node_types → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres:   v1.0.0
  foundry-graph-age:  v1.0.0
  foundry-http:       v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
graph:
  backend: age
  graph_name: test_graph
  node_types:
    - label: Item
      id_field: id
  edge_types:
    - label: HAS_TAG
      from: UndeclaredType
      to: Item
      directed: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for edge.from referencing undeclared node type, got nil")
	}
	if !strings.Contains(err.Error(), "UndeclaredType") {
		t.Errorf("expected error to mention the undeclared node type, got: %v", err)
	}
}

func TestParse_GraphEdgeToNotDeclaredNodeType(t *testing.T) {
	// edge.to references a node label not in node_types → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres:   v1.0.0
  foundry-graph-age:  v1.0.0
  foundry-http:       v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
graph:
  backend: age
  graph_name: test_graph
  node_types:
    - label: Item
      id_field: id
  edge_types:
    - label: HAS_TAG
      from: Item
      to: UndeclaredTag
      directed: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for edge.to referencing undeclared node type, got nil")
	}
	if !strings.Contains(err.Error(), "UndeclaredTag") {
		t.Errorf("expected error to mention the undeclared node type, got: %v", err)
	}
}

func TestParse_GraphEdgeMissingFromField(t *testing.T) {
	// edge_type missing required from field → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres:   v1.0.0
  foundry-graph-age:  v1.0.0
  foundry-http:       v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
graph:
  backend: age
  graph_name: test_graph
  node_types:
    - label: Item
      id_field: id
  edge_types:
    - label: HAS_SELF
      to: Item
      directed: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for edge_type missing from field, got nil")
	}
	if !strings.Contains(err.Error(), "from") {
		t.Errorf("expected error to mention 'from', got: %v", err)
	}
}

func TestParse_GraphEdgeMissingToField(t *testing.T) {
	// edge_type missing required to field → error
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres:   v1.0.0
  foundry-graph-age:  v1.0.0
  foundry-http:       v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
graph:
  backend: age
  graph_name: test_graph
  node_types:
    - label: Item
      id_field: id
  edge_types:
    - label: HAS_SELF
      from: Item
      directed: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for edge_type missing to field, got nil")
	}
	if !strings.Contains(err.Error(), "to") {
		t.Errorf("expected error to mention 'to', got: %v", err)
	}
}

func TestParse_GraphValid(t *testing.T) {
	// Complete valid graph spec with cross-referenced node types
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres:   v1.0.0
  foundry-graph-age:  v1.0.0
  foundry-http:       v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
  - name: Tag
    plural: tags
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
graph:
  backend: age
  graph_name: test_graph
  node_types:
    - label: Item
      id_field: id
      properties: [id]
    - label: Tag
      id_field: id
      properties: [id]
  edge_types:
    - label: HAS_TAG
      from: Item
      to: Tag
      directed: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err != nil {
		t.Fatalf("expected valid graph spec to parse without error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Resource naming and structure validation
// --------------------------------------------------------------------------

func TestParse_ResourceNameNotPascalCase(t *testing.T) {
	// Resource name must be PascalCase — lowercase is rejected.
	// The JSON schema enforces this with a regex pattern; the error message
	// mentions the offending value and "pattern" rather than "PascalCase".
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: widget
    plural: widgets
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for non-PascalCase resource name, got nil")
	}
	// Schema validator fires: "widget" does not match pattern ^[A-Z][a-zA-Z0-9]*$
	// Semantic validator fires: "name must be PascalCase"
	// Accept either message — both indicate correct enforcement.
	if !strings.Contains(err.Error(), "widget") && !strings.Contains(err.Error(), "PascalCase") {
		t.Errorf("expected error to mention the bad value or 'PascalCase', got: %v", err)
	}
}

func TestParse_ResourcePluralNotLowercase(t *testing.T) {
	// Resource plural must be lowercase-kebab — PascalCase plural is rejected.
	spec := writeTempSpec(t, baseWithPostgres+`resources:
  - name: Widget
    plural: Widgets
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for non-lowercase plural, got nil")
	}
	if !strings.Contains(err.Error(), "plural") {
		t.Errorf("expected error to mention 'plural', got: %v", err)
	}
}

func TestParse_DuplicateResourceNames(t *testing.T) {
	// Two resources with the same name → error.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: id
        type: uuid
    operations: [create]
    events: false
  - name: Widget
    plural: widget-copies
    fields:
      - name: id
        type: uuid
    operations: [read]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for duplicate resource names, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected error to mention 'duplicate', got: %v", err)
	}
}

func TestParse_ResourceMustHaveAtLeastOneField(t *testing.T) {
	// A resource with no fields at all → error.
	spec := writeTempSpec(t, baseWithPostgres+`resources:
  - name: Empty
    plural: empties
    fields: []
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for resource with no fields, got nil")
	}
	if !strings.Contains(err.Error(), "field") {
		t.Errorf("expected error to mention 'field', got: %v", err)
	}
}

func TestParse_ResourceMustHaveAtLeastOneOperation(t *testing.T) {
	// A resource with an empty operations list → error.
	spec := writeTempSpec(t, baseWithPostgres+`resources:
  - name: NoOp
    plural: noops
    fields:
      - name: id
        type: uuid
    operations: []
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for resource with no operations, got nil")
	}
	if !strings.Contains(err.Error(), "operation") {
		t.Errorf("expected error to mention 'operation', got: %v", err)
	}
}

func TestParse_ResourceUnknownOperation(t *testing.T) {
	// An operation not in [create, read, update, delete, list] → error.
	spec := writeTempSpec(t, baseWithPostgres+`resources:
  - name: Widget
    plural: widgets
    fields:
      - name: id
        type: uuid
    operations: [create, patch]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for unknown operation 'patch', got nil")
	}
	if !strings.Contains(err.Error(), "patch") {
		t.Errorf("expected error to mention 'patch', got: %v", err)
	}
}

func TestParse_ResourceDuplicateOperation(t *testing.T) {
	// Duplicate operations in the list → error.
	spec := writeTempSpec(t, baseWithPostgres+`resources:
  - name: Widget
    plural: widgets
    fields:
      - name: id
        type: uuid
    operations: [create, create, read]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for duplicate operation 'create', got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected error to mention 'duplicate', got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Field naming and structure validation
// --------------------------------------------------------------------------

func TestParse_FieldNameNotSnakeCase(t *testing.T) {
	// Field name must be snake_case — camelCase is rejected.
	// The JSON schema enforces this with a regex pattern.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: myField
        type: string
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for camelCase field name, got nil")
	}
	// Schema: "myField" does not match pattern ^[a-z][a-z0-9_]*$
	// Semantic: "name must be snake_case"
	if !strings.Contains(err.Error(), "myField") && !strings.Contains(err.Error(), "snake_case") {
		t.Errorf("expected error to mention the bad field name or 'snake_case', got: %v", err)
	}
}

func TestParse_DuplicateFieldNames(t *testing.T) {
	// Two fields with the same name in one resource → error.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: label
        type: string
      - name: label
        type: string
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for duplicate field names, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected error to mention 'duplicate', got: %v", err)
	}
}

func TestParse_MaxLengthOnNonStringField(t *testing.T) {
	// max_length is only valid on string fields — applying it to uuid → error.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: ref_id
        type: uuid
        max_length: 36
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for max_length on non-string field, got nil")
	}
	if !strings.Contains(err.Error(), "max_length") {
		t.Errorf("expected error to mention 'max_length', got: %v", err)
	}
}

func TestParse_InvalidAutoValue(t *testing.T) {
	// auto must be 'created' or 'updated' — any other value is rejected.
	spec := writeTempSpec(t, baseWithPostgres+`resources:
  - name: Widget
    plural: widgets
    fields:
      - name: ts
        type: timestamp
        auto: modified
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for invalid auto value 'modified', got nil")
	}
	if !strings.Contains(err.Error(), "auto") {
		t.Errorf("expected error to mention 'auto', got: %v", err)
	}
}

func TestParse_SoftDeleteMustBeTimestamp(t *testing.T) {
	// soft_delete fields must have type 'timestamp' — using string → error.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: deleted_at
        type: string
        soft_delete: true
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for soft_delete field with non-timestamp type, got nil")
	}
	if !strings.Contains(err.Error(), "soft_delete") {
		t.Errorf("expected error to mention 'soft_delete', got: %v", err)
	}
}

func TestParse_AtMostOneSoftDeleteField(t *testing.T) {
	// Only one soft_delete field is allowed per resource.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-http:     v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: deleted_at
        type: timestamp
        soft_delete: true
      - name: removed_at
        type: timestamp
        soft_delete: true
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for two soft_delete fields, got nil")
	}
	if !strings.Contains(err.Error(), "soft_delete") {
		t.Errorf("expected error to mention 'soft_delete', got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Auth block validation
// --------------------------------------------------------------------------

func TestParse_AuthJWTRequiresJWKURL(t *testing.T) {
	// auth.type=jwt requires auth.jwk_url to be set.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-auth-jwt: v1.0.0
auth:
  type: jwt
  required: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for auth.type=jwt without jwk_url, got nil")
	}
	if !strings.Contains(err.Error(), "jwk_url") {
		t.Errorf("expected error to mention 'jwk_url', got: %v", err)
	}
}

func TestParse_AuthJWTRequiresFoundryAuthJWT(t *testing.T) {
	// auth.type=jwt requires foundry-auth-jwt in components.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
auth:
  type: jwt
  jwk_url: https://example.com/.well-known/jwks.json
  required: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for auth.type=jwt without foundry-auth-jwt, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-auth-jwt") {
		t.Errorf("expected error to mention 'foundry-auth-jwt', got: %v", err)
	}
}

func TestParse_AuthJWTValid(t *testing.T) {
	// Complete valid auth.type=jwt configuration.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-auth-jwt: v1.0.0
auth:
  type: jwt
  jwk_url: https://sso.example.com/.well-known/jwks.json
  required: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err != nil {
		t.Fatalf("expected valid auth config to parse without error, got: %v", err)
	}
}
