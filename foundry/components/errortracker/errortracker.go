// Package errortracker provides foundry-errortracker — the default error
// tracking component that logs errors to stderr using structured slog output.
//
// It implements spec.ErrorTracker and is installed by calling
// app.SetErrorTracker(errortracker.New()). Teams that want to integrate Sentry
// or another external service should use the foundry-errortracker-sentry
// component instead, which is an optional add-on with external dependencies.
//
// Configuration (spec observability.error_tracking block):
//
//	observability:
//	  error_tracking:
//	    min_level: "error"   # minimum severity to report: "warn" or "error" (default: "error")
//	    include_tags: true   # include structured tags in log output (default: true)
package errortracker

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-errortracker"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000011"
)

// LogErrorTracker logs errors to the process-wide slog logger.
// It satisfies spec.ErrorTracker and is safe for concurrent use.
type LogErrorTracker struct {
	mu          sync.RWMutex
	logger      *slog.Logger
	includeTags bool
}

// New returns a LogErrorTracker using the default slog logger.
func New() *LogErrorTracker {
	return &LogErrorTracker{
		logger:      slog.Default(),
		includeTags: true,
	}
}

// NewWithLogger returns a LogErrorTracker using the provided logger.
func NewWithLogger(l *slog.Logger) *LogErrorTracker {
	return &LogErrorTracker{logger: l, includeTags: true}
}

// ReportError logs the error at ERROR level with all provided tags as slog attributes.
// If err is nil, this is a no-op.
func (t *LogErrorTracker) ReportError(ctx context.Context, err error, tags map[string]string) {
	if err == nil {
		return
	}
	t.mu.RLock()
	l := t.logger
	includeTags := t.includeTags
	t.mu.RUnlock()

	args := []any{"error", err.Error()}
	if includeTags && len(tags) > 0 {
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, k, tags[k])
		}
	}
	l.ErrorContext(ctx, "error reported", args...)
}

// Flush is a no-op for the log tracker — slog writes synchronously.
func (t *LogErrorTracker) Flush(_ context.Context) {}

// ErrorTrackerComponent implements spec.Component to install the log tracker.
type ErrorTrackerComponent struct {
	cfg     etConfig
	tracker *LogErrorTracker
}

type etConfig struct {
	minLevel    string
	includeTags bool
}

// NewComponent returns an ErrorTrackerComponent with defaults.
func NewComponent() *ErrorTrackerComponent {
	return &ErrorTrackerComponent{
		cfg: etConfig{minLevel: "error", includeTags: true},
	}
}

func (c *ErrorTrackerComponent) Name() string      { return componentName }
func (c *ErrorTrackerComponent) Version() string   { return componentVersion }
func (c *ErrorTrackerComponent) AuditHash() string { return auditHash }

// Configure reads the observability.error_tracking section.
func (c *ErrorTrackerComponent) Configure(cfg spec.ComponentConfig) error {
	if lvl, ok := cfg["min_level"].(string); ok && lvl != "" {
		lower := strings.ToLower(lvl)
		if lower != "warn" && lower != "warning" && lower != "error" {
			return fmt.Errorf("foundry-errortracker: min_level must be 'warn' or 'error', got %q", lvl)
		}
		c.cfg.minLevel = lower
	}
	if it, ok := cfg["include_tags"].(bool); ok {
		c.cfg.includeTags = it
	}
	return nil
}

// Register installs the LogErrorTracker as the application error tracker.
func (c *ErrorTrackerComponent) Register(app *spec.Application) error {
	c.tracker = &LogErrorTracker{
		logger:      slog.Default(),
		includeTags: c.cfg.includeTags,
	}
	app.SetErrorTracker(c.tracker)
	return nil
}

func (c *ErrorTrackerComponent) Start(_ context.Context) error { return nil }
func (c *ErrorTrackerComponent) Stop(_ context.Context) error  { return nil }

// Tracker returns the underlying LogErrorTracker. Returns nil before Register.
func (c *ErrorTrackerComponent) Tracker() *LogErrorTracker { return c.tracker }
