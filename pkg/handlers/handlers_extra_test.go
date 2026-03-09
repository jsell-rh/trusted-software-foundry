package handlers

// handlers_extra_test.go covers:
//   SetMetadataID, NewMetadataHandler, MetadataHandler.Get
//   ValidateNotEmpty — value present, nil ptr, empty string
//   ValidateEmpty — nil ptr, non-nil non-empty, empty
//   ValidateInclusionIn — match, no match, no category

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/pkg/api"
)

// --------------------------------------------------------------------------
// SetMetadataID / NewMetadataHandler / Get
// --------------------------------------------------------------------------

func TestNewMetadataHandler_NotNil(t *testing.T) {
	h := NewMetadataHandler()
	if h == nil {
		t.Error("NewMetadataHandler() returned nil")
	}
}

func TestMetadataHandler_Get_StatusOK(t *testing.T) {
	h := NewMetadataHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestMetadataHandler_Get_ValidJSON(t *testing.T) {
	h := NewMetadataHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	var out api.Metadata
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Errorf("response body is not valid JSON: %v", err)
	}
}

func TestSetMetadataID(t *testing.T) {
	original := metadataID
	t.Cleanup(func() { metadataID = original })

	SetMetadataID("my-service")
	if metadataID != "my-service" {
		t.Errorf("metadataID = %q, want my-service", metadataID)
	}
}

func TestMetadataHandler_Get_ReflectsMetadataID(t *testing.T) {
	original := metadataID
	t.Cleanup(func() { metadataID = original })
	SetMetadataID("custom-svc")

	h := NewMetadataHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var out api.Metadata
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.ID != "custom-svc" {
		t.Errorf("Metadata.ID = %q, want custom-svc", out.ID)
	}
}

// --------------------------------------------------------------------------
// ValidateNotEmpty
// --------------------------------------------------------------------------

type testNotEmptyModel struct {
	Name    string
	PtrName *string
}

func TestValidateNotEmpty_NonEmpty_NoError(t *testing.T) {
	m := &testNotEmptyModel{Name: "hello"}
	v := ValidateNotEmpty(m, "Name", "name")
	if err := v(); err != nil {
		t.Errorf("ValidateNotEmpty(non-empty) = %v, want nil", err)
	}
}

func TestValidateNotEmpty_EmptyString_Error(t *testing.T) {
	m := &testNotEmptyModel{Name: ""}
	v := ValidateNotEmpty(m, "Name", "name")
	if err := v(); err == nil {
		t.Error("ValidateNotEmpty(empty string) should return error")
	}
}

func TestValidateNotEmpty_NilPtr_Error(t *testing.T) {
	m := &testNotEmptyModel{PtrName: nil}
	v := ValidateNotEmpty(m, "PtrName", "ptr_name")
	if err := v(); err == nil {
		t.Error("ValidateNotEmpty(nil ptr) should return error")
	}
}

func TestValidateNotEmpty_NonNilNonEmptyPtr_NoError(t *testing.T) {
	s := "hello"
	m := &testNotEmptyModel{PtrName: &s}
	v := ValidateNotEmpty(m, "PtrName", "ptr_name")
	if err := v(); err != nil {
		t.Errorf("ValidateNotEmpty(non-empty ptr) = %v, want nil", err)
	}
}

// --------------------------------------------------------------------------
// ValidateEmpty
// --------------------------------------------------------------------------

func TestValidateEmpty_EmptyString_NoError(t *testing.T) {
	m := &testNotEmptyModel{Name: ""}
	v := ValidateEmpty(m, "Name", "name")
	if err := v(); err != nil {
		t.Errorf("ValidateEmpty(empty) = %v, want nil", err)
	}
}

func TestValidateEmpty_NonEmpty_Error(t *testing.T) {
	m := &testNotEmptyModel{Name: "something"}
	v := ValidateEmpty(m, "Name", "name")
	if err := v(); err == nil {
		t.Error("ValidateEmpty(non-empty) should return error")
	}
}

func TestValidateEmpty_NilPtr_NoError(t *testing.T) {
	m := &testNotEmptyModel{PtrName: nil}
	v := ValidateEmpty(m, "PtrName", "ptr_name")
	if err := v(); err != nil {
		t.Errorf("ValidateEmpty(nil ptr) = %v, want nil", err)
	}
}

func TestValidateEmpty_NonNilNonEmptyPtr_Error(t *testing.T) {
	s := "nonempty"
	m := &testNotEmptyModel{PtrName: &s}
	v := ValidateEmpty(m, "PtrName", "ptr_name")
	if err := v(); err == nil {
		t.Error("ValidateEmpty(non-empty ptr) should return error")
	}
}

// --------------------------------------------------------------------------
// ValidateInclusionIn
// --------------------------------------------------------------------------

func TestValidateInclusionIn_Match(t *testing.T) {
	val := "apple"
	list := []string{"apple", "banana", "cherry"}
	v := ValidateInclusionIn(&val, list, nil)
	if err := v(); err != nil {
		t.Errorf("ValidateInclusionIn(match) = %v, want nil", err)
	}
}

func TestValidateInclusionIn_CaseInsensitiveMatch(t *testing.T) {
	val := "APPLE"
	list := []string{"apple", "banana"}
	v := ValidateInclusionIn(&val, list, nil)
	if err := v(); err != nil {
		t.Errorf("ValidateInclusionIn(case-insensitive match) = %v, want nil", err)
	}
}

func TestValidateInclusionIn_NoMatch_Error(t *testing.T) {
	val := "grape"
	list := []string{"apple", "banana"}
	v := ValidateInclusionIn(&val, list, nil)
	if err := v(); err == nil {
		t.Error("ValidateInclusionIn(no match) should return error")
	}
}

func TestValidateInclusionIn_NoMatch_WithCategory(t *testing.T) {
	val := "grape"
	list := []string{"apple", "banana"}
	cat := "fruit"
	v := ValidateInclusionIn(&val, list, &cat)
	if err := v(); err == nil {
		t.Error("ValidateInclusionIn(no match, with category) should return error")
	}
}
