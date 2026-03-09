package services

// event_errors_test.go covers the error branches in EventService methods that
// the standard daomocks cannot reach (Create, Delete, All, FindUnreconciled,
// FindBySourceAndType). Uses inline error-injecting stubs.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/pkg/api"
	"github.com/jsell-rh/trusted-software-foundry/pkg/errors"
)

// --------------------------------------------------------------------------
// Inline EventDao stubs
// --------------------------------------------------------------------------

type alwaysErrorEventDao struct{}

func (d *alwaysErrorEventDao) Get(_ context.Context, _ string) (*api.Event, error) {
	return nil, fmt.Errorf("db unavailable")
}
func (d *alwaysErrorEventDao) Create(_ context.Context, _ *api.Event) (*api.Event, error) {
	return nil, fmt.Errorf("insert failed")
}
func (d *alwaysErrorEventDao) Replace(_ context.Context, _ *api.Event) (*api.Event, error) {
	return nil, fmt.Errorf("update failed")
}
func (d *alwaysErrorEventDao) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("delete failed")
}
func (d *alwaysErrorEventDao) FindByIDs(_ context.Context, _ []string) (api.EventList, error) {
	return nil, fmt.Errorf("find failed")
}
func (d *alwaysErrorEventDao) All(_ context.Context) (api.EventList, error) {
	return nil, fmt.Errorf("all failed")
}
func (d *alwaysErrorEventDao) FindUnreconciled(_ context.Context, _ time.Duration) (api.EventList, error) {
	return nil, fmt.Errorf("unreconciled failed")
}
func (d *alwaysErrorEventDao) FindBySourceAndType(_ context.Context, _ string, _ api.EventType) (api.EventList, error) {
	return nil, fmt.Errorf("source/type failed")
}

type happyReplaceEventDao struct{}

func (d *happyReplaceEventDao) Get(_ context.Context, _ string) (*api.Event, error) {
	return nil, nil
}
func (d *happyReplaceEventDao) Create(_ context.Context, e *api.Event) (*api.Event, error) {
	return e, nil
}
func (d *happyReplaceEventDao) Replace(_ context.Context, e *api.Event) (*api.Event, error) {
	return e, nil
}
func (d *happyReplaceEventDao) Delete(_ context.Context, _ string) error { return nil }
func (d *happyReplaceEventDao) FindByIDs(_ context.Context, _ []string) (api.EventList, error) {
	return api.EventList{}, nil
}
func (d *happyReplaceEventDao) All(_ context.Context) (api.EventList, error) {
	return api.EventList{}, nil
}
func (d *happyReplaceEventDao) FindUnreconciled(_ context.Context, _ time.Duration) (api.EventList, error) {
	return api.EventList{}, nil
}
func (d *happyReplaceEventDao) FindBySourceAndType(_ context.Context, _ string, _ api.EventType) (api.EventList, error) {
	return api.EventList{}, nil
}

// --------------------------------------------------------------------------
// Error-branch tests
// --------------------------------------------------------------------------

func TestEventService_Create_Error(t *testing.T) {
	svc := NewEventService(&alwaysErrorEventDao{})
	_, err := svc.Create(context.Background(), &api.Event{})
	if err == nil {
		t.Fatal("expected error from Create, got nil")
	}
	if err.Code != errors.ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral", err.Code)
	}
}

func TestEventService_Replace_HappyPath(t *testing.T) {
	svc := NewEventService(&happyReplaceEventDao{})
	evt := &api.Event{Source: "tsc", EventType: "update"}
	evt.ID = "ev-r"
	got, err := svc.Replace(context.Background(), evt)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if got.ID != "ev-r" {
		t.Errorf("Replace returned ID %q, want ev-r", got.ID)
	}
}

func TestEventService_Delete_Error(t *testing.T) {
	svc := NewEventService(&alwaysErrorEventDao{})
	err := svc.Delete(context.Background(), "some-id")
	if err == nil {
		t.Fatal("expected error from Delete, got nil")
	}
	if err.Code != errors.ErrorGeneral {
		t.Errorf("Code = %d, want ErrorGeneral", err.Code)
	}
}

func TestEventService_All_Error(t *testing.T) {
	svc := NewEventService(&alwaysErrorEventDao{})
	_, err := svc.All(context.Background())
	if err == nil {
		t.Fatal("expected error from All, got nil")
	}
}

func TestEventService_FindUnreconciled_Error(t *testing.T) {
	svc := NewEventService(&alwaysErrorEventDao{})
	_, err := svc.FindUnreconciled(context.Background(), time.Hour)
	if err == nil {
		t.Fatal("expected error from FindUnreconciled, got nil")
	}
}

func TestEventService_FindBySourceAndType_Error(t *testing.T) {
	svc := NewEventService(&alwaysErrorEventDao{})
	_, err := svc.FindBySourceAndType(context.Background(), "src", "create")
	if err == nil {
		t.Fatal("expected error from FindBySourceAndType, got nil")
	}
}

func TestEventService_FindByIDs_HappyPath(t *testing.T) {
	svc := NewEventService(&happyReplaceEventDao{})
	events, err := svc.FindByIDs(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("FindByIDs: %v", err)
	}
	_ = events
}
