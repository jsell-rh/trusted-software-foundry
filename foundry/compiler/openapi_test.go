package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --- helpers ---

func dinoIR() *spec.IRSpec {
	return &spec.IRSpec{
		APIVersion: "foundry/v1",
		Kind:       "Application",
		Metadata:   spec.IRMetadata{Name: "dinosaur-registry"},
		Resources: []spec.IRResource{
			{
				Name:   "Dinosaur",
				Plural: "dinosaurs",
				Fields: []spec.IRField{
					{Name: "species", Type: "string", Required: true, MaxLength: 100},
					{Name: "description", Type: "string"},
					{Name: "weight_kg", Type: "float"},
					{Name: "created_at", Type: "timestamp", Auto: "created"},
					{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
				},
				Operations: []string{"create", "read", "update", "delete", "list"},
			},
		},
		API: &spec.IRAPI{
			REST: &spec.IRRESTConfig{BasePath: "/api/v1"},
		},
	}
}

// --- BuildOpenAPISpec unit tests ---

func TestBuildOpenAPISpec_ContainsInfo(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	if !strings.Contains(yaml, `openapi: "3.0.3"`) {
		t.Error("missing openapi version header")
	}
	if !strings.Contains(yaml, `"dinosaur-registry"`) {
		t.Error("missing app name in info block")
	}
}

func TestBuildOpenAPISpec_BasePath(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	if !strings.Contains(yaml, `url: "/api/v1"`) {
		t.Errorf("missing base path in servers block: %s", yaml)
	}
}

func TestBuildOpenAPISpec_CollectionPaths(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	if !strings.Contains(yaml, "  /dinosaurs:") {
		t.Error("missing collection path /dinosaurs")
	}
	if !strings.Contains(yaml, "operationId: \"listDinosaur\"") {
		t.Error("missing list operationId")
	}
	if !strings.Contains(yaml, "operationId: \"createDinosaur\"") {
		t.Error("missing create operationId")
	}
}

func TestBuildOpenAPISpec_ItemPaths(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	if !strings.Contains(yaml, "  /dinosaurs/{id}:") {
		t.Error("missing item path /dinosaurs/{id}")
	}
	if !strings.Contains(yaml, "operationId: \"getDinosaur\"") {
		t.Error("missing get operationId")
	}
	if !strings.Contains(yaml, "operationId: \"updateDinosaur\"") {
		t.Error("missing update operationId")
	}
	if !strings.Contains(yaml, "operationId: \"deleteDinosaur\"") {
		t.Error("missing delete operationId")
	}
}

func TestBuildOpenAPISpec_ErrorResponses(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	for _, code := range []string{"400", "401", "403", "404", "409", "500"} {
		if !strings.Contains(yaml, `"`+code+`":`) {
			t.Errorf("missing error response for HTTP %s", code)
		}
	}
	if !strings.Contains(yaml, `$ref: "#/components/schemas/ServiceError"`) {
		t.Error("missing ServiceError $ref in error responses")
	}
}

func TestBuildOpenAPISpec_ServiceErrorSchema(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	if !strings.Contains(yaml, "    ServiceError:") {
		t.Error("missing ServiceError schema")
	}
	for _, field := range []string{"code", "http_status", "reason"} {
		if !strings.Contains(yaml, field+":") {
			t.Errorf("ServiceError schema missing field %q", field)
		}
	}
}

func TestBuildOpenAPISpec_ResourceSchemas(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	if !strings.Contains(yaml, "    Dinosaur:") {
		t.Error("missing Dinosaur schema")
	}
	if !strings.Contains(yaml, "    DinosaurInput:") {
		t.Error("missing DinosaurInput schema")
	}
	if !strings.Contains(yaml, "    DinosaurList:") {
		t.Error("missing DinosaurList schema")
	}
}

func TestBuildOpenAPISpec_FieldTypes(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	// species: string with maxLength
	if !strings.Contains(yaml, "maxLength: 100") {
		t.Error("missing maxLength for species field")
	}
	// weight_kg: float → number/double
	if !strings.Contains(yaml, "format: double") {
		t.Error("missing double format for float field")
	}
	// created_at: timestamp → string/date-time, readOnly
	if !strings.Contains(yaml, "format: date-time") {
		t.Error("missing date-time format for timestamp field")
	}
}

func TestBuildOpenAPISpec_AutoFieldsReadOnly(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	// created_at has Auto:"created", must be readOnly
	// Look for readOnly: true in the output
	if !strings.Contains(yaml, "readOnly: true") {
		t.Error("auto fields should be readOnly: true")
	}
}

func TestBuildOpenAPISpec_InputSchemaExcludesAutoFields(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	// DinosaurInput should not include created_at or deleted_at
	// Find the DinosaurInput block and verify
	idx := strings.Index(yaml, "    DinosaurInput:")
	if idx < 0 {
		t.Fatal("DinosaurInput schema not found")
	}
	// Find the next top-level schema after DinosaurInput
	rest := yaml[idx:]
	nextSchema := strings.Index(rest[4:], "    Dinosaur") // skip "    DinosaurInput:" itself
	var inputSection string
	if nextSchema > 0 {
		inputSection = rest[:nextSchema+4]
	} else {
		inputSection = rest
	}
	if strings.Contains(inputSection, "created_at:") {
		t.Error("DinosaurInput should not include auto field created_at")
	}
	if strings.Contains(inputSection, "deleted_at:") {
		t.Error("DinosaurInput should not include soft-delete field deleted_at")
	}
}

func TestBuildOpenAPISpec_ListEnvelope(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())
	if !strings.Contains(yaml, `example: "DinosaurList"`) {
		t.Error("missing DinosaurList kind example")
	}
	// List schema must reference the resource
	if !strings.Contains(yaml, `$ref: "#/components/schemas/Dinosaur"`) {
		t.Error("missing Dinosaur $ref in list items")
	}
}

func TestBuildOpenAPISpec_PluralFallback(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "test-app"},
		Resources: []spec.IRResource{
			{
				Name: "Fleet",
				// no Plural — should fall back to "fleets"
				Fields:     []spec.IRField{{Name: "name", Type: "string"}},
				Operations: []string{"list"},
			},
		},
	}
	yaml := buildOpenAPISpec(ir)
	if !strings.Contains(yaml, "  /fleets:") {
		t.Error("plural fallback: expected /fleets path")
	}
}

func TestBuildOpenAPISpec_SelectiveOps(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "read-only-app"},
		Resources: []spec.IRResource{
			{
				Name:       "Report",
				Plural:     "reports",
				Fields:     []spec.IRField{{Name: "title", Type: "string"}},
				Operations: []string{"list", "read"}, // no create/update/delete
			},
		},
	}
	yaml := buildOpenAPISpec(ir)
	if !strings.Contains(yaml, "operationId: \"listReport\"") {
		t.Error("list op should be present")
	}
	if !strings.Contains(yaml, "operationId: \"getReport\"") {
		t.Error("read op should be present")
	}
	if strings.Contains(yaml, "operationId: \"createReport\"") {
		t.Error("create op should not be present")
	}
	if strings.Contains(yaml, "operationId: \"updateReport\"") {
		t.Error("update op should not be present")
	}
	if strings.Contains(yaml, "operationId: \"deleteReport\"") {
		t.Error("delete op should not be present")
	}
}

func TestBuildOpenAPISpec_MultipleResources(t *testing.T) {
	ir := &spec.IRSpec{
		Metadata: spec.IRMetadata{Name: "multi-app"},
		Resources: []spec.IRResource{
			{Name: "Alpha", Plural: "alphas", Fields: []spec.IRField{}, Operations: []string{"list"}},
			{Name: "Beta", Plural: "betas", Fields: []spec.IRField{}, Operations: []string{"list"}},
		},
	}
	yaml := buildOpenAPISpec(ir)
	if !strings.Contains(yaml, "  /alphas:") {
		t.Error("missing /alphas path")
	}
	if !strings.Contains(yaml, "  /betas:") {
		t.Error("missing /betas path")
	}
}

// --- GenerateOpenAPI file-level test ---

func TestGenerateOpenAPI_WritesFile(t *testing.T) {
	dir := t.TempDir()
	ir := dinoIR()
	if err := GenerateOpenAPI(ir, dir); err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}
	path := filepath.Join(dir, "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("openapi.yaml not written: %v", err)
	}
	if !strings.Contains(string(data), `openapi: "3.0.3"`) {
		t.Error("written file does not contain openapi version")
	}
}

func TestGenerateOpenAPI_BadDir(t *testing.T) {
	ir := dinoIR()
	err := GenerateOpenAPI(ir, "/nonexistent/dir/that/does/not/exist")
	if err == nil {
		t.Error("expected error writing to nonexistent dir")
	}
}

// --- IRTypeToOpenAPI ---

func TestIRTypeToOpenAPI(t *testing.T) {
	cases := []struct {
		irType   string
		wantType string
		wantFmt  string
	}{
		{"string", "string", ""},
		{"int", "integer", "int64"},
		{"float", "number", "double"},
		{"bool", "boolean", ""},
		{"timestamp", "string", "date-time"},
		{"uuid", "string", "uuid"},
		{"unknown", "string", ""},
	}
	for _, tc := range cases {
		t.Run(tc.irType, func(t *testing.T) {
			typ, fmt := irTypeToOpenAPI(tc.irType)
			if typ != tc.wantType {
				t.Errorf("type = %q, want %q", typ, tc.wantType)
			}
			if fmt != tc.wantFmt {
				t.Errorf("format = %q, want %q", fmt, tc.wantFmt)
			}
		})
	}
}

// --- Golden file test ---

func TestBuildOpenAPISpec_Golden(t *testing.T) {
	yaml := buildOpenAPISpec(dinoIR())

	goldenPath := filepath.Join("testdata", "dinosaur_openapi.yaml.golden")

	// Update golden: run with UPDATE_GOLDEN=1
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(yaml), 0644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		// Golden doesn't exist yet — write it
		if os.IsNotExist(err) {
			if err2 := os.MkdirAll(filepath.Dir(goldenPath), 0755); err2 != nil {
				t.Fatalf("mkdir testdata: %v", err2)
			}
			if err2 := os.WriteFile(goldenPath, []byte(yaml), 0644); err2 != nil {
				t.Fatalf("write initial golden: %v", err2)
			}
			t.Logf("golden file created: %s", goldenPath)
			return
		}
		t.Fatalf("read golden: %v", err)
	}

	if yaml != string(want) {
		t.Errorf("generated OpenAPI spec differs from golden.\nGot:\n%s\nWant:\n%s", yaml, want)
	}
}
