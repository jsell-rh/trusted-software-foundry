package compiler_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

func TestSBOM_FleetManager(t *testing.T) {
	specPath := filepath.Join("..", "examples", "fleet-manager", "app.foundry.yaml")

	ir, err := compiler.Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	buf, err := compiler.SBOMToWriter(ir)
	if err != nil {
		t.Fatalf("SBOMToWriter: %v", err)
	}

	// Parse as generic JSON map for validation.
	var bom map[string]any
	if err := json.Unmarshal(buf.Bytes(), &bom); err != nil {
		t.Fatalf("SBOM is not valid JSON: %v", err)
	}

	// Assert: CycloneDX format fields.
	if bom["bomFormat"] != "CycloneDX" {
		t.Errorf("bomFormat = %v, want CycloneDX", bom["bomFormat"])
	}
	if bom["specVersion"] != "1.5" {
		t.Errorf("specVersion = %v, want 1.5", bom["specVersion"])
	}

	// Assert: all 16 fleet-manager components appear in the SBOM.
	content := buf.String()
	for name := range ir.Components {
		if !strings.Contains(content, name) {
			t.Errorf("SBOM missing component %q", name)
		}
	}

	// Assert: PURL format for a known component.
	if !strings.Contains(content, "pkg:golang") {
		t.Error("SBOM missing pkg:golang PURLs")
	}
	if !strings.Contains(content, "foundry:type") {
		t.Error("SBOM missing foundry:type property")
	}

	// Assert: application metadata component present.
	if !strings.Contains(content, "fleet-manager@1.0.0") {
		t.Error("SBOM missing application bom-ref fleet-manager@1.0.0")
	}
}

func TestSBOM_DeterministicOrder(t *testing.T) {
	// SBOM components must be sorted deterministically so diffs are stable.
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "test-app", Version: "1.0.0"},
		Components: map[string]string{
			"foundry-postgres": "v1.0.0",
			"foundry-http":     "v1.0.0",
			"foundry-kafka":    "v1.0.0",
		},
	}

	buf1, _ := compiler.SBOMToWriter(ir)
	buf2, _ := compiler.SBOMToWriter(ir)

	if buf1.String() != buf2.String() {
		t.Error("SBOM output is not deterministic across two calls with the same input")
	}

	// Components should be alphabetically sorted.
	content := buf1.String()
	httpIdx := strings.Index(content, "foundry-http")
	kafkaIdx := strings.Index(content, "foundry-kafka")
	postgresIdx := strings.Index(content, "foundry-postgres")
	if !(httpIdx < kafkaIdx && kafkaIdx < postgresIdx) {
		t.Errorf("components not in alphabetical order: http=%d kafka=%d postgres=%d", httpIdx, kafkaIdx, postgresIdx)
	}
}
