package events

// events_test.go provides coverage for the foundry-events component:
//   New, Name, Version, AuditHash, Configure, Register, Start, Stop,
//   loop, dispatch, channelNameToResource.
//
// Note: the Start() error path (LISTEN failure) is not exercised here because
// pq.Listener.Listen() blocks on reconnectCond.Wait() indefinitely when the
// database is unreachable — it requires a real PostgreSQL connection.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

// --------------------------------------------------------------------------
// Constructor and accessors
// --------------------------------------------------------------------------

func TestComponentMetadata(t *testing.T) {
	c := New()
	if c.Name() != componentName {
		t.Errorf("Name() = %q, want %q", c.Name(), componentName)
	}
	if c.Version() != componentVersion {
		t.Errorf("Version() = %q, want %q", c.Version(), componentVersion)
	}
	if c.AuditHash() == "" {
		t.Error("AuditHash() must not be empty")
	}
}

// --------------------------------------------------------------------------
// channelNameToResource
// --------------------------------------------------------------------------

func TestChannelNameToResource(t *testing.T) {
	cases := []struct{ channel, want string }{
		{"dinosaur_events", "Dinosaur"},
		{"user_events", "User"},
		{"_events", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := channelNameToResource(tc.channel)
		if got != tc.want {
			t.Errorf("channelNameToResource(%q) = %q, want %q", tc.channel, got, tc.want)
		}
	}
}

// --------------------------------------------------------------------------
// Subscribe
// --------------------------------------------------------------------------

func TestSubscribe(t *testing.T) {
	c := New()
	called := false
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {
		called = true
	})
	c.mu.RLock()
	handlers := c.handlers["Dinosaur"]
	c.mu.RUnlock()
	if len(handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(handlers))
	}
	_ = called
}

func TestSubscribe_MultipleHandlers(t *testing.T) {
	c := New()
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {})
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {})
	c.Subscribe("Plant", func(ctx context.Context, resource string, payload EventPayload) {})

	c.mu.RLock()
	dinoCnt := len(c.handlers["Dinosaur"])
	plantCnt := len(c.handlers["Plant"])
	c.mu.RUnlock()

	if dinoCnt != 2 {
		t.Errorf("Dinosaur handlers = %d, want 2", dinoCnt)
	}
	if plantCnt != 1 {
		t.Errorf("Plant handlers = %d, want 1", plantCnt)
	}
}

// --------------------------------------------------------------------------
// Configure
// --------------------------------------------------------------------------

func TestConfigure(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": "host=localhost dbname=test"}); err != nil {
		t.Fatalf("Configure() error: %v", err)
	}
	if c.dsn != "host=localhost dbname=test" {
		t.Errorf("dsn = %q, want %q", c.dsn, "host=localhost dbname=test")
	}
}

func TestConfigure_NonStringDSNIgnored(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": 12345}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.dsn != "" {
		t.Errorf("dsn = %q, want empty for non-string value", c.dsn)
	}
}

func TestConfigure_EmptyDSNIgnored(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": ""}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.dsn != "" {
		t.Errorf("dsn = %q, want empty for empty-string value", c.dsn)
	}
}

func TestConfigure_EmptyConfig(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure({}): %v", err)
	}
}

// --------------------------------------------------------------------------
// Register
// --------------------------------------------------------------------------

func TestRegister_NoDSN_FallsBackToDefault(t *testing.T) {
	c := New()
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if c.dsn == "" {
		t.Error("dsn should be set to a default after Register with empty DSN")
	}
}

func TestRegister_DSNPreserved(t *testing.T) {
	c := New()
	c.dsn = "host=custom-db port=5432"
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if c.dsn != "host=custom-db port=5432" {
		t.Errorf("dsn = %q, want original preserved", c.dsn)
	}
}

func TestRegister_AutoSubscribesEventsResources(t *testing.T) {
	c := New()
	resources := []spec.ResourceDefinition{
		{Name: "Dinosaur", Events: true},
		{Name: "Plant", Events: false},
	}
	app := spec.NewApplication(resources)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	c.mu.RLock()
	_, hasDino := c.handlers["Dinosaur"]
	_, hasPlant := c.handlers["Plant"]
	c.mu.RUnlock()

	if !hasDino {
		t.Error("expected Dinosaur to be auto-subscribed (Events: true)")
	}
	if hasPlant {
		t.Error("expected Plant NOT to be auto-subscribed (Events: false)")
	}
}

func TestRegister_DoesNotOverwriteExistingHandlers(t *testing.T) {
	c := New()
	var called bool
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {
		called = true
	})

	resources := []spec.ResourceDefinition{
		{Name: "Dinosaur", Events: true},
	}
	app := spec.NewApplication(resources)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	c.mu.RLock()
	handlers := c.handlers["Dinosaur"]
	c.mu.RUnlock()

	// Register must not replace existing handlers — it only ensures an entry
	// exists when there is none.
	if len(handlers) != 1 {
		t.Errorf("handlers = %d, want 1 (pre-registered must be preserved)", len(handlers))
	}
	_ = called
}

// --------------------------------------------------------------------------
// Start / Stop — lifecycle (no Listen() calls needed; empty handler map)
// --------------------------------------------------------------------------

// newFastListener returns a pq.Listener with a 1 ms reconnect interval so that
// any internal goroutine wakes up and notices isClosed=true quickly after Close.
func newFastListener() *pq.Listener {
	// This DSN will always fail to connect (port 60999 is almost certainly
	// closed). With minReconn=1ms the internal goroutine cycles every ~1ms,
	// so Close() returns within a few milliseconds.
	const badDSN = "host=127.0.0.1 port=60999 dbname=test sslmode=disable"
	noop := func(pq.ListenerEventType, error) {}
	return pq.NewListener(badDSN, time.Millisecond, time.Millisecond, noop)
}

func TestStart_NoHandlers_Succeeds(t *testing.T) {
	// With no handlers/resources Start makes zero Listen() calls, so it
	// succeeds even when the DSN points to an unreachable server.
	c := New()
	c.listener = newFastListener() // pre-inject so Start skips NewListener call

	// Patch Start: Start re-creates c.listener internally, so we must set the
	// DSN so pq.NewListener inside Start also uses a bad DSN. We also accept
	// that Start creates its own listener (the one we injected gets replaced).
	c.dsn = "host=127.0.0.1 port=60999 dbname=test sslmode=disable"

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start with no handlers: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestStop_AfterStart_NoHandlers(t *testing.T) {
	c := New()
	c.dsn = "host=127.0.0.1 port=60999 dbname=test sslmode=disable"

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// --------------------------------------------------------------------------
// dispatch — tested directly (no live DB required)
// --------------------------------------------------------------------------

func TestDispatch_ValidPayload_HandlerCalled(t *testing.T) {
	c := New()

	var (
		mu          sync.Mutex
		gotResource string
		gotPayload  EventPayload
	)
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {
		mu.Lock()
		gotResource = resource
		gotPayload = payload
		mu.Unlock()
	})

	raw, _ := json.Marshal(EventPayload{Action: "created", ID: "abc-123"})
	c.dispatch(context.Background(), &pq.Notification{
		Channel: "dinosaur_events",
		Extra:   string(raw),
	})

	mu.Lock()
	defer mu.Unlock()
	if gotResource != "Dinosaur" {
		t.Errorf("resource = %q, want Dinosaur", gotResource)
	}
	if gotPayload.Action != "created" {
		t.Errorf("action = %q, want created", gotPayload.Action)
	}
	if gotPayload.ID != "abc-123" {
		t.Errorf("id = %q, want abc-123", gotPayload.ID)
	}
}

func TestDispatch_InvalidJSON_NoPanic(t *testing.T) {
	c := New()
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {
		t.Error("handler must not be called for invalid JSON")
	})

	// Must not panic; error is logged and the function returns.
	c.dispatch(context.Background(), &pq.Notification{
		Channel: "dinosaur_events",
		Extra:   "not-valid-json{{{",
	})
}

func TestDispatch_NoHandlers_NoPanic(t *testing.T) {
	c := New()
	// No handler registered for Plant — must not crash.
	raw, _ := json.Marshal(EventPayload{Action: "deleted", ID: "xyz"})
	c.dispatch(context.Background(), &pq.Notification{
		Channel: "plant_events",
		Extra:   string(raw),
	})
}

func TestDispatch_MultipleHandlers_AllCalled(t *testing.T) {
	c := New()
	var count int
	var mu sync.Mutex
	inc := func(ctx context.Context, resource string, payload EventPayload) {
		mu.Lock()
		count++
		mu.Unlock()
	}
	c.Subscribe("Dinosaur", inc)
	c.Subscribe("Dinosaur", inc)
	c.Subscribe("Dinosaur", inc)

	raw, _ := json.Marshal(EventPayload{Action: "updated", ID: "id1"})
	c.dispatch(context.Background(), &pq.Notification{
		Channel: "dinosaur_events",
		Extra:   string(raw),
	})

	mu.Lock()
	defer mu.Unlock()
	if count != 3 {
		t.Errorf("handler call count = %d, want 3", count)
	}
}

// --------------------------------------------------------------------------
// loop — inject notifications via pq.Listener.Notify (no live DB needed)
//
// pq.Listener.Notify is an exported chan *pq.Notification we can reassign to a
// channel we control. The pq internal goroutine writes to its own internal
// channel (l.notificationChan), so replacing l.Notify does not cause races.
// --------------------------------------------------------------------------

// injectableListener creates a pq.Listener and replaces its exported Notify
// channel with a locally owned buffered channel. The returned channel can be
// written to freely by test code. The pq goroutine retries the bad DSN every
// 1 ms; calling Close() on the listener signals it to stop within a few ms.
func injectableListener() (*pq.Listener, chan *pq.Notification) {
	const badDSN = "host=127.0.0.1 port=60999 dbname=test sslmode=disable"
	noop := func(pq.ListenerEventType, error) {}
	l := pq.NewListener(badDSN, time.Millisecond, time.Millisecond, noop)
	ch := make(chan *pq.Notification, 32)
	l.Notify = ch
	return l, ch
}

func TestLoop_ContextCancel_Exits(t *testing.T) {
	c := New()
	l, _ := injectableListener()
	c.listener = l

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		c.loop(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// loop exited cleanly via ctx.Done()
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not exit after context cancel")
	}
}

func TestLoop_NilNotification_Skips(t *testing.T) {
	// A nil notification is a keepalive ping — loop must continue without
	// calling any handler, then exit when the context is cancelled.
	c := New()
	l, ch := injectableListener()
	c.listener = l
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {
		t.Error("handler must not be called for a nil keepalive notification")
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		c.loop(ctx)
		close(done)
	}()

	ch <- nil // keepalive
	// Give loop a moment to process nil before cancelling.
	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not exit after context cancel")
	}
}

func TestLoop_RealNotification_DispatchesThroughLoop(t *testing.T) {
	c := New()
	l, ch := injectableListener()
	c.listener = l

	var (
		mu     sync.Mutex
		called bool
	)
	c.Subscribe("Dinosaur", func(ctx context.Context, resource string, payload EventPayload) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		c.loop(ctx)
		close(done)
	}()

	raw, _ := json.Marshal(EventPayload{Action: "created", ID: "loop-test"})
	ch <- &pq.Notification{Channel: "dinosaur_events", Extra: string(raw)}

	// Allow loop to process the notification before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not exit after context cancel")
	}

	mu.Lock()
	wasCalled := called
	mu.Unlock()
	if !wasCalled {
		t.Error("handler was not called for notification sent through loop")
	}
}

func TestLoop_NotifyChannelClosed_Exits(t *testing.T) {
	// When the pq goroutine exits it closes l.Notify (which we've replaced
	// with our own channel). The loop must exit via the !ok case.
	c := New()
	l, _ := injectableListener()
	c.listener = l

	ctx := context.Background() // no cancel — loop exits via channel close

	done := make(chan struct{})
	go func() {
		c.loop(ctx)
		close(done)
	}()

	// Close the listener: its internal goroutine checks isClosed, exits, and
	// calls close(l.Notify) which closes our injected channel.
	// With 1 ms minReconn the goroutine notices within a few ms.
	l.Close() //nolint:errcheck

	select {
	case <-done:
		// loop exited via !ok branch
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not exit after Notify channel was closed")
	}
}

func TestStop_CancelSetButNoListener(t *testing.T) {
	// Cover the "return nil" path in Stop when listener is nil after cancel runs.
	c := New()
	// Set cancel to a no-op so the "if c.cancel != nil" branch is taken.
	c.cancel = func() {}
	// Pre-close done so <-c.done unblocks immediately.
	close(c.done)
	// c.listener is nil — Stop should return nil via the final "return nil".
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop with nil listener: %v", err)
	}
}
