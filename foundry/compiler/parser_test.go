package compiler

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testDataDir returns the absolute path to the foundry module root so tests can
// locate spec files and examples regardless of how "go test" is invoked.
func testDataDir() string {
	_, file, _, _ := runtime.Caller(0)
	// file is .../foundry/compiler/parser_test.go
	// module root is two levels up: foundry/compiler → foundry → module root
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func schemaPathForTest() string {
	return filepath.Join(testDataDir(), "foundry", "spec", "schema.json")
}

func goldenSpecPath() string {
	return filepath.Join(testDataDir(), "foundry", "examples", "dinosaur-registry", "app.foundry.yaml")
}

// writeTempSpec writes YAML content to a temp file and returns the path.
// The file is removed when the test finishes.
func writeTempSpec(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "spec-*.yaml")
	if err != nil {
		t.Fatalf("creating temp spec: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp spec: %v", err)
	}
	f.Close()
	return f.Name()
}

// --- Golden-path test ---

func TestParse_ValidSpec(t *testing.T) {
	ir, err := ParseWithSchema(goldenSpecPath(), schemaPathForTest())
	if err != nil {
		t.Fatalf("expected valid spec to parse without error, got: %v", err)
	}

	if ir.Metadata.Name != "dinosaur-registry" {
		t.Errorf("metadata.name: want %q, got %q", "dinosaur-registry", ir.Metadata.Name)
	}
	if ir.Metadata.Version != "1.0.0" {
		t.Errorf("metadata.version: want %q, got %q", "1.0.0", ir.Metadata.Version)
	}
	if _, ok := ir.Components["foundry-http"]; !ok {
		t.Error("expected foundry-http in components")
	}
	if len(ir.Resources) == 0 {
		t.Error("expected at least one resource")
	}
}

// --- Missing required fields ---

func TestParse_MissingAPIVersion(t *testing.T) {
	spec := writeTempSpec(t, `
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for missing apiVersion, got nil")
	}
	if !strings.Contains(err.Error(), "apiVersion") {
		t.Errorf("expected error to mention 'apiVersion', got: %v", err)
	}
}

func TestParse_MissingKind(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for missing kind, got nil")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("expected error to mention 'kind', got: %v", err)
	}
}

func TestParse_MissingMetadata(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for missing metadata, got nil")
	}
	if !strings.Contains(err.Error(), "metadata") {
		t.Errorf("expected error to mention 'metadata', got: %v", err)
	}
}

func TestParse_MissingMetadataName(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for missing metadata.name, got nil")
	}
}

func TestParse_MissingComponents(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for missing components, got nil")
	}
	if !strings.Contains(err.Error(), "components") {
		t.Errorf("expected error to mention 'components', got: %v", err)
	}
}

// --- Unknown component names ---

func TestParse_UnknownComponent(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
  foundry-unknown-gadget: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for unknown component name, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-unknown-gadget") {
		t.Errorf("expected error to mention the unknown component name, got: %v", err)
	}
}

func TestParse_AllKnownComponents(t *testing.T) {
	// All 16 known components should be accepted by the validator.
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: full-stack
  version: 1.0.0
components:
  foundry-http:           v1.0.0
  foundry-postgres:       v1.0.0
  foundry-auth-jwt:       v1.0.0
  foundry-grpc:           v1.0.0
  foundry-health:         v1.0.0
  foundry-metrics:        v1.0.0
  foundry-events:         v1.0.0
  foundry-auth-spicedb:   v1.0.0
  foundry-graph-age:      v1.0.0
  foundry-kafka:          v1.0.0
  foundry-nats:           v1.0.0
  foundry-redis:          v1.0.0
  foundry-redis-streams:  v1.0.0
  foundry-temporal:       v1.0.0
  foundry-tenancy:        v1.0.0
  foundry-service-router: v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: id
        type: uuid
    operations: [create, read]
    events: true
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err != nil {
		t.Fatalf("expected all known components to be accepted, got: %v", err)
	}
}

// --- Invalid field types ---

func TestParse_InvalidFieldType(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
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
      - name: price
        type: decimal
    operations: [create]
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for invalid field type 'decimal', got nil")
	}
	if !strings.Contains(err.Error(), "decimal") {
		t.Errorf("expected error to mention the invalid type, got: %v", err)
	}
}

func TestParse_ValidFieldTypes(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: type-test
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-postgres: v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Record
    plural: records
    fields:
      - name: label
        type: string
      - name: count
        type: int
      - name: ratio
        type: float
      - name: active
        type: bool
      - name: created_at
        type: timestamp
      - name: ref_id
        type: uuid
    operations: [create, read]
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err != nil {
		t.Fatalf("expected all valid field types to be accepted, got: %v", err)
	}
}

// --- Wrong const values ---

func TestParse_WrongAPIVersion(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v2
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for wrong apiVersion, got nil")
	}
}

func TestParse_WrongKind(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Service
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for wrong kind, got nil")
	}
}

// --- Invalid name patterns ---

func TestParse_InvalidMetadataName(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: MyApp
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for PascalCase metadata.name (must be kebab-case), got nil")
	}
}

func TestParse_InvalidComponentVersion(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: 1.0.0
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for component version without 'v' prefix, got nil")
	}
}

// --- Schema path skip ---

func TestParseWithSchema_EmptySchemaPath_SkipsSchemaValidation(t *testing.T) {
	// Without schema validation, a spec that would fail schema checks
	// but passes semantic validate (or fails only on semantics) should
	// only produce semantic errors — not schema errors.
	// Use a completely minimal valid spec to confirm skip works.
	specPath := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: minimal-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`)
	// Empty schemaPath skips schema validation.
	_, err := ParseWithSchema(specPath, "")
	// This spec is semantically valid too (no resources, no DB required).
	if err != nil {
		t.Fatalf("expected no error with schema validation skipped, got: %v", err)
	}
}

// --- File not found ---

func TestParse_FileNotFound(t *testing.T) {
	_, err := ParseWithSchema("/nonexistent/path/app.foundry.yaml", schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for missing spec file, got nil")
	}
}

// --- Tenancy strategy validation ---

func TestParse_TenancyInvalidStrategy(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-tenancy:  v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
    operations: [create, read]
    events: false
tenancy:
  field: org_id
  strategy: column
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for invalid tenancy strategy, got nil")
	}
	if !strings.Contains(err.Error(), "tenancy.strategy") {
		t.Errorf("expected error to mention tenancy.strategy, got: %v", err)
	}
}

func TestParse_TenancyValidStrategies(t *testing.T) {
	for _, strategy := range []string{"row", "schema", "database"} {
		t.Run(strategy, func(t *testing.T) {
			spec := writeTempSpec(t, `
apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-postgres: v1.0.0
  foundry-tenancy:  v1.0.0
database:
  type: postgres
  migrations: auto
resources:
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
    operations: [create, read]
    events: false
tenancy:
  field: org_id
  strategy: `+strategy+`
`)
			_, err := ParseWithSchema(spec, schemaPathForTest())
			if err != nil {
				t.Errorf("strategy %q should be valid, got error: %v", strategy, err)
			}
		})
	}
}

// --- Service component cross-reference ---

func TestParse_ServiceUndeclaredComponent(t *testing.T) {
	spec := writeTempSpec(t, `
apiVersion: foundry/v1
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
    operations: [create, read]
    events: false
services:
  - name: api
    role: gateway
    port: 8080
    components:
      - foundry-http
      - foundry-grpc
`)
	_, err := ParseWithSchema(spec, schemaPathForTest())
	if err == nil {
		t.Fatal("expected error for service using undeclared component, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-grpc") {
		t.Errorf("expected error to mention foundry-grpc, got: %v", err)
	}
}
