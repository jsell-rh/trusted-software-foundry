package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// forge sbom — Software Bill of Materials (CycloneDX JSON format)
//
// Generates a CycloneDX 1.5 SBOM enumerating every trusted component pinned
// in the IR spec. This enables:
//   - Supply chain security audits (knowing exactly what's in every binary)
//   - Vulnerability scanning (match component names/versions against CVE DBs)
//   - License compliance review
//   - Sigstore/RHTAS attestation workflows
//
// Output: sbom.cdx.json in the output directory.
// --------------------------------------------------------------------------

// CycloneDX JSON schema version used.
const cycloneDXSchemaVersion = "1.5"

// SBOM generates a CycloneDX 1.5 JSON SBOM for the IR spec and writes it to
// <outputDir>/sbom.cdx.json. The SBOM enumerates all trusted components with
// their pinned versions and the application metadata.
func SBOM(ir *spec.IRSpec, outputDir string) error {
	bom := buildCycloneDX(ir)
	data, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling SBOM: %w", err)
	}
	dest := filepath.Join(outputDir, "sbom.cdx.json")
	if err := os.WriteFile(dest, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("writing sbom.cdx.json: %w", err)
	}
	return nil
}

// SBOMToWriter marshals the SBOM to a bytes.Buffer for testing without filesystem.
func SBOMToWriter(ir *spec.IRSpec) (*bytes.Buffer, error) {
	bom := buildCycloneDX(ir)
	data, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(data)
	buf.WriteByte('\n')
	return &buf, nil
}

// --------------------------------------------------------------------------
// CycloneDX types — minimal subset of CycloneDX 1.5 JSON schema.
// --------------------------------------------------------------------------

type cdxBOM struct {
	BOMFormat    string         `json:"bomFormat"`
	SpecVersion  string         `json:"specVersion"`
	SerialNumber string         `json:"serialNumber,omitempty"`
	Version      int            `json:"version"`
	Metadata     cdxMetadata    `json:"metadata"`
	Components   []cdxComponent `json:"components"`
}

type cdxMetadata struct {
	Timestamp string       `json:"timestamp"`
	Tools     []cdxTool    `json:"tools"`
	Component cdxComponent `json:"component"`
}

type cdxTool struct {
	Vendor  string `json:"vendor"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type cdxComponent struct {
	Type        string        `json:"type"`
	BOMRef      string        `json:"bom-ref,omitempty"`
	Name        string        `json:"name"`
	Version     string        `json:"version,omitempty"`
	Description string        `json:"description,omitempty"`
	PackageURL  string        `json:"purl,omitempty"`
	Properties  []cdxProperty `json:"properties,omitempty"`
}

type cdxProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func buildCycloneDX(ir *spec.IRSpec) cdxBOM {
	// Sort component names for deterministic output.
	names := make([]string, 0, len(ir.Components))
	for name := range ir.Components {
		names = append(names, name)
	}
	sort.Strings(names)

	var components []cdxComponent
	for _, name := range names {
		version := ir.Components[name]
		c := cdxComponent{
			Type:       "library",
			BOMRef:     name + "@" + version,
			Name:       name,
			Version:    version,
			PackageURL: buildPURL(name, version),
			Properties: []cdxProperty{
				{Name: "foundry:type", Value: "trusted-component"},
				{Name: "foundry:role", Value: componentRole(name)},
			},
		}
		components = append(components, c)
	}

	// Application component (the compiled spec itself).
	appComponent := cdxComponent{
		Type:        "application",
		BOMRef:      ir.Metadata.Name + "@" + ir.Metadata.Version,
		Name:        ir.Metadata.Name,
		Version:     ir.Metadata.Version,
		Description: ir.Metadata.Description,
		PackageURL:  "pkg:generic/" + ir.Metadata.Name + "@" + ir.Metadata.Version,
	}

	return cdxBOM{
		BOMFormat:   "CycloneDX",
		SpecVersion: cycloneDXSchemaVersion,
		Version:     1,
		Metadata: cdxMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Tools: []cdxTool{
				{Vendor: "Trusted Software Foundry", Name: "forge", Version: "v1.0.0"},
			},
			Component: appComponent,
		},
		Components: components,
	}
}

// buildPURL constructs a Package URL (PURL) for a foundry trusted component.
// Format: pkg:golang/github.com/jsell-rh/trusted-software-foundry/<path>@<version>
func buildPURL(componentName, version string) string {
	// Map component names to their Go package paths.
	// foundry-http → foundry/components/http, foundry-auth-jwt → foundry/components/auth/jwt, etc.
	pkgPath := componentNameToPath(componentName)
	return fmt.Sprintf("pkg:golang/github.com%%2Fjsell-rh%%2Ftrusted-software-foundry/%s@%s",
		pkgPath, version)
}

// componentNameToPath maps a foundry component name to its Go package path within the module.
func componentNameToPath(name string) string {
	// Special cases for nested packages.
	switch name {
	case "foundry-auth-jwt":
		return "foundry/components/auth/jwt"
	case "foundry-auth-spicedb":
		return "foundry/components/spicedb"
	case "foundry-graph-age":
		return "foundry/components/graph/age"
	case "foundry-redis-streams":
		return "foundry/components/redis-streams"
	case "foundry-service-router":
		return "foundry/components/service-router"
	}
	// Generic: foundry-<name> → foundry/components/<name>
	suffix := strings.TrimPrefix(name, "foundry-")
	return "foundry/components/" + suffix
}

// componentRole returns a human-readable role description for a foundry component.
func componentRole(name string) string {
	roles := map[string]string{
		"foundry-postgres":       "relational-database",
		"foundry-http":           "http-server",
		"foundry-grpc":           "grpc-server",
		"foundry-auth-jwt":       "authentication",
		"foundry-auth-spicedb":   "authorization",
		"foundry-tenancy":        "multi-tenancy",
		"foundry-health":         "health-check",
		"foundry-metrics":        "observability",
		"foundry-events":         "event-bus",
		"foundry-kafka":          "event-streaming",
		"foundry-nats":           "messaging",
		"foundry-redis":          "distributed-state",
		"foundry-redis-streams":  "stream-processing",
		"foundry-temporal":       "workflow-engine",
		"foundry-graph-age":      "graph-database",
		"foundry-service-router": "service-mesh",
	}
	if r, ok := roles[name]; ok {
		return r
	}
	return "component"
}
