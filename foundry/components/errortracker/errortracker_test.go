package errortracker_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/components/errortracker"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestNew_NotNil(t *testing.T) {
	tr := errortracker.New()
	if tr == nil {
		t.Fatal("New() should not return nil")
	}
}

func TestReportError_Nil(t *testing.T) {
	var buf bytes.Buffer
	tr := errortracker.NewWithLogger(newTestLogger(&buf))
	// nil error must not panic or log anything
	tr.ReportError(context.Background(), nil, nil)
	if buf.Len() != 0 {
		t.Errorf("nil error should not produce log output, got: %s", buf.String())
	}
}

func TestReportError_LogsError(t *testing.T) {
	var buf bytes.Buffer
	tr := errortracker.NewWithLogger(newTestLogger(&buf))
	tr.ReportError(context.Background(), errors.New("something broke"), nil)

	out := buf.String()
	if !strings.Contains(out, "something broke") {
		t.Errorf("expected error message in log, got: %s", out)
	}
}

func TestReportError_IncludesTags(t *testing.T) {
	var buf bytes.Buffer
	tr := errortracker.NewWithLogger(newTestLogger(&buf))
	tr.ReportError(context.Background(), errors.New("db failure"), map[string]string{
		"component": "foundry-postgres",
		"operation": "insert",
	})

	out := buf.String()
	if !strings.Contains(out, "foundry-postgres") {
		t.Errorf("expected component tag in log, got: %s", out)
	}
	if !strings.Contains(out, "insert") {
		t.Errorf("expected operation tag in log, got: %s", out)
	}
}

func TestReportError_NilTags(t *testing.T) {
	var buf bytes.Buffer
	tr := errortracker.NewWithLogger(newTestLogger(&buf))
	// Should not panic with nil tags.
	tr.ReportError(context.Background(), errors.New("oops"), nil)
	if !strings.Contains(buf.String(), "oops") {
		t.Error("expected error in output")
	}
}

func TestFlush_NoOp(t *testing.T) {
	tr := errortracker.New()
	// Flush is a no-op — must not block or panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	tr.Flush(ctx)
}

func TestNewComponent_Metadata(t *testing.T) {
	c := errortracker.NewComponent()
	if c.Name() != "foundry-errortracker" {
		t.Errorf("Name() = %q, want foundry-errortracker", c.Name())
	}
	if c.Version() == "" {
		t.Error("Version() should not be empty")
	}
	if len(c.AuditHash()) != 64 {
		t.Errorf("AuditHash() len = %d, want 64", len(c.AuditHash()))
	}
}

func TestNewComponent_Configure_ValidLevel(t *testing.T) {
	for _, lvl := range []string{"warn", "error", "warning", "WARN", "ERROR"} {
		t.Run(lvl, func(t *testing.T) {
			c := errortracker.NewComponent()
			err := c.Configure(spec.ComponentConfig{"min_level": lvl})
			if err != nil {
				t.Errorf("Configure(%q) = %v, want nil", lvl, err)
			}
		})
	}
}

func TestNewComponent_Configure_InvalidLevel(t *testing.T) {
	c := errortracker.NewComponent()
	err := c.Configure(spec.ComponentConfig{"min_level": "debug"})
	if err == nil {
		t.Error("Configure with invalid min_level should return error")
	}
	if !strings.Contains(err.Error(), "min_level") {
		t.Errorf("error should mention min_level: %v", err)
	}
}

func TestNewComponent_Configure_Empty(t *testing.T) {
	c := errortracker.NewComponent()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Errorf("Configure({}) = %v, want nil", err)
	}
}

func TestNewComponent_Register_InstallsTracker(t *testing.T) {
	c := errortracker.NewComponent()
	_ = c.Configure(spec.ComponentConfig{})
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if c.Tracker() == nil {
		t.Error("Tracker() should not be nil after Register")
	}

	// App should now return the installed tracker, not a NoopErrorTracker.
	tracker := app.ErrorTracker()
	if _, ok := tracker.(spec.NoopErrorTracker); ok {
		t.Error("App should return the installed tracker, not NoopErrorTracker")
	}
}

func TestApplication_ErrorTracker_DefaultIsNoop(t *testing.T) {
	app := spec.NewApplication(nil)
	tracker := app.ErrorTracker()
	if tracker == nil {
		t.Fatal("ErrorTracker() must never return nil")
	}
	if _, ok := tracker.(spec.NoopErrorTracker); !ok {
		t.Errorf("Default tracker should be NoopErrorTracker, got %T", tracker)
	}
}

func TestApplication_SetErrorTracker_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	tr := errortracker.NewWithLogger(newTestLogger(&buf))
	app := spec.NewApplication(nil)
	app.SetErrorTracker(tr)

	app.ErrorTracker().ReportError(context.Background(), errors.New("test error"), map[string]string{
		"op": "test",
	})

	if !strings.Contains(buf.String(), "test error") {
		t.Errorf("installed tracker should receive errors, got: %s", buf.String())
	}
}

func TestConcurrentReportError(t *testing.T) {
	var buf bytes.Buffer
	tr := errortracker.NewWithLogger(slog.New(slog.NewJSONHandler(&buf, nil)))

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(n int) {
			tr.ReportError(context.Background(), errors.New("concurrent error"), map[string]string{
				"goroutine": "yes",
			})
			if n == 49 {
				close(done)
			}
		}(i)
	}
	<-done
}

func TestStartStop_NoError(t *testing.T) {
	c := errortracker.NewComponent()
	_ = c.Configure(spec.ComponentConfig{})
	_ = c.Register(spec.NewApplication(nil))

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Errorf("Start() = %v, want nil", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}
