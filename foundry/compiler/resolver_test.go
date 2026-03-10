package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

var allComponents = []struct {
	name    string
	version string
	module  string
}{
	{"foundry-http", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/http"},
	{"foundry-postgres", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/postgres"},
	{"foundry-auth-jwt", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/auth/jwt"},
	{"foundry-auth-ocm", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/auth/ocm"},
	{"foundry-health", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/health"},
	{"foundry-metrics", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/metrics"},
	{"foundry-tenancy", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/tenancy"},
	{"foundry-logging", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/logging"},
	{"foundry-errortracker", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/errortracker"},
	{"foundry-errortracker-sentry", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/foundry/components/errortracker-sentry"},
}

func testdataRegistryDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "registry")
}

// --- StubRegistry tests ---

func TestStubRegistry_KnownComponents(t *testing.T) {
	reg := NewStubRegistry()
	for _, tc := range allComponents {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := reg.Lookup(tc.name, tc.version)
			if err != nil {
				t.Fatalf("Lookup(%q, %q) unexpected error: %v", tc.name, tc.version, err)
			}
			if entry.Name != tc.name {
				t.Errorf("entry.Name = %q, want %q", entry.Name, tc.name)
			}
			if entry.Version != tc.version {
				t.Errorf("entry.Version = %q, want %q", entry.Version, tc.version)
			}
			if entry.Module != tc.module {
				t.Errorf("entry.Module = %q, want %q", entry.Module, tc.module)
			}
		})
	}
}

func TestStubRegistry_UnknownComponent(t *testing.T) {
	reg := NewStubRegistry()
	_, err := reg.Lookup("foundry-unknown", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for unknown component, got nil")
	}
	if !strings.Contains(err.Error(), "unknown component") {
		t.Errorf("error %q should mention 'unknown component'", err.Error())
	}
}

func TestStubRegistry_WrongVersion(t *testing.T) {
	reg := NewStubRegistry()
	_, err := reg.Lookup("foundry-http", "v9.9.9")
	if err == nil {
		t.Fatal("expected error for wrong version, got nil")
	}
	if !strings.Contains(err.Error(), "v9.9.9") {
		t.Errorf("error %q should mention the requested version", err.Error())
	}
}

// --- FileRegistry tests ---

func TestFileRegistry_KnownComponents(t *testing.T) {
	dir := testdataRegistryDir(t)
	reg := NewFileRegistry(dir)
	for _, tc := range allComponents {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := reg.Lookup(tc.name, tc.version)
			if err != nil {
				t.Fatalf("Lookup(%q, %q) unexpected error: %v", tc.name, tc.version, err)
			}
			if entry.Name != tc.name {
				t.Errorf("entry.Name = %q, want %q", entry.Name, tc.name)
			}
			if entry.Version != tc.version {
				t.Errorf("entry.Version = %q, want %q", entry.Version, tc.version)
			}
			if entry.Module != tc.module {
				t.Errorf("entry.Module = %q, want %q", entry.Module, tc.module)
			}
			if entry.AuditHash == "" {
				t.Error("entry.AuditHash should not be empty")
			}
		})
	}
}

func TestFileRegistry_UnknownComponent(t *testing.T) {
	dir := testdataRegistryDir(t)
	reg := NewFileRegistry(dir)
	_, err := reg.Lookup("foundry-unknown", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for unknown component, got nil")
	}
	if !strings.Contains(err.Error(), "not found in registry") {
		t.Errorf("error %q should mention 'not found in registry'", err.Error())
	}
}

func TestFileRegistry_WrongVersion(t *testing.T) {
	dir := testdataRegistryDir(t)
	reg := NewFileRegistry(dir)
	_, err := reg.Lookup("foundry-http", "v9.9.9")
	if err == nil {
		t.Fatal("expected error for wrong version, got nil")
	}
	if !strings.Contains(err.Error(), "not found in registry") {
		t.Errorf("error %q should mention 'not found in registry'", err.Error())
	}
}

// --- parseRegistryEntry unit tests ---

func TestParseRegistryEntry_Valid(t *testing.T) {
	data := []byte(`
name: foundry-http
version: v1.0.0
module: github.com/openshift-online/tsc-components/http
audit_hash: abc123
`)
	entry, err := parseRegistryEntry(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Name != "foundry-http" {
		t.Errorf("Name = %q, want %q", entry.Name, "foundry-http")
	}
	if entry.Module != "github.com/openshift-online/tsc-components/http" {
		t.Errorf("Module = %q, want %q", entry.Module, "github.com/openshift-online/tsc-components/http")
	}
	if entry.AuditHash != "abc123" {
		t.Errorf("AuditHash = %q, want %q", entry.AuditHash, "abc123")
	}
}

func TestParseRegistryEntry_MissingName(t *testing.T) {
	data := []byte(`
version: v1.0.0
module: github.com/openshift-online/tsc-components/http
audit_hash: abc123
`)
	_, err := parseRegistryEntry(data)
	if err == nil {
		t.Fatal("expected error for missing name field")
	}
}

func TestParseRegistryEntry_MissingVersion(t *testing.T) {
	data := []byte(`
name: foundry-http
module: github.com/openshift-online/tsc-components/http
audit_hash: abc123
`)
	_, err := parseRegistryEntry(data)
	if err == nil {
		t.Fatal("expected error for missing version field")
	}
}

func TestParseRegistryEntry_MissingModule(t *testing.T) {
	data := []byte(`
name: foundry-http
version: v1.0.0
audit_hash: abc123
`)
	_, err := parseRegistryEntry(data)
	if err == nil {
		t.Fatal("expected error for missing module field")
	}
}

func TestParseRegistryEntry_InvalidYAML(t *testing.T) {
	data := []byte(`{invalid yaml: [`)
	_, err := parseRegistryEntry(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// --------------------------------------------------------------------------
// verifyAuditHash tests — covers the 0% branch that is the core security
// mechanism of the Trusted Software Foundry.
// --------------------------------------------------------------------------

// computeTestHash walks dir and returns the SHA-256 of all file contents,
// mirroring the algorithm used by verifyAuditHash.
func computeTestHash(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("computing test hash: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// makeCustomRegistryAndSource creates:
//   - A registry dir with one entry for "foundry-custom" at v1.0.0 using expectedHash
//   - A source dir with the component source files that produce expectedHash
//
// Returns (registryDir, sourceDir).
func makeCustomRegistryAndSource(t *testing.T, module, expectedHash string) (registryDir, sourceDir string) {
	t.Helper()

	// Registry dir
	registryDir = t.TempDir()
	compRegistryDir := filepath.Join(registryDir, "foundry-custom")
	if err := os.MkdirAll(compRegistryDir, 0755); err != nil {
		t.Fatal(err)
	}
	regYAML := fmt.Sprintf("name: foundry-custom\nversion: v1.0.0\nmodule: %s\naudit_hash: %s\n", module, expectedHash)
	if err := os.WriteFile(filepath.Join(compRegistryDir, "v1.0.0.yaml"), []byte(regYAML), 0644); err != nil {
		t.Fatal(err)
	}
	return registryDir, ""
}

func TestVerifyAuditHash_Match(t *testing.T) {
	// Build a fake component source tree, compute its hash, register it, then
	// verify that ResolveAll succeeds when the hash matches.
	sourceBase := t.TempDir()
	compDir := filepath.Join(sourceBase, "foundry-custom", "v1.0.0")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "component.go"), []byte("package custom\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "register.go"), []byte("package custom\nfunc New() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	correctHash := computeTestHash(t, compDir)

	// Build registry with the correct hash
	registryDir := t.TempDir()
	regEntryDir := filepath.Join(registryDir, "foundry-custom")
	if err := os.MkdirAll(regEntryDir, 0755); err != nil {
		t.Fatal(err)
	}
	regYAML := fmt.Sprintf(
		"name: foundry-custom\nversion: v1.0.0\nmodule: github.com/test/foundry-custom\naudit_hash: %s\n",
		correctHash,
	)
	if err := os.WriteFile(filepath.Join(regEntryDir, "v1.0.0.yaml"), []byte(regYAML), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewFileRegistry(registryDir)
	resolver := NewResolver(reg, sourceBase)
	components, err := resolver.ResolveAll(map[string]string{"foundry-custom": "v1.0.0"})
	if err != nil {
		t.Fatalf("ResolveAll failed with correct audit hash: %v", err)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 resolved component, got %d", len(components))
	}
	if components[0].AuditHash != correctHash {
		t.Errorf("component AuditHash = %q, want %q", components[0].AuditHash, correctHash)
	}
}

func TestVerifyAuditHash_Mismatch(t *testing.T) {
	// Build a fake component source tree, but register a WRONG hash.
	// ResolveAll must return an error mentioning "hash mismatch".
	sourceBase := t.TempDir()
	compDir := filepath.Join(sourceBase, "foundry-custom", "v1.0.0")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "component.go"), []byte("package custom\n"), 0644); err != nil {
		t.Fatal(err)
	}

	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

	registryDir := t.TempDir()
	regEntryDir := filepath.Join(registryDir, "foundry-custom")
	if err := os.MkdirAll(regEntryDir, 0755); err != nil {
		t.Fatal(err)
	}
	regYAML := fmt.Sprintf(
		"name: foundry-custom\nversion: v1.0.0\nmodule: github.com/test/foundry-custom\naudit_hash: %s\n",
		wrongHash,
	)
	if err := os.WriteFile(filepath.Join(regEntryDir, "v1.0.0.yaml"), []byte(regYAML), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewFileRegistry(registryDir)
	resolver := NewResolver(reg, sourceBase)
	_, err := resolver.ResolveAll(map[string]string{"foundry-custom": "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for audit hash mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("expected error to mention 'hash mismatch', got: %v", err)
	}
}

func TestVerifyAuditHash_MissingSourceDirectory(t *testing.T) {
	// sourceDir is set but the component source directory does not exist.
	// ResolveAll must return an error mentioning the walking failure.
	sourceBase := t.TempDir()
	// Note: do NOT create the compDir — it doesn't exist on disk.

	registryDir := t.TempDir()
	regEntryDir := filepath.Join(registryDir, "foundry-custom")
	if err := os.MkdirAll(regEntryDir, 0755); err != nil {
		t.Fatal(err)
	}
	regYAML := "name: foundry-custom\nversion: v1.0.0\nmodule: github.com/test/foundry-custom\naudit_hash: abc123\n"
	if err := os.WriteFile(filepath.Join(regEntryDir, "v1.0.0.yaml"), []byte(regYAML), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewFileRegistry(registryDir)
	resolver := NewResolver(reg, sourceBase)
	_, err := resolver.ResolveAll(map[string]string{"foundry-custom": "v1.0.0"})
	if err == nil {
		t.Fatal("expected error when component source directory is missing, got nil")
	}
	if !strings.Contains(err.Error(), "audit hash verification failed") {
		t.Errorf("expected error to mention 'audit hash verification failed', got: %v", err)
	}
}

// TestFileRegistry_MalformedYAMLEntry verifies that FileRegistry.Lookup returns
// a wrapped error mentioning "parsing registry entry" when the on-disk YAML file
// exists but contains fields that fail parseRegistryEntry validation (missing module).
// This covers the resolver.go:63 error path that is not triggered by the
// "not found" tests (which exercise the os.IsNotExist path instead).
func TestFileRegistry_MalformedYAMLEntry(t *testing.T) {
	registryDir := t.TempDir()
	entryDir := filepath.Join(registryDir, "foundry-broken")
	if err := os.MkdirAll(entryDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Missing required 'module' field — parseRegistryEntry will return an error.
	malformedYAML := "name: foundry-broken\nversion: v1.0.0\naudit_hash: abc123\n"
	if err := os.WriteFile(filepath.Join(entryDir, "v1.0.0.yaml"), []byte(malformedYAML), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewFileRegistry(registryDir)
	_, err := reg.Lookup("foundry-broken", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for malformed registry entry, got nil")
	}
	if !strings.Contains(err.Error(), "parsing registry entry") {
		t.Errorf("expected 'parsing registry entry' in error, got: %v", err)
	}
}

// TestFileRegistry_UnreadableEntry verifies that FileRegistry.Lookup returns
// a wrapped error (not an "not found" error) when the registry YAML file exists
// but is unreadable (permission denied). This covers the non-os.IsNotExist
// branch at resolver.go:58.
func TestFileRegistry_UnreadableEntry(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read any file; skip")
	}
	registryDir := t.TempDir()
	entryDir := filepath.Join(registryDir, "foundry-locked")
	if err := os.MkdirAll(entryDir, 0755); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(entryDir, "v1.0.0.yaml")
	if err := os.WriteFile(entryPath, []byte("name: foundry-locked\nversion: v1.0.0\nmodule: github.com/test/foundry-locked\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Make the file unreadable.
	if err := os.Chmod(entryPath, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(entryPath, 0644) // restore so TempDir cleanup works

	reg := NewFileRegistry(registryDir)
	_, err := reg.Lookup("foundry-locked", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for unreadable registry entry, got nil")
	}
	// Must NOT be the "not found" message — it's a permission error.
	if strings.Contains(err.Error(), "not found in registry") {
		t.Errorf("expected a read error, not a 'not found' error; got: %v", err)
	}
	// Should mention reading the entry.
	if !strings.Contains(err.Error(), "reading registry entry") {
		t.Errorf("expected 'reading registry entry' in error, got: %v", err)
	}
}

// TestVerifyAuditHash_UnreadableFile verifies that verifyAuditHash returns an
// error when a file inside the component directory exists (walkable) but is
// not readable. This covers the os.Open error branch inside the Walk callback
// (resolver.go:160-162). The test skips on root since root can open any file.
func TestVerifyAuditHash_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can open any file; skip")
	}

	sourceBase := t.TempDir()
	compDir := filepath.Join(sourceBase, "foundry-custom", "v1.0.0")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a file then make it unreadable (mode 0000).
	secretFile := filepath.Join(compDir, "secret.go")
	if err := os.WriteFile(secretFile, []byte("package custom\n"), 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(secretFile, 0644) // restore so TempDir cleanup works

	registryDir := t.TempDir()
	regEntryDir := filepath.Join(registryDir, "foundry-custom")
	if err := os.MkdirAll(regEntryDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regEntryDir, "v1.0.0.yaml"),
		[]byte("name: foundry-custom\nversion: v1.0.0\nmodule: github.com/test/foundry-custom\naudit_hash: abc123\n"),
		0644); err != nil {
		t.Fatal(err)
	}

	reg := NewFileRegistry(registryDir)
	resolver := NewResolver(reg, sourceBase)
	_, err := resolver.ResolveAll(map[string]string{"foundry-custom": "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for unreadable component file, got nil")
	}
	if !strings.Contains(err.Error(), "audit hash verification failed") {
		t.Errorf("expected 'audit hash verification failed' in error, got: %v", err)
	}
}

// TestCopyHookFiles_EmptyImplementation verifies that a hook with an empty
// Implementation field is silently skipped by copyHookFiles. This covers the
// `if h.Implementation == ""` continue branch (generator_v2.go:478-480).
func TestCopyHookFiles_EmptyImplementation(t *testing.T) {
	ir := &spec.IRSpec{
		Hooks: []spec.IRHook{
			{Name: "no-impl", Point: "pre-db", Implementation: ""},
			{Name: "with-impl", Point: "pre-db", Implementation: "hooks/real.go"},
		},
	}
	// No source files exist → both hooks skipped (empty-impl via continue,
	// with-impl via ReadFile error graceful skip).
	outDir := t.TempDir()
	if _, err := copyHookFiles(ir, outDir, ""); err != nil {
		t.Fatalf("copyHookFiles: %v", err)
	}
	// Output directory should be empty — no files copied.
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("expected empty output dir, got: %v", entries)
	}
}
