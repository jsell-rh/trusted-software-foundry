package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/components/logging"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// captureHandler is a slog.Handler that captures log records for inspection.
type captureHandler struct {
	buf   *bytes.Buffer
	inner slog.Handler
}

func newJSONCapture(buf *bytes.Buffer, level slog.Level) *captureHandler {
	opts := &slog.HandlerOptions{Level: level}
	return &captureHandler{buf: buf, inner: slog.NewJSONHandler(buf, opts)}
}

func (h *captureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}
func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}
func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &captureHandler{buf: h.buf, inner: h.inner.WithAttrs(attrs)}
}
func (h *captureHandler) WithGroup(name string) slog.Handler {
	return &captureHandler{buf: h.buf, inner: h.inner.WithGroup(name)}
}

func TestNew_Defaults(t *testing.T) {
	c := logging.New()
	if c.Name() != "foundry-logging" {
		t.Errorf("Name() = %q, want %q", c.Name(), "foundry-logging")
	}
	if c.Version() == "" {
		t.Error("Version() should not be empty")
	}
	if len(c.AuditHash()) != 64 {
		t.Errorf("AuditHash() length = %d, want 64", len(c.AuditHash()))
	}
}

func TestConfigure_ValidLevels(t *testing.T) {
	cases := []struct {
		level string
	}{
		{"debug"}, {"info"}, {"warn"}, {"warning"}, {"error"},
		{"DEBUG"}, {"INFO"}, {"WARN"}, {"ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			c := logging.New()
			err := c.Configure(spec.ComponentConfig{"level": tc.level})
			if err != nil {
				t.Errorf("Configure(%q) error = %v, want nil", tc.level, err)
			}
		})
	}
}

func TestConfigure_InvalidLevel(t *testing.T) {
	c := logging.New()
	err := c.Configure(spec.ComponentConfig{"level": "verbose"})
	if err == nil {
		t.Error("Configure with invalid level should return error")
	}
	if !strings.Contains(err.Error(), "level") {
		t.Errorf("error message should mention 'level', got: %v", err)
	}
}

func TestConfigure_ValidFormats(t *testing.T) {
	for _, fmt := range []string{"json", "text", "JSON", "TEXT"} {
		t.Run(fmt, func(t *testing.T) {
			c := logging.New()
			err := c.Configure(spec.ComponentConfig{"format": fmt})
			if err != nil {
				t.Errorf("Configure(format=%q) error = %v, want nil", fmt, err)
			}
		})
	}
}

func TestConfigure_InvalidFormat(t *testing.T) {
	c := logging.New()
	err := c.Configure(spec.ComponentConfig{"format": "xml"})
	if err == nil {
		t.Error("Configure with invalid format should return error")
	}
	if !strings.Contains(err.Error(), "format") {
		t.Errorf("error message should mention 'format', got: %v", err)
	}
}

func TestConfigure_EmptyConfig(t *testing.T) {
	c := logging.New()
	err := c.Configure(spec.ComponentConfig{})
	if err != nil {
		t.Errorf("Configure with empty config should not error, got: %v", err)
	}
}

func TestRegister_SetsDefaultLogger(t *testing.T) {
	c := logging.New()
	err := c.Configure(spec.ComponentConfig{
		"level":  "debug",
		"format": "json",
		"output": "stdout",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	app := spec.NewApplication(nil)
	err = c.Register(app)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if c.Logger() == nil {
		t.Error("Logger() should not be nil after Register")
	}
}

func TestRegister_InvalidOutput(t *testing.T) {
	c := logging.New()
	_ = c.Configure(spec.ComponentConfig{"output": "/nonexistent/path/to/log.log"})
	app := spec.NewApplication(nil)
	err := c.Register(app)
	if err == nil {
		t.Error("Register with invalid output path should return error")
	}
}

func TestFromContext_ReturnsDefaultWhenNone(t *testing.T) {
	ctx := context.Background()
	l := logging.FromContext(ctx)
	if l == nil {
		t.Error("FromContext with no logger should return non-nil default")
	}
}

func TestWithLogger_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := slog.New(handler)

	ctx := context.Background()
	ctx = logging.WithLogger(ctx, l)

	got := logging.FromContext(ctx)
	if got != l {
		t.Error("FromContext should return the logger set by WithLogger")
	}
}

func TestFromContext_IsolatesContexts(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	l1 := slog.New(slog.NewJSONHandler(&buf1, nil))
	l2 := slog.New(slog.NewJSONHandler(&buf2, nil))

	ctx1 := logging.WithLogger(context.Background(), l1)
	ctx2 := logging.WithLogger(context.Background(), l2)

	if logging.FromContext(ctx1) != l1 {
		t.Error("ctx1 should return l1")
	}
	if logging.FromContext(ctx2) != l2 {
		t.Error("ctx2 should return l2")
	}
}

func TestLogging_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := slog.New(handler)

	l.Info("test message", "key", "value", "count", 42)

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("log output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if record["msg"] != "test message" {
		t.Errorf("msg = %v, want %q", record["msg"], "test message")
	}
	if record["key"] != "value" {
		t.Errorf("key = %v, want %q", record["key"], "value")
	}
}

func TestLogging_LevelFiltering(t *testing.T) {
	cases := []struct {
		configLevel  slog.Level
		logLevel     slog.Level
		shouldAppear bool
	}{
		{slog.LevelInfo, slog.LevelDebug, false},
		{slog.LevelInfo, slog.LevelInfo, true},
		{slog.LevelInfo, slog.LevelWarn, true},
		{slog.LevelInfo, slog.LevelError, true},
		{slog.LevelDebug, slog.LevelDebug, true},
		{slog.LevelError, slog.LevelWarn, false},
		{slog.LevelError, slog.LevelError, true},
	}

	for _, tc := range cases {
		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: tc.configLevel})
		l := slog.New(h)

		l.Log(context.Background(), tc.logLevel, "probe")

		appeared := buf.Len() > 0
		if appeared != tc.shouldAppear {
			t.Errorf("configLevel=%v logLevel=%v: appeared=%v want=%v",
				tc.configLevel, tc.logLevel, appeared, tc.shouldAppear)
		}
	}
}

func TestLogging_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := slog.New(handler)

	l.Warn("something bad", "err", "disk full")

	out := buf.String()
	if !strings.Contains(out, "something bad") {
		t.Errorf("text output should contain message, got: %s", out)
	}
	if !strings.Contains(out, "disk full") {
		t.Errorf("text output should contain field value, got: %s", out)
	}
	// Text format should NOT be valid JSON
	if json.Valid([]byte(strings.TrimSpace(out))) {
		t.Errorf("text format output should not be valid JSON, got: %s", out)
	}
}

func TestLogging_StructuredFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := slog.New(handler)

	// Emit a log record with multiple typed fields (rh-trex field naming conventions).
	l.Info("request completed",
		"method", "GET",
		"path", "/api/dinosaurs",
		"status", 200,
		"latency_ms", 42,
		"account_id", "acc-123",
		"op_id", "op-456",
	)

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	expected := map[string]any{
		"method":     "GET",
		"path":       "/api/dinosaurs",
		"account_id": "acc-123",
		"op_id":      "op-456",
	}
	for k, v := range expected {
		if record[k] != v {
			t.Errorf("field %q = %v, want %v", k, record[k], v)
		}
	}
}

func TestStartStop_NoError(t *testing.T) {
	c := logging.New()
	_ = c.Configure(spec.ComponentConfig{"output": "stdout"})
	app := spec.NewApplication(nil)
	_ = c.Register(app)

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Errorf("Start() = %v, want nil", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}
