package compiler_test

// compile_errors_test.go covers the Compile() error paths that are not
// exercised by the happy-path integration tests. It ensures that every error
// stage (parse, resolve, generate) surfaces a useful, wrapped error message.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

// --------------------------------------------------------------------------
// Compile() — parse error paths
// --------------------------------------------------------------------------

func TestCompile_MissingSpecFile(t *testing.T) {
	// Compile with a path that doesn't exist → parse error.
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	err := c.Compile("/nonexistent/app.foundry.yaml", t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing spec file, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected 'parse' in error, got: %v", err)
	}
}

func TestCompile_InvalidSpecYAML(t *testing.T) {
	// Compile with syntactically invalid YAML → parse error.
	specFile := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(specFile, []byte("not: valid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	err := c.Compile(specFile, t.TempDir())
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected 'parse' in error, got: %v", err)
	}
}

func TestCompile_SpecFailsValidation(t *testing.T) {
	// Compile with a spec that parses but fails semantic validation
	// (missing apiVersion) → parse/validate error.
	specFile := filepath.Join(t.TempDir(), "invalid.yaml")
	invalidYAML := `kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
`
	if err := os.WriteFile(specFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatal(err)
	}
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	err := c.Compile(specFile, t.TempDir())
	if err == nil {
		t.Fatal("expected error for spec failing validation, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected 'parse' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Compile() — resolve error paths
// --------------------------------------------------------------------------

func TestCompile_UnknownComponentInSpec(t *testing.T) {
	// Spec references a component unknown to the registry → resolve error.
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	yaml := `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v1.0.0
  foundry-nonexistent-gadget: v9.0.0
`
	if err := os.WriteFile(specFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	// Note: the unknown component also triggers schema validation before reaching
	// the resolver. Either a "parse" or "resolve" error is acceptable since the
	// unknown component is caught at the validation layer.
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	err := c.Compile(specFile, t.TempDir())
	if err == nil {
		t.Fatal("expected error for unknown component, got nil")
	}
	// error must mention the bad component name (validation or resolve)
	if !strings.Contains(err.Error(), "foundry-nonexistent-gadget") && !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected error to mention the bad component or 'parse', got: %v", err)
	}
}

func TestCompile_ResolverVersionMismatch(t *testing.T) {
	// StubRegistry only knows v1.0.0; spec requests v9.9.9 → resolve error.
	// (The schema allows any v-prefixed semver, so this passes schema validation.)
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	yaml := `apiVersion: foundry/v1
kind: Application
metadata:
  name: test-app
  version: 1.0.0
components:
  foundry-http: v9.9.9
`
	if err := os.WriteFile(specFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	c := compiler.New(compiler.NewStubRegistry(), "", "")
	err := c.Compile(specFile, t.TempDir())
	if err == nil {
		t.Fatal("expected error for version not in registry, got nil")
	}
	if !strings.Contains(err.Error(), "resolve") {
		t.Errorf("expected 'resolve' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Compile() — generate error paths
// --------------------------------------------------------------------------

func TestCompile_UnwritableOutputDir(t *testing.T) {
	// Create a read-only parent directory; Compile should fail with a generate error
	// when it tries to create the output dir.
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere; skip unwritable-dir test")
	}

	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	yaml := `apiVersion: foundry/v1
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
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
`
	if err := os.WriteFile(specFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	// Make a read-only parent dir so MkdirAll of the output subdir fails.
	roParent := filepath.Join(t.TempDir(), "readonly")
	if err := os.MkdirAll(roParent, 0555); err != nil {
		t.Fatal(err)
	}
	unwritableOut := filepath.Join(roParent, "output")

	c := compiler.New(compiler.NewStubRegistry(), "", "")
	err := c.Compile(specFile, unwritableOut)
	if err == nil {
		t.Fatal("expected error for unwritable output dir, got nil")
	}
	if !strings.Contains(err.Error(), "generate") {
		t.Errorf("expected 'generate' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Compile() — go mod tidy error path (rhtexAIPath != "")
// --------------------------------------------------------------------------

// TestCompile_GoModTidyError verifies that when Compile is given a non-empty
// rhtexAIPath that points to a non-existent directory, the generated go.mod
// contains a replace directive pointing to that path, and the subsequent
// `go mod tidy` step fails, returning an error mentioning "go mod tidy".
// This exercises the Compile step-5 block (lines 61-67) that is otherwise
// unreachable when rhtexAIPath is empty (as in all other tests).
func TestCompile_GoModTidyError(t *testing.T) {
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	yaml := `apiVersion: foundry/v1
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
  - name: Item
    plural: items
    fields:
      - name: id
        type: uuid
        required: true
    operations: [create, read]
    events: false
`
	if err := os.WriteFile(specFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	// Pass a fake local path — the replace directive will reference it and
	// go mod tidy will fail because the directory doesn't exist.
	fakeLocalPath := "/nonexistent/foundry-local"
	c := compiler.New(compiler.NewStubRegistry(), "", fakeLocalPath)
	err := c.Compile(specFile, t.TempDir())
	if err == nil {
		t.Fatal("expected error from go mod tidy with invalid replace path, got nil")
	}
	if !strings.Contains(err.Error(), "go mod tidy") {
		t.Errorf("expected 'go mod tidy' in error, got: %v", err)
	}
}
