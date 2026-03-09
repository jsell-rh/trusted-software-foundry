package compiler

// schema_validator_test.go covers the unexported schema validator functions
// that are not reachable through ParseWithSchema alone. Tests here call the
// validator functions directly to cover defensive error branches.

import (
	"testing"
)

// --------------------------------------------------------------------------
// resolveRef — $defs name not found and non-$defs prefix
// --------------------------------------------------------------------------

// TestResolveRef_KnownDef verifies the happy path: a valid #/$defs/ reference
// resolves to the definition schema map.
func TestResolveRef_KnownDef(t *testing.T) {
	sv := &schemaValidator{
		defs: map[string]interface{}{
			"Widget": map[string]interface{}{"type": "object"},
		},
	}
	got := sv.resolveRef("#/$defs/Widget")
	if got == nil {
		t.Fatal("expected non-nil result for known $defs name")
	}
	if got["type"] != "object" {
		t.Errorf("expected type=object in resolved def, got: %v", got)
	}
}

// TestResolveRef_UnknownDef verifies that a #/$defs/ reference to a name that
// does not exist in defs returns nil gracefully. This is the defensive branch
// guarding against schema $ref mismatches.
func TestResolveRef_UnknownDef(t *testing.T) {
	sv := &schemaValidator{
		defs: map[string]interface{}{
			"Widget": map[string]interface{}{"type": "object"},
		},
	}
	got := sv.resolveRef("#/$defs/NonExistentType")
	if got != nil {
		t.Errorf("expected nil for unknown $defs name, got: %v", got)
	}
}

// TestResolveRef_NonDefsPrefixRef verifies that a $ref that does not start with
// "#/$defs/" (e.g. a remote or root-relative ref) returns nil. The validator only
// supports the local $defs convention used by tsc/spec/schema.json.
func TestResolveRef_NonDefsPrefixRef(t *testing.T) {
	sv := &schemaValidator{
		defs: map[string]interface{}{},
	}
	for _, ref := range []string{
		"#/components/schemas/Widget",
		"https://example.com/schema.json",
		"other-schema.json",
		"",
	} {
		got := sv.resolveRef(ref)
		if got != nil {
			t.Errorf("resolveRef(%q) = %v, want nil", ref, got)
		}
	}
}

// --------------------------------------------------------------------------
// newSchemaValidator — defs absent from schema root
// --------------------------------------------------------------------------

// TestNewSchemaValidator_NoDefs verifies that a schema without a $defs key
// produces a validator with nil defs (not a panic). The validator falls back
// to ignoring $ref resolution gracefully.
func TestNewSchemaValidator_NoDefs(t *testing.T) {
	schemaJSON := []byte(`{"type": "object"}`)
	sv, err := newSchemaValidator(schemaJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sv.defs != nil {
		t.Errorf("expected nil defs for schema without $defs, got: %v", sv.defs)
	}
	// resolveRef must not panic when defs is nil
	result := sv.resolveRef("#/$defs/Widget")
	if result != nil {
		t.Errorf("resolveRef with nil defs must return nil, got: %v", result)
	}
}
