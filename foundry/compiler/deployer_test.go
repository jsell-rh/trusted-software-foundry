package compiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

// TestDeploy_FleetManager verifies that forge deploy generates correct Kubernetes
// manifests for the fleet-manager example (multi-service, full complexity).
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

	// Assert: kustomization.yaml generated
	kust, err := os.ReadFile(filepath.Join(deployDir, "kustomization.yaml"))
	if err != nil {
		t.Fatalf("kustomization.yaml not generated: %v", err)
	}
	kustStr := string(kust)
	for _, want := range []string{"kustomize.config.k8s.io", "api-server", "provisioner", "graph-indexer", "secrets.yaml"} {
		if !strings.Contains(kustStr, want) {
			t.Errorf("kustomization.yaml missing %q", want)
		}
	}

	// Assert: manifests for all 3 services generated
	for _, svc := range []string{"api-server", "provisioner", "graph-indexer"} {
		for _, filename := range []string{"deployment.yaml", "service.yaml"} {
			p := filepath.Join(deployDir, svc, filename)
			data, err := os.ReadFile(p)
			if err != nil {
				t.Errorf("%s/%s not generated: %v", svc, filename, err)
				continue
			}
			s := string(data)
			if !strings.Contains(s, "fleet-manager-"+svc) {
				t.Errorf("%s/%s missing app name 'fleet-manager-%s'", svc, filename, svc)
			}
		}
	}

	// Assert: api-server deployment has health probes (foundry-health component)
	apiDeploy, _ := os.ReadFile(filepath.Join(deployDir, "api-server", "deployment.yaml"))
	apiStr := string(apiDeploy)
	for _, want := range []string{"livenessProbe", "readinessProbe", "/healthz", "8083",
		"DATABASE_URL", "JWK_CERT_URL", "fleet-manager-secrets",
		"runAsNonRoot", "resources"} {
		if !strings.Contains(apiStr, want) {
			t.Errorf("api-server/deployment.yaml missing %q", want)
		}
	}

	// Assert: secrets.yaml has all required env var keys
	secrets, err := os.ReadFile(filepath.Join(deployDir, "secrets.yaml"))
	if err != nil {
		t.Fatalf("secrets.yaml not generated: %v", err)
	}
	secretsStr := string(secrets)
	for _, want := range []string{"database-url", "jwk-cert-url", "kafka-broker-url", "redis-url", "temporal-host"} {
		if !strings.Contains(secretsStr, want) {
			t.Errorf("secrets.yaml missing key %q", want)
		}
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
