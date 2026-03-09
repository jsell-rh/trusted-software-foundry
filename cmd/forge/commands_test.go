package main

// commands_test.go covers the cobra command wrappers that were not exercised
// by the existing cli_test.go and agent_tools_test.go:
//
//   - scaffoldCmd: missing --name, stdout output, file output, write error
//   - explainCmd: parse-error path
//   - compileCmd: --rh-trex-ai flag (alternate success message)
//
// lintCmd's os.Exit(1) path is intentionally omitted — os.Exit terminates
// the test binary and cannot be exercised with a unit test.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// scaffoldCmd
// --------------------------------------------------------------------------

func TestScaffoldCmd_MissingName(t *testing.T) {
	cmd := scaffoldCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --name is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--name is required") {
		t.Errorf("expected '--name is required' in error, got: %v", err)
	}
}

func TestScaffoldCmd_ToStdout(t *testing.T) {
	// Capture os.Stdout; default --output is "-" which prints to stdout.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	cmd := scaffoldCmd()
	cmd.SetArgs([]string{"--name", "my-service", "--resource", "Widget"})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	if runErr != nil {
		t.Fatalf("scaffoldCmd to stdout failed: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "name: my-service") {
		t.Errorf("expected 'name: my-service' in scaffold output, got:\n%s", out)
	}
	if !strings.Contains(out, "name: Widget") {
		t.Errorf("expected 'name: Widget' in scaffold output, got:\n%s", out)
	}
}

func TestScaffoldCmd_ToFile(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "app.foundry.yaml")

	origStdout := os.Stdout
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	cmd := scaffoldCmd()
	cmd.SetArgs([]string{
		"--name", "fleet-manager",
		"--version", "2.0.0",
		"--output", outFile,
	})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	if runErr != nil {
		t.Fatalf("scaffoldCmd to file failed: %v", runErr)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if !strings.Contains(string(data), "name: fleet-manager") {
		t.Errorf("expected 'name: fleet-manager' in output file")
	}
	if !strings.Contains(string(data), "version: 2.0.0") {
		t.Errorf("expected 'version: 2.0.0' in output file")
	}
}

func TestScaffoldCmd_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories; skip")
	}

	// Create a read-only directory so WriteFile fails.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(dir, 0755)

	outFile := filepath.Join(dir, "app.foundry.yaml")

	cmd := scaffoldCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--name", "my-service", "--output", outFile})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unwritable output file, got nil")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Errorf("expected 'writing' in error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// explainCmd — parse error path and description truncation
// --------------------------------------------------------------------------

func TestExplainCmd_BadSpecPath(t *testing.T) {
	cmd := explainCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"/nonexistent/path/to/app.foundry.yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent spec path, got nil")
	}
	if !strings.Contains(err.Error(), "parsing spec") {
		t.Errorf("expected 'parsing spec' in error, got: %v", err)
	}
}

// TestExplainCmd_GRPCDefaultPort verifies that when grpc.port is unspecified
// (zero value), explainCmd defaults to port 9000 in the output.
func TestExplainCmd_GRPCDefaultPort(t *testing.T) {
	// grpc.port is intentionally omitted so IRGRPCConfig.Port == 0 → defaults to 9000.
	yamlContent := `apiVersion: foundry/v1
kind: Application
metadata:
  name: grpc-default-app
  version: 1.0.0
components:
  foundry-http:     v1.0.0
  foundry-postgres: v1.0.0
  foundry-auth-jwt: v1.0.0
  foundry-grpc:     v1.0.0
  foundry-health:   v1.0.0
  foundry-metrics:  v1.0.0
  foundry-events:   v1.0.0
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
auth:
  type: jwt
  jwk_url: "https://example.com/.well-known/jwks.json"
api:
  grpc:
    enabled: true
`
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	if err := os.WriteFile(specFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	cmd := explainCmd()
	cmd.SetArgs([]string{specFile})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	if runErr != nil {
		t.Fatalf("explainCmd failed: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "gRPC port: 9000") {
		t.Errorf("expected 'gRPC port: 9000' in output (default fallback), got:\n%s", out)
	}
}

// TestExplainCmd_LongDescription verifies that a description longer than 120
// characters is truncated to 117 chars + "..." in the explain output.
func TestExplainCmd_LongDescription(t *testing.T) {
	// Build a minimal valid spec with a very long description.
	longDesc := strings.Repeat("x", 130)
	yamlContent := `apiVersion: foundry/v1
kind: Application
metadata:
  name: long-desc-app
  version: 1.0.0
  description: >
    ` + longDesc + `
components:
  foundry-http: v1.0.0
`
	specFile := filepath.Join(t.TempDir(), "app.foundry.yaml")
	if err := os.WriteFile(specFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	cmd := explainCmd()
	cmd.SetArgs([]string{specFile})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	if runErr != nil {
		t.Fatalf("explainCmd failed: %v", runErr)
	}
	out := buf.String()
	// Truncated description ends with "..."
	if !strings.Contains(out, "...") {
		t.Errorf("expected truncated description ending with '...', got:\n%s", out)
	}
}

// --------------------------------------------------------------------------
// compileCmd — compile failure error path and --rh-trex-ai alternate message
// --------------------------------------------------------------------------

// TestCompileCmd_CompileFailure verifies that compileCmd returns an error
// wrapping "compilation failed" when the spec file cannot be parsed.
func TestCompileCmd_CompileFailure(t *testing.T) {
	// A spec file that doesn't parse (wrong extension / missing) triggers the error.
	cmd := compileCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--output", t.TempDir(), "/nonexistent/app.foundry.yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for bad spec path, got nil")
	}
	if !strings.Contains(err.Error(), "compilation failed") {
		t.Errorf("expected 'compilation failed' in error, got: %v", err)
	}
}

func TestCompileCmd_WithRhTrexAI(t *testing.T) {
	specPath := filepath.Join("..", "..", "tsc", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("dinosaur-registry spec not found")
	}

	// Use the real checkout root so the replace directive in go.mod resolves.
	// This file is at cmd/forge/commands_test.go; two levels up is the module root.
	rhTrexAIPath, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving rh-trex-ai path: %v", err)
	}

	outDir := t.TempDir()

	// Capture stdout to verify the alternate run message is printed.
	origStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("os.Pipe: %v", pipeErr)
	}
	os.Stdout = w

	cmd := compileCmd()
	// Provide the real --rh-trex-ai path to trigger the alternate message branch.
	cmd.SetArgs([]string{
		"--output", outDir,
		"--rh-trex-ai", rhTrexAIPath,
		specPath,
	})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	if runErr != nil {
		t.Fatalf("compileCmd with --rh-trex-ai failed: %v", runErr)
	}

	out := buf.String()
	if !strings.Contains(out, "Compiled") {
		t.Errorf("expected 'Compiled' in output, got: %q", out)
	}
	// The --rh-trex-ai branch prints "go build -o app ." without the disclaimer.
	if !strings.Contains(out, "go build -o app .") {
		t.Errorf("expected 'go build -o app .' in output with --rh-trex-ai, got: %q", out)
	}
}
