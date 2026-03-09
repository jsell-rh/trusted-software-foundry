package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/compiler"
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
