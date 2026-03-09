package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exampleSpecPath is the golden input for generator tests.
// Go tests run with working directory set to the package directory.
const exampleSpecPath = "../examples/dinosaur-registry/app.tsc.yaml"

// generateFromExample parses the golden spec, resolves components via StubRegistry,
// runs Generate into a temp dir, and returns (tempDir, resolvedComponents).
func generateFromExample(t *testing.T) (outDir string, components []ResolvedComponent) {
	t.Helper()
	ir, err := Parse(exampleSpecPath)
	if err != nil {
		t.Fatalf("Parse(%q): %v", exampleSpecPath, err)
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err = resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	outDir = t.TempDir()
	if err := NewGenerator(outDir, "").Generate(ir, components); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return outDir, components
}

// ---- sortComponents tests ----

func TestSortComponents_PostgresFirst(t *testing.T) {
	input := []ResolvedComponent{
		{Name: "foundry-events"},
		{Name: "foundry-health"},
		{Name: "foundry-auth-jwt"},
		{Name: "foundry-grpc"},
		{Name: "foundry-http"},
		{Name: "foundry-metrics"},
		{Name: "foundry-postgres"},
	}
	sorted := sortComponents(input)
	if len(sorted) != len(input) {
		t.Fatalf("sortComponents returned %d components, want %d", len(sorted), len(input))
	}
	if sorted[0].Name != "foundry-postgres" {
		t.Errorf("first component must be foundry-postgres, got %q", sorted[0].Name)
	}
}

func TestSortComponents_StableOrder(t *testing.T) {
	input := []ResolvedComponent{
		{Name: "foundry-metrics"},
		{Name: "foundry-http"},
		{Name: "foundry-postgres"},
	}
	sorted := sortComponents(input)
	if sorted[0].Name != "foundry-postgres" {
		t.Errorf("want foundry-postgres first, got %q", sorted[0].Name)
	}
	if sorted[1].Name != "foundry-http" {
		t.Errorf("want foundry-http second, got %q", sorted[1].Name)
	}
	if sorted[2].Name != "foundry-metrics" {
		t.Errorf("want foundry-metrics third, got %q", sorted[2].Name)
	}
}

func TestSortComponents_UnknownComponentLast(t *testing.T) {
	input := []ResolvedComponent{
		{Name: "custom-component"},
		{Name: "foundry-postgres"},
		{Name: "foundry-http"},
	}
	sorted := sortComponents(input)
	if sorted[0].Name != "foundry-postgres" {
		t.Errorf("want foundry-postgres first, got %q", sorted[0].Name)
	}
	if sorted[len(sorted)-1].Name != "custom-component" {
		t.Errorf("want custom-component last, got %q", sorted[len(sorted)-1].Name)
	}
}

// ---- main.go generation tests ----

func TestGenerateMainGo_ImportsAllComponents(t *testing.T) {
	outDir, components := generateFromExample(t)

	content, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	for _, c := range components {
		if !strings.Contains(string(content), c.Module) {
			t.Errorf("main.go missing import for component %q (module %q)", c.Name, c.Module)
		}
	}
}

func TestGenerateMainGo_NewApplicationCall(t *testing.T) {
	outDir, _ := generateFromExample(t)

	content, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	src := string(content)

	if !strings.Contains(src, "spec.NewApplication(resources)") {
		t.Error("main.go missing spec.NewApplication(resources) call")
	}
	// dinosaur-registry has 1 resource; verify its definition appears
	if !strings.Contains(src, `"Dinosaur"`) {
		t.Error("main.go missing Dinosaur resource name in ResourceDefinition slice")
	}
}

func TestGenerateMainGo_AddComponentCallsForEach(t *testing.T) {
	outDir, components := generateFromExample(t)

	content, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	src := string(content)

	for _, c := range components {
		alias := strings.ReplaceAll(c.Name, "-", "")
		call := alias + ".New()"
		if !strings.Contains(src, call) {
			t.Errorf("main.go missing AddComponent call for %q (expected %s)", c.Name, call)
		}
	}
	if got := strings.Count(src, "app.AddComponent("); got != len(components) {
		t.Errorf("expected %d app.AddComponent calls, got %d", len(components), got)
	}
}

func TestGenerateMainGo_PostgresBeforeOtherComponents(t *testing.T) {
	outDir, _ := generateFromExample(t)

	content, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	src := string(content)

	postgresIdx := strings.Index(src, "foundrypostgres.New()")
	if postgresIdx == -1 {
		t.Fatal("main.go missing foundrypostgres.New() call")
	}

	// All DB-dependent components must appear after postgres in main.go
	for _, call := range []string{
		"foundryauthjwt.New()",
		"foundryhttp.New()",
		"foundrygrpc.New()",
		"foundryhealth.New()",
		"foundrymetrics.New()",
		"foundryevents.New()",
	} {
		idx := strings.Index(src, call)
		if idx != -1 && idx < postgresIdx {
			t.Errorf("%s appears before foundrypostgres.New() in main.go", call)
		}
	}
}

func TestSortComponents_AllComponentsHavePriority(t *testing.T) {
	// Every component in the trusted catalog must be listed in componentPriority.
	// If a component is missing, it silently gets the same priority as all other
	// unknowns and the ordering becomes non-deterministic between Go test runs.
	// This test catches any future components added to the stub registry that
	// were not also given an explicit priority.
	allKnown := []string{
		"foundry-postgres",
		"foundry-auth-jwt",
		"foundry-auth-spicedb",
		"foundry-tenancy",
		"foundry-http",
		"foundry-grpc",
		"foundry-health",
		"foundry-metrics",
		"foundry-events",
		"foundry-kafka",
		"foundry-nats",
		"foundry-redis",
		"foundry-redis-streams",
		"foundry-temporal",
		"foundry-graph-age",
		"foundry-service-router",
	}
	for _, name := range allKnown {
		if _, ok := componentPriority[name]; !ok {
			t.Errorf("component %q is not listed in componentPriority — add it with an appropriate priority", name)
		}
	}
}

func TestSortComponents_AuthBeforeHTTP(t *testing.T) {
	// Auth providers and tenancy must be registered before HTTP so that
	// middleware is installed before the HTTP server wires routes.
	input := []ResolvedComponent{
		{Name: "foundry-http"},
		{Name: "foundry-auth-spicedb"},
		{Name: "foundry-auth-jwt"},
		{Name: "foundry-tenancy"},
		{Name: "foundry-postgres"},
	}
	sorted := sortComponents(input)

	posOf := func(name string) int {
		for i, c := range sorted {
			if c.Name == name {
				return i
			}
		}
		return -1
	}

	httpPos := posOf("foundry-http")
	for _, name := range []string{"foundry-postgres", "foundry-auth-jwt", "foundry-auth-spicedb", "foundry-tenancy"} {
		if pos := posOf(name); pos > httpPos {
			t.Errorf("%s (pos %d) must appear before foundry-http (pos %d)", name, pos, httpPos)
		}
	}
}

func TestSortComponents_ServiceRouterLast(t *testing.T) {
	// foundry-service-router must be last — it routes to already-registered services.
	input := []ResolvedComponent{
		{Name: "foundry-service-router"},
		{Name: "foundry-http"},
		{Name: "foundry-postgres"},
		{Name: "foundry-nats"},
		{Name: "foundry-kafka"},
	}
	sorted := sortComponents(input)
	last := sorted[len(sorted)-1].Name
	if last != "foundry-service-router" {
		t.Errorf("foundry-service-router should be last, got %q", last)
	}
}

// ---- migrations generation tests ----

func TestGenerateMigrations_OneFilePerResource(t *testing.T) {
	ir, err := Parse(exampleSpecPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	outDir := t.TempDir()
	if err := NewGenerator(outDir, "").Generate(ir, components); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(outDir, "migrations"))
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entries) != len(ir.Resources) {
		t.Errorf("expected %d migration file(s), got %d", len(ir.Resources), len(entries))
	}
}

func TestGenerateMigrations_SQLColumnTypes(t *testing.T) {
	outDir, _ := generateFromExample(t)

	entries, err := os.ReadDir(filepath.Join(outDir, "migrations"))
	if err != nil {
		t.Fatalf("reading migrations dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no migration files generated")
	}

	// Read the dinosaurs migration (first and only file for dinosaur-registry)
	sqlBytes, err := os.ReadFile(filepath.Join(outDir, "migrations", entries[0].Name()))
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}
	sql := string(sqlBytes)

	// species: string + max_length=255 → VARCHAR(255)
	if !strings.Contains(sql, "VARCHAR(255)") {
		t.Errorf("expected VARCHAR(255) for species field (string, max_length=255); got:\n%s", sql)
	}
	// description: string without max_length → TEXT
	if !strings.Contains(sql, "TEXT") {
		t.Errorf("expected TEXT for description field (string, no max_length); got:\n%s", sql)
	}
	// Standard timestamp columns are always present
	if !strings.Contains(sql, "TIMESTAMP") {
		t.Errorf("expected TIMESTAMP column type in migration; got:\n%s", sql)
	}
	// BOOLEAN should appear for any bool fields; not in dinosaur spec but irTypeToSQL coverage
	// Instead verify the table header is correct
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS") {
		t.Errorf("migration missing CREATE TABLE IF NOT EXISTS statement; got:\n%s", sql)
	}
}
