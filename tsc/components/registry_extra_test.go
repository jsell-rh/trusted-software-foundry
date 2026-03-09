package components_test

// registry_extra_test.go covers branches not reached by registry_test.go:
//   New: duplicate entry panic, empty Name panic, empty Version panic,
//        empty Hash panic
//   CatalogSize
//   HashSourceDir: walk error (non-existent dir), hidden-file skip

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/components"
)

// --------------------------------------------------------------------------
// New — panic branches
// --------------------------------------------------------------------------

func TestNew_DuplicateCatalogEntry_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate catalog entry, got nil")
		} else if !strings.Contains(fmt.Sprint(r), "duplicate") {
			t.Errorf("unexpected panic value: %v", r)
		}
	}()
	components.New(
		components.AuditRecord{Name: "foundry-http", Version: "v1.0.0", Hash: "aaa"},
		components.AuditRecord{Name: "foundry-http", Version: "v1.0.0", Hash: "bbb"},
	)
}

func TestNew_EmptyName_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty Name, got nil")
		}
	}()
	components.New(components.AuditRecord{Name: "", Version: "v1.0.0", Hash: "abc"})
}

func TestNew_EmptyVersion_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty Version, got nil")
		}
	}()
	components.New(components.AuditRecord{Name: "foundry-http", Version: "", Hash: "abc"})
}

func TestNew_EmptyHash_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty Hash, got nil")
		}
	}()
	components.New(components.AuditRecord{Name: "foundry-http", Version: "v1.0.0", Hash: ""})
}

// --------------------------------------------------------------------------
// CatalogSize
// --------------------------------------------------------------------------

func TestCatalogSize(t *testing.T) {
	reg := components.New(
		components.AuditRecord{Name: "foundry-http", Version: "v1.0.0", Hash: "aaa"},
		components.AuditRecord{Name: "foundry-postgres", Version: "v1.0.0", Hash: "bbb"},
	)
	if got := reg.CatalogSize(); got != 2 {
		t.Errorf("CatalogSize() = %d, want 2", got)
	}
}

func TestCatalogSize_Empty(t *testing.T) {
	reg := components.New()
	if got := reg.CatalogSize(); got != 0 {
		t.Errorf("CatalogSize() = %d, want 0", got)
	}
}

// --------------------------------------------------------------------------
// HashSourceDir — walk error (non-existent dir)
// --------------------------------------------------------------------------

func TestHashSourceDir_NonExistentDir(t *testing.T) {
	_, err := components.HashSourceDir("/tmp/tsc-no-such-dir-xyz-12345")
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
	if !strings.Contains(err.Error(), "hashing dir") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// HashSourceDir — unreadable file causes error
// --------------------------------------------------------------------------

func TestHashSourceDir_UnreadableFile_Error(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — cannot test unreadable files")
	}
	dir := t.TempDir()
	fpath := filepath.Join(dir, "secret.go")
	if err := os.WriteFile(fpath, []byte("package p\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Remove read permission so os.ReadFile fails during walk.
	if err := os.Chmod(fpath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(fpath, 0600) }) //nolint:errcheck

	_, err := components.HashSourceDir(dir)
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
	if !strings.Contains(err.Error(), "hashing dir") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// HashSourceDir — hidden files are skipped
// --------------------------------------------------------------------------

func TestHashSourceDir_HiddenFileSkipped(t *testing.T) {
	dir := t.TempDir()

	// Write a visible file and a hidden file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	h1, err := components.HashSourceDir(dir)
	if err != nil {
		t.Fatalf("HashSourceDir: %v", err)
	}

	// Remove the hidden file — the hash should not change since it is skipped.
	if err := os.Remove(filepath.Join(dir, ".hidden")); err != nil {
		t.Fatal(err)
	}
	h2, err := components.HashSourceDir(dir)
	if err != nil {
		t.Fatalf("HashSourceDir after removing hidden: %v", err)
	}

	if h1 != h2 {
		t.Error("hidden file should be skipped: hash changed when hidden file was removed")
	}
}
