// Package metrics provides the foundry-metrics trusted component — a
// Prometheus-compatible metrics endpoint and health probes, implemented
// using stdlib only (no prometheus/client_golang dependency).
//
// Prometheus text format: https://prometheus.io/docs/instrumenting/exposition_formats/
//
// Configuration (spec observability.metrics block):
//
//	observability:
//	  metrics:
//	    bind: ":8080"          # listen address (default :8080)
//	    path: /metrics         # metrics path (default /metrics)
//	    health_path: /healthz  # liveness probe (default /healthz)
//	    ready_path: /readyz    # readiness probe (default /readyz)
package metrics

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-metrics"
	componentVersion = "v1.1.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000003"

	defaultBind       = ":8080"
	defaultPath       = "/metrics"
	defaultHealthPath = "/healthz"
	defaultReadyPath  = "/readyz"
)

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// Registry holds all metrics. It is safe for concurrent use.
type Registry struct {
	mu       sync.Mutex
	counters map[string]*counter
	histos   map[string]*histogram
}

// NewRegistry returns an empty, independent registry.
// Use this in tests to avoid polluting the global registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]*counter),
		histos:   make(map[string]*histogram),
	}
}

var globalRegistry = NewRegistry()

// Global returns the process-wide metrics registry.
func Global() *Registry { return globalRegistry }

// counter is an atomic monotonic counter.
type counter struct {
	labels map[string]string
	value  atomic.Int64
}

// histogram tracks latency distributions using fixed upper-bound buckets (ms).
type histogram struct {
	mu      sync.Mutex
	labels  map[string]string
	buckets []float64 // upper bounds in ms
	counts  []int64   // cumulative count per bucket
	sum     float64   // sum of all observations in ms
	total   int64     // total observation count
}

var defaultBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

// IncCounter increments a counter metric by 1.
func (r *Registry) IncCounter(name string, labels map[string]string) {
	key := metricKey(name, labels)
	r.mu.Lock()
	c, ok := r.counters[key]
	if !ok {
		c = &counter{labels: copyLabels(labels)}
		r.counters[key] = c
	}
	r.mu.Unlock()
	c.value.Add(1)
}

// ObserveLatency records a latency observation in milliseconds.
func (r *Registry) ObserveLatency(name string, labels map[string]string, ms float64) {
	key := metricKey(name, labels)
	r.mu.Lock()
	h, ok := r.histos[key]
	if !ok {
		buckets := defaultBuckets
		h = &histogram{
			labels:  copyLabels(labels),
			buckets: buckets,
			counts:  make([]int64, len(buckets)),
		}
		r.histos[key] = h
	}
	r.mu.Unlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += ms
	h.total++
	for i, b := range h.buckets {
		if ms <= b {
			h.counts[i]++
		}
	}
}

// Text renders all metrics in Prometheus text exposition format.
func (r *Registry) Text() string {
	r.mu.Lock()
	counterKeys := make([]string, 0, len(r.counters))
	for k := range r.counters {
		counterKeys = append(counterKeys, k)
	}
	sort.Strings(counterKeys)
	countersSnap := make(map[string]*counter, len(r.counters))
	for k, c := range r.counters {
		countersSnap[k] = c
	}

	histoKeys := make([]string, 0, len(r.histos))
	for k := range r.histos {
		histoKeys = append(histoKeys, k)
	}
	sort.Strings(histoKeys)
	histosSnap := make(map[string]*histogram, len(r.histos))
	for k, h := range r.histos {
		histosSnap[k] = h
	}
	r.mu.Unlock()

	var sb strings.Builder

	for _, key := range counterKeys {
		c := countersSnap[key]
		name := extractName(key)
		fmt.Fprintf(&sb, "# TYPE %s counter\n", name)
		fmt.Fprintf(&sb, "%s%s %d\n", name, labelStr(c.labels), c.value.Load())
	}

	for _, key := range histoKeys {
		h := histosSnap[key]
		h.mu.Lock()
		name := extractName(key)
		fmt.Fprintf(&sb, "# TYPE %s histogram\n", name)
		var cumCount int64
		for i, b := range h.buckets {
			cumCount += h.counts[i]
			lbls := mergeLabel(h.labels, "le", formatFloat(b))
			fmt.Fprintf(&sb, "%s_bucket%s %d\n", name, labelStr(lbls), cumCount)
		}
		infLbls := mergeLabel(h.labels, "le", "+Inf")
		fmt.Fprintf(&sb, "%s_bucket%s %d\n", name, labelStr(infLbls), h.total)
		fmt.Fprintf(&sb, "%s_sum%s %s\n", name, labelStr(h.labels), formatFloat(h.sum))
		fmt.Fprintf(&sb, "%s_count%s %d\n", name, labelStr(h.labels), h.total)
		h.mu.Unlock()
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// Instrumented HTTP handler
// ---------------------------------------------------------------------------

// InstrumentedHandler wraps an http.Handler to record request count and latency
// using the global registry.
func InstrumentedHandler(name string, next http.Handler) http.Handler {
	return InstrumentedHandlerWithRegistry(globalRegistry, name, next)
}

// InstrumentedHandlerWithRegistry wraps an http.Handler using a specific registry.
// Use this in tests to record metrics into an isolated registry.
func InstrumentedHandlerWithRegistry(r *Registry, name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, req)
		ms := float64(time.Since(start).Microseconds()) / 1000.0

		status := fmt.Sprintf("%d", rw.status)
		isErr := "false"
		if rw.status >= 400 {
			isErr = "true"
		}
		reqLabels := map[string]string{"handler": name, "method": req.Method, "status": status}
		r.IncCounter("foundry_http_requests_total", reqLabels)
		r.ObserveLatency("foundry_http_request_duration_ms", reqLabels, ms)
		errLabels := map[string]string{"handler": name, "is_error": isErr}
		r.IncCounter("foundry_http_errors_total", errLabels)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

// MetricsComponent implements spec.Component for metrics + health serving.
type MetricsComponent struct {
	mu         sync.Mutex
	cfg        config
	registry   *Registry
	server     *http.Server
	readyFuncs []func() error
}

type config struct {
	bind       string
	path       string
	healthPath string
	readyPath  string
}

// New returns a MetricsComponent backed by the global registry.
func New() *MetricsComponent {
	return NewWithRegistry(globalRegistry)
}

// NewWithRegistry returns a MetricsComponent backed by the given registry.
// Use this in tests to isolate metrics.
func NewWithRegistry(r *Registry) *MetricsComponent {
	return &MetricsComponent{
		registry: r,
		cfg: config{
			bind:       defaultBind,
			path:       defaultPath,
			healthPath: defaultHealthPath,
			readyPath:  defaultReadyPath,
		},
	}
}

func (c *MetricsComponent) Name() string      { return componentName }
func (c *MetricsComponent) Version() string   { return componentVersion }
func (c *MetricsComponent) AuditHash() string { return auditHash }

// AddReadinessCheck registers a function called on /readyz.
// If any check returns a non-nil error, the probe returns HTTP 503.
func (c *MetricsComponent) AddReadinessCheck(fn func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readyFuncs = append(c.readyFuncs, fn)
}

// Configure reads the observability.metrics section.
func (c *MetricsComponent) Configure(cfg spec.ComponentConfig) error {
	if bind, ok := cfg["bind"].(string); ok && bind != "" {
		c.cfg.bind = bind
	}
	if path, ok := cfg["path"].(string); ok && path != "" {
		c.cfg.path = path
	}
	if hp, ok := cfg["health_path"].(string); ok && hp != "" {
		c.cfg.healthPath = hp
	}
	if rp, ok := cfg["ready_path"].(string); ok && rp != "" {
		c.cfg.readyPath = rp
	}
	return nil
}

// Register is a no-op — foundry-metrics manages its own server independently.
func (c *MetricsComponent) Register(_ *spec.Application) error { return nil }

// BuildMux constructs the HTTP mux for this component without starting a server.
// Exposed for testing.
func BuildMux(c *MetricsComponent) *http.ServeMux {
	return c.buildMux()
}

func (c *MetricsComponent) buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Prometheus text exposition endpoint.
	mux.HandleFunc(c.cfg.path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, c.registry.Text())
	})

	// Liveness probe — always 200 if the server is running.
	mux.HandleFunc(c.cfg.healthPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// Readiness probe — runs all registered checks.
	mux.HandleFunc(c.cfg.readyPath, func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		checks := make([]func() error, len(c.readyFuncs))
		copy(checks, c.readyFuncs)
		c.mu.Unlock()

		var errs []string
		for _, fn := range checks {
			if err := fn(); err != nil {
				errs = append(errs, err.Error())
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if len(errs) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not ready","errors":["%s"]}`, strings.Join(errs, `","`))
			return
		}
		fmt.Fprint(w, `{"status":"ready"}`)
	})

	return mux
}

// Start launches the metrics+health HTTP server.
func (c *MetricsComponent) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.server = &http.Server{
		Addr:         c.cfg.bind,
		Handler:      c.buildMux(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("foundry-metrics: listen %s: %w", c.cfg.bind, err)
	default:
	}
	return nil
}

// Stop gracefully shuts down the metrics server.
func (c *MetricsComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	srv := c.server
	c.mu.Unlock()
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("foundry-metrics: shutdown: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

func metricKey(name string, labels map[string]string) string {
	return name + labelStr(labels)
}

func extractName(key string) string {
	idx := strings.Index(key, "{")
	if idx == -1 {
		return key
	}
	return key[:idx]
}

func labelStr(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteString(`="`)
		sb.WriteString(labels[k])
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	return sb.String()
}

func mergeLabel(base map[string]string, k, v string) map[string]string {
	m := make(map[string]string, len(base)+1)
	for key, val := range base {
		m[key] = val
	}
	m[k] = v
	return m
}

func copyLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	m := make(map[string]string, len(labels))
	for k, v := range labels {
		m[k] = v
	}
	return m
}

func formatFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "+Inf"
	}
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.1f", f)
	}
	return fmt.Sprintf("%g", f)
}
