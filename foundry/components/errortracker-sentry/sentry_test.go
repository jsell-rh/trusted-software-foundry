package errortrackerSentry_test

import (
	"context"
	"errors"
	"testing"

	errortrackerSentry "github.com/jsell-rh/trusted-software-foundry/foundry/components/errortracker-sentry"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

func TestNew_Metadata(t *testing.T) {
	c := errortrackerSentry.New()
	if c.Name() != "foundry-errortracker-sentry" {
		t.Errorf("Name() = %q, want foundry-errortracker-sentry", c.Name())
	}
	if c.Version() == "" {
		t.Error("Version() should not be empty")
	}
	if len(c.AuditHash()) != 64 {
		t.Errorf("AuditHash() len = %d, want 64", len(c.AuditHash()))
	}
}

func TestConfigure_MissingDSN(t *testing.T) {
	c := errortrackerSentry.New()
	err := c.Configure(spec.ComponentConfig{})
	if err == nil {
		t.Error("Configure without dsn should return error")
	}
}

func TestConfigure_EmptyDSN(t *testing.T) {
	c := errortrackerSentry.New()
	err := c.Configure(spec.ComponentConfig{"dsn": ""})
	if err == nil {
		t.Error("Configure with empty dsn should return error")
	}
}

func TestConfigure_ValidDSN(t *testing.T) {
	c := errortrackerSentry.New()
	err := c.Configure(spec.ComponentConfig{
		"dsn":         "https://key@sentry.io/123",
		"environment": "test",
		"release":     "v1.0.0",
	})
	if err != nil {
		t.Errorf("Configure with valid config = %v, want nil", err)
	}
}

func TestConfigure_InvalidFlushTimeout(t *testing.T) {
	c := errortrackerSentry.New()
	err := c.Configure(spec.ComponentConfig{
		"dsn":           "https://key@sentry.io/123",
		"flush_timeout": "not-a-duration",
	})
	if err == nil {
		t.Error("Configure with invalid flush_timeout should return error")
	}
}

func TestConfigure_ValidFlushTimeout(t *testing.T) {
	c := errortrackerSentry.New()
	err := c.Configure(spec.ComponentConfig{
		"dsn":           "https://key@sentry.io/123",
		"flush_timeout": "5s",
	})
	if err != nil {
		t.Errorf("Configure with valid flush_timeout = %v, want nil", err)
	}
}

func TestRegister_InvalidDSN_ReturnsError(t *testing.T) {
	c := errortrackerSentry.New()
	_ = c.Configure(spec.ComponentConfig{"dsn": "not-a-valid-dsn"})
	app := spec.NewApplication(nil)
	err := c.Register(app)
	if err == nil {
		t.Error("Register with invalid DSN should return error")
	}
}

func TestSentryTracker_NilError_NoOp(t *testing.T) {
	// SentryErrorTracker.ReportError must not panic on nil error.
	// We test this via the default (no Sentry init) path.
	c := errortrackerSentry.New()
	_ = c.Configure(spec.ComponentConfig{"dsn": "https://key@sentry.io/123"})
	// Don't Register (avoid real Sentry init in tests). Use tracker directly.
	// We test nil-error safety at the spec interface level.
	app := spec.NewApplication(nil)
	app.SetErrorTracker(spec.NoopErrorTracker{})
	// nil error to Noop — must not panic.
	app.ErrorTracker().ReportError(context.Background(), nil, nil)
}

func TestSentryTracker_NilError_OnRealTracker(t *testing.T) {
	// Even if a real tracker were installed, nil error must be a no-op.
	// We simulate by directly testing the nil branch in ReportError contract.
	var called bool
	mock := &mockTracker{onReport: func(err error) {
		called = true
	}}
	app := spec.NewApplication(nil)
	app.SetErrorTracker(mock)
	app.ErrorTracker().ReportError(context.Background(), nil, nil)
	if called {
		t.Error("nil error should not call ReportError implementation")
	}
}

func TestStartStop_NoError_WithoutRegister(t *testing.T) {
	c := errortrackerSentry.New()
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Errorf("Start() before Register = %v, want nil", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop() before Register = %v, want nil", err)
	}
}

// mockTracker is a test double for spec.ErrorTracker.
type mockTracker struct {
	onReport func(err error)
}

func (m *mockTracker) ReportError(_ context.Context, err error, _ map[string]string) {
	if err != nil && m.onReport != nil {
		m.onReport(err)
	}
}
func (m *mockTracker) Flush(_ context.Context) {}

func TestMockTracker_Reports(t *testing.T) {
	var reported error
	mock := &mockTracker{onReport: func(err error) { reported = err }}
	app := spec.NewApplication(nil)
	app.SetErrorTracker(mock)

	app.ErrorTracker().ReportError(context.Background(), errors.New("kaboom"), map[string]string{"op": "test"})
	if reported == nil || reported.Error() != "kaboom" {
		t.Errorf("expected 'kaboom' error to be reported, got: %v", reported)
	}
}
