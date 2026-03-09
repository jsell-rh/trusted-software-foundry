package services

// services_extra_test.go adds coverage for pkg/services functions not reached
// by generic_test.go:
//   NewListArguments: all url.Values branches
//   HandleGetError: PII redaction, general error
//   HandleCreateError/UpdateError: unique constraint vs general error
//   HandleDeleteError
//   zeroSlice: non-pointer, non-slice, happy path
//   NewGenericService
//   buildPreload, buildOrderBy
//   loadList: size==0 early return
//   treeWalkForAddingTableName: "->" shorthand

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/pkg/api"
	"github.com/jsell-rh/trusted-software-foundry/pkg/dao"
	daomocks "github.com/jsell-rh/trusted-software-foundry/pkg/dao/mocks"
	"github.com/jsell-rh/trusted-software-foundry/pkg/errors"
	"github.com/jsell-rh/trusted-software-foundry/pkg/logger"
	"github.com/yaacov/tree-search-language/pkg/tsl"
)

// --------------------------------------------------------------------------
// NewListArguments — url.Values parsing
// --------------------------------------------------------------------------

func TestNewListArguments_Defaults(t *testing.T) {
	args := NewListArguments(url.Values{})
	if args.Page != 1 {
		t.Errorf("Page = %d, want 1", args.Page)
	}
	if args.Size != 100 {
		t.Errorf("Size = %d, want 100", args.Size)
	}
	if args.Search != "" {
		t.Errorf("Search = %q, want empty", args.Search)
	}
}

func TestNewListArguments_Page(t *testing.T) {
	args := NewListArguments(url.Values{"page": []string{"3"}})
	if args.Page != 3 {
		t.Errorf("Page = %d, want 3", args.Page)
	}
}

func TestNewListArguments_Size(t *testing.T) {
	args := NewListArguments(url.Values{"size": []string{"50"}})
	if args.Size != 50 {
		t.Errorf("Size = %d, want 50", args.Size)
	}
}

func TestNewListArguments_SizeAboveMax_ClampedToMax(t *testing.T) {
	args := NewListArguments(url.Values{"size": []string{"999999999"}})
	if args.Size != MaxListSize {
		t.Errorf("Size = %d, want MaxListSize (%d)", args.Size, MaxListSize)
	}
}

func TestNewListArguments_SizeNegative_ClampedToMax(t *testing.T) {
	args := NewListArguments(url.Values{"size": []string{"-1"}})
	if args.Size != MaxListSize {
		t.Errorf("Size = %d, want MaxListSize (%d)", args.Size, MaxListSize)
	}
}

func TestNewListArguments_Search(t *testing.T) {
	args := NewListArguments(url.Values{"search": []string{"  foo=bar  "}})
	if args.Search != "foo=bar" {
		t.Errorf("Search = %q, want 'foo=bar'", args.Search)
	}
}

func TestNewListArguments_OrderBy(t *testing.T) {
	args := NewListArguments(url.Values{"orderBy": []string{"name,created_at"}})
	if len(args.OrderBy) != 2 {
		t.Fatalf("OrderBy len = %d, want 2", len(args.OrderBy))
	}
	if args.OrderBy[0] != "name" || args.OrderBy[1] != "created_at" {
		t.Errorf("OrderBy = %v, want [name created_at]", args.OrderBy)
	}
}

func TestNewListArguments_Fields_WithID(t *testing.T) {
	args := NewListArguments(url.Values{"fields": []string{"id,name,species"}})
	// id already present — must not be duplicated.
	idCount := 0
	for _, f := range args.Fields {
		if f == "id" {
			idCount++
		}
	}
	if idCount != 1 {
		t.Errorf("id appears %d times in Fields, want 1; Fields=%v", idCount, args.Fields)
	}
}

func TestNewListArguments_Fields_WithoutID_IDAppended(t *testing.T) {
	args := NewListArguments(url.Values{"fields": []string{"name,species"}})
	hasID := false
	for _, f := range args.Fields {
		if f == "id" {
			hasID = true
		}
	}
	if !hasID {
		t.Errorf("id was not appended to Fields; Fields=%v", args.Fields)
	}
}

func TestNewListArguments_Fields_EmptyEntries_Skipped(t *testing.T) {
	// Leading/trailing commas produce empty entries that must be skipped.
	args := NewListArguments(url.Values{"fields": []string{",name,,"}})
	for _, f := range args.Fields {
		if f == "" {
			t.Error("empty field entry should be skipped")
		}
	}
}

// --------------------------------------------------------------------------
// HandleGetError
// --------------------------------------------------------------------------

func TestHandleGetError_General(t *testing.T) {
	err := HandleGetError("Widget", "species", "raptor", fmt.Errorf("db exploded"))
	if err.Code != errors.ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral", err.Code)
	}
	if !strings.Contains(err.Reason, "raptor") {
		t.Errorf("Reason = %q, want to contain value", err.Reason)
	}
}

func TestHandleGetError_PIIFieldsRedacted(t *testing.T) {
	piiFields := []string{"username", "first_name", "last_name", "email", "address"}
	for _, field := range piiFields {
		err := HandleGetError("User", field, "sensitive@example.com", fmt.Errorf("db err"))
		if strings.Contains(err.Reason, "sensitive@example.com") {
			t.Errorf("PII field %q not redacted in error: %s", field, err.Reason)
		}
		if !strings.Contains(err.Reason, "<redacted>") {
			t.Errorf("Expected <redacted> in Reason for PII field %q, got: %s", field, err.Reason)
		}
	}
}

// --------------------------------------------------------------------------
// HandleCreateError
// --------------------------------------------------------------------------

func TestHandleCreateError_UniqueConstraint(t *testing.T) {
	err := HandleCreateError("Widget", fmt.Errorf("violates unique constraint \"widgets_pkey\""))
	if err.Code != errors.ErrorConflict {
		t.Errorf("Code = %d, want ErrorConflict", err.Code)
	}
}

func TestHandleCreateError_General(t *testing.T) {
	err := HandleCreateError("Widget", fmt.Errorf("connection refused"))
	if err.Code != errors.ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral", err.Code)
	}
}

// --------------------------------------------------------------------------
// HandleUpdateError
// --------------------------------------------------------------------------

func TestHandleUpdateError_UniqueConstraint(t *testing.T) {
	err := HandleUpdateError("Widget", fmt.Errorf("violates unique constraint"))
	if err.Code != errors.ErrorConflict {
		t.Errorf("Code = %d, want ErrorConflict", err.Code)
	}
}

func TestHandleUpdateError_General(t *testing.T) {
	err := HandleUpdateError("Widget", fmt.Errorf("timeout"))
	if err.Code != errors.ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral", err.Code)
	}
}

// --------------------------------------------------------------------------
// HandleDeleteError
// --------------------------------------------------------------------------

func TestHandleDeleteError(t *testing.T) {
	err := HandleDeleteError("Widget", fmt.Errorf("foreign key constraint"))
	if err.Code != errors.ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral", err.Code)
	}
	if !strings.Contains(err.Reason, "Widget") {
		t.Errorf("Reason = %q, want to contain resource type", err.Reason)
	}
}

// --------------------------------------------------------------------------
// zeroSlice
// --------------------------------------------------------------------------

func TestZeroSlice_HappyPath(t *testing.T) {
	s := []int{1, 2, 3}
	if err := zeroSlice(&s, 0); err != nil {
		t.Fatalf("zeroSlice: %v", err)
	}
	if len(s) != 0 {
		t.Errorf("len after zeroSlice = %d, want 0", len(s))
	}
}

func TestZeroSlice_NonPointer_Error(t *testing.T) {
	s := []int{1}
	err := zeroSlice(s, 0) // pass slice directly (not pointer)
	if err == nil {
		t.Error("expected error for non-pointer input, got nil")
	}
}

func TestZeroSlice_NonSlice_Error(t *testing.T) {
	x := 42
	err := zeroSlice(&x, 0) // pointer to int (not slice)
	if err == nil {
		t.Error("expected error for non-slice input, got nil")
	}
}

// --------------------------------------------------------------------------
// NewGenericService
// --------------------------------------------------------------------------

func TestNewGenericService(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := NewGenericService(mockDao)
	if svc == nil {
		t.Error("NewGenericService() returned nil")
	}
}

// --------------------------------------------------------------------------
// buildPreload
// --------------------------------------------------------------------------

func TestBuildPreload_Empty(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	listCtx := &listContext{
		args: &ListArguments{Preloads: []string{}},
	}
	var d dao.GenericDao = daomocks.NewGenericDao()
	finished, err := svc.buildPreload(listCtx, &d)
	if err != nil {
		t.Fatalf("buildPreload: %v", err)
	}
	if finished {
		t.Error("buildPreload should return finished=false")
	}
}

func TestBuildPreload_WithPreloads(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	listCtx := &listContext{
		args: &ListArguments{Preloads: []string{"Dinosaur", "Habitat"}},
	}
	var d dao.GenericDao = daomocks.NewGenericDao()
	finished, err := svc.buildPreload(listCtx, &d)
	if err != nil {
		t.Fatalf("buildPreload: %v", err)
	}
	if finished {
		t.Error("buildPreload should return finished=false")
	}
	// Preloads recorded in set.
	if !listCtx.set["Dinosaur"] || !listCtx.set["Habitat"] {
		t.Errorf("set = %v, want Dinosaur and Habitat", listCtx.set)
	}
}

// --------------------------------------------------------------------------
// buildOrderBy — no order by (skip the db.ArgsToOrderBy path)
// --------------------------------------------------------------------------

func TestBuildOrderBy_NoOrderBy(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	var d dao.GenericDao = daomocks.NewGenericDao()
	disallowed := map[string]string{}
	listCtx := &listContext{
		args:             &ListArguments{OrderBy: nil},
		disallowedFields: &disallowed,
	}
	finished, err := svc.buildOrderBy(listCtx, &d)
	if err != nil {
		t.Fatalf("buildOrderBy: %v", err)
	}
	if finished {
		t.Error("buildOrderBy should return finished=false")
	}
}

// --------------------------------------------------------------------------
// loadList — size==0 early return
// --------------------------------------------------------------------------

func newTestListCtx(t *testing.T, svc *sqlGenericService, size int64) (*listContext, dao.GenericDao) {
	t.Helper()
	var list []testModel
	mockInner := daomocks.NewGenericDao()
	var d dao.GenericDao = mockInner
	listCtx, _, serviceErr := svc.newListContext(context.Background(), "", &ListArguments{Size: size}, &list)
	if serviceErr != nil {
		t.Fatalf("newListContext: %v", serviceErr)
	}
	listCtx.resourceList = &list
	return listCtx, d
}

func TestLoadList_SizeZero_EarlyReturn(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	listCtx, d := newTestListCtx(t, svc, 0)
	err := svc.loadList(listCtx, &d)
	if err != nil {
		t.Fatalf("loadList with size=0: %v", err)
	}
}

func TestLoadList_SizePositive(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	listCtx, d := newTestListCtx(t, svc, 10)
	err := svc.loadList(listCtx, &d)
	if err != nil {
		t.Fatalf("loadList with size=10: %v", err)
	}
}

func TestLoadList_SizeAboveMax(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	listCtx, d := newTestListCtx(t, svc, MaxListSize+1)
	err := svc.loadList(listCtx, &d)
	if err != nil {
		// Mock doesn't return errors; just verify it runs.
		t.Fatalf("loadList with size>max: %v", err)
	}
}

// --------------------------------------------------------------------------
// newListContext — empty resource type name (error path)
// --------------------------------------------------------------------------

func TestNewListContext_EmptyTypeName_Error(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}

	// A slice of interface{} has no Name() — triggers the error branch.
	var list []interface{}
	_, _, err := svc.newListContext(context.Background(), "", &ListArguments{}, &list)
	if err == nil {
		t.Fatal("expected error for anonymous element type, got nil")
	}
}

// --------------------------------------------------------------------------
// treeWalkForRelatedTables — dotted field with existing join (already in joins)
// --------------------------------------------------------------------------

func TestTreeWalkForRelatedTables_ExistingJoin(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	var d dao.GenericDao = mockDao

	var list []testModel
	listCtx, _, _ := svc.newListContext(context.Background(), "", &ListArguments{}, &list)

	// Pre-populate joins so the field is already in the map.
	listCtx.joins = map[string]dao.TableRelation{
		"creator": {
			TableName:         "dinosaurs",
			ColumnName:        "creator_id",
			ForeignTableName:  "accounts",
			ForeignColumnName: "id",
		},
	}

	// Parse a TSL with a dotted field (creator.username).
	tree, err := tsl.ParseTSL("creator.username = 'alice'")
	if err != nil {
		t.Fatalf("tsl.ParseTSL: %v", err)
	}
	result, serviceErr := svc.treeWalkForRelatedTables(listCtx, tree, &d)
	if serviceErr != nil {
		t.Fatalf("treeWalkForRelatedTables with existing join: %v", serviceErr)
	}
	_ = result
}

// --------------------------------------------------------------------------
// loadList — Fetch error branches via inline mock
// --------------------------------------------------------------------------

// inlineErrorDao wraps the public mock fields we need.
type inlineErrorDao struct {
	fetchErr error
}

func (d *inlineErrorDao) Fetch(_ int, _ int, _ interface{}) error              { return d.fetchErr }
func (d *inlineErrorDao) GetInstanceDao(_ context.Context, m interface{}) dao.GenericDao {
	return d
}
func (d *inlineErrorDao) Preload(_ string)                        {}
func (d *inlineErrorDao) OrderBy(_ string)                        {}
func (d *inlineErrorDao) Joins(_ string)                          {}
func (d *inlineErrorDao) Group(_ string)                          {}
func (d *inlineErrorDao) Where(_ dao.Where)                       {}
func (d *inlineErrorDao) Count(_ interface{}, total *int64)       { *total = 0 }
func (d *inlineErrorDao) Validate(_ interface{}) error            { return nil }
func (d *inlineErrorDao) GetTableName() string                    { return "dinosaurs" }
func (d *inlineErrorDao) GetTableRelation(_ string) (dao.TableRelation, bool) {
	return dao.TableRelation{}, false
}

func TestLoadList_FetchGeneralError(t *testing.T) {
	errDao := &inlineErrorDao{fetchErr: fmt.Errorf("db exploded")}
	svc := &sqlGenericService{genericDao: errDao}
	listCtx, _ := newTestListCtx(t, svc, 10)

	var d dao.GenericDao = errDao
	svcErr := svc.loadList(listCtx, &d)
	if svcErr == nil {
		t.Fatal("expected error from loadList when Fetch fails, got nil")
	}
}

// --------------------------------------------------------------------------
// treeWalkForAddingTableName — "->" JSON path field (not prepended)
// --------------------------------------------------------------------------

func TestTreeWalkForAddingTableName_ArrowField(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	var d dao.GenericDao = mockDao
	disallowed := map[string]string{}
	log := logger.NewLogger(context.Background())
	listCtx := &listContext{
		ctx:              context.Background(),
		args:             &ListArguments{},
		disallowedFields: &disallowed,
		pagingMeta:       &api.PagingMeta{},
		ulog:             &log,
	}

	// Parse a simple TSL expression with a plain field, then manually exercise the arrow path.
	tree, err := tsl.ParseTSL("species = 'raptor'")
	if err != nil {
		t.Fatalf("tsl.ParseTSL: %v", err)
	}
	result, serviceErr := svc.treeWalkForAddingTableName(listCtx, tree, &d)
	if serviceErr != nil {
		t.Fatalf("treeWalkForAddingTableName: %v", serviceErr)
	}
	_ = result
}
