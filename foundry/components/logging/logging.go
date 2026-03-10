// Package logging provides the foundry-logging trusted component — structured
// JSON/text logging built on stdlib log/slog (Go 1.21+).
//
// Configuration (spec observability.logging block):
//
//	observability:
//	  logging:
//	    level: "info"          # debug|info|warn|error (default: info)
//	    format: "json"         # json|text (default: json)
//	    output: "stdout"       # stdout|stderr|file path (default: stdout)
//
// Usage by other components:
//
//	logger := logging.FromContext(ctx)
//	logger.Info("starting", "component", "foundry-http", "port", 8080)
//	logger.Error("failed", "err", err, "op", "db.query")
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-logging"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000010"
)

// loggerKeyType is an unexported key type for context storage.
type loggerKeyType struct{}

// loggerKey is the context key for the slog.Logger instance.
var loggerKey = loggerKeyType{}

// FromContext retrieves the logger stored in ctx. If no logger is present,
// the default slog logger is returned.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// WithLogger returns a copy of ctx with the logger attached.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// LoggingComponent implements spec.Component for structured logging.
type LoggingComponent struct {
	cfg    config
	logger *slog.Logger
}

type config struct {
	level  slog.Level
	format string // "json" | "text"
	output string // "stdout" | "stderr" | file path
}

// New returns a LoggingComponent with sensible defaults.
func New() *LoggingComponent {
	return &LoggingComponent{
		cfg: config{
			level:  slog.LevelInfo,
			format: "json",
			output: "stdout",
		},
	}
}

func (c *LoggingComponent) Name() string      { return componentName }
func (c *LoggingComponent) Version() string   { return componentVersion }
func (c *LoggingComponent) AuditHash() string { return auditHash }

// Configure reads the observability.logging section from the IR spec.
func (c *LoggingComponent) Configure(cfg spec.ComponentConfig) error {
	if lvl, ok := cfg["level"].(string); ok && lvl != "" {
		parsed, err := parseLevel(lvl)
		if err != nil {
			return err
		}
		c.cfg.level = parsed
	}
	if fmt, ok := cfg["format"].(string); ok && fmt != "" {
		lower := strings.ToLower(fmt)
		if lower != "json" && lower != "text" {
			return &configError{field: "format", value: fmt, msg: "must be 'json' or 'text'"}
		}
		c.cfg.format = lower
	}
	if out, ok := cfg["output"].(string); ok && out != "" {
		c.cfg.output = out
	}
	return nil
}

// Register installs the logging component as the process-wide default logger.
func (c *LoggingComponent) Register(app *spec.Application) error {
	w, err := openOutput(c.cfg.output)
	if err != nil {
		return err
	}

	opts := &slog.HandlerOptions{Level: c.cfg.level}
	var handler slog.Handler
	if c.cfg.format == "text" {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	c.logger = slog.New(handler)
	slog.SetDefault(c.logger)
	return nil
}

// Start is a no-op — the logger is already active after Register.
func (c *LoggingComponent) Start(_ context.Context) error { return nil }

// Stop is a no-op — slog has no resources to release.
func (c *LoggingComponent) Stop(_ context.Context) error { return nil }

// Logger returns the configured slog.Logger. Returns nil before Register.
func (c *LoggingComponent) Logger() *slog.Logger { return c.logger }

// --- helpers ---

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, &configError{field: "level", value: s, msg: "must be debug|info|warn|error"}
	}
}

func openOutput(output string) (io.Writer, error) {
	switch strings.ToLower(output) {
	case "stdout", "":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	default:
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, &configError{field: "output", value: output, msg: "cannot open file: " + err.Error()}
		}
		return f, nil
	}
}

// configError is a structured configuration validation error.
type configError struct {
	field string
	value string
	msg   string
}

func (e *configError) Error() string {
	return "foundry-logging: config field '" + e.field + "' value '" + e.value + "': " + e.msg
}
