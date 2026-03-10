package compiler

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// Generator writes the wiring code for a compiled Foundry application.
// It generates ONLY:
//   - main.go       (Application bootstrap using spec.Application API)
//   - go.mod        (dependency-locked module with pinned component versions)
//   - migrations/   (one .sql file per resource, derived from the IR)
//
// The compiler never generates business logic, handler implementations,
// service code, or DAO code — those live in the trusted components.
type Generator struct {
	outputDir   string
	foundryPath string // local filesystem path to trusted-software-foundry checkout (for go.mod replace)
	specDir     string // directory containing the spec file; used to resolve relative hook paths
}

// NewGenerator creates a Generator that writes into outputDir.
// foundryPath is the absolute path to the local trusted-software-foundry checkout used in the
// go.mod replace directive, enabling the generated project to `go build` immediately.
// Pass an empty string to omit the replace directive (for published modules).
func NewGenerator(outputDir, foundryPath string) *Generator {
	return &Generator{outputDir: outputDir, foundryPath: foundryPath}
}

// newGeneratorWithSpecDir is like NewGenerator but also records the directory
// that contains the spec file so that relative hook paths can be resolved correctly.
func newGeneratorWithSpecDir(outputDir, foundryPath, specDir string) *Generator {
	return &Generator{outputDir: outputDir, foundryPath: foundryPath, specDir: specDir}
}

// componentPriority defines the registration order for trusted components.
// Lower index = registered earlier.
//
// Ordering rationale:
//  1. foundry-postgres first — sets the DB; auth/tenancy/graph components depend on it
//  2. Auth providers before HTTP — middleware is installed during HTTP setup
//  3. foundry-tenancy before HTTP — tenant row-isolation middleware must wrap all handlers
//  4. foundry-http / foundry-grpc — HTTP/gRPC servers wire in registered middleware
//  5. Observability (health, metrics) — standalone, no ordering deps
//  6. Messaging and streaming backends — standalone, no ordering deps
//  7. foundry-graph-age — uses Postgres (via AGE extension), registers after postgres
//  8. foundry-service-router last — routes traffic to already-registered services
var componentPriority = map[string]int{
	"foundry-postgres":       0,
	"foundry-auth-jwt":       1,
	"foundry-auth-spicedb":   2,
	"foundry-tenancy":        3,
	"foundry-http":           4,
	"foundry-grpc":           5,
	"foundry-health":         6,
	"foundry-metrics":        7,
	"foundry-events":         8,
	"foundry-kafka":          9,
	"foundry-nats":           10,
	"foundry-redis":          11,
	"foundry-redis-streams":  12,
	"foundry-temporal":       13,
	"foundry-graph-age":      14,
	"foundry-service-router": 15,
}

// sortComponents returns a copy of components sorted into safe registration order.
// foundry-postgres is always first; remaining components follow their defined priority.
// Components not listed in componentPriority are appended last, sorted by name.
func sortComponents(components []ResolvedComponent) []ResolvedComponent {
	sorted := make([]ResolvedComponent, len(components))
	copy(sorted, components)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi, oki := componentPriority[sorted[i].Name]
		pj, okj := componentPriority[sorted[j].Name]
		if !oki {
			pi = len(componentPriority)
		}
		if !okj {
			pj = len(componentPriority)
		}
		if pi != pj {
			return pi < pj
		}
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

// Generate writes all output files for the given spec and resolved components.
func (g *Generator) Generate(ir *spec.IRSpec, components []ResolvedComponent) error {
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory %q: %w", g.outputDir, err)
	}

	ordered := sortComponents(components)

	// Compute app module path (needed by go.mod and hook_registry.go imports).
	appModule := "github.com/jsell-rh/" + strings.ReplaceAll(ir.Metadata.Name, " ", "-")

	if err := g.writeMainGo(ir, ordered); err != nil {
		return fmt.Errorf("generating main.go: %w", err)
	}
	if err := g.writeGoMod(ir, ordered); err != nil {
		return fmt.Errorf("generating go.mod: %w", err)
	}
	if err := g.writeMigrations(ir); err != nil {
		return fmt.Errorf("generating migrations: %w", err)
	}
	if err := g.writeServiceMains(ir, ordered); err != nil {
		return fmt.Errorf("generating service mains: %w", err)
	}
	if err := g.writeDockerCompose(ir); err != nil {
		return fmt.Errorf("generating docker-compose.yaml: %w", err)
	}
	if err := g.writeHookRegistry(ir, appModule); err != nil {
		return fmt.Errorf("generating hook_registry.go: %w", err)
	}
	missingHooks, err := copyHookFiles(ir, g.outputDir, g.specDir)
	if err != nil {
		return fmt.Errorf("copying hook files: %w", err)
	}
	if err := g.writeHookStubs(missingHooks); err != nil {
		return fmt.Errorf("generating hook stubs: %w", err)
	}
	if err := g.writeAuthzSchemaStub(ir); err != nil {
		return fmt.Errorf("generating authz schema stub: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------------
// main.go — uses spec.Application API (frozen by TSF-Architect)
// --------------------------------------------------------------------------

var mainGoTemplate = template.Must(template.New("main").Funcs(template.FuncMap{
	"importAlias": func(name string) string {
		// "foundry-http" → "foundryhttp", "foundry-auth-jwt" → "foundryauthjwt"
		return strings.ReplaceAll(name, "-", "")
	},
	"cleanAlias": func(name string) string {
		return strings.ReplaceAll(name, "-", "")
	},
}).Parse(`// Code generated by forge compile. DO NOT EDIT.
// Spec: {{ .IR.Metadata.Name }} v{{ .IR.Metadata.Version }}
// Components: {{ .BOM }}
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
{{ range .Components }}	{{ cleanAlias .Name }} "{{ .Module }}"
{{ end }})

func main() {
	// Build resource definitions from the IR spec
	resources := []spec.ResourceDefinition{
{{ range .IR.Resources }}		{
			Name:       "{{ .Name }}",
			Plural:     "{{ .Plural }}",
			Operations: []string{ {{ range .Operations }}"{{ . }}", {{ end }} },
			Events:     {{ .Events }},
			Fields: []spec.FieldDefinition{
{{ range .Fields }}				{Name: "{{ .Name }}", Type: "{{ .Type }}", Required: {{ .Required }}, MaxLength: {{ .MaxLength }}, Auto: "{{ .Auto }}", SoftDelete: {{ .SoftDelete }}},
{{ end }}			},
		},
{{ end }}	}

	app := spec.NewApplication(resources)

	// Add trusted components in dependency order (postgres first — others depend on DB)
{{ range .Components }}	app.AddComponent({{ cleanAlias .Name }}.New())
{{ end }}
	// Component configuration derived from the IR spec
	configs := map[string]spec.ComponentConfig{
{{ if .IR.Database }}		"foundry-postgres": {"type": "{{ .IR.Database.Type }}", "migrations": "{{ .IR.Database.Migrations }}"},
{{ end }}{{ if .IR.Auth }}		"foundry-auth-jwt": {"type": "{{ .IR.Auth.Type }}", "jwk_url": env("JWK_CERT_URL", "{{ .IR.Auth.JWKURL }}"), "required": {{ .IR.Auth.Required }}, "allow_mock": env("OCM_MOCK_ENABLED", "false")},
{{ end }}{{ if .IR.API }}{{ if .IR.API.REST }}		"foundry-http":    {"base_path": "{{ .IR.API.REST.BasePath }}", "version_header": {{ .IR.API.REST.VersionHeader }}},
{{ end }}{{ if .IR.API.GRPC }}		"foundry-grpc":    {"enabled": {{ .IR.API.GRPC.Enabled }}{{ if .IR.API.GRPC.Port }}, "port": {{ .IR.API.GRPC.Port }}{{ end }}},
{{ end }}{{ end }}{{ if .IR.Observ }}{{ if .IR.Observ.HealthCheck }}		"foundry-health":  {"port": {{ .IR.Observ.HealthCheck.Port }}},
{{ end }}{{ if .IR.Observ.Metrics }}		"foundry-metrics": {"port": {{ .IR.Observ.Metrics.Port }}, "path": "{{ .IR.Observ.Metrics.Path }}"},
{{ end }}{{ end }}	}

	if err := app.Configure(configs); err != nil {
		fmt.Fprintf(os.Stderr, "configure: %v\n", err)
		os.Exit(1)
	}

	if err := app.Register(); err != nil {
		fmt.Fprintf(os.Stderr, "register: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
`))

type mainGoData struct {
	IR         *spec.IRSpec
	Components []ResolvedComponent
	BOM        string // bill-of-materials summary
}

func (g *Generator) writeMainGo(ir *spec.IRSpec, components []ResolvedComponent) error {
	bom := make([]string, len(components))
	for i, c := range components {
		bom[i] = c.Name + "@" + c.Version
	}

	data := mainGoData{
		IR:         ir,
		Components: components,
		BOM:        strings.Join(bom, ", "),
	}

	var buf bytes.Buffer
	if err := mainGoTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("rendering main.go template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting main.go (template produced invalid Go syntax): %w", err)
	}

	return os.WriteFile(filepath.Join(g.outputDir, "main.go"), formatted, 0644)
}

// --------------------------------------------------------------------------
// go.mod
// --------------------------------------------------------------------------

// goModTemplate generates a go.mod for the compiled application.
// All trusted components live in github.com/jsell-rh/trusted-software-foundry, so there
// is a single dependency. A replace directive points to the local checkout so the
// generated project can `go build` immediately without publishing.
var goModTemplate = template.Must(template.New("gomod").Parse(`// Code generated by forge compile. DO NOT EDIT.
// Spec: {{ .AppName }} — compiled by forge
module {{ .AppModule }}

go 1.24.0

require github.com/jsell-rh/trusted-software-foundry v0.0.0
{{ if .FoundryPath }}
replace github.com/jsell-rh/trusted-software-foundry => {{ .FoundryPath }}
{{ end }}`))

type goModData struct {
	AppName     string
	AppModule   string
	FoundryPath string // absolute path to trusted-software-foundry local checkout for replace directive
}

func (g *Generator) writeGoMod(ir *spec.IRSpec, _ []ResolvedComponent) error {
	appModule := "github.com/jsell-rh/" + strings.ReplaceAll(ir.Metadata.Name, " ", "-")

	data := goModData{
		AppName:     ir.Metadata.Name,
		AppModule:   appModule,
		FoundryPath: g.foundryPath,
	}

	var buf bytes.Buffer
	if err := goModTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("rendering go.mod template: %w", err)
	}

	return os.WriteFile(filepath.Join(g.outputDir, "go.mod"), buf.Bytes(), 0644)
}

// --------------------------------------------------------------------------
// migrations/ — one SQL file per resource
// --------------------------------------------------------------------------

var migrationTemplate = template.Must(template.New("migration").Parse(`-- Code generated by forge compile. DO NOT EDIT.
-- Resource: {{ .Resource.Name }} ({{ .TableName }})
CREATE TABLE IF NOT EXISTS {{ .TableName }} (
    id         VARCHAR(32)  NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
{{ range .Columns }}    {{ .ColumnName }} {{ .SQLType }}{{ if .NotNull }} NOT NULL{{ end }},
{{ end }}    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_{{ .TableName }}_deleted_at ON {{ .TableName }} (deleted_at);
`))

type migrationData struct {
	Resource  spec.IRResource
	TableName string
	Columns   []migrationColumn
}

type migrationColumn struct {
	ColumnName string
	SQLType    string
	NotNull    bool
}

func (g *Generator) writeMigrations(ir *spec.IRSpec) error {
	migrDir := filepath.Join(g.outputDir, "migrations")
	if err := os.MkdirAll(migrDir, 0755); err != nil {
		return fmt.Errorf("creating migrations directory: %w", err)
	}

	for i, res := range ir.Resources {
		tableName := strings.ToLower(res.Plural)
		if tableName == "" {
			tableName = strings.ToLower(res.Name) + "s"
		}

		cols := make([]migrationColumn, 0, len(res.Fields))
		for _, f := range res.Fields {
			// Skip auto-managed and soft-delete fields — handled in the base table.
			if f.Auto == "created" || f.Auto == "updated" || f.SoftDelete {
				continue
			}
			cols = append(cols, migrationColumn{
				ColumnName: strings.ToLower(f.Name),
				SQLType:    irTypeToSQL(f.Type, f.MaxLength),
				NotNull:    f.Required,
			})
		}

		data := migrationData{
			Resource:  res,
			TableName: tableName,
			Columns:   cols,
		}

		var buf bytes.Buffer
		if err := migrationTemplate.Execute(&buf, data); err != nil {
			return fmt.Errorf("rendering migration for %q: %w", res.Name, err)
		}

		filename := fmt.Sprintf("%04d_%s.sql", i+1, tableName)
		if err := os.WriteFile(filepath.Join(migrDir, filename), buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("writing migration %q: %w", filename, err)
		}
	}

	return nil
}

// irTypeToSQL maps IR field types (from spec.FieldDefinition.Type) to PostgreSQL column types.
func irTypeToSQL(irType string, maxLen int) string {
	switch irType {
	case "string":
		if maxLen > 0 {
			return fmt.Sprintf("VARCHAR(%d)", maxLen)
		}
		return "TEXT"
	case "int", "integer":
		return "INTEGER"
	case "int64":
		return "BIGINT"
	case "bool", "boolean":
		return "BOOLEAN"
	case "timestamp":
		return "TIMESTAMP"
	case "float", "float64":
		return "DOUBLE PRECISION"
	case "uuid":
		return "UUID"
	default:
		return "TEXT"
	}
}
