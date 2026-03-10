package compiler

// validation_test.go covers the semantic cross-check validation rules in
// foundry/spec/validate.go that are not covered by parser_test.go.
// Rules tested here:
//   - bi_temporal block component and database requirements
//   - workflows block component and field requirements
//   - Component cross-references for every advanced feature block
//   - Hook validation (point, implementation pattern, routes/topic scope, duplicates)
//   - Graph edge cross-reference against declared node_type labels

import (
	"os"
	"path/filepath"
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

// --------------------------------------------------------------------------
// validateAgainstSchema error path tests
// --------------------------------------------------------------------------

// TestParseWithSchema_MissingSchemaFile verifies that ParseWithSchema returns
// an error wrapping "reading schema file" when the schema path does not exist.
// This covers the os.ReadFile error branch in validateAgainstSchema.
func TestParseWithSchema_MissingSchemaFile(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, "/nonexistent/path/schema.json")
	if err == nil {
		t.Fatal("expected error for missing schema file, got nil")
	}
	if !strings.Contains(err.Error(), "reading schema") {
		t.Errorf("expected 'reading schema' in error, got: %v", err)
	}
}

// TestParseWithSchema_InvalidSchemaJSON verifies that ParseWithSchema returns
// an error mentioning "loading schema" when the schema file contains invalid
// JSON. This covers the newSchemaValidator error branch in validateAgainstSchema.
func TestParseWithSchema_InvalidSchemaJSON(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	// Write a schema file containing invalid JSON.
	schemaFile := filepath.Join(t.TempDir(), "bad-schema.json")
	if err := os.WriteFile(schemaFile, []byte(`{invalid json`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseWithSchema(spec, schemaFile)
	if err == nil {
		t.Fatal("expected error for invalid schema JSON, got nil")
	}
	if !strings.Contains(err.Error(), "loading schema") {
		t.Errorf("expected 'loading schema' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// checkJSONType — array and object type-mismatch errors
// --------------------------------------------------------------------------

// TestParse_OperationsNotArray verifies that a scalar string in the operations
// field (which must be a JSON array) triggers the checkJSONType array error.
func TestParse_OperationsNotArray(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-postgres: v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: id
        type: uuid
        required: true
    operations: "create"
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for non-array operations value, got nil")
	}
	if !strings.Contains(err.Error(), "operations") {
		t.Errorf("expected error to mention 'operations', got: %v", err)
	}
}

// TestParse_DatabaseNotObject verifies that a scalar value for the database
// block (which must be a JSON object) triggers the checkJSONType object error.
func TestParse_DatabaseNotObject(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-postgres: v1.0.0
database: "postgres"
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for non-object database value, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error to mention 'database', got: %v", err)
	}
}

// --------------------------------------------------------------------------
// checkJSONType — boolean, string, and integer type-mismatch errors
// --------------------------------------------------------------------------

func TestParse_ResourceEventsWrongType(t *testing.T) {
	// events field expects a boolean; passing a string "yes" triggers a
	// JSON Schema type-mismatch error in the schema validator.
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
        required: true
    operations: [create]
    events: "yes"
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for non-boolean events value, got nil")
	}
	if !strings.Contains(err.Error(), "events") {
		t.Errorf("expected error to mention 'events', got: %v", err)
	}
}

func TestParse_BiTemporalEnabledWrongType(t *testing.T) {
	// bi_temporal.enabled expects boolean; passing an integer triggers type error.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-temporal: v1.0.0
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
        required: true
    operations: [create]
    events: false
bi_temporal:
  enabled: 1
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for integer-typed bi_temporal.enabled, got nil")
	}
	if !strings.Contains(err.Error(), "enabled") && !strings.Contains(err.Error(), "bi_temporal") {
		t.Errorf("expected error to mention 'enabled' or 'bi_temporal', got: %v", err)
	}
}

func TestParse_APIVersionWrongType(t *testing.T) {
	// apiVersion expects a specific string; passing an integer value triggers type mismatch.
	spec := writeTempSpec(t, `apiVersion: 1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for integer apiVersion, got nil")
	}
	if !strings.Contains(err.Error(), "apiVersion") {
		t.Errorf("expected error to mention 'apiVersion', got: %v", err)
	}
}

func TestParse_PortNotInteger(t *testing.T) {
	// Port fields expect integer; passing a float (1.5) triggers the
	// "expected integer, got float" branch in checkJSONType.
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:   v1.0.0
  foundry-health: v1.0.0
observability:
  health_check:
    port: 1.5
    path: /healthz
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for float-typed port, got nil")
	}
	if !strings.Contains(err.Error(), "port") && !strings.Contains(err.Error(), "integer") {
		t.Errorf("expected error to mention 'port' or 'integer', got: %v", err)
	}
}

// --------------------------------------------------------------------------
// validateObject — minProperties, propertyNames enum, pattern mismatch
// --------------------------------------------------------------------------

// TestParse_ComponentsMapEmpty verifies that an empty components map is rejected.
// The schema declares minProperties: 1 on the components object; this test
// exercises the minProperties violation branch in validateObject (line ~140).
func TestParse_ComponentsMapEmpty(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components: {}
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create]
    events: false
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for empty components map, got nil")
	}
	if !strings.Contains(err.Error(), "component") && !strings.Contains(err.Error(), "properties") {
		t.Errorf("expected error to mention 'component' or 'properties', got: %v", err)
	}
}

// TestParse_UnknownComponentName verifies that a component name not in the
// trusted catalog is rejected at schema validation time. The schema's
// propertyNames.enum constraint on the components object fires the
// "unknown property name" branch in validateObject (line ~164).
func TestParse_UnknownComponentName(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  not-a-real-component: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for unknown component name, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-real-component") && !strings.Contains(err.Error(), "property") {
		t.Errorf("expected error to mention the unknown component or 'property', got: %v", err)
	}
}

// TestParse_AppNamePatternViolation verifies that an app name failing the
// pattern constraint (^[a-z][a-z0-9-]*$) is rejected. This exercises the
// pattern mismatch error path in validateNode (line ~88).
func TestParse_AppNamePatternViolation(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: "My App"
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for app name violating pattern, got nil")
	}
	if !strings.Contains(err.Error(), "name") && !strings.Contains(err.Error(), "pattern") {
		t.Errorf("expected error to mention 'name' or 'pattern', got: %v", err)
	}
}

// TestParse_PortBelowMinimum verifies that a port value of 0 (below the
// schema minimum of 1) is rejected. This exercises the `num < min` branch
// in validateNode (the numeric minimum check).
func TestParse_PortBelowMinimum(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:   v1.0.0
  foundry-health: v1.0.0
observability:
  health_check:
    port: 0
    path: /healthz
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for port below minimum, got nil")
	}
	if !strings.Contains(err.Error(), "port") && !strings.Contains(err.Error(), "minimum") {
		t.Errorf("expected error to mention 'port' or 'minimum', got: %v", err)
	}
}

// TestParse_PortAboveMaximum verifies that a port value above 65535 is
// rejected. This exercises the `num > max` branch in validateNode (numeric
// maximum check).
func TestParse_PortAboveMaximum(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:   v1.0.0
  foundry-health: v1.0.0
observability:
  health_check:
    port: 99999
    path: /healthz
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for port above maximum, got nil")
	}
	if !strings.Contains(err.Error(), "port") && !strings.Contains(err.Error(), "maximum") {
		t.Errorf("expected error to mention 'port' or 'maximum', got: %v", err)
	}
}

// TestParse_PortAsString verifies that a quoted string port value is rejected
// as a non-integer type. When YAML parses `port: "8080"` as a string, the
// schema's integer type check fires the non-float64 else branch in
// checkJSONType (line ~273).
func TestParse_PortAsString(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:   v1.0.0
  foundry-health: v1.0.0
observability:
  health_check:
    port: "8080"
    path: /healthz
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for string-typed port, got nil")
	}
	if !strings.Contains(err.Error(), "port") && !strings.Contains(err.Error(), "integer") {
		t.Errorf("expected error to mention 'port' or 'integer', got: %v", err)
	}
}

// TestParseWithSchema_InvalidYAMLSyntax verifies that validateAgainstSchema
// returns an error when the YAML is syntactically invalid (YAMLToJSON fails).
// This covers the first error return in validateAgainstSchema (line ~76).
func TestParseWithSchema_InvalidYAMLSyntax(t *testing.T) {
	// Write YAML that cannot be converted to JSON (invalid syntax).
	tmp := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(tmp, []byte("{unclosed: [bracket\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseWithSchema(tmp, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	// The error should mention YAML or JSON conversion.
	if !strings.Contains(err.Error(), "YAML") && !strings.Contains(err.Error(), "JSON") && !strings.Contains(err.Error(), "schema") {
		t.Errorf("expected conversion error in output, got: %v", err)
	}
}

// TestParse_AuthJWTMissingJWKURL verifies that auth.type=jwt without a jwk_url
// is rejected. The schema has an if/then constraint: if auth.type == "jwt" then
// jwk_url is required. This exercises the if/then branch in validateObject
// (line ~198) when the if condition holds and the then constraint fires.
func TestParse_AuthJWTMissingJWKURL(t *testing.T) {
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
		t.Fatal("expected error for jwt auth without jwk_url, got nil")
	}
	if !strings.Contains(err.Error(), "jwk_url") && !strings.Contains(err.Error(), "jwt") {
		t.Errorf("expected error to mention 'jwk_url' or 'jwt', got: %v", err)
	}
}

// TestParseWithSchema_YAMLRootScalar verifies that validateAgainstSchema returns
// an error when YAML is syntactically valid but converts to a JSON non-object
// (e.g. a scalar), causing json.Unmarshal into map[string]interface{} to fail.
// This covers the json.Unmarshal error path in validateAgainstSchema (line ~91).
func TestParseWithSchema_YAMLRootScalar(t *testing.T) {
	// A YAML scalar converts to a JSON string, which cannot unmarshal into
	// map[string]interface{}; this is distinct from the YAMLToJSON error path.
	tmp := filepath.Join(t.TempDir(), "scalar.yaml")
	if err := os.WriteFile(tmp, []byte("just a scalar string\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseWithSchema(tmp, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for YAML root scalar, got nil")
	}
}

// TestParseWithSchema_MissingSpecFile verifies that ParseWithSchema returns a
// "reading spec file" error when the spec path does not exist. This covers
// the os.ReadFile error branch in ParseWithSchema (line ~38).
func TestParseWithSchema_MissingSpecFile(t *testing.T) {
	_, err := ParseWithSchema("/nonexistent/spec/file.yaml", schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for missing spec file, got nil")
	}
	if !strings.Contains(err.Error(), "reading spec file") {
		t.Errorf("expected 'reading spec file' in error, got: %v", err)
	}
}

// TestParseWithSchema_GoyamlUnmarshalError verifies that ParseWithSchema returns
// a "parsing YAML" error when goyaml.Unmarshal fails. Calling with an empty
// schemaPath skips JSON Schema validation so the invalid YAML reaches the
// goyaml.Unmarshal call. This covers the Unmarshal error path (parser.go:55).
func TestParseWithSchema_GoyamlUnmarshalError(t *testing.T) {
	// Write YAML that is syntactically invalid for go-yaml v3 Unmarshal.
	// An unclosed flow sequence triggers a parse error in go-yaml.
	tmp := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(tmp, []byte("{key: [unclosed_bracket\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Empty schemaPath skips JSON Schema validation; the invalid YAML reaches goyaml.Unmarshal.
	_, err := ParseWithSchema(tmp, "")
	if err == nil {
		t.Fatal("expected error for invalid YAML in goyaml.Unmarshal, got nil")
	}
	if !strings.Contains(err.Error(), "parsing YAML") {
		t.Errorf("expected 'parsing YAML' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Tenancy — empty strategy (omitted) is valid (strategy is optional)
// --------------------------------------------------------------------------

func TestParse_TenancyEmptyStrategyValid(t *testing.T) {
	// strategy is optional — omitting it is valid
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:    v1.0.0
  foundry-tenancy: v1.0.0
tenancy: {}
`)
	if _, err := Parse(spec); err != nil {
		t.Errorf("empty tenancy strategy: unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Services — component cross-check
// --------------------------------------------------------------------------

func TestParse_ServiceComponentNotInSBOM(t *testing.T) {
	spec := writeTempSpec(t, baseMinimal+`services:
  - name: api-service
    role: rest-api
    components: [foundry-postgres]
`)
	_, err := Parse(spec)
	if err == nil {
		t.Fatal("expected error for service referencing undeclared component, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-postgres") {
		t.Errorf("expected error to mention 'foundry-postgres', got: %v", err)
	}
}

func TestParse_ServiceComponentInSBOM_Valid(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-postgres: v1.0.0
database:
  type: postgres
services:
  - name: api-service
    role: rest-api
    components: [foundry-http, foundry-postgres]
`)
	if _, err := Parse(spec); err != nil {
		t.Errorf("service with declared components: unexpected error: %v", err)
	}
}

func TestParse_ServiceMultipleComponentsOneUndeclared(t *testing.T) {
	spec := writeTempSpec(t, `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
services:
  - name: worker
    role: worker
    components: [foundry-http, foundry-kafka]
`)
	_, err := Parse(spec)
	if err == nil {
		t.Fatal("expected error for undeclared component in service, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-kafka") {
		t.Errorf("expected error to mention 'foundry-kafka', got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Events — pg-notify requires no component
// --------------------------------------------------------------------------

func TestParse_EventsPgNotifyNoComponentRequired(t *testing.T) {
	// pg-notify uses the existing postgres connection — no events component needed
	spec := writeTempSpec(t, baseWithPostgres+`events:
  backend: pg-notify
`)
	if _, err := Parse(spec); err != nil {
		t.Errorf("events.backend=pg-notify: unexpected error: %v", err)
	}
}

func TestParse_EventsKafkaNoComponentFails(t *testing.T) {
	// kafka backend without foundry-kafka should fail
	spec := writeTempSpec(t, baseMinimal+`events:
  backend: kafka
  broker_url: ${KAFKA_URL}
`)
	_, err := Parse(spec)
	if err == nil {
		t.Fatal("expected error for kafka backend without foundry-kafka, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-kafka") {
		t.Errorf("expected 'foundry-kafka' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Authz — opa and casbin backends (no component required)
// --------------------------------------------------------------------------

func TestParse_AuthzOPANoComponentRequired(t *testing.T) {
	spec := writeTempSpec(t, baseMinimal+`authz:
  backend: opa
`)
	if _, err := Parse(spec); err != nil {
		t.Errorf("authz.backend=opa: unexpected error: %v", err)
	}
}

func TestParse_AuthzCasbinNoComponentRequired(t *testing.T) {
	spec := writeTempSpec(t, baseMinimal+`authz:
  backend: casbin
`)
	if _, err := Parse(spec); err != nil {
		t.Errorf("authz.backend=casbin: unexpected error: %v", err)
	}
}

func TestParse_AuthzSpiceDBMissingComponent(t *testing.T) {
	spec := writeTempSpec(t, baseMinimal+`authz:
  backend: spicedb
`)
	_, err := Parse(spec)
	if err == nil {
		t.Fatal("expected error for spicedb without foundry-auth-spicedb, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-auth-spicedb") {
		t.Errorf("expected 'foundry-auth-spicedb' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Graph — edge cross-reference with no node_types declared
// --------------------------------------------------------------------------
// --------------------------------------------------------------------------
// Hook — name must be kebab-case
// --------------------------------------------------------------------------

func TestParse_HookNameNotKebabCase(t *testing.T) {
	spec := writeTempSpec(t, baseMinimal+`hooks:
  - name: MyHookHandler
    point: pre-handler
    implementation: hooks/my_hook.go
`)
	_, err := Parse(spec)
	if err == nil {
		t.Fatal("expected error for hook name not in kebab-case, got nil")
	}
	if !strings.Contains(err.Error(), "MyHookHandler") {
		t.Errorf("expected hook name in error, got: %v", err)
	}
}

func TestParse_HookNameWithNumbers_Valid(t *testing.T) {
	spec := writeTempSpec(t, baseMinimal+`hooks:
  - name: auth-v2-handler
    point: pre-handler
    implementation: hooks/auth_v2.go
`)
	if _, err := Parse(spec); err != nil {
		t.Errorf("hook name with numbers: unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Bi-temporal — disabled block requires no components
// --------------------------------------------------------------------------

func TestParse_BiTemporalDisabledNoTemporal_Valid(t *testing.T) {
	// bi_temporal block present but enabled=false → no temporal component required
	spec := writeTempSpec(t, baseWithPostgres+`bi_temporal:
  enabled: false
`)
	if _, err := Parse(spec); err != nil {
		t.Errorf("bi_temporal.enabled=false: unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Workflows — valid workflow definition
// --------------------------------------------------------------------------
// --------------------------------------------------------------------------
// State — Redis key strategies
// --------------------------------------------------------------------------
// --------------------------------------------------------------------------
// Authz — relations and policies
// --------------------------------------------------------------------------