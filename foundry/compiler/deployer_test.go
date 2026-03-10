package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

// TestDeploy_FleetManager verifies that forge deploy generates correct Kubernetes
// manifests for the fleet-manager example (single-service, rh-trex parity).
func TestDeploy_FleetManager(t *testing.T) {
	specPath := filepath.Join("..", "examples", "fleet-manager", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("fleet-manager spec not found at %s", specPath)
	}

	ir, err := compiler.Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	outDir := t.TempDir()
	if err := compiler.Deploy(ir, outDir); err != nil {
		t.Fatalf("Deploy() failed: %v", err)
	}

	deployDir := filepath.Join(outDir, "deploy")

	// Assert: flat deploy structure (single service)
	for _, f := range []string{"deployment.yaml", "service.yaml", "kustomization.yaml"} {
		if _, err := os.Stat(filepath.Join(deployDir, f)); err != nil {
			t.Errorf("deploy/%s not generated: %v", f, err)
		}
	}

	// Assert: deployment contains DATABASE_URL secret ref (postgres component)
	deploy, err := os.ReadFile(filepath.Join(deployDir, "deployment.yaml"))
	if err != nil {
		t.Fatalf("deployment.yaml not generated: %v", err)
	}
	deployStr := string(deploy)
	for _, want := range []string{"fleet-manager", "DATABASE_URL", "runAsNonRoot", "resources"} {
		if !strings.Contains(deployStr, want) {
			t.Errorf("deployment.yaml missing %q", want)
		}
	}

	// Assert: kustomization.yaml references the app
	kust, err := os.ReadFile(filepath.Join(deployDir, "kustomization.yaml"))
	if err != nil {
		t.Fatalf("kustomization.yaml not generated: %v", err)
	}
	if !strings.Contains(string(kust), "kustomize.config.k8s.io") {
		t.Error("kustomization.yaml missing kustomize header")
	}
}

// TestDeploy_SingleService verifies that a spec without a services: block
// generates a flat deploy/ structure (not per-service subdirectories).
func TestDeploy_SingleService(t *testing.T) {
	specPath := filepath.Join("..", "examples", "dinosaur-registry", "app.foundry.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("dinosaur-registry spec not found at %s", specPath)
	}

	ir, err := compiler.Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	outDir := t.TempDir()
	if err := compiler.Deploy(ir, outDir); err != nil {
		t.Fatalf("Deploy() failed: %v", err)
	}

	deployDir := filepath.Join(outDir, "deploy")

	// Assert: flat structure (deployment.yaml and service.yaml at deploy root)
	for _, f := range []string{"deployment.yaml", "service.yaml", "kustomization.yaml"} {
		if _, err := os.Stat(filepath.Join(deployDir, f)); err != nil {
			t.Errorf("deploy/%s not generated: %v", f, err)
		}
	}

	// Assert: deployment contains DATABASE_URL secret ref (postgres component)
	deploy, _ := os.ReadFile(filepath.Join(deployDir, "deployment.yaml"))
	if !strings.Contains(string(deploy), "DATABASE_URL") {
		t.Error("deployment.yaml missing DATABASE_URL for postgres-backed app")
	}
}
