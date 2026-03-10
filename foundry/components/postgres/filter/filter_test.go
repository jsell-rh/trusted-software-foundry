package filter_test

import (
	"errors"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/components/postgres/filter"
)

var allFields = map[string]bool{
	"species": true,
	"weight":  true,
	"active":  true,
	"name":    true,
	"age":     true,
	"score":   true,
}

func TestBuildWhere_Empty(t *testing.T) {
	sql, args, err := filter.BuildWhere("", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "" || len(args) != 0 {
		t.Errorf("expected empty result for empty search, got sql=%q args=%v", sql, args)
	}
}

func TestBuildWhere_WhitespaceOnly(t *testing.T) {
	sql, args, err := filter.BuildWhere("   \t  ", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "" || len(args) != 0 {
		t.Errorf("expected empty result for whitespace search, got sql=%q args=%v", sql, args)
	}
}

func TestBuildWhere_EqualString(t *testing.T) {
	sql, args, err := filter.BuildWhere("species='Stegosaurus'", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "species = $1" {
		t.Errorf("sql = %q, want %q", sql, "species = $1")
	}
	if len(args) != 1 || args[0] != "Stegosaurus" {
		t.Errorf("args = %v, want [Stegosaurus]", args)
	}
}

func TestBuildWhere_EqualNumber(t *testing.T) {
	sql, args, err := filter.BuildWhere("weight=1000", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "weight = $1" {
		t.Errorf("sql = %q, want %q", sql, "weight = $1")
	}
	if len(args) != 1 || args[0] != float64(1000) {
		t.Errorf("args = %v, want [1000]", args)
	}
}

func TestBuildWhere_EqualBoolTrue(t *testing.T) {
	sql, args, err := filter.BuildWhere("active=true", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "active = $1" {
		t.Errorf("sql = %q, want %q", sql, "active = $1")
	}
	if len(args) != 1 || args[0] != true {
		t.Errorf("args = %v, want [true]", args)
	}
}

func TestBuildWhere_EqualBoolFalse(t *testing.T) {
	_, args, err := filter.BuildWhere("active=false", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != false {
		t.Errorf("args = %v, want [false]", args)
	}
}

func TestBuildWhere_NotEqual(t *testing.T) {
	sql, args, err := filter.BuildWhere("species!='T-Rex'", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "species != $1" {
		t.Errorf("sql = %q, want %q", sql, "species != $1")
	}
	if args[0] != "T-Rex" {
		t.Errorf("args[0] = %v, want T-Rex", args[0])
	}
}

func TestBuildWhere_GreaterThan(t *testing.T) {
	sql, args, err := filter.BuildWhere("weight>500", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "weight > $1" {
		t.Errorf("sql = %q, want %q", sql, "weight > $1")
	}
	if args[0] != float64(500) {
		t.Errorf("args[0] = %v, want 500", args[0])
	}
}

func TestBuildWhere_LessThan(t *testing.T) {
	sql, _, err := filter.BuildWhere("weight<200", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "weight < $1" {
		t.Errorf("sql = %q", sql)
	}
}

func TestBuildWhere_LessThanOrEqual(t *testing.T) {
	sql, args, err := filter.BuildWhere("age<=10", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "age <= $1" {
		t.Errorf("sql = %q, want %q", sql, "age <= $1")
	}
	_ = args
}

func TestBuildWhere_GreaterThanOrEqual(t *testing.T) {
	sql, args, err := filter.BuildWhere("score>=90", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "score >= $1" {
		t.Errorf("sql = %q, want %q", sql, "score >= $1")
	}
	_ = args
}

func TestBuildWhere_ANDCombination(t *testing.T) {
	sql, args, err := filter.BuildWhere("species='Stegosaurus' AND weight>1000", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "(species = $1 AND weight > $2)"
	if sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "Stegosaurus" || args[1] != float64(1000) {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhere_ORCombination(t *testing.T) {
	sql, args, err := filter.BuildWhere("species='T-Rex' OR species='Raptor'", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "(species = $1 OR species = $2)"
	if sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhere_NOT(t *testing.T) {
	sql, args, err := filter.BuildWhere("NOT active=true", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "(NOT active = $1)"
	if sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}
	_ = args
}

func TestBuildWhere_Parentheses(t *testing.T) {
	sql, args, err := filter.BuildWhere("(species='T-Rex' OR species='Raptor') AND active=true", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "((species = $1 OR species = $2) AND active = $3)"
	if sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
}

func TestBuildWhere_CaseInsensitiveKeywords(t *testing.T) {
	tests := []struct {
		search string
		want   string
	}{
		{"species='A' and weight>1", "(species = $1 AND weight > $2)"},
		{"species='A' AND weight>1", "(species = $1 AND weight > $2)"},
		{"active=TRUE", "active = $1"},
		{"active=False", "active = $1"},
		{"NOT active=true", "(NOT active = $1)"},
		{"not active=true", "(NOT active = $1)"},
	}
	for _, tt := range tests {
		sql, _, err := filter.BuildWhere(tt.search, allFields)
		if err != nil {
			t.Errorf("search=%q: %v", tt.search, err)
			continue
		}
		if sql != tt.want {
			t.Errorf("search=%q: sql=%q, want %q", tt.search, sql, tt.want)
		}
	}
}

func TestBuildWhere_UnknownField(t *testing.T) {
	_, _, err := filter.BuildWhere("secret='injection'", allFields)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	var unknownErr *filter.ErrUnknownField
	if !errors.As(err, &unknownErr) {
		t.Errorf("expected ErrUnknownField, got %T: %v", err, err)
	}
	if unknownErr.Field != "secret" {
		t.Errorf("ErrUnknownField.Field = %q, want %q", unknownErr.Field, "secret")
	}
}

func TestBuildWhere_SQLInjectionAttempt_Field(t *testing.T) {
	// Attempt to inject SQL via a malformed field name.
	_, _, err := filter.BuildWhere("1=1; DROP TABLE users --='x'", allFields)
	if err == nil {
		t.Fatal("expected error for injected field name")
	}
}

func TestBuildWhere_SQLInjectionAttempt_Value(t *testing.T) {
	// Value with SQL injection content — must be safe because it becomes $1.
	sql, args, err := filter.BuildWhere("species='foo; DROP TABLE users --'", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The dangerous string becomes a bound parameter, never interpolated.
	if sql != "species = $1" {
		t.Errorf("sql = %q", sql)
	}
	if args[0] != "foo; DROP TABLE users --" {
		t.Errorf("args[0] = %v", args[0])
	}
}

func TestBuildWhere_UnterminatedString(t *testing.T) {
	_, _, err := filter.BuildWhere("species='unterminated", allFields)
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
}

func TestBuildWhere_UnexpectedToken(t *testing.T) {
	_, _, err := filter.BuildWhere("species = 'a' garbage", allFields)
	if err == nil {
		t.Fatal("expected error for trailing garbage token")
	}
}

func TestBuildWhere_MissingValue(t *testing.T) {
	_, _, err := filter.BuildWhere("species=", allFields)
	if err == nil {
		t.Fatal("expected error for missing value")
	}
}

func TestBuildWhere_NilAllowedFields_RejectsAll(t *testing.T) {
	_, _, err := filter.BuildWhere("species='foo'", nil)
	if err == nil {
		t.Fatal("expected error when allowedFields is nil")
	}
}

func TestBuildWhere_EmptyAllowedFields_RejectsAll(t *testing.T) {
	_, _, err := filter.BuildWhere("species='foo'", map[string]bool{})
	if err == nil {
		t.Fatal("expected error when allowedFields is empty")
	}
}

func TestBuildWhere_ThreeWayAND(t *testing.T) {
	sql, args, err := filter.BuildWhere("species='T-Rex' AND weight>500 AND active=true", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Parser is left-associative: ((A AND B) AND C)
	want := "((species = $1 AND weight > $2) AND active = $3)"
	if sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
}

func TestBuildWhere_DecimalNumber(t *testing.T) {
	sql, args, err := filter.BuildWhere("score>=9.5", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != "score >= $1" {
		t.Errorf("sql = %q", sql)
	}
	if args[0] != float64(9.5) {
		t.Errorf("args[0] = %v, want 9.5", args[0])
	}
}

func TestBuildWhere_MissingCloseParen(t *testing.T) {
	_, _, err := filter.BuildWhere("(species='foo'", allFields)
	if err == nil {
		t.Fatal("expected error for missing close paren")
	}
}

func TestBuildWhere_MultipleArgs_CorrectOrder(t *testing.T) {
	// Verify args are in left-to-right order.
	sql, args, err := filter.BuildWhere("name='Alice' OR age>30", allFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "(name = $1 OR age > $2)"
	if sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}
	if len(args) != 2 || args[0] != "Alice" || args[1] != float64(30) {
		t.Errorf("args = %v", args)
	}
}
