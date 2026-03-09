package util

// util_extra_test.go covers all functions in pkg/util/utils.go:
//   ToPtr, FromPtr, FromEmptyPtr, Empty
//   EmptyStringToNil, NilToEmptyString
//   GetAccountIDFromContext

import (
	"context"
	"testing"
)

// --------------------------------------------------------------------------
// ToPtr
// --------------------------------------------------------------------------

func TestToPtr_Int(t *testing.T) {
	v := 42
	p := ToPtr(v)
	if p == nil {
		t.Fatal("ToPtr returned nil")
	}
	if *p != v {
		t.Errorf("*ToPtr(%d) = %d, want %d", v, *p, v)
	}
}

func TestToPtr_String(t *testing.T) {
	p := ToPtr("hello")
	if *p != "hello" {
		t.Errorf("*ToPtr = %q, want hello", *p)
	}
}

func TestToPtr_Bool(t *testing.T) {
	p := ToPtr(true)
	if !*p {
		t.Error("*ToPtr(true) = false")
	}
}

// --------------------------------------------------------------------------
// FromPtr
// --------------------------------------------------------------------------

func TestFromPtr_NonNil(t *testing.T) {
	v := 99
	got := FromPtr(&v)
	if got != v {
		t.Errorf("FromPtr = %d, want %d", got, v)
	}
}

func TestFromPtr_Nil_ReturnsZero(t *testing.T) {
	var p *int
	got := FromPtr(p)
	if got != 0 {
		t.Errorf("FromPtr(nil) = %d, want 0", got)
	}
}

func TestFromPtr_NilString_ReturnsEmpty(t *testing.T) {
	var p *string
	got := FromPtr(p)
	if got != "" {
		t.Errorf("FromPtr(nil string) = %q, want empty", got)
	}
}

// --------------------------------------------------------------------------
// FromEmptyPtr
// --------------------------------------------------------------------------

func TestFromEmptyPtr_NonNil(t *testing.T) {
	v := 7
	got := FromEmptyPtr(&v)
	if got == nil || *got != v {
		t.Errorf("FromEmptyPtr(&7) = %v, want &7", got)
	}
}

func TestFromEmptyPtr_Nil_ReturnsZeroPtr(t *testing.T) {
	var p *int
	got := FromEmptyPtr(p)
	if got == nil {
		t.Fatal("FromEmptyPtr(nil) returned nil, want &0")
	}
	if *got != 0 {
		t.Errorf("*FromEmptyPtr(nil) = %d, want 0", *got)
	}
}

// --------------------------------------------------------------------------
// Empty
// --------------------------------------------------------------------------

func TestEmpty_Int(t *testing.T) {
	if Empty[int]() != 0 {
		t.Error("Empty[int]() should return 0")
	}
}

func TestEmpty_String(t *testing.T) {
	if Empty[string]() != "" {
		t.Error("Empty[string]() should return empty string")
	}
}

func TestEmpty_Bool(t *testing.T) {
	if Empty[bool]() != false {
		t.Error("Empty[bool]() should return false")
	}
}

// --------------------------------------------------------------------------
// EmptyStringToNil
// --------------------------------------------------------------------------

func TestEmptyStringToNil_Empty(t *testing.T) {
	got := EmptyStringToNil("")
	if got != nil {
		t.Errorf("EmptyStringToNil(\"\") = %v, want nil", got)
	}
}

func TestEmptyStringToNil_NonEmpty(t *testing.T) {
	got := EmptyStringToNil("hello")
	if got == nil {
		t.Fatal("EmptyStringToNil(\"hello\") returned nil")
	}
	if *got != "hello" {
		t.Errorf("*EmptyStringToNil(\"hello\") = %q, want hello", *got)
	}
}

// --------------------------------------------------------------------------
// NilToEmptyString
// --------------------------------------------------------------------------

func TestNilToEmptyString_Nil(t *testing.T) {
	got := NilToEmptyString(nil)
	if got != "" {
		t.Errorf("NilToEmptyString(nil) = %q, want empty", got)
	}
}

func TestNilToEmptyString_NonNil(t *testing.T) {
	s := "world"
	got := NilToEmptyString(&s)
	if got != s {
		t.Errorf("NilToEmptyString(&%q) = %q, want %q", s, got, s)
	}
}

// --------------------------------------------------------------------------
// GetAccountIDFromContext
// --------------------------------------------------------------------------

func TestGetAccountIDFromContext_Set(t *testing.T) {
	ctx := context.WithValue(context.Background(), "accountID", "acc-123")
	got := GetAccountIDFromContext(ctx)
	if got != "acc-123" {
		t.Errorf("GetAccountIDFromContext = %q, want acc-123", got)
	}
}

func TestGetAccountIDFromContext_NotSet(t *testing.T) {
	got := GetAccountIDFromContext(context.Background())
	if got != "" {
		t.Errorf("GetAccountIDFromContext (not set) = %q, want empty", got)
	}
}

func TestGetAccountIDFromContext_IntValue(t *testing.T) {
	// context value that is an int — should be stringified via fmt.Sprintf
	ctx := context.WithValue(context.Background(), "accountID", 42)
	got := GetAccountIDFromContext(ctx)
	if got != "42" {
		t.Errorf("GetAccountIDFromContext(int 42) = %q, want \"42\"", got)
	}
}
