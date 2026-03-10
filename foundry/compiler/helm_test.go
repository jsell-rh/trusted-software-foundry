package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

// TestGenerateHelm_FleetManager verifies that forge deploy --helm generates a
// valid Helm chart structure for the fleet-manager example.
func TestGenerateHelm_FleetManager(t *testing.T) {
	specPath := filepath.Join("..", "examples", "fleet-manager", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("fleet-manager spec not found at %s", specPath)
	}

	ir, err := compiler.Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	outDir := t.TempDir()
	if err := compiler.GenerateHelm(ir, outDir); err != nil {
		t.Fatalf("GenerateHelm() failed: %v", err)
	}

	helmDir := filepath.Join(outDir, "deploy", "helm", "fleet-manager")

	// Assert: required top-level Helm files exist.
	for _, f := range []string{"Chart.yaml", "values.yaml", "templates/_helpers.tpl", "templates/NOTES.txt"} {
		if _, err := os.Stat(filepath.Join(helmDir, f)); err != nil {
			t.Errorf("helm/%s not generated: %v", f, err)
		}
	}

	// Assert: Chart.yaml has correct metadata.
	chart, err := os.ReadFile(filepath.Join(helmDir, "Chart.yaml"))
	if err != nil {
		t.Fatalf("Chart.yaml not readable: %v", err)
	}
	chartStr := string(chart)
	for _, want := range []string{"name: fleet-manager", "apiVersion: v2", "description:"} {
		if !strings.Contains(chartStr, want) {
			t.Errorf("Chart.yaml missing %q", want)
		}
	}

	// Assert: values.yaml has replicaCount and image sections.
	values, err := os.ReadFile(filepath.Join(helmDir, "values.yaml"))
	if err != nil {
		t.Fatalf("values.yaml not readable: %v", err)
	}
	valStr := string(values)
	for _, want := range []string{"replicaCount:", "image:", "repository:", "tag:", "service:"} {
		if !strings.Contains(valStr, want) {
			t.Errorf("values.yaml missing %q", want)
		}
	}

	// Assert: deployment and service templates exist (single-service app).
	for _, tmpl := range []string{"deployment.yaml", "service.yaml"} {
		p := filepath.Join(helmDir, "templates", tmpl)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("templates/%s not generated: %v", tmpl, err)
			continue
		}
		if !strings.Contains(string(data), "{{") {
			t.Errorf("templates/%s has no Helm template directives", tmpl)
		}
	}

	// Assert: secret template generated.
	secretTmpl, err := os.ReadFile(filepath.Join(helmDir, "templates", "secret.yaml"))
	if err != nil {
		t.Fatalf("templates/secret.yaml not generated: %v", err)
	}
	if !strings.Contains(string(secretTmpl), "kind: Secret") {
		t.Error("templates/secret.yaml missing 'kind: Secret'")
	}
}

// TestGenerateHelm_DinosaurRegistry verifies single-service Helm chart generation.
func TestGenerateHelm_DinosaurRegistry(t *testing.T) {
	specPath := filepath.Join("..", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("dinosaur-registry spec not found at %s", specPath)
	}

	ir, err := compiler.Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	outDir := t.TempDir()
	if err := compiler.GenerateHelm(ir, outDir); err != nil {
		t.Fatalf("GenerateHelm() failed: %v", err)
	}

	helmDir := filepath.Join(outDir, "deploy", "helm", ir.Metadata.Name)

	// Minimal assertions for single-service app.
	for _, f := range []string{"Chart.yaml", "values.yaml", "templates/deployment.yaml", "templates/service.yaml"} {
		if _, err := os.Stat(filepath.Join(helmDir, f)); err != nil {
			t.Errorf("%s not generated: %v", f, err)
		}
	}
}
