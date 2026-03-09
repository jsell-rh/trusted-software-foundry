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
)

var allComponents = []struct {
	name    string
	version string
	module  string
}{
	{"foundry-http", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/http"},
	{"foundry-postgres", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/postgres"},
	{"foundry-auth-jwt", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/auth/jwt"},
	{"foundry-grpc", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/grpc"},
	{"foundry-health", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/health"},
	{"foundry-metrics", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/metrics"},
	{"foundry-events", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/events"},
	{"foundry-auth-spicedb", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/spicedb"},
	{"foundry-graph-age", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/graph/age"},
	{"foundry-kafka", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/kafka"},
	{"foundry-nats", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/nats"},
	{"foundry-redis", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/redis"},
	{"foundry-redis-streams", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/redis-streams"},
	{"foundry-temporal", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/temporal"},
	{"foundry-tenancy", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/tenancy"},
	{"foundry-service-router", "v1.0.0", "github.com/jsell-rh/trusted-software-foundry/tsc/components/service-router"},
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
	_, err := reg.Lookup("tsc-unknown", "v1.0.0")
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
	_, err := reg.Lookup("tsc-unknown", "v1.0.0")
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
