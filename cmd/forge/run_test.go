package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// buildDevCompose — unit tests (no docker needed)
// --------------------------------------------------------------------------

func minimalIR(name string) *spec.IRSpec {
	return &spec.IRSpec{
		Metadata:   spec.IRMetadata{Name: name, Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v1.0.0"},
	}
}

func TestBuildDevCompose_NoInfrastructure(t *testing.T) {
	ir := minimalIR("my-app")
	got := buildDevCompose(ir)
	if !strings.Contains(got, "version:") {
		t.Error("expected docker-compose version declaration")
	}
	if strings.Contains(got, "postgres:") {
		t.Error("should not include postgres without database block")
	}
}

func TestBuildDevCompose_WithDatabase(t *testing.T) {
	ir := minimalIR("my-app")
	ir.Database = &spec.IRDatabase{Type: "postgres"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "postgres:") {
		t.Error("expected postgres service")
	}
	if !strings.Contains(got, "POSTGRES_DB: my_app") {
		t.Errorf("expected database name my_app (hyphen→underscore), got:\n%s", got)
	}
	if !strings.Contains(got, "5432:5432") {
		t.Error("expected postgres port 5432")
	}
	if !strings.Contains(got, "DATABASE_URL=") {
		t.Error("expected DATABASE_URL hint comment")
	}
}

func TestBuildDevCompose_WithResources_NoDatabaseBlock(t *testing.T) {
	// resources without explicit database block still needs postgres
	ir := minimalIR("my-app")
	ir.Resources = []spec.IRResource{{Name: "Widget", Plural: "widgets"}}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "postgres:") {
		t.Error("expected postgres service when resources are declared")
	}
}

func TestBuildDevCompose_WithKafka(t *testing.T) {
	ir := minimalIR("my-app")
	ir.Events = &spec.IREventsConfig{Backend: "kafka"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "kafka:") {
		t.Error("expected kafka service")
	}
	if !strings.Contains(got, "zookeeper:") {
		t.Error("expected zookeeper service")
	}
	if !strings.Contains(got, "9092:9092") {
		t.Error("expected kafka port 9092")
	}
	if !strings.Contains(got, "KAFKA_BROKER_URL=") {
		t.Error("expected KAFKA_BROKER_URL hint comment")
	}
}

func TestBuildDevCompose_WithNATS(t *testing.T) {
	ir := minimalIR("my-app")
	ir.Events = &spec.IREventsConfig{Backend: "nats"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "nats:") {
		t.Error("expected nats service")
	}
	if !strings.Contains(got, "4222:4222") {
		t.Error("expected nats port 4222")
	}
	if !strings.Contains(got, "NATS_URL=") {
		t.Error("expected NATS_URL hint comment")
	}
}

func TestBuildDevCompose_WithRedisState(t *testing.T) {
	ir := minimalIR("my-app")
	ir.State = &spec.IRStateConfig{URL: "${REDIS_URL}"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "redis:") {
		t.Error("expected redis service")
	}
	if !strings.Contains(got, "6379:6379") {
		t.Error("expected redis port 6379")
	}
	if !strings.Contains(got, "REDIS_URL=") {
		t.Error("expected REDIS_URL hint comment")
	}
}

func TestBuildDevCompose_WithRedisStreams(t *testing.T) {
	ir := minimalIR("my-app")
	ir.Events = &spec.IREventsConfig{Backend: "redis-streams"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "redis:") {
		t.Error("expected redis service for redis-streams backend")
	}
}

func TestBuildDevCompose_WithSpiceDB(t *testing.T) {
	ir := minimalIR("my-app")
	ir.Authz = &spec.IRAuthzConfig{Backend: "spicedb"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "spicedb:") {
		t.Error("expected spicedb service")
	}
	if !strings.Contains(got, "50051:50051") {
		t.Error("expected spicedb port 50051")
	}
	if !strings.Contains(got, "SPICEDB_ENDPOINT=") {
		t.Error("expected SPICEDB_ENDPOINT hint comment")
	}
}

func TestBuildDevCompose_AppNameHyphenToUnderscore(t *testing.T) {
	ir := minimalIR("my-service")
	ir.Database = &spec.IRDatabase{Type: "postgres"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "my_service") {
		t.Errorf("expected hyphen→underscore in DB name, got:\n%s", got)
	}
}

func TestBuildDevCompose_FullStack(t *testing.T) {
	ir := minimalIR("full-stack")
	ir.Database = &spec.IRDatabase{Type: "postgres"}
	ir.Events = &spec.IREventsConfig{Backend: "kafka"}
	ir.State = &spec.IRStateConfig{URL: "${REDIS_URL}"}
	ir.Authz = &spec.IRAuthzConfig{Backend: "spicedb"}
	got := buildDevCompose(ir)
	for _, want := range []string{"postgres:", "kafka:", "zookeeper:", "redis:", "spicedb:"} {
		if !strings.Contains(got, want) {
			t.Errorf("full stack: missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildDevCompose_HealthCheckIncluded(t *testing.T) {
	ir := minimalIR("test-app")
	ir.Database = &spec.IRDatabase{Type: "postgres"}
	got := buildDevCompose(ir)
	if !strings.Contains(got, "healthcheck:") {
		t.Error("expected healthcheck block for postgres")
	}
	if !strings.Contains(got, "pg_isready") {
		t.Error("expected pg_isready in healthcheck")
	}
}

// --------------------------------------------------------------------------
// forge run CLI — flag parsing and --build-only behaviour
// --------------------------------------------------------------------------

func TestRunCmd_BuildOnly(t *testing.T) {
	// Write a minimal valid spec to a temp file.
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "app.foundry.yaml")
	if err := os.WriteFile(specPath, []byte(`apiVersion: foundry/v1
kind: Application
metadata:
  name: run-test
  version: 1.0.0
components:
  foundry-http: v1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmp, "out")
	cmd := rootCmd()
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"run", specPath, "--output", outDir, "--build-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("forge run --build-only: %v\noutput: %s", err, buf.String())
	}

	// Verify docker-compose.dev.yaml was generated.
	composePath := filepath.Join(outDir, "docker-compose.dev.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Errorf("docker-compose.dev.yaml not generated at %s", composePath)
	}

	// Verify the output includes next-step instructions.
	output := buf.String()
	if !strings.Contains(output, "docker compose") {
		t.Errorf("expected docker compose instructions in output, got: %s", output)
	}
	if !strings.Contains(output, "go run") {
		t.Errorf("expected 'go run' instructions in output, got: %s", output)
	}
}

func TestRunCmd_MissingSpec(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"run", "/nonexistent/app.foundry.yaml", "--build-only"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing spec file, got nil")
	}
}

func TestRunCmd_NoArgs(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"run"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing spec argument, got nil")
	}
}
