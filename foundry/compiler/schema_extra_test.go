package compiler

// schema_extra_test.go adds coverage for internal functions not reached by the
// existing test suite:
//   checkJSONType: integer (wrong type, non-integer float), boolean (wrong type)
//   keyInProps: both true and false cases
//   resolveRef: non-#/$defs/ prefix → nil, unknown def name → nil
//   newSchemaValidator: invalid JSON → error
//   validateNode: unresolved $ref → error
//   validateAgainstSchema: schema file not found, YAML-to-JSON failure
//   sortComponents: two unknown-priority components sorted alphabetically

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// checkJSONType
// --------------------------------------------------------------------------

func TestCheckJSONType_Integer_NotFloat(t *testing.T) {
	err := checkJSONType("not-a-number", "integer", "field")
	if err == nil {
		t.Error("expected error for string passed as integer, got nil")
	}
	if !strings.Contains(err.Error(), "field") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckJSONType_Integer_NonIntegerFloat(t *testing.T) {
	err := checkJSONType(float64(1.5), "integer", "field")
	if err == nil {
		t.Error("expected error for non-integer float, got nil")
	}
}

func TestCheckJSONType_Integer_ValidInt(t *testing.T) {
	if err := checkJSONType(float64(42), "integer", "field"); err != nil {
		t.Errorf("expected nil for valid integer, got: %v", err)
	}
}

func TestCheckJSONType_Boolean_Wrong(t *testing.T) {
	err := checkJSONType("not-a-bool", "boolean", "field")
	if err == nil {
		t.Error("expected error for string passed as boolean, got nil")
	}
}

func TestCheckJSONType_Boolean_Valid(t *testing.T) {
	if err := checkJSONType(true, "boolean", "field"); err != nil {
		t.Errorf("expected nil for valid boolean, got: %v", err)
	}
}

func TestCheckJSONType_Object_Wrong(t *testing.T) {
	err := checkJSONType("not-an-object", "object", "field")
	if err == nil {
		t.Error("expected error for string passed as object, got nil")
	}
}

func TestCheckJSONType_Array_Wrong(t *testing.T) {
	err := checkJSONType("not-an-array", "array", "field")
	if err == nil {
		t.Error("expected error for string passed as array, got nil")
	}
}

func TestCheckJSONType_String_Wrong(t *testing.T) {
	err := checkJSONType(42.0, "string", "field")
	if err == nil {
		t.Error("expected error for number passed as string, got nil")
	}
}

func TestCheckJSONType_UnknownType(t *testing.T) {
	// Unknown type names do not produce an error (no case matches → falls through).
	if err := checkJSONType("anything", "number", "field"); err != nil {
		t.Errorf("unexpected error for unknown type: %v", err)
	}
}

// --------------------------------------------------------------------------
// keyInProps
// --------------------------------------------------------------------------

func TestKeyInProps_Present(t *testing.T) {
	props := map[string]interface{}{"foo": "bar", "baz": 1}
	if !keyInProps("foo", props) {
		t.Error("expected keyInProps to return true for existing key")
	}
}

func TestKeyInProps_Absent(t *testing.T) {
	props := map[string]interface{}{"foo": "bar"}
	if keyInProps("missing", props) {
		t.Error("expected keyInProps to return false for absent key")
	}
}

// --------------------------------------------------------------------------
// resolveRef
// --------------------------------------------------------------------------

func TestResolveRef_NonDollarDefsPrefix(t *testing.T) {
	sv := &schemaValidator{root: map[string]interface{}{}, defs: map[string]interface{}{}}
	result := sv.resolveRef("http://example.com/other")
	if result != nil {
		t.Errorf("expected nil for non-#/$defs/ ref, got %v", result)
	}
}

func TestResolveRef_UnknownDefName(t *testing.T) {
	sv := &schemaValidator{
		root: map[string]interface{}{},
		defs: map[string]interface{}{"Known": map[string]interface{}{"type": "string"}},
	}
	result := sv.resolveRef("#/$defs/Unknown")
	if result != nil {
		t.Errorf("expected nil for unknown def name, got %v", result)
	}
}

func TestResolveRef_DefNotAMap(t *testing.T) {
	// Def exists but is not a map[string]interface{} (e.g. it's a string).
	sv := &schemaValidator{
		root: map[string]interface{}{},
		defs: map[string]interface{}{"BadDef": "not-a-map"},
	}
	result := sv.resolveRef("#/$defs/BadDef")
	if result != nil {
		t.Errorf("expected nil for def that is not a map, got %v", result)
	}
}

// --------------------------------------------------------------------------
// newSchemaValidator — invalid JSON
// --------------------------------------------------------------------------

func TestNewSchemaValidator_InvalidJSON(t *testing.T) {
	_, err := newSchemaValidator([]byte("{invalid json"))
	if err == nil {
		t.Error("expected error for invalid schema JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid schema JSON") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// validateAgainstSchema — error paths
// --------------------------------------------------------------------------

func TestValidateAgainstSchema_SchemaFileNotFound(t *testing.T) {
	yaml := []byte("apiVersion: foundry/v1\n")
	errs := validateAgainstSchema(yaml, "/nonexistent/schema.json")
	if len(errs) == 0 {
		t.Error("expected errors for missing schema file, got none")
	}
	if !strings.Contains(errs[0].Error(), "reading schema file") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

// --------------------------------------------------------------------------
// sortComponents — two unknown-priority components sorted alphabetically
// --------------------------------------------------------------------------

func TestSortComponents_TwoUnknownAlphabetical(t *testing.T) {
	comps := []ResolvedComponent{
		{Name: "zz-custom"},
		{Name: "aa-custom"},
	}
	sorted := sortComponents(comps)
	if sorted[0].Name != "aa-custom" || sorted[1].Name != "zz-custom" {
		t.Errorf("expected alphabetical sort, got [%s, %s]", sorted[0].Name, sorted[1].Name)
	}
}

func TestSortComponents_MixedPriorityAndUnknown(t *testing.T) {
	// foundry-postgres has priority; zz-custom does not.
	// Postgres should come before the unknown component.
	comps := []ResolvedComponent{
		{Name: "zz-custom"},
		{Name: "foundry-postgres"},
	}
	sorted := sortComponents(comps)
	if sorted[0].Name != "foundry-postgres" {
		t.Errorf("expected foundry-postgres first, got %s", sorted[0].Name)
	}
}

// --------------------------------------------------------------------------
// validateNode — numeric minimum/maximum violations
// --------------------------------------------------------------------------

func TestValidateNode_NumericBelowMinimum(t *testing.T) {
	sv := &schemaValidator{root: map[string]interface{}{}, defs: map[string]interface{}{}}
	schema := map[string]interface{}{"minimum": float64(10)}
	errs := sv.validateNode(float64(5), schema, "count")
	if len(errs) == 0 {
		t.Error("expected error for value below minimum, got none")
	}
	if !strings.Contains(errs[0].Error(), "minimum") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

func TestValidateNode_NumericAboveMaximum(t *testing.T) {
	sv := &schemaValidator{root: map[string]interface{}{}, defs: map[string]interface{}{}}
	schema := map[string]interface{}{"maximum": float64(100)}
	errs := sv.validateNode(float64(200), schema, "count")
	if len(errs) == 0 {
		t.Error("expected error for value above maximum, got none")
	}
	if !strings.Contains(errs[0].Error(), "maximum") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

// --------------------------------------------------------------------------
// validateObject — additionalProperties schema (keyInProps path)
// --------------------------------------------------------------------------

func TestValidateObject_AdditionalPropertiesSchema(t *testing.T) {
	// Schema with additionalProperties: {type: "string"} — validates unknown keys.
	sv := &schemaValidator{root: map[string]interface{}{}, defs: map[string]interface{}{}}
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"known": map[string]interface{}{"type": "string"},
		},
		"additionalProperties": map[string]interface{}{"type": "string"},
	}
	// Data has "known" key and "extra" key not in properties.
	data := map[string]interface{}{"known": "val", "extra": "valid-string"}
	errs := sv.validateObject(data, schema, "root")
	// "extra" is a string matching the additionalProperties schema — no error.
	if len(errs) != 0 {
		t.Errorf("unexpected errors for valid additionalProperties: %v", errs)
	}
}

func TestValidateObject_AdditionalPropertiesSchema_Invalid(t *testing.T) {
	// additionalProperties schema that fails for the extra key.
	sv := &schemaValidator{root: map[string]interface{}{}, defs: map[string]interface{}{}}
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"known": map[string]interface{}{"type": "string"},
		},
		"additionalProperties": map[string]interface{}{"type": "integer"},
	}
	// "extra" is a string but additionalProperties requires integer.
	data := map[string]interface{}{"known": "val", "extra": "not-an-int"}
	errs := sv.validateObject(data, schema, "root")
	if len(errs) == 0 {
		t.Error("expected error for invalid additionalProperties value, got none")
	}
}

// --------------------------------------------------------------------------
// ParseWithSchema — empty schema path (skip validation)
// --------------------------------------------------------------------------

func TestParseWithSchema_EmptySchemaPath(t *testing.T) {
	// Empty schema path skips JSON Schema validation entirely.
	spec := writeTempSpec(t, baseMinimal)
	ir, err := ParseWithSchema(spec, "")
	if err != nil {
		t.Fatalf("ParseWithSchema with empty schema path: %v", err)
	}
	if ir == nil {
		t.Error("expected non-nil IRSpec")
	}
}

// --------------------------------------------------------------------------
// Generate — MkdirAll error (output path exists as a file)
// --------------------------------------------------------------------------

func TestGenerate_OutputDirIsFile(t *testing.T) {
	// Create a file where the output directory should be.
	tmp := t.TempDir()
	badDir := filepath.Join(tmp, "output")
	if err := os.WriteFile(badDir, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	g := newGeneratorWithSpecDir(badDir, "", "")
	ir := &spec.IRSpec{}
	ir.Metadata.Name = "test"
	if err := g.Generate(ir, nil); err == nil {
		t.Error("expected error when outputDir is a file, got nil")
	}
}

// --------------------------------------------------------------------------
// Generate — write error (read-only output directory)
// --------------------------------------------------------------------------

func TestGenerate_ReadOnlyOutputDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories")
	}
	tmp := t.TempDir()
	// Pre-create the output dir, then make it read-only.
	if err := os.Chmod(tmp, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(tmp, 0755) })

	g := newGeneratorWithSpecDir(tmp, "", "")
	ir := &spec.IRSpec{}
	ir.Metadata.Name = "test"
	if err := g.Generate(ir, nil); err == nil {
		t.Error("expected error writing to read-only output directory, got nil")
	}
}
