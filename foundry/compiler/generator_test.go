package compiler

import (
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exampleSpecPath is the golden input for generator tests.
// Go tests run with working directory set to the package directory.
const exampleSpecPath = "../examples/dinosaur-registry/app.foundry.yaml"

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

// --------------------------------------------------------------------------
// irTypeToSQL unit tests — exhaustive coverage of all type branches
// --------------------------------------------------------------------------

func TestIRTypeToSQL_AllTypes(t *testing.T) {
	tests := []struct {
		irType  string
		maxLen  int
		wantSQL string
	}{
		// string variants
		{"string", 0, "TEXT"},
		{"string", 255, "VARCHAR(255)"},
		{"string", 64, "VARCHAR(64)"},
		// integer variants
		{"int", 0, "INTEGER"},
		{"integer", 0, "INTEGER"},
		// 64-bit integer
		{"int64", 0, "BIGINT"},
		// boolean variants
		{"bool", 0, "BOOLEAN"},
		{"boolean", 0, "BOOLEAN"},
		// float variants
		{"float", 0, "DOUBLE PRECISION"},
		{"float64", 0, "DOUBLE PRECISION"},
		// timestamp
		{"timestamp", 0, "TIMESTAMP"},
		// uuid
		{"uuid", 0, "UUID"},
		// default catch-all (unknown type maps to TEXT)
		{"jsonb", 0, "TEXT"},
		{"decimal", 0, "TEXT"},
		{"", 0, "TEXT"},
	}

	for _, tc := range tests {
		t.Run(tc.irType+"_maxLen"+string(rune('0'+tc.maxLen%10)), func(t *testing.T) {
			got := irTypeToSQL(tc.irType, tc.maxLen)
			if got != tc.wantSQL {
				t.Errorf("irTypeToSQL(%q, %d) = %q, want %q", tc.irType, tc.maxLen, got, tc.wantSQL)
			}
		})
	}
}

// --------------------------------------------------------------------------
// sortComponents — alphabetical tiebreaker for unknown components
// --------------------------------------------------------------------------

// TestSortComponents_TwoUnknownTiebreakByName exercises the alphabetical
// tiebreaker inside sortComponents (line: `return sorted[i].Name < sorted[j].Name`).
// This branch fires when two components both lack a priority entry and must be
// ordered by name to guarantee deterministic output across Go test runs.
func TestSortComponents_TwoUnknownTiebreakByName(t *testing.T) {
	input := []ResolvedComponent{
		{Name: "zeta-service"},
		{Name: "alpha-service"},
		{Name: "foundry-postgres"}, // known, must come first
	}
	sorted := sortComponents(input)

	if sorted[0].Name != "foundry-postgres" {
		t.Errorf("foundry-postgres must be first, got %q", sorted[0].Name)
	}
	alphaIdx, zetaIdx := -1, -1
	for i, c := range sorted {
		switch c.Name {
		case "alpha-service":
			alphaIdx = i
		case "zeta-service":
			zetaIdx = i
		}
	}
	if alphaIdx == -1 || zetaIdx == -1 {
		t.Fatal("one or both unknown components missing from sorted output")
	}
	if alphaIdx > zetaIdx {
		t.Errorf("alpha-service (pos %d) should appear before zeta-service (pos %d) via alphabetical tiebreak", alphaIdx, zetaIdx)
	}
}

// --------------------------------------------------------------------------
// Generate — write error path when output dir is read-only
// --------------------------------------------------------------------------

// TestGenerate_WriteError verifies that Generate returns a wrapped error
// mentioning "generating main.go" when the output directory exists but is
// read-only (so WriteFile for main.go fails). This covers the first error
// path in the Generate sub-function call chain.
func TestGenerate_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories; skip")
	}

	ir, err := Parse(exampleSpecPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	// Create the output dir then make it read-only.
	// os.MkdirAll will succeed (dir exists), but subsequent WriteFile will fail.
	outDir := t.TempDir()
	if err := os.Chmod(outDir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(outDir, 0755) // restore so TempDir cleanup works

	genErr := NewGenerator(outDir, "").Generate(ir, components)
	if genErr == nil {
		t.Fatal("expected error for read-only output dir, got nil")
	}
	if !strings.Contains(genErr.Error(), "generating main.go") {
		t.Errorf("expected 'generating main.go' in error, got: %v", genErr)
	}
}

// --------------------------------------------------------------------------
// writeMigrations — empty plural field and MkdirAll error
// --------------------------------------------------------------------------

// TestWriteMigrations_EmptyPlural verifies that when a resource has an empty
// Plural field, writeMigrations derives the table name from Name+"s". This is a
// defensive code path since the schema requires plural, but the IR allows it.
func TestWriteMigrations_EmptyPlural(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "test-app", Version: "1.0.0"},
		Resources: []spec.IRResource{
			{
				Name:   "Widget",
				Plural: "", // empty — triggers the Name+"s" fallback
				Fields: []spec.IRField{
					{Name: "id", Type: "uuid", Required: true},
				},
				Operations: []string{"create"},
			},
		},
	}
	outDir := t.TempDir()
	g := NewGenerator(outDir, "")
	if err := g.writeMigrations(ir); err != nil {
		t.Fatalf("writeMigrations: %v", err)
	}

	// Table name should be "widgets" (Name + "s", lowercased)
	entries, err := os.ReadDir(filepath.Join(outDir, "migrations"))
	if err != nil {
		t.Fatalf("migrations/ not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 migration file, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Name(), "widgets") {
		t.Errorf("expected migration filename to contain 'widgets', got %q", entries[0].Name())
	}
}

// TestWriteMigrations_SkipsAutoAndSoftDeleteFields verifies that auto-managed
// fields (created/updated) and soft-delete fields are excluded from the
// generated CREATE TABLE statement (they are managed by the base schema).
func TestWriteMigrations_SkipsAutoAndSoftDeleteFields(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "test-app", Version: "1.0.0"},
		Resources: []spec.IRResource{
			{
				Name:   "Order",
				Plural: "orders",
				Fields: []spec.IRField{
					{Name: "id", Type: "uuid", Required: true},
					{Name: "created_at", Type: "timestamp", Auto: "created"},
					{Name: "updated_at", Type: "timestamp", Auto: "updated"},
					{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
					{Name: "total", Type: "int", Required: true},
				},
				Operations: []string{"create"},
			},
		},
	}
	outDir := t.TempDir()
	g := NewGenerator(outDir, "")
	if err := g.writeMigrations(ir); err != nil {
		t.Fatalf("writeMigrations: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(outDir, "migrations"))
	if err != nil {
		t.Fatalf("migrations/ not created: %v", err)
	}
	sql, err := os.ReadFile(filepath.Join(outDir, "migrations", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	sqlStr := string(sql)

	// The non-auto field must appear in the Columns range
	if !strings.Contains(sqlStr, "total") {
		t.Error("migration must include the 'total' field")
	}
	// Auto-managed fields appear exactly once in the template header — not doubled via Columns.
	for _, f := range []string{"created_at", "updated_at"} {
		if n := strings.Count(sqlStr, f); n != 1 {
			t.Errorf("field %q appears %d times in migration SQL, want exactly 1 (template header only)", f, n)
		}
	}
}

// ---- goStr template function tests ----

// TestGoStr_SafelyEscapesSpecialChars verifies that string values with special
// characters (backslashes, quotes, newlines) produce valid Go source when
// embedded in the generated main.go via the goStr template function.
func TestGoStr_SafelyEscapesSpecialChars(t *testing.T) {
	ir := &spec.IRSpec{
		APIVersion: "foundry/v1",
		Kind:       "Application",
		Metadata:   spec.IRMetadata{Name: "test-app", Version: "1.0.0"},
		Components: map[string]string{
			"foundry-postgres": "v1.0.0",
			"foundry-http":     "v1.0.0",
		},
		Resources: []spec.IRResource{
			{
				Name:   "Widget",
				Plural: "widgets",
				Fields: []spec.IRField{{Name: "name", Type: "string"}},
				Operations: []string{"create", "read"},
			},
		},
		Database: &spec.IRDatabase{Type: "postgres", Migrations: "migrations"},
		API: &spec.IRAPI{
			REST: &spec.IRRESTConfig{
				// BasePath contains a backslash and a tab — would produce invalid
				// Go syntax if embedded without %q escaping.
				BasePath: `/api\v1`,
			},
		},
	}

	resolver := NewResolver(NewStubRegistry(), "")
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	outDir := t.TempDir()
	g := NewGenerator(outDir, "")
	// Generate must succeed — format.Source validates the output Go syntax.
	if err := g.writeMainGo(ir, components); err != nil {
		t.Fatalf("writeMainGo failed with special-char BasePath: %v", err)
	}

	mainGo, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	// The backslash should appear properly escaped in the generated Go source.
	if !strings.Contains(string(mainGo), `\\`) {
		t.Errorf("generated main.go should contain escaped backslash (\\\\); got:\n%s", string(mainGo))
	}
}

// ---- expandEnvVar tests ----

func TestExpandEnvVar_EnvVarRef(t *testing.T) {
	got := expandEnvVar("${MY_SECRET_KEY}")
	want := `env("MY_SECRET_KEY", "")`
	if got != want {
		t.Errorf("expandEnvVar(%q) = %q, want %q", "${MY_SECRET_KEY}", got, want)
	}
}

func TestExpandEnvVar_LiteralURL(t *testing.T) {
	got := expandEnvVar("http://localhost:9092")
	want := `"http://localhost:9092"`
	if got != want {
		t.Errorf("expandEnvVar(%q) = %q, want %q", "http://localhost:9092", got, want)
	}
}

func TestExpandEnvVar_EmptyString(t *testing.T) {
	got := expandEnvVar("")
	want := `""`
	if got != want {
		t.Errorf("expandEnvVar(%q) = %q, want %q", "", got, want)
	}
}

func TestGenerateMainGo_DatabaseDSNFromEnv(t *testing.T) {
	outDir, _ := generateFromExample(t)
	mainGo, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(mainGo)
	if !strings.Contains(s, `env("DATABASE_DSN"`) {
		t.Errorf("generated main.go should read DATABASE_DSN from env; got:\n%s", s)
	}
}

func TestGenerateMainGo_KafkaBrokerFromEnv(t *testing.T) {
	ir, err := Parse("../examples/kartograph/app.foundry.yaml")
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
	mainGo, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(mainGo)
	// The spec uses ${KARTOGRAPH_EVENTS_BROKER_URL} which should be expanded to env(...)
	if !strings.Contains(s, `env("KARTOGRAPH_EVENTS_BROKER_URL"`) {
		t.Errorf("generated main.go should expand ${KARTOGRAPH_EVENTS_BROKER_URL} to env(...); got snippet:\n%s", s)
	}
}
