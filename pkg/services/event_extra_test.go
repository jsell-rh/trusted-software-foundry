package services

// event_extra_test.go tests EventService and covers HandleGetError's NotFound
// branch (gorm.ErrRecordNotFound) and addJoins.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jsell-rh/trusted-software-foundry/pkg/api"
	"github.com/jsell-rh/trusted-software-foundry/pkg/dao"
	daomocks "github.com/jsell-rh/trusted-software-foundry/pkg/dao/mocks"
	"github.com/jsell-rh/trusted-software-foundry/pkg/errors"
)

// --------------------------------------------------------------------------
// HandleGetError — gorm.ErrRecordNotFound branch (Is404)
// --------------------------------------------------------------------------

func TestHandleGetError_NotFound_GormSentinel(t *testing.T) {
	err := HandleGetError("Widget", "id", "abc123", gorm.ErrRecordNotFound)
	if err.Code != errors.ErrorNotFound {
		t.Errorf("Code = %d, want ErrorNotFound", err.Code)
	}
	if !err.Is404() {
		t.Error("Is404() should be true for gorm.ErrRecordNotFound")
	}
}

// --------------------------------------------------------------------------
// NewEventService
// --------------------------------------------------------------------------

func TestNewEventService(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	if svc == nil {
		t.Error("NewEventService() returned nil")
	}
}

// --------------------------------------------------------------------------
// EventService.Get
// --------------------------------------------------------------------------

func TestEventService_Get_HappyPath(t *testing.T) {
	mockDao := daomocks.NewEventDao()
	svc := NewEventService(mockDao)

	// Create an event directly in the mock's store via Create.
	evt := &api.Event{Meta: api.Meta{}, Source: "test", EventType: "create"}
	evt.ID = "event-1"
	created, svcErr := svc.Create(context.Background(), evt)
	if svcErr != nil {
		t.Fatalf("Create: %v", svcErr)
	}

	got, svcErr := svc.Get(context.Background(), created.ID)
	if svcErr != nil {
		t.Fatalf("Get: %v", svcErr)
	}
	if got.ID != created.ID {
		t.Errorf("Get ID = %q, want %q", got.ID, created.ID)
	}
}

func TestEventService_Get_NotFound(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	_, svcErr := svc.Get(context.Background(), "nonexistent")
	if svcErr == nil {
		t.Fatal("expected error for missing event, got nil")
	}
	if svcErr.Code != errors.ErrorNotFound {
		t.Errorf("Code = %d, want ErrorNotFound", svcErr.Code)
	}
}

// --------------------------------------------------------------------------
// EventService.Create
// --------------------------------------------------------------------------

func TestEventService_Create_HappyPath(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	evt := &api.Event{Source: "tsc", EventType: "create"}
	evt.ID = "ev-2"
	created, svcErr := svc.Create(context.Background(), evt)
	if svcErr != nil {
		t.Fatalf("Create: %v", svcErr)
	}
	if created.ID != "ev-2" {
		t.Errorf("created ID = %q, want ev-2", created.ID)
	}
}

// --------------------------------------------------------------------------
// EventService.Replace — mock always errors (NotImplemented)
// --------------------------------------------------------------------------

func TestEventService_Replace_Error(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	evt := &api.Event{Source: "tsc", EventType: "update"}
	evt.ID = "ev-3"
	_, svcErr := svc.Replace(context.Background(), evt)
	if svcErr == nil {
		t.Fatal("expected error from Replace (mock returns NotImplemented)")
	}
}

// --------------------------------------------------------------------------
// EventService.Delete
// --------------------------------------------------------------------------

func TestEventService_Delete_HappyPath(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	evt := &api.Event{Source: "tsc", EventType: "create"}
	evt.ID = "ev-4"
	_, _ = svc.Create(context.Background(), evt)

	svcErr := svc.Delete(context.Background(), "ev-4")
	if svcErr != nil {
		t.Fatalf("Delete: %v", svcErr)
	}
}

// --------------------------------------------------------------------------
// EventService.FindByIDs — mock returns NotImplemented
// --------------------------------------------------------------------------

func TestEventService_FindByIDs_Error(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	_, svcErr := svc.FindByIDs(context.Background(), []string{"id1", "id2"})
	if svcErr == nil {
		t.Fatal("expected error from FindByIDs (mock returns NotImplemented)")
	}
}

// --------------------------------------------------------------------------
// EventService.All
// --------------------------------------------------------------------------

func TestEventService_All_Empty(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	events, svcErr := svc.All(context.Background())
	if svcErr != nil {
		t.Fatalf("All: %v", svcErr)
	}
	_ = events // empty list is fine
}

func TestEventService_All_WithEvents(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	for i := 0; i < 3; i++ {
		evt := &api.Event{Source: "src", EventType: "create"}
		evt.ID = fmt.Sprintf("ev-%d", i)
		_, _ = svc.Create(context.Background(), evt)
	}
	events, svcErr := svc.All(context.Background())
	if svcErr != nil {
		t.Fatalf("All: %v", svcErr)
	}
	if len(events) != 3 {
		t.Errorf("All returned %d events, want 3", len(events))
	}
}

// --------------------------------------------------------------------------
// EventService.FindUnreconciled
// --------------------------------------------------------------------------

func TestEventService_FindUnreconciled(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	events, svcErr := svc.FindUnreconciled(context.Background(), time.Hour)
	if svcErr != nil {
		t.Fatalf("FindUnreconciled: %v", svcErr)
	}
	_ = events
}

// --------------------------------------------------------------------------
// EventService.FindBySourceAndType
// --------------------------------------------------------------------------

func TestEventService_FindBySourceAndType(t *testing.T) {
	svc := NewEventService(daomocks.NewEventDao())
	evt := &api.Event{Source: "tsc", EventType: "create"}
	evt.ID = "ev-src"
	_, _ = svc.Create(context.Background(), evt)

	results, svcErr := svc.FindBySourceAndType(context.Background(), "tsc", "create")
	if svcErr != nil {
		t.Fatalf("FindBySourceAndType: %v", svcErr)
	}
	if len(results) != 1 {
		t.Errorf("FindBySourceAndType returned %d events, want 1", len(results))
	}
}

// --------------------------------------------------------------------------
// addJoins — with populated joins
// --------------------------------------------------------------------------

func TestAddJoins_WithJoins(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}

	var d dao.GenericDao = mockDao
	listCtx := &listContext{
		set: make(map[string]bool),
		joins: map[string]dao.TableRelation{
			"creator": {
				TableName:         "dinosaurs",
				ColumnName:        "creator_id",
				ForeignTableName:  "accounts",
				ForeignColumnName: "id",
			},
		},
		groupBy: nil,
	}
	svc.addJoins(listCtx, &d)

	// After addJoins the joins map should be reset.
	if len(listCtx.joins) != 0 {
		t.Errorf("joins not reset after addJoins; len=%d", len(listCtx.joins))
	}
}

func TestAddJoins_SkipsAlreadyPreloaded(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	var d dao.GenericDao = mockDao

	listCtx := &listContext{
		set: map[string]bool{"accounts": true}, // already preloaded
		joins: map[string]dao.TableRelation{
			"creator": {
				TableName:        "dinosaurs",
				ColumnName:       "creator_id",
				ForeignTableName: "accounts",
			},
		},
	}
	svc.addJoins(listCtx, &d)
	// groupBy should NOT include accounts since it was already in set.
	for _, g := range listCtx.groupBy {
		if strings.Contains(g, "accounts") {
			t.Errorf("accounts should have been skipped (already preloaded) but got groupBy=%v", listCtx.groupBy)
		}
	}
}

// --------------------------------------------------------------------------
// buildSearch — empty search (exercises addJoins with no joins)
// --------------------------------------------------------------------------

func TestBuildSearch_EmptySearch(t *testing.T) {
	mockDao := daomocks.NewGenericDao()
	svc := &sqlGenericService{genericDao: mockDao}
	var d dao.GenericDao = mockDao

	disallowed := map[string]string{}
	listCtx := &listContext{
		args:             &ListArguments{Search: ""},
		disallowedFields: &disallowed,
		set:              make(map[string]bool),
		joins:            map[string]dao.TableRelation{},
	}
	finished, err := svc.buildSearch(listCtx, &d)
	if err != nil {
		t.Fatalf("buildSearch empty: %v", err)
	}
	if !finished {
		t.Error("buildSearch should return finished=true")
	}
}
