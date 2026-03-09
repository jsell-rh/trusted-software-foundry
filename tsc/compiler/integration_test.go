package compiler_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/compiler"
)

// TestE2E_DinosaurRegistry is the end-to-end integration test for the TSC compiler.
// It compiles the canonical dinosaur-registry example spec and verifies that:
//  1. Compilation succeeds without errors
//  2. The output directory contains main.go, go.mod, and migrations/
//  3. The generated main.go imports spec.NewApplication and all component packages
//  4. The generated SQL migration has the correct table structure
//  5. gofmt reports no formatting issues in generated main.go
func TestE2E_DinosaurRegistry(t *testing.T) {
	specPath := filepath.Join("..", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("example spec not found at %s — run from tsc-compiler worktree", specPath)
	}

	outDir := t.TempDir()

	c := compiler.New(compiler.NewStubRegistry(), "", "")
	if err := c.Compile(specPath, outDir); err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}

	// Assert: main.go exists and contains expected structure
	mainGoPath := filepath.Join(outDir, "main.go")
	mainGo, err := os.ReadFile(mainGoPath)
	if err != nil {
		t.Fatalf("main.go not generated: %v", err)
	}
	mainGoStr := string(mainGo)

	for _, want := range []string{
		"spec.NewApplication(",
		"app.AddComponent(",
		"app.Configure(",
		"app.Register(",
		"app.Run(",
		"tsc/spec",
		"DO NOT EDIT",
		"dinosaur-registry",
	} {
		if !strings.Contains(mainGoStr, want) {
			t.Errorf("main.go missing expected content: %q", want)
		}
	}

	// Assert: foundry-postgres AddComponent call appears before foundry-http (dependency order).
	// Search only in the AddComponent block, not the import block.
	addComponentSection := mainGoStr[strings.Index(mainGoStr, "app.AddComponent("):]
	postgresAddIdx := strings.Index(addComponentSection, "foundrypostgres")
	httpAddIdx := strings.Index(addComponentSection, "foundryhttp")
	if postgresAddIdx < 0 {
		t.Error("main.go missing foundry-postgres AddComponent call")
	}
	if httpAddIdx < 0 {
		t.Error("main.go missing foundry-http AddComponent call")
	}
	if postgresAddIdx > 0 && httpAddIdx > 0 && postgresAddIdx > httpAddIdx {
		t.Error("foundry-postgres must be added before foundry-http (dependency order: postgres sets DB, others need DB)")
	}

	// Assert: go.mod exists
	goModPath := filepath.Join(outDir, "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		t.Fatalf("go.mod not generated: %v", err)
	}

	// Assert: migrations/ directory with at least one .sql file
	migrDir := filepath.Join(outDir, "migrations")
	entries, err := os.ReadDir(migrDir)
	if err != nil {
		t.Fatalf("migrations/ not generated: %v", err)
	}
	var sqlFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}
	if len(sqlFiles) == 0 {
		t.Fatal("migrations/ contains no .sql files")
	}

	// Assert: migration SQL has correct structure
	sqlPath := filepath.Join(migrDir, sqlFiles[0])
	sqlBytes, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatalf("reading migration %s: %v", sqlFiles[0], err)
	}
	sqlStr := string(sqlBytes)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS",
		"dinosaurs",
		"VARCHAR(255)",  // species field with max_length: 255
		"PRIMARY KEY",
		"deleted_at",
	} {
		if !strings.Contains(sqlStr, want) {
			t.Errorf("migration SQL missing expected content: %q\nSQL:\n%s", want, sqlStr)
		}
	}

	// Assert: generated main.go is gofmt-clean
	if gofmt, err := exec.LookPath("gofmt"); err == nil {
		cmd := exec.Command(gofmt, "-l", mainGoPath)
		out, _ := cmd.Output()
		if len(strings.TrimSpace(string(out))) > 0 {
			t.Errorf("generated main.go is not gofmt-clean: %s", out)
		}
	}
}

// TestE2E_FleetManager is the end-to-end integration test for the full-capability
// fleet-manager example spec. It exercises all 8 advanced IR blocks:
// graph, services, events, authz, state, temporal, tenancy, hooks.
//
// Verifies that:
//  1. Compilation succeeds (spec is valid AND compiler handles all advanced blocks)
//  2. 3 SQL migrations generated (Cluster, ClusterUpgrade, NodePool)
//  3. 3 service main files generated (main_api_server.go, main_provisioner.go, main_graph_indexer.go)
//  4. docker-compose.yaml generated
//  5. foundry/types.go generated (hook types package)
//  6. hook_registry.go generated with 5 hook call sites
//  7. All generated Go files pass gofmt
func TestE2E_FleetManager(t *testing.T) {
	specPath := filepath.Join("..", "examples", "fleet-manager", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("fleet-manager spec not found at %s", specPath)
	}

	outDir := t.TempDir()

	c := compiler.New(compiler.NewStubRegistry(), "", "")
	if err := c.Compile(specPath, outDir); err != nil {
		t.Fatalf("Compile(fleet-manager) failed: %v", err)
	}

	// Assert: 3 SQL migrations (one per resource)
	migrDir := filepath.Join(outDir, "migrations")
	entries, err := os.ReadDir(migrDir)
	if err != nil {
		t.Fatalf("migrations/ not generated: %v", err)
	}
	var sqlFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}
	if len(sqlFiles) != 3 {
		t.Errorf("expected 3 migration files, got %d: %v", len(sqlFiles), sqlFiles)
	}

	// Assert: service mains generated
	for _, svc := range []string{"api_server", "provisioner", "graph_indexer"} {
		p := filepath.Join(outDir, "main_"+svc+".go")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("service main %s not generated: %v", "main_"+svc+".go", err)
		}
	}

	// Assert: docker-compose.yaml generated
	if _, err := os.Stat(filepath.Join(outDir, "docker-compose.yaml")); err != nil {
		t.Error("docker-compose.yaml not generated")
	}

	// Assert: foundry/types.go generated (because spec has hooks)
	foundryTypesPath := filepath.Join(outDir, "foundry", "types.go")
	if _, err := os.Stat(foundryTypesPath); err != nil {
		t.Error("foundry/types.go not generated (required for hook implementations)")
	}

	// Assert: hook_registry.go generated
	hookRegPath := filepath.Join(outDir, "hook_registry.go")
	if hookReg, err := os.ReadFile(hookRegPath); err != nil {
		t.Errorf("hook_registry.go not generated: %v", err)
	} else {
		hookRegStr := string(hookReg)
		// 5 hooks declared in fleet-manager spec
		for _, hookName := range []string{
			"audit-logger",
			"tenant-isolation-check",
			"cluster-status-enricher",
			"event-schema-validator",
			"graph-sync-consumer",
		} {
			if !strings.Contains(hookRegStr, hookName) {
				t.Errorf("hook_registry.go missing hook %q", hookName)
			}
		}
	}

	// Assert: hook implementation files are copied into the output directory.
	// The compiler resolves hook paths relative to the spec file's directory,
	// so all 5 fleet-manager hooks should be present.
	for _, hookFile := range []string{
		"hooks/audit_logger.go",
		"hooks/tenant_isolation.go",
		"hooks/cluster_status_enricher.go",
		"hooks/event_schema_validator.go",
		"hooks/graph_sync_consumer.go",
	} {
		hookPath := filepath.Join(outDir, hookFile)
		if _, err := os.Stat(hookPath); err != nil {
			t.Errorf("hook file %s not copied to output: %v", hookFile, err)
		}
	}

	// Assert: all generated Go files are gofmt-clean
	if gofmt, err := exec.LookPath("gofmt"); err == nil {
		goFiles, _ := filepath.Glob(filepath.Join(outDir, "*.go"))
		for _, f := range goFiles {
			cmd := exec.Command(gofmt, "-l", f)
			out, _ := cmd.Output()
			if len(strings.TrimSpace(string(out))) > 0 {
				t.Errorf("generated file %s is not gofmt-clean", filepath.Base(f))
			}
		}
	}
}
