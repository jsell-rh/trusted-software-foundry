package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ResolvedComponent is a component whose registry entry and audit hash have been verified.
type ResolvedComponent struct {
	Name      string
	Version   string
	Module    string // Go module path, e.g. "github.com/openshift-online/foundry-components/http"
	AuditHash string // Expected SHA-256 from registry (hex)
}

// Registry is the interface that a trusted component registry must satisfy.
// TSC-Architect will define the canonical interface; this skeleton is forward-compatible.
// Once TSC-Architect publishes foundry/spec/component.go, we will import and embed it here.
type Registry interface {
	// Lookup returns the registry entry for a component at the given version.
	Lookup(name, version string) (*RegistryEntry, error)
}

// RegistryEntry is the registry record for one audited component version.
type RegistryEntry struct {
	Name      string
	Version   string
	Module    string
	AuditHash string // SHA-256 hex of the component source at audit time
}

// FileRegistry is a filesystem-backed registry that reads YAML index files.
// This is the development/local implementation; production will use a signed catalog.
type FileRegistry struct {
	indexDir string // directory containing per-component YAML files
}

// NewFileRegistry creates a registry backed by YAML files in indexDir.
func NewFileRegistry(indexDir string) *FileRegistry {
	return &FileRegistry{indexDir: indexDir}
}

// Lookup implements Registry using the local YAML index.
// Format: <indexDir>/<name>/<version>.yaml
func (r *FileRegistry) Lookup(name, version string) (*RegistryEntry, error) {
	path := filepath.Join(r.indexDir, name, version+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("component %q version %q not found in registry (path: %s)", name, version, path)
		}
		return nil, fmt.Errorf("reading registry entry for %q %q: %w", name, version, err)
	}

	entry, err := parseRegistryEntry(data)
	if err != nil {
		return nil, fmt.Errorf("parsing registry entry for %q %q: %w", name, version, err)
	}
	return entry, nil
}

// registryEntryYAML is the YAML wire format for a registry entry file.
// File path: <indexDir>/<name>/<version>.yaml
type registryEntryYAML struct {
	Name      string `yaml:"name"`
	Version   string `yaml:"version"`
	Module    string `yaml:"module"`
	AuditHash string `yaml:"audit_hash"`
}

// parseRegistryEntry parses a YAML registry entry.
// Expected format:
//
//	name: foundry-http
//	version: v1.0.0
//	module: github.com/openshift-online/foundry-components/http
//	audit_hash: <sha256-hex>
func parseRegistryEntry(data []byte) (*RegistryEntry, error) {
	var raw registryEntryYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid registry entry YAML: %w", err)
	}
	if raw.Name == "" {
		return nil, fmt.Errorf("registry entry missing required field: name")
	}
	if raw.Version == "" {
		return nil, fmt.Errorf("registry entry missing required field: version")
	}
	if raw.Module == "" {
		return nil, fmt.Errorf("registry entry missing required field: module")
	}
	return &RegistryEntry{
		Name:      raw.Name,
		Version:   raw.Version,
		Module:    raw.Module,
		AuditHash: raw.AuditHash,
	}, nil
}

// Resolver resolves and verifies component entries from the registry.
type Resolver struct {
	registry Registry
	// sourceDir is the directory containing local component source for hash verification.
	// May be empty when using pre-computed registry hashes.
	sourceDir string
}

// NewResolver creates a Resolver backed by the given registry.
func NewResolver(registry Registry, sourceDir string) *Resolver {
	return &Resolver{registry: registry, sourceDir: sourceDir}
}

// ResolveAll resolves all components declared in the spec components map and verifies their audit hashes.
// Returns a slice of resolved components.
func (r *Resolver) ResolveAll(components map[string]string) ([]ResolvedComponent, error) {
	resolved := make([]ResolvedComponent, 0, len(components))

	for name, version := range components {
		entry, err := r.registry.Lookup(name, version)
		if err != nil {
			return nil, fmt.Errorf("resolving component %q: %w", name, err)
		}

		if r.sourceDir != "" {
			if err := r.verifyAuditHash(name, version, entry.AuditHash); err != nil {
				return nil, fmt.Errorf("audit hash verification failed for %q %q: %w", name, version, err)
			}
		}

		resolved = append(resolved, ResolvedComponent{
			Name:      name,
			Version:   version,
			Module:    entry.Module,
			AuditHash: entry.AuditHash,
		})
	}

	return resolved, nil
}

// verifyAuditHash computes the SHA-256 of the component source directory and compares
// it to the expected hash from the registry audit record.
func (r *Resolver) verifyAuditHash(name, version, expectedHex string) error {
	componentDir := filepath.Join(r.sourceDir, name, version)

	h := sha256.New()
	err := filepath.Walk(componentDir, func(path string, info os.FileInfo, err error) error {
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
	})
	if err != nil {
		return fmt.Errorf("walking component source at %q: %w", componentDir, err)
	}

	actualHex := hex.EncodeToString(h.Sum(nil))
	if actualHex != expectedHex {
		return fmt.Errorf("hash mismatch: registry=%q actual=%q", expectedHex, actualHex)
	}
	return nil
}
