package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

const multiSvcYAML = `apiVersion: foundry/v1
kind: Application
metadata:
  name: multi-svc-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-postgres: v1.0.0
  foundry-auth-jwt: v1.0.0
  foundry-health:   v1.0.0
  foundry-metrics:  v1.0.0
  foundry-events:   v1.0.0
  foundry-grpc:     v1.0.0
resources:
  - name: Widget
    plural: widgets
    fields:
      - name: id
        type: uuid
        required: true
      - name: label
        type: string
        required: true
    operations: [create, read, list]
    events: true
database:
  type: postgres
  migrations: auto
api:
  rest:
    base_path: /api/v1
    version_header: true
services:
  - name: api-server
    role: rest-api
    port: 8080
    components: [foundry-http, foundry-postgres, foundry-auth-jwt]
    resources: all
  - name: worker
    role: worker
    components: [foundry-events, foundry-postgres]
    resources: all
`

// TestE2E_MultiService verifies that a spec with a services: block generates
// separate main_<service>.go files and a docker-compose.yaml.
func TestE2E_MultiService(t *testing.T) {
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	if err := os.WriteFile(specFile, []byte(multiSvcYAML), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	if err := c.Compile(specFile, outDir); err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}

	// Assert: separate main_<service>.go files generated
	for _, svc := range []string{"api_server", "worker"} {
		path := filepath.Join(outDir, "main_"+svc+".go")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("main_%s.go not generated: %v", svc, err)
			continue
		}
		s := string(data)
		if !strings.Contains(s, "DO NOT EDIT") {
			t.Errorf("main_%s.go missing DO NOT EDIT header", svc)
		}
		if !strings.Contains(s, "spec.NewApplication(") {
			t.Errorf("main_%s.go missing spec.NewApplication call", svc)
		}
	}

	// Assert: api-server main uses PascalCase function name
	apiServerMain, _ := os.ReadFile(filepath.Join(outDir, "main_api_server.go"))
	if !strings.Contains(string(apiServerMain), "func mainApiServer()") {
		t.Errorf("main_api_server.go: expected func mainApiServer(), got:\n%s", string(apiServerMain))
	}

	// Assert: worker main uses correct function name
	workerMain, _ := os.ReadFile(filepath.Join(outDir, "main_worker.go"))
	if !strings.Contains(string(workerMain), "func mainWorker()") {
		t.Errorf("main_worker.go: expected func mainWorker(), got:\n%s", string(workerMain))
	}

	// Assert: docker-compose.yaml generated
	composePath := filepath.Join(outDir, "docker-compose.yaml")
	compose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("docker-compose.yaml not generated: %v", err)
	}
	composeStr := string(compose)
	for _, want := range []string{"api-server", "worker", "postgres", "8080"} {
		if !strings.Contains(composeStr, want) {
			t.Errorf("docker-compose.yaml missing %q", want)
		}
	}
}

const hooksYAML = `apiVersion: foundry/v1
kind: Application
metadata:
  name: hooked-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-postgres: v1.0.0
  foundry-auth-jwt: v1.0.0
  foundry-health:   v1.0.0
  foundry-metrics:  v1.0.0
  foundry-events:   v1.0.0
  foundry-grpc:     v1.0.0
resources:
  - name: Order
    plural: orders
    fields:
      - name: id
        type: uuid
        required: true
      - name: total
        type: int
        required: true
    operations: [create, read, list]
    events: false
database:
  type: postgres
  migrations: auto
hooks:
  - name: audit-log
    point: pre-handler
    routes: ["/api/v1/orders"]
    implementation: hooks/audit_log.go
  - name: enrich-response
    point: post-handler
    implementation: hooks/enrich_response.go
`

const authzYAML = `apiVersion: foundry/v1
kind: Application
metadata:
  name: authz-app
  version: 1.0.0
components:
  foundry-http:           v1.0.0
  foundry-postgres:       v1.0.0
  foundry-auth-spicedb:   v1.0.0
resources:
  - name: Document
    plural: documents
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read, list]
    events: false
database:
  type: postgres
  migrations: auto
authz:
  backend: spicedb
  schema_file: authz/schema.zed
  relations:
    - resource: Document
      relation: owner
      subject: User
    - resource: Document
      relation: viewer
      subject: Organization
`

// TestE2E_AuthzSchemaStub verifies that a spec with an authz block generates
// an authz/schema.zed stub file with the correct structure.
func TestE2E_AuthzSchemaStub(t *testing.T) {
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	if err := os.WriteFile(specFile, []byte(authzYAML), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	if err := c.Compile(specFile, outDir); err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}

	// Assert: authz/schema.zed generated
	zedPath := filepath.Join(outDir, "authz", "schema.zed")
	zed, err := os.ReadFile(zedPath)
	if err != nil {
		t.Fatalf("authz/schema.zed not generated: %v", err)
	}
	zedStr := string(zed)

	for _, want := range []string{
		"generated stub",
		"authz-app",
		"definition document",
		"definition user",
		"definition organization",
		"Document.owner",
		"Document.viewer",
	} {
		if !strings.Contains(zedStr, want) {
			t.Errorf("authz/schema.zed missing %q\nContent:\n%s", want, zedStr)
		}
	}
}

// TestE2E_AuthzSchemaStub_NoOverwrite verifies that an existing authz/schema.zed
// is not overwritten when forge compile runs again.
func TestE2E_AuthzSchemaStub_NoOverwrite(t *testing.T) {
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	if err := os.WriteFile(specFile, []byte(authzYAML), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", "")

	// First compile — generates the stub.
	if err := c.Compile(specFile, outDir); err != nil {
		t.Fatalf("first Compile() failed: %v", err)
	}

	// Overwrite the stub with custom content.
	zedPath := filepath.Join(outDir, "authz", "schema.zed")
	customContent := "// hand-written schema by an engineer\ndefinition document { relation owner: user }\n"
	if err := os.WriteFile(zedPath, []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Second compile — must NOT overwrite custom content.
	if err := c.Compile(specFile, outDir); err != nil {
		t.Fatalf("second Compile() failed: %v", err)
	}

	data, err := os.ReadFile(zedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != customContent {
		t.Errorf("authz/schema.zed was overwritten by second compile\ngot: %s", string(data))
	}
}

// TestE2E_HookFiles_CopiedRelativeToSpec verifies that hook implementation files
// are found and copied even when forge compile runs from a different directory.
// This tests the specDir-relative path resolution in copyHookFiles.
func TestE2E_HookFiles_CopiedRelativeToSpec(t *testing.T) {
	// Build a spec directory with a hook file inside it.
	specDir := t.TempDir()
	hooksDir := filepath.Join(specDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a minimal hook implementation file alongside the spec.
	hookContent := "package hooks\n// minimal hook placeholder\n"
	if err := os.WriteFile(filepath.Join(hooksDir, "audit_log.go"), []byte(hookContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write the spec file (re-uses hooksYAML which declares hooks/audit_log.go).
	specFile := filepath.Join(specDir, "app.foundry.yaml")
	if err := os.WriteFile(specFile, []byte(hooksYAML), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	if err := c.Compile(specFile, outDir); err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}

	// Assert: the hook file was copied to the output directory despite
	// the spec being in a different directory from the CWD.
	dest := filepath.Join(outDir, "hooks", "audit_log.go")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("hook file not copied to output: %v\nExpected: %s", err, dest)
	}
	if string(data) != hookContent {
		t.Errorf("hook file content mismatch\ngot: %s\nwant: %s", string(data), hookContent)
	}
}

// TestE2E_HooksCodegen verifies that a spec with a hooks: block generates
// foundry/types.go and hook_registry.go with the correct type-safe call sites.
func TestE2E_HooksCodegen(t *testing.T) {
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	if err := os.WriteFile(specFile, []byte(hooksYAML), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	if err := c.Compile(specFile, outDir); err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}

	// Assert: foundry/types.go generated with all required types
	typesPath := filepath.Join(outDir, "foundry", "types.go")
	types, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("foundry/types.go not generated: %v", err)
	}
	typesStr := string(types)
	for _, want := range []string{
		"HookContext",
		"PostHandlerRequest",
		"DBOperation",
		"DBResult",
		"EventMessage",
		"ConsumedEvent",
		"Logger",
		"Tracer",
	} {
		if !strings.Contains(typesStr, want) {
			t.Errorf("foundry/types.go missing type %q", want)
		}
	}

	// Assert: hook_registry.go generated with correct call sites
	regPath := filepath.Join(outDir, "hook_registry.go")
	reg, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatalf("hook_registry.go not generated: %v", err)
	}
	regStr := string(reg)
	for _, want := range []string{
		"DO NOT EDIT",
		// pre-handler: func(hctx *foundry.HookContext, w http.ResponseWriter, r *http.Request)
		"AuditLogPreHandler",
		"http.ResponseWriter",
		"*http.Request",
		// post-handler: func(hctx *foundry.HookContext, req *foundry.PostHandlerRequest)
		"EnrichResponsePostHandler",
		"*foundry.PostHandlerRequest",
	} {
		if !strings.Contains(regStr, want) {
			t.Errorf("hook_registry.go missing %q\nContent:\n%s", want, regStr)
		}
	}
}
