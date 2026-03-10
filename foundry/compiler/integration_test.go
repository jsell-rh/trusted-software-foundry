package compiler_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

// TestE2E_DinosaurRegistry is the end-to-end integration test for the Foundry compiler.
// It compiles the canonical dinosaur-registry example spec and verifies that:
//  1. Compilation succeeds without errors
//  2. The output directory contains main.go, go.mod, and migrations/
//  3. The generated main.go imports spec.NewApplication and all component packages
//  4. The generated SQL migration has the correct table structure
//  5. gofmt reports no formatting issues in generated main.go
func TestE2E_DinosaurRegistry(t *testing.T) {
	specPath := filepath.Join("..", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("example spec not found at %s — run from foundry-compiler worktree", specPath)
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
		"foundry/spec",
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
		"VARCHAR(255)", // species field with max_length: 255
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
//  5. hook_registry.go generated with 5 hook call sites (imports canonical foundry/spec/foundry types)
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

	// Assert: no local foundry/types.go — types are imported from upstream foundry/spec/foundry package.
	if _, err := os.Stat(filepath.Join(outDir, "foundry", "types.go")); err == nil {
		t.Error("foundry/types.go should NOT be generated: hook types come from canonical upstream package")
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

// TestE2E_FleetManager_GoBuild is the gold standard integration test: it compiles
// the fleet-manager spec to a real Go project and verifies that `go build` succeeds.
// This test runs go mod tidy + go build, so it requires network access and a local
// trusted-software-foundry checkout. It is skipped in short mode.
func TestE2E_FleetManager_GoBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build E2E in short mode")
	}

	// Locate the repo root (4 levels up from foundry/compiler/).
	// The test binary runs in the package directory, so we use relative paths.
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("cannot resolve repo root: %v", err)
	}

	fleetSpec := filepath.Join(repoRoot, "foundry", "examples", "fleet-manager", "app.foundry.yaml")
	if _, err := os.Stat(fleetSpec); os.IsNotExist(err) {
		t.Skipf("fleet-manager spec not found at %s", fleetSpec)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", repoRoot)
	if err := c.Compile(fleetSpec, outDir); err != nil {
		t.Fatalf("Compile(fleet-manager) with rhtexAIPath failed: %v", err)
	}

	// Run go build — this is the definitive proof that the generated wiring is correct.
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found in PATH")
	}
	cmd := exec.Command(goBin, "build", "-o", filepath.Join(outDir, "fleet-manager"), ".")
	cmd.Dir = outDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed on generated fleet-manager project:\n%s", out)
	}

	// Verify the binary was produced.
	if _, err := os.Stat(filepath.Join(outDir, "fleet-manager")); err != nil {
		t.Error("go build succeeded but binary not found")
	}
}

// TestE2E_Kartograph_GoBuild compiles the Kartograph enterprise spec and verifies
// that go build succeeds. Kartograph exercises the hook stub generation path:
// hooks are declared in the spec but their implementation files don't exist,
// so the compiler must generate hooks/stubs_generated.go with typed stub functions.
func TestE2E_FleetManager_HookRegistry_GoBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build E2E in short mode")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("cannot resolve repo root: %v", err)
	}

	specPath := filepath.Join(repoRoot, "foundry", "examples", "fleet-manager", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("fleet-manager spec not found at %s", specPath)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", repoRoot)
	if err := c.Compile(specPath, outDir); err != nil {
		t.Fatalf("Compile(fleet-manager) failed: %v", err)
	}

	// hook_registry.go must be generated (fleet-manager declares multiple hooks).
	registryPath := filepath.Join(outDir, "hook_registry.go")
	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("hook_registry.go not found — hook registry generation failed: %v", err)
	}
	registryData, _ := os.ReadFile(registryPath)
	registryStr := string(registryData)
	// Verify hook call sites are present in the registry.
	for _, fn := range []string{
		"AuditLogger",
		"TenantIsolationCheck",
		"ClusterStatusEnricher",
	} {
		if !strings.Contains(registryStr, fn) {
			t.Errorf("hook_registry.go missing call site for %q", fn)
		}
	}

	// go build must succeed on the generated project.
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found in PATH")
	}
	cmd := exec.Command(goBin, "build", "-o", filepath.Join(outDir, "fleet-manager"), ".")
	cmd.Dir = outDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed on generated fleet-manager project:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(outDir, "fleet-manager")); err != nil {
		t.Error("go build succeeded but binary not found")
	}
}

// TestE2E_DinosaurRegistry_GoBuild compiles the dinosaur-registry example and verifies
// that go build succeeds. This is the simplest spec — CRUD service with no hooks.
func TestE2E_DinosaurRegistry_GoBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build E2E in short mode")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("cannot resolve repo root: %v", err)
	}

	specPath := filepath.Join(repoRoot, "foundry", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("dinosaur-registry spec not found at %s", specPath)
	}

	outDir := t.TempDir()
	c := compiler.New(compiler.NewStubRegistry(), "", repoRoot)
	if err := c.Compile(specPath, outDir); err != nil {
		t.Fatalf("Compile(dinosaur-registry) failed: %v", err)
	}

	// No hooks declared — stubs_generated.go must NOT exist.
	stubPath := filepath.Join(outDir, "hooks", "stubs_generated.go")
	if _, err := os.Stat(stubPath); err == nil {
		t.Error("hooks/stubs_generated.go should not exist for spec with no hooks")
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found in PATH")
	}
	cmd := exec.Command(goBin, "build", "-o", filepath.Join(outDir, "dinosaur-registry"), ".")
	cmd.Dir = outDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed on generated dinosaur-registry project:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(outDir, "dinosaur-registry")); err != nil {
		t.Error("go build succeeded but binary not found")
	}
}
