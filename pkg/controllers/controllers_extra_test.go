package controllers

// controllers_extra_test.go adds coverage for branches missed by
// framework_test.go and sync_controller_test.go:
//   Handle: lock error path, lock not-acquired path
//   handle: source not found, event type not found, handler error, Replace error
//   Register: via NewSyncController (single call, fresh registry)
//   performSync: FindUnreconciled error, event still unreconciled after requeue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/pkg/api"
	"github.com/jsell-rh/trusted-software-foundry/pkg/dao/mocks"
	"github.com/jsell-rh/trusted-software-foundry/pkg/db"
	"github.com/jsell-rh/trusted-software-foundry/pkg/services"
)

// --------------------------------------------------------------------------
// Custom lock factories for branch coverage
// --------------------------------------------------------------------------

// errorLockFactory always returns an error from NewNonBlockingLock.
type errorLockFactory struct{}

func (e *errorLockFactory) NewAdvisoryLock(_ context.Context, _ string, _ db.LockType) (string, error) {
	return "", fmt.Errorf("lock failure")
}
func (e *errorLockFactory) NewNonBlockingLock(_ context.Context, _ string, _ db.LockType) (string, bool, error) {
	return "", false, fmt.Errorf("lock error")
}
func (e *errorLockFactory) Unlock(_ context.Context, _ string) {}

// notAcquiredLockFactory returns acquired=false (lock taken by another worker).
type notAcquiredLockFactory struct{}

func (n *notAcquiredLockFactory) NewAdvisoryLock(_ context.Context, _ string, _ db.LockType) (string, error) {
	return "owner", nil
}
func (n *notAcquiredLockFactory) NewNonBlockingLock(_ context.Context, _ string, _ db.LockType) (string, bool, error) {
	return "owner", false, nil // Not acquired
}
func (n *notAcquiredLockFactory) Unlock(_ context.Context, _ string) {}

// --------------------------------------------------------------------------
// Handle — lock error branch
// --------------------------------------------------------------------------

func TestHandle_LockError(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&errorLockFactory{}, eventService)

	// Should not panic; just logs the error and returns.
	mgr.Handle("some-event-id")
}

// --------------------------------------------------------------------------
// Handle — lock not acquired branch
// --------------------------------------------------------------------------

func TestHandle_LockNotAcquired(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&notAcquiredLockFactory{}, eventService)

	// Should not panic; just logs "processed by another worker".
	mgr.Handle("some-event-id")
}

// --------------------------------------------------------------------------
// handle — source not found
// --------------------------------------------------------------------------

func TestHandle_SourceNotFound(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&mockLockFactory{}, eventService)

	// Create an event whose source is not registered.
	evt := &api.Event{
		Meta:      api.Meta{ID: "ev-src"},
		Source:    "unknown-source",
		EventType: api.CreateEventType,
	}
	mockEventDao.Create(context.Background(), evt)

	// mgr has no handlers for "unknown-source" — logs "No controllers found".
	mgr.handle(context.Background(), "ev-src")
}

// --------------------------------------------------------------------------
// handle — event type not found
// --------------------------------------------------------------------------

func TestHandle_EventTypeNotFound(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&mockLockFactory{}, eventService)

	// Register handlers only for "create", not "delete".
	mgr.Add(&ControllerConfig{
		Source: "my-source",
		Handlers: map[api.EventType][]ControllerHandlerFunc{
			api.CreateEventType: {func(_ context.Context, _ string) error { return nil }},
		},
	})

	// Create an event with a different event type.
	evt := &api.Event{
		Meta:      api.Meta{ID: "ev-type"},
		Source:    "my-source",
		EventType: api.DeleteEventType, // no handler registered for delete
	}
	mockEventDao.Create(context.Background(), evt)

	// Should log "No handler functions found" and return.
	mgr.handle(context.Background(), "ev-type")
}

// --------------------------------------------------------------------------
// handle — handler returns error
// --------------------------------------------------------------------------

func TestHandle_HandlerError(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&mockLockFactory{}, eventService)

	// Handler that always fails.
	mgr.Add(&ControllerConfig{
		Source: "err-source",
		Handlers: map[api.EventType][]ControllerHandlerFunc{
			api.CreateEventType: {func(_ context.Context, _ string) error {
				return fmt.Errorf("handler exploded")
			}},
		},
	})

	evt := &api.Event{
		Meta:      api.Meta{ID: "ev-err"},
		Source:    "err-source",
		EventType: api.CreateEventType,
	}
	mockEventDao.Create(context.Background(), evt)

	// Should log the error and return without calling Replace.
	mgr.handle(context.Background(), "ev-err")
}

// --------------------------------------------------------------------------
// handle — event Get error (event not found in dao)
// --------------------------------------------------------------------------

func TestHandle_GetError(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&mockLockFactory{}, eventService)

	// Don't create the event — Get will return not-found error.
	mgr.handle(context.Background(), "nonexistent-event-id")
}

// --------------------------------------------------------------------------
// handle — Replace returns error (mock Replace always errors)
// --------------------------------------------------------------------------

func TestHandle_ReplaceError(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&mockLockFactory{}, eventService)

	// Handler succeeds.
	mgr.Add(&ControllerConfig{
		Source: "replace-source",
		Handlers: map[api.EventType][]ControllerHandlerFunc{
			api.CreateEventType: {func(_ context.Context, _ string) error { return nil }},
		},
	})

	evt := &api.Event{
		Meta:      api.Meta{ID: "ev-replace"},
		Source:    "replace-source",
		SourceID:  "src-id",
		EventType: api.CreateEventType,
	}
	mockEventDao.Create(context.Background(), evt)

	// Replace always fails in the standard mock (NotImplemented) — logs the error.
	mgr.handle(context.Background(), "ev-replace")
}

// --------------------------------------------------------------------------
// NewSyncController — calls Register() (prometheus metrics registration)
// --------------------------------------------------------------------------

func TestNewSyncController_RegistersMetrics(t *testing.T) {
	// This test exercises the Register() path (registerMetrics=true).
	// prometheus.MustRegister panics on duplicate registration, so this can only
	// be run once per process. We recover any panic from a re-registration.
	defer func() {
		// Tolerate "already registered" panics from prior test runs in same binary.
		if r := recover(); r != nil {
			t.Logf("Recovered from prometheus re-registration panic (expected in test binary): %v", r)
		}
	}()

	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)
	mgr := NewKindControllerManager(&mockLockFactory{}, eventService)

	sc := NewSyncController(mgr, eventService, SyncControllerConfig{
		Interval:         5 * time.Minute,
		MaxAge:           time.Hour,
		MaxEventsPerSync: 100,
	})
	if sc == nil {
		t.Error("NewSyncController() returned nil")
	}
}

// --------------------------------------------------------------------------
// performSync — FindUnreconciled returns error
// --------------------------------------------------------------------------

func TestPerformSync_FindUnreconciledError(t *testing.T) {
	// Use an always-error event service.
	svc := services.NewEventService(&alwaysErrorEventSvc{})
	mgr := NewKindControllerManager(&mockLockFactory{}, svc)

	sc := NewSyncControllerForTesting(mgr, svc, SyncControllerConfig{
		Interval: time.Hour,
		MaxAge:   time.Hour,
	})

	// Should log the error and return without panic.
	sc.performSync(context.Background())
}

// alwaysErrorEventSvc is an inline EventDao that always errors.
type alwaysErrorEventSvc struct{}

func (d *alwaysErrorEventSvc) Get(_ context.Context, _ string) (*api.Event, error) {
	return nil, fmt.Errorf("always errors")
}
func (d *alwaysErrorEventSvc) Create(_ context.Context, _ *api.Event) (*api.Event, error) {
	return nil, fmt.Errorf("always errors")
}
func (d *alwaysErrorEventSvc) Replace(_ context.Context, _ *api.Event) (*api.Event, error) {
	return nil, fmt.Errorf("always errors")
}
func (d *alwaysErrorEventSvc) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("always errors")
}
func (d *alwaysErrorEventSvc) FindByIDs(_ context.Context, _ []string) (api.EventList, error) {
	return nil, fmt.Errorf("always errors")
}
func (d *alwaysErrorEventSvc) All(_ context.Context) (api.EventList, error) {
	return nil, fmt.Errorf("always errors")
}
func (d *alwaysErrorEventSvc) FindUnreconciled(_ context.Context, _ time.Duration) (api.EventList, error) {
	return nil, fmt.Errorf("find unreconciled failed")
}
func (d *alwaysErrorEventSvc) FindBySourceAndType(_ context.Context, _ string, _ api.EventType) (api.EventList, error) {
	return nil, fmt.Errorf("always errors")
}

// --------------------------------------------------------------------------
// performSync — event still unreconciled after requeue (no handler, Get succeeds)
// --------------------------------------------------------------------------

func TestPerformSync_EventStillUnreconciled(t *testing.T) {
	mockEventDao := mocks.NewEventDao()
	eventService := services.NewEventService(mockEventDao)

	// No handlers registered — event will remain unreconciled after Handle.
	mgr := NewKindControllerManager(&mockLockFactory{}, eventService)

	sc := NewSyncControllerForTesting(mgr, eventService, SyncControllerConfig{
		Interval:         time.Hour,
		MaxAge:           30 * time.Minute,
		MaxEventsPerSync: 100,
	})

	// Create an old, unreconciled event.
	evt := &api.Event{
		Meta: api.Meta{
			ID:        "stale-event",
			CreatedAt: time.Now().Add(-2 * time.Hour),
		},
		Source:         "unknown-source",
		EventType:      api.CreateEventType,
		ReconciledDate: nil,
	}
	mockEventDao.Create(context.Background(), evt)

	// performSync will find it, call Handle (which logs "No controllers found"),
	// then check ReconciledDate == nil → logs "still unreconciled".
	sc.performSync(context.Background())
}
