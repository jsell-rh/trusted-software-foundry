package compiler

// generator_write_errors_test.go covers the error paths in generator sub-functions
// that require OS-level I/O failures (read-only directories). Each test calls the
// unexported method directly to isolate exactly which error return is exercised,
// rather than routing through the full Generate pipeline (which returns on the
// first failure and would never reach later sub-function error paths).
//
// All tests are skipped when running as root (root bypasses permission checks).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

// makeReadOnly returns a cleanup func. The output dir is chmod'd 0555 so
// that MkdirAll/WriteFile inside it fail. The cleanup restores 0755 so
// t.TempDir cleanup can remove the directory.
func makeReadOnly(t *testing.T, dir string) func() {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories; skipping")
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod 0555 %s: %v", dir, err)
	}
	return func() { os.Chmod(dir, 0755) }
}

// --------------------------------------------------------------------------
// writeFoundryTypes — MkdirAll error when output dir is read-only
// --------------------------------------------------------------------------

func TestWriteFoundryTypes_MkdirAllError(t *testing.T) {
	ir := &spec.IRSpec{
		Hooks: []spec.IRHook{
			{Name: "audit", Point: "pre-db", Implementation: "hooks/audit.go"},
		},
	}
	outDir := t.TempDir()
	defer makeReadOnly(t, outDir)()

	g := NewGenerator(outDir, "")
	err := g.writeFoundryTypes(ir)
	if err == nil {
		t.Fatal("expected error when outDir is read-only, got nil")
	}
	if !strings.Contains(err.Error(), "creating foundry/ dir") {
		t.Errorf("expected 'creating foundry/ dir' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// writeHookRegistry — WriteFile error when output dir is read-only
// --------------------------------------------------------------------------

func TestWriteHookRegistry_WriteFileError(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "hook-app", Version: "1.0.0"},
		Hooks: []spec.IRHook{
			{Name: "audit", Point: "pre-db", Implementation: "hooks/audit.go"},
		},
	}
	outDir := t.TempDir()
	defer makeReadOnly(t, outDir)()

	g := NewGenerator(outDir, "")
	err := g.writeHookRegistry(ir, "github.com/example/hook-app")
	if err == nil {
		t.Fatal("expected error when outDir is read-only, got nil")
	}
	// The error is from os.WriteFile — the message contains the output path
	if !strings.Contains(err.Error(), "hook_registry.go") {
		t.Errorf("expected 'hook_registry.go' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// writeMigrations — MkdirAll error when output dir is read-only
// --------------------------------------------------------------------------

func TestWriteMigrations_MkdirAllError(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "test-app", Version: "1.0.0"},
		Resources: []spec.IRResource{
			{
				Name:   "Widget",
				Plural: "widgets",
				Fields: []spec.IRField{{Name: "id", Type: "uuid", Required: true}},
				Operations: []string{"create"},
			},
		},
	}
	outDir := t.TempDir()
	defer makeReadOnly(t, outDir)()

	g := NewGenerator(outDir, "")
	err := g.writeMigrations(ir)
	if err == nil {
		t.Fatal("expected error when outDir is read-only, got nil")
	}
	if !strings.Contains(err.Error(), "creating migrations directory") {
		t.Errorf("expected 'creating migrations directory' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// copyHookFiles — MkdirAll error when output dir is read-only
// --------------------------------------------------------------------------

func TestCopyHookFiles_MkdirAllError(t *testing.T) {
	// Create a spec dir with an actual hook file so ReadFile succeeds,
	// then make the output dir read-only so MkdirAll of the hooks/ subdir fails.
	specDir := t.TempDir()
	hooksDir := filepath.Join(specDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "audit.go"), []byte("package hooks\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ir := &spec.IRSpec{
		Hooks: []spec.IRHook{
			{Name: "audit", Point: "pre-db", Implementation: "hooks/audit.go"},
		},
	}
	outDir := t.TempDir()
	defer makeReadOnly(t, outDir)()

	err := copyHookFiles(ir, outDir, specDir)
	if err == nil {
		t.Fatal("expected error when outDir is read-only, got nil")
	}
	if !strings.Contains(err.Error(), "creating dir for") {
		t.Errorf("expected 'creating dir for' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// writeMigrations — WriteFile error when migrations dir exists but is read-only
// --------------------------------------------------------------------------

func TestWriteMigrations_WriteFileError(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "test-app", Version: "1.0.0"},
		Resources: []spec.IRResource{
			{
				Name:   "Widget",
				Plural: "widgets",
				Fields: []spec.IRField{{Name: "id", Type: "uuid", Required: true}},
				Operations: []string{"create"},
			},
		},
	}
	outDir := t.TempDir()

	// Pre-create the migrations dir and make it read-only.
	// MkdirAll will succeed (dir exists), but WriteFile inside it will fail.
	migrDir := filepath.Join(outDir, "migrations")
	if err := os.MkdirAll(migrDir, 0755); err != nil {
		t.Fatal(err)
	}
	defer makeReadOnly(t, migrDir)()

	g := NewGenerator(outDir, "")
	err := g.writeMigrations(ir)
	if err == nil {
		t.Fatal("expected error when migrations dir is read-only, got nil")
	}
	if !strings.Contains(err.Error(), "writing migration") {
		t.Errorf("expected 'writing migration' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// writeServiceMains — WriteFile error when output dir is read-only
// --------------------------------------------------------------------------

func TestWriteServiceMains_WriteFileError(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "svc-app", Version: "1.0.0"},
		Services: []spec.IRService{
			{Name: "api", Role: "gateway", Port: 8080},
		},
	}
	components := []ResolvedComponent{
		{Name: "foundry-http", Module: "github.com/jsell-rh/trusted-software-foundry/components/http", Version: "v1.0.0"},
	}
	outDir := t.TempDir()
	defer makeReadOnly(t, outDir)()

	g := NewGenerator(outDir, "")
	err := g.writeServiceMains(ir, components)
	if err == nil {
		t.Fatal("expected error when outDir is read-only, got nil")
	}
	if !strings.Contains(err.Error(), "main_api.go") && !strings.Contains(err.Error(), "writing") {
		t.Errorf("expected 'main_api.go' or 'writing' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// writeDockerCompose — WriteFile error when output dir is read-only
// --------------------------------------------------------------------------

func TestWriteDockerCompose_WriteFileError(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "svc-app", Version: "1.0.0"},
		Services: []spec.IRService{
			{Name: "api", Role: "gateway", Port: 8080},
		},
	}
	outDir := t.TempDir()
	defer makeReadOnly(t, outDir)()

	g := NewGenerator(outDir, "")
	err := g.writeDockerCompose(ir)
	if err == nil {
		t.Fatal("expected error when outDir is read-only, got nil")
	}
	if !strings.Contains(err.Error(), "docker-compose") {
		t.Errorf("expected 'docker-compose' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Generate — error return paths for specific sub-functions
//
// By pre-creating a regular FILE with the same name as a sub-function's
// expected subdirectory, os.MkdirAll fails with "not a directory", causing
// Generate to return the "generating X:" wrapped error for that sub-function.
// This lets us test each error path in isolation without making earlier
// sub-functions (which write to the top-level outputDir) fail.
// --------------------------------------------------------------------------

// TestGenerate_MigrationsError verifies that Generate returns an error
// mentioning "generating migrations" when writeMigrations fails. This is
// triggered by pre-creating a regular file named "migrations" in the output
// dir, so os.MkdirAll("outDir/migrations") gets "not a directory".
func TestGenerate_MigrationsError(t *testing.T) {
	ir, err := Parse(exampleSpecPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	outDir := t.TempDir()
	// Pre-create a regular FILE named "migrations" — MkdirAll("migrations/") will fail.
	if err := os.WriteFile(filepath.Join(outDir, "migrations"), []byte("blocker"), 0644); err != nil {
		t.Fatal(err)
	}

	genErr := NewGenerator(outDir, "").Generate(ir, components)
	if genErr == nil {
		t.Fatal("expected error from Generate when migrations dir is a file, got nil")
	}
	if !strings.Contains(genErr.Error(), "generating migrations") {
		t.Errorf("expected 'generating migrations' in error, got: %v", genErr)
	}
}

// TestGenerate_FoundryTypesError verifies that Generate returns an error
// mentioning "generating foundry/types.go" when writeFoundryTypes fails.
// A spec with hooks triggers writeFoundryTypes; pre-creating a file named
// "foundry" prevents MkdirAll("outDir/foundry") from succeeding.
func TestGenerate_FoundryTypesError(t *testing.T) {
	// Build a minimal IR with hooks directly so we don't need to
	// parse a spec file that requires hooks in the schema-validated form.
	// Use the fleet-manager example which has hooks.
	fleetSpec := "../../tsc/examples/fleet-manager/app.foundry.yaml"
	if _, statErr := os.Stat(fleetSpec); os.IsNotExist(statErr) {
		t.Skip("fleet-manager example not found")
	}

	ir, err := Parse(fleetSpec)
	if err != nil {
		t.Fatalf("Parse fleet-manager: %v", err)
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if len(ir.Hooks) == 0 {
		t.Skip("fleet-manager spec has no hooks; test requires a spec with hooks")
	}

	outDir := t.TempDir()
	// Pre-create a FILE named "foundry" — MkdirAll("outDir/foundry") will fail.
	if err := os.WriteFile(filepath.Join(outDir, "foundry"), []byte("blocker"), 0644); err != nil {
		t.Fatal(err)
	}

	genErr := NewGenerator(outDir, "").Generate(ir, components)
	if genErr == nil {
		t.Fatal("expected error from Generate when foundry/ is a file, got nil")
	}
	if !strings.Contains(genErr.Error(), "foundry") {
		t.Errorf("expected 'foundry' in error, got: %v", genErr)
	}
}

// TestGenerate_GoModError verifies that Generate returns an error mentioning
// "generating go.mod" when writeGoMod fails. Pre-creating a DIRECTORY named
// "go.mod" causes os.WriteFile to fail with EISDIR while writeMainGo (which
// writes "main.go", not "go.mod") succeeds first.
func TestGenerate_GoModError(t *testing.T) {
	ir, err := Parse(exampleSpecPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	outDir := t.TempDir()
	// Pre-create a DIRECTORY named "go.mod" — WriteFile("go.mod") will fail (EISDIR).
	if err := os.MkdirAll(filepath.Join(outDir, "go.mod"), 0755); err != nil {
		t.Fatal(err)
	}

	genErr := NewGenerator(outDir, "").Generate(ir, components)
	if genErr == nil {
		t.Fatal("expected error from Generate when go.mod is a dir, got nil")
	}
	if !strings.Contains(genErr.Error(), "generating go.mod") {
		t.Errorf("expected 'generating go.mod' in error, got: %v", genErr)
	}
}

// TestGenerate_HookRegistryError verifies that Generate returns an error
// mentioning "generating hook_registry.go" when writeHookRegistry fails.
// Pre-creating a DIRECTORY named "hook_registry.go" causes the WriteFile
// to fail while all sub-functions before it (including writeFoundryTypes
// which creates the foundry/ dir successfully) succeed.
func TestGenerate_HookRegistryError(t *testing.T) {
	fleetSpec := "../../tsc/examples/fleet-manager/app.foundry.yaml"
	if _, statErr := os.Stat(fleetSpec); os.IsNotExist(statErr) {
		t.Skip("fleet-manager example not found")
	}

	ir, err := Parse(fleetSpec)
	if err != nil {
		t.Fatalf("Parse fleet-manager: %v", err)
	}
	if len(ir.Hooks) == 0 {
		t.Skip("fleet-manager has no hooks; test requires hooks")
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	outDir := t.TempDir()
	// Pre-create a DIRECTORY named "hook_registry.go" — WriteFile fails (EISDIR).
	// writeFoundryTypes creates outDir/foundry/ first (succeeds); then
	// writeHookRegistry tries to write outDir/hook_registry.go which is a dir.
	if err := os.MkdirAll(filepath.Join(outDir, "hook_registry.go"), 0755); err != nil {
		t.Fatal(err)
	}

	genErr := NewGenerator(outDir, "").Generate(ir, components)
	if genErr == nil {
		t.Fatal("expected error from Generate when hook_registry.go is a dir, got nil")
	}
	if !strings.Contains(genErr.Error(), "hook_registry.go") {
		t.Errorf("expected 'hook_registry.go' in error, got: %v", genErr)
	}
}

// TestGenerate_ServiceMainsError verifies that Generate returns an error
// mentioning "generating service mains" when writeServiceMains fails. Pre-creating
// a DIRECTORY with the expected service main filename causes WriteFile to fail (EISDIR).
func TestGenerate_ServiceMainsError(t *testing.T) {
	fleetSpec := "../../tsc/examples/fleet-manager/app.foundry.yaml"
	if _, statErr := os.Stat(fleetSpec); os.IsNotExist(statErr) {
		t.Skip("fleet-manager example not found")
	}

	ir, err := Parse(fleetSpec)
	if err != nil {
		t.Fatalf("Parse fleet-manager: %v", err)
	}
	if len(ir.Services) == 0 {
		t.Skip("fleet-manager has no services; test requires services")
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	outDir := t.TempDir()
	// Determine the first service main filename and pre-create it as a DIR.
	firstSvc := ir.Services[0].Name
	svcMainFile := "main_" + strings.ReplaceAll(firstSvc, "-", "_") + ".go"
	if err := os.MkdirAll(filepath.Join(outDir, svcMainFile), 0755); err != nil {
		t.Fatal(err)
	}

	genErr := NewGenerator(outDir, "").Generate(ir, components)
	if genErr == nil {
		t.Fatal("expected error from Generate when service main file is a dir, got nil")
	}
	if !strings.Contains(genErr.Error(), "generating service mains") {
		t.Errorf("expected 'generating service mains' in error, got: %v", genErr)
	}
}

// TestGenerate_DockerComposeError verifies that Generate returns an error
// mentioning "generating docker-compose.yaml" when writeDockerCompose fails.
// Pre-creating a DIRECTORY named "docker-compose.yaml" causes WriteFile to fail.
func TestGenerate_DockerComposeError(t *testing.T) {
	fleetSpec := "../../tsc/examples/fleet-manager/app.foundry.yaml"
	if _, statErr := os.Stat(fleetSpec); os.IsNotExist(statErr) {
		t.Skip("fleet-manager example not found")
	}

	ir, err := Parse(fleetSpec)
	if err != nil {
		t.Fatalf("Parse fleet-manager: %v", err)
	}
	if len(ir.Services) == 0 {
		t.Skip("fleet-manager has no services; test requires services")
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	outDir := t.TempDir()
	// Pre-create a DIRECTORY named "docker-compose.yaml" — WriteFile fails (EISDIR).
	// writeServiceMains runs first (writes main_*.go files, which succeed).
	if err := os.MkdirAll(filepath.Join(outDir, "docker-compose.yaml"), 0755); err != nil {
		t.Fatal(err)
	}

	genErr := NewGenerator(outDir, "").Generate(ir, components)
	if genErr == nil {
		t.Fatal("expected error from Generate when docker-compose.yaml is a dir, got nil")
	}
	if !strings.Contains(genErr.Error(), "generating docker-compose.yaml") {
		t.Errorf("expected 'generating docker-compose.yaml' in error, got: %v", genErr)
	}
}

// --------------------------------------------------------------------------
// copyHookFiles — WriteFile error when hooks/ dir is read-only
// --------------------------------------------------------------------------

// TestCopyHookFiles_WriteFileError verifies that copyHookFiles returns an
// error mentioning "copying" when MkdirAll succeeds but WriteFile fails.
// This is achieved by pre-creating an empty hooks/ dir and making it read-only.
func TestCopyHookFiles_WriteFileError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories; skipping")
	}
	specDir := t.TempDir()
	hooksDir := filepath.Join(specDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "audit.go"), []byte("package hooks\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ir := &spec.IRSpec{
		Hooks: []spec.IRHook{
			{Name: "audit", Point: "pre-db", Implementation: "hooks/audit.go"},
		},
	}
	outDir := t.TempDir()

	// Pre-create the destination hooks/ dir and make it read-only so
	// MkdirAll (of the existing dir) succeeds but WriteFile inside it fails.
	destHooksDir := filepath.Join(outDir, "hooks")
	if err := os.MkdirAll(destHooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(destHooksDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(destHooksDir, 0755)

	err := copyHookFiles(ir, outDir, specDir)
	if err == nil {
		t.Fatal("expected error when hooks/ dir is read-only, got nil")
	}
	if !strings.Contains(err.Error(), "copying") {
		t.Errorf("expected 'copying' in error, got: %v", err)
	}
}
