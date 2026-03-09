package main

// cli_test.go exercises the Cobra command wrappers for lint and compile.
// These tests verify that CLI flags are wired correctly and that commands
// produce the expected output/exit behaviour for happy and error paths.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// lintCmd tests
// --------------------------------------------------------------------------

// captureLint runs lintCmd against specPath and returns (stdout, error).
// The command is expected to succeed (exit 0). For specs that would call
// os.Exit(1), use a subprocess test instead.
func captureLint(t *testing.T, specPath string) (string, error) {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	cmd := lintCmd()
	cmd.SetArgs([]string{specPath})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	return buf.String(), runErr
}

func TestLint_ValidSpec_DinosaurRegistry(t *testing.T) {
	specPath := filepath.Join("..", "..", "tsc", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("dinosaur-registry spec not found")
	}

	out, err := captureLint(t, specPath)
	if err != nil {
		t.Fatalf("lintCmd failed for valid spec: %v", err)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("expected 'OK' in lint output, got: %q", out)
	}
	if !strings.Contains(out, "dinosaur-registry") || !strings.Contains(out, "app.foundry.yaml") {
		t.Errorf("expected spec path in lint output, got: %q", out)
	}
}

func TestLint_ValidSpec_FleetManager(t *testing.T) {
	specPath := filepath.Join("..", "..", "tsc", "examples", "fleet-manager", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("fleet-manager spec not found")
	}

	out, err := captureLint(t, specPath)
	if err != nil {
		t.Fatalf("lintCmd failed for valid fleet-manager spec: %v", err)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("expected 'OK' in lint output, got: %q", out)
	}
}

func TestLint_ValidSpec_Scaffolded(t *testing.T) {
	// A scaffolded spec must lint clean — this guards renderScaffold
	// against producing specs that the linter would reject.
	spec := renderScaffold("lint-test-app", "1.0.0", []string{"Record", "Tag"})
	tmp := filepath.Join(t.TempDir(), "app.foundry.yaml")
	if err := os.WriteFile(tmp, []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := captureLint(t, tmp)
	if err != nil {
		t.Fatalf("lintCmd failed for scaffolded spec: %v\nSpec:\n%s", err, spec)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("expected 'OK' in lint output, got: %q", out)
	}
}

// Note: TestLint_InvalidSpec is intentionally omitted. lintCmd calls os.Exit(1)
// on validation failure, which would terminate the test process. The error path
// is covered at the compiler.ParseWithSchema level in tsc/compiler/parser_test.go
// and tsc/compiler/validation_test.go. To test lint's exit-code behaviour,
// use an exec.Command subprocess test (deferred to future work).

// --------------------------------------------------------------------------
// compileCmd tests
// --------------------------------------------------------------------------

func TestCompileCmd_MissingOutputFlag(t *testing.T) {
	// compileCmd requires --output; without it, RunE returns an error
	// (cobra propagates this without calling os.Exit).
	specPath := filepath.Join("..", "..", "tsc", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("dinosaur-registry spec not found")
	}

	cmd := compileCmd()
	cmd.SetArgs([]string{specPath}) // no --output flag
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --output is missing, got nil")
	}
	if !strings.Contains(err.Error(), "output") && !strings.Contains(err.Error(), "--output") {
		t.Errorf("expected error to mention '--output', got: %v", err)
	}
}

func TestCompileCmd_ValidSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "tsc", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("dinosaur-registry spec not found")
	}

	outDir := t.TempDir()

	// Capture stdout
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	cmd := compileCmd()
	cmd.SetArgs([]string{"--output", outDir, specPath})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	if runErr != nil {
		t.Fatalf("compileCmd failed: %v", runErr)
	}

	out := buf.String()
	if !strings.Contains(out, "Compiled") {
		t.Errorf("expected 'Compiled' in output, got: %q", out)
	}

	// Verify the output directory has the generated files
	if _, err := os.Stat(filepath.Join(outDir, "main.go")); err != nil {
		t.Errorf("main.go not generated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "go.mod")); err != nil {
		t.Errorf("go.mod not generated: %v", err)
	}
}

func TestCompileCmd_WithFileRegistry(t *testing.T) {
	specPath := filepath.Join("..", "..", "tsc", "examples", "dinosaur-registry", "app.foundry.yaml")
	registryDir := filepath.Join("..", "..", "tsc", "compiler", "testdata", "registry")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("dinosaur-registry spec not found")
	}
	if _, err := os.Stat(registryDir); os.IsNotExist(err) {
		t.Skip("testdata/registry not found")
	}

	outDir := t.TempDir()

	// Suppress stdout
	origStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	cmd := compileCmd()
	cmd.SetArgs([]string{"--output", outDir, "--registry", registryDir, specPath})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	if runErr != nil {
		t.Fatalf("compileCmd with --registry failed: %v", runErr)
	}

	if _, err := os.Stat(filepath.Join(outDir, "main.go")); err != nil {
		t.Errorf("main.go not generated when using FileRegistry: %v", err)
	}
}
