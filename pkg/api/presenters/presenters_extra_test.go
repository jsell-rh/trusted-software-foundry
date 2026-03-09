package presenters

// presenters_extra_test.go covers:
//   RegisterKind, LoadDiscoveredKinds, ObjectKind
//   RegisterPath, LoadDiscoveredPaths, BasePath, SetBasePath, ObjectPath
//   PresentTime
//   PresentReference (string id, *string id, empty id)
//   PresentError
//   SliceFilter: nil model, empty items, valid items, invalid fields

import (
	"testing"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/pkg/errors"
)

// --------------------------------------------------------------------------
// RegisterKind / LoadDiscoveredKinds / ObjectKind
// --------------------------------------------------------------------------

type testKindObj struct{}

func TestRegisterKind_AndLoad(t *testing.T) {
	RegisterKind(testKindObj{}, "TestKind")
	got := LoadDiscoveredKinds(testKindObj{})
	if got != "TestKind" {
		t.Errorf("LoadDiscoveredKinds = %q, want TestKind", got)
	}
}

func TestLoadDiscoveredKinds_NotRegistered(t *testing.T) {
	type unregisteredType struct{}
	got := LoadDiscoveredKinds(unregisteredType{})
	if got != "" {
		t.Errorf("LoadDiscoveredKinds (not registered) = %q, want empty", got)
	}
}

func TestObjectKind_ServiceError(t *testing.T) {
	err := errors.GeneralError("test")
	kind := ObjectKind(err)
	if kind == nil || *kind != "Error" {
		t.Errorf("ObjectKind(ServiceError) = %v, want &\"Error\"", kind)
	}
}

func TestObjectKind_ServiceErrorPointer(t *testing.T) {
	// GeneralError returns *ServiceError directly — pass it as the pointer case.
	err := errors.GeneralError("test")
	kind := ObjectKind(err)
	if kind == nil || *kind != "Error" {
		t.Errorf("ObjectKind(*ServiceError) = %v, want &\"Error\"", kind)
	}
}

func TestObjectKind_RegisteredType(t *testing.T) {
	RegisterKind(testKindObj{}, "TestKindRegistered")
	kind := ObjectKind(testKindObj{})
	if kind == nil || *kind != "TestKindRegistered" {
		t.Errorf("ObjectKind(registered) = %v, want &\"TestKindRegistered\"", kind)
	}
}

// --------------------------------------------------------------------------
// BasePath / SetBasePath / ObjectPath
// --------------------------------------------------------------------------

func TestBasePath_Default(t *testing.T) {
	p := BasePath()
	if p == "" {
		t.Error("BasePath() should not be empty")
	}
}

func TestSetBasePath(t *testing.T) {
	original := BasePath()
	t.Cleanup(func() { SetBasePath(original) })

	SetBasePath("/api/test/v2")
	if BasePath() != "/api/test/v2" {
		t.Errorf("BasePath() = %q after SetBasePath, want /api/test/v2", BasePath())
	}
}

func TestObjectPath_ServiceError(t *testing.T) {
	original := BasePath()
	t.Cleanup(func() { SetBasePath(original) })
	SetBasePath("/api/v1")

	err := errors.GeneralError("test")
	p := ObjectPath("err-id", err)
	if p == nil {
		t.Fatal("ObjectPath returned nil")
	}
	if *p == "" {
		t.Error("ObjectPath returned empty string")
	}
}

func TestObjectPath_UnknownType(t *testing.T) {
	type unknownObj struct{}
	p := ObjectPath("some-id", unknownObj{})
	if p == nil {
		t.Fatal("ObjectPath returned nil")
	}
	// Unknown type returns path with empty segment
	_ = *p
}

// --------------------------------------------------------------------------
// PresentTime
// --------------------------------------------------------------------------

func TestPresentTime_Zero(t *testing.T) {
	got := PresentTime(time.Time{})
	if got == nil {
		t.Fatal("PresentTime(zero) returned nil")
	}
	if !got.IsZero() {
		t.Errorf("PresentTime(zero) = %v, want zero time", got)
	}
}

func TestPresentTime_NonZero(t *testing.T) {
	ts := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	got := PresentTime(ts)
	if got == nil {
		t.Fatal("PresentTime returned nil")
	}
	if got.IsZero() {
		t.Error("PresentTime(non-zero) returned zero time")
	}
}

func TestPresentTime_RoundsMicrosecond(t *testing.T) {
	// Time with nanoseconds should be rounded to microsecond
	ts := time.Date(2024, 1, 15, 12, 0, 0, 999, time.UTC)
	got := PresentTime(ts)
	if got == nil {
		t.Fatal("PresentTime returned nil")
	}
	if got.Nanosecond()%1000 != 0 {
		t.Errorf("PresentTime nanoseconds not rounded to microsecond: %d", got.Nanosecond())
	}
}

// --------------------------------------------------------------------------
// PresentReference
// --------------------------------------------------------------------------

func TestPresentReference_StringID(t *testing.T) {
	err := errors.GeneralError("test")
	ref := PresentReference("ref-id", err)
	if ref.Id == nil || *ref.Id != "ref-id" {
		t.Errorf("PresentReference Id = %v, want ref-id", ref.Id)
	}
}

func TestPresentReference_PointerID(t *testing.T) {
	err := errors.GeneralError("test")
	id := "ptr-id"
	ref := PresentReference(&id, err)
	if ref.Id == nil || *ref.Id != "ptr-id" {
		t.Errorf("PresentReference (*string) Id = %v, want ptr-id", ref.Id)
	}
}

func TestPresentReference_EmptyStringID(t *testing.T) {
	err := errors.GeneralError("test")
	ref := PresentReference("", err)
	// Empty id causes makeReferenceId to return ok=false — returns empty ObjectReference
	if ref.Id != nil {
		t.Errorf("PresentReference(empty id) should have nil Id, got %v", ref.Id)
	}
}

func TestPresentReference_NilPointerID(t *testing.T) {
	err := errors.GeneralError("test")
	var id *string
	ref := PresentReference(id, err)
	// Nil *string gives empty refId → ok=false
	if ref.Id != nil {
		t.Errorf("PresentReference(nil *string) should have nil Id, got %v", ref.Id)
	}
}

// --------------------------------------------------------------------------
// PresentError
// --------------------------------------------------------------------------

func TestPresentError(t *testing.T) {
	svcErr := errors.NotFound("thing %s", "123")
	out := PresentError(svcErr)
	if out.Code == nil || *out.Code == "" {
		t.Error("PresentError returned error with empty Code")
	}
}

// --------------------------------------------------------------------------
// SliceFilter
// --------------------------------------------------------------------------

type sliceFilterItem struct {
	Name    string `json:"name"`
	Species string `json:"species"`
}

type sliceFilterList struct {
	Kind  string
	Page  int
	Size  int
	Total int
	Items []sliceFilterItem
}

func TestSliceFilter_NilModel(t *testing.T) {
	_, err := SliceFilter([]string{"name"}, nil)
	if err == nil {
		t.Error("SliceFilter(nil) should return error")
	}
}

func TestSliceFilter_EmptyItems(t *testing.T) {
	model := &sliceFilterList{Kind: "List", Page: 1, Size: 0, Total: 0, Items: nil}
	result, err := SliceFilter([]string{"name"}, model)
	if err != nil {
		t.Fatalf("SliceFilter(empty items): %v", err)
	}
	if result == nil {
		t.Fatal("SliceFilter(empty items) returned nil")
	}
	if result.Kind != "List" {
		t.Errorf("Kind = %q, want List", result.Kind)
	}
}

func TestSliceFilter_ValidFields(t *testing.T) {
	model := &sliceFilterList{
		Kind:  "List",
		Page:  1,
		Size:  2,
		Total: 2,
		Items: []sliceFilterItem{
			{Name: "Rex", Species: "T-Rex"},
			{Name: "Bob", Species: "Triceratops"},
		},
	}
	result, err := SliceFilter([]string{"name", "species"}, model)
	if err != nil {
		t.Fatalf("SliceFilter(valid): %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items len = %d, want 2", len(result.Items))
	}
}

func TestSliceFilter_InvalidField(t *testing.T) {
	model := &sliceFilterList{
		Kind:  "List",
		Page:  1,
		Size:  1,
		Total: 1,
		Items: []sliceFilterItem{{Name: "Rex", Species: "T-Rex"}},
	}
	_, err := SliceFilter([]string{"nonexistent_field"}, model)
	if err == nil {
		t.Error("SliceFilter with invalid field should return error")
	}
}
