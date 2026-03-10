// Package errortrackerSentry provides foundry-errortracker-sentry — an optional
// Sentry integration for error tracking. This is the ONLY component in the TSF
// platform that is permitted to import sentry-go.
//
// The platform core (foundry/spec, foundry/compiler, and all other components)
// remains stdlib-only. Teams that want Sentry integration add this component
// alongside foundry-errortracker.
//
// Configuration (spec observability.error_tracking block):
//
//	observability:
//	  error_tracking:
//	    dsn: "https://key@sentry.io/project"  # required
//	    environment: "production"              # default: "production"
//	    release: "v1.2.3"                     # optional
//	    flush_timeout: 2s                      # default: 2s
//	    traces_sample_rate: 0.1               # default: 0.0 (disabled)
package errortrackerSentry

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-errortracker-sentry"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000012"

	defaultEnvironment  = "production"
	defaultFlushTimeout = 2 * time.Second
)

// SentryErrorTracker implements spec.ErrorTracker using the Sentry SDK.
type SentryErrorTracker struct {
	flushTimeout time.Duration
}

// ReportError captures an error event to Sentry with structured tags.
func (t *SentryErrorTracker) ReportError(ctx context.Context, err error, tags map[string]string) {
	if err == nil {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		if len(tags) > 0 {
			// Sort for deterministic ordering.
			keys := make([]string, 0, len(tags))
			for k := range tags {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				scope.SetTag(k, tags[k])
			}
		}
		hub.CaptureException(err)
	})
}

// Flush blocks until all buffered events are delivered or ctx is cancelled.
func (t *SentryErrorTracker) Flush(ctx context.Context) {
	deadline := t.flushTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining < deadline {
			deadline = remaining
		}
	}
	sentry.Flush(deadline)
}

// SentryComponent implements spec.Component to install Sentry error tracking.
type SentryComponent struct {
	cfg     sentryConfig
	tracker *SentryErrorTracker
}

type sentryConfig struct {
	dsn              string
	environment      string
	release          string
	flushTimeout     time.Duration
	tracesSampleRate float64
}

// New returns a SentryComponent with defaults.
func New() *SentryComponent {
	return &SentryComponent{
		cfg: sentryConfig{
			environment:  defaultEnvironment,
			flushTimeout: defaultFlushTimeout,
		},
	}
}

func (c *SentryComponent) Name() string      { return componentName }
func (c *SentryComponent) Version() string   { return componentVersion }
func (c *SentryComponent) AuditHash() string { return auditHash }

// Configure reads the observability.error_tracking section.
func (c *SentryComponent) Configure(cfg spec.ComponentConfig) error {
	dsn, ok := cfg["dsn"].(string)
	if !ok || dsn == "" {
		return fmt.Errorf("foundry-errortracker-sentry: 'dsn' is required")
	}
	c.cfg.dsn = dsn

	if env, ok := cfg["environment"].(string); ok && env != "" {
		c.cfg.environment = env
	}
	if rel, ok := cfg["release"].(string); ok {
		c.cfg.release = rel
	}
	if rate, ok := cfg["traces_sample_rate"].(float64); ok {
		c.cfg.tracesSampleRate = rate
	}
	if ft, ok := cfg["flush_timeout"].(string); ok && ft != "" {
		d, err := time.ParseDuration(ft)
		if err != nil {
			return fmt.Errorf("foundry-errortracker-sentry: flush_timeout: %w", err)
		}
		c.cfg.flushTimeout = d
	}
	return nil
}

// Register initialises the Sentry SDK and installs the SentryErrorTracker.
func (c *SentryComponent) Register(app *spec.Application) error {
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              c.cfg.dsn,
		Environment:      c.cfg.environment,
		Release:          c.cfg.release,
		TracesSampleRate: c.cfg.tracesSampleRate,
		// Attach stack traces to all events for richer debugging.
		AttachStacktrace: true,
	})
	if err != nil {
		return fmt.Errorf("foundry-errortracker-sentry: sentry.Init: %w", err)
	}

	c.tracker = &SentryErrorTracker{flushTimeout: c.cfg.flushTimeout}
	app.SetErrorTracker(c.tracker)
	return nil
}

func (c *SentryComponent) Start(_ context.Context) error { return nil }

// Stop flushes pending events before shutting down.
func (c *SentryComponent) Stop(ctx context.Context) error {
	if c.tracker != nil {
		c.tracker.Flush(ctx)
	}
	return nil
}

// Tracker returns the underlying SentryErrorTracker. Returns nil before Register.
func (c *SentryComponent) Tracker() *SentryErrorTracker { return c.tracker }
