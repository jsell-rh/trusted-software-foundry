package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

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
// hook_registry.go with the correct type-safe call sites.
// Hook types (HookContext, DBOperation, etc.) are imported from the canonical
// upstream package (foundry/spec/foundry) rather than being generated locally.
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

	// Assert: no local foundry/types.go — types come from upstream canonical package.
	if _, err := os.Stat(filepath.Join(outDir, "foundry", "types.go")); err == nil {
		t.Error("foundry/types.go should NOT be generated: types come from foundry/spec/foundry upstream package")
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
		// imports canonical upstream foundry types package
		"foundry/spec/foundry",
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
