package metrics_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/foundry/components/metrics"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// freshRegistry returns a new registry for test isolation.
func freshRegistry() *metrics.Registry {
	return metrics.NewRegistry()
}

func TestNew_Metadata(t *testing.T) {
	c := metrics.New()
	if c.Name() != "foundry-metrics" {
		t.Errorf("Name() = %q, want foundry-metrics", c.Name())
	}
	if c.Version() == "" {
		t.Error("Version() should not be empty")
	}
	if len(c.AuditHash()) != 64 {
		t.Errorf("AuditHash() len = %d, want 64", len(c.AuditHash()))
	}
}

func TestConfigure_Defaults(t *testing.T) {
	c := metrics.New()
	err := c.Configure(spec.ComponentConfig{})
	if err != nil {
		t.Errorf("Configure({}) = %v, want nil", err)
	}
}

func TestConfigure_CustomPaths(t *testing.T) {
	c := metrics.New()
	err := c.Configure(spec.ComponentConfig{
		"bind":        ":9191",
		"path":        "/prom",
		"health_path": "/live",
		"ready_path":  "/ready",
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
}

func TestRegistry_Counter(t *testing.T) {
	r := freshRegistry()
	r.IncCounter("my_requests_total", map[string]string{"method": "GET"})
	r.IncCounter("my_requests_total", map[string]string{"method": "GET"})
	r.IncCounter("my_requests_total", map[string]string{"method": "POST"})

	text := r.Text()
	if !strings.Contains(text, "my_requests_total") {
		t.Errorf("expected my_requests_total in output:\n%s", text)
	}
	if !strings.Contains(text, `method="GET"`) {
		t.Errorf("expected GET label in output:\n%s", text)
	}
}

func TestRegistry_CounterAccumulates(t *testing.T) {
	r := freshRegistry()
	for i := 0; i < 5; i++ {
		r.IncCounter("ops_total", nil)
	}
	text := r.Text()
	if !strings.Contains(text, "ops_total 5") {
		t.Errorf("expected counter value 5 in output:\n%s", text)
	}
}

func TestRegistry_Histogram(t *testing.T) {
	r := freshRegistry()
	r.ObserveLatency("http_latency_ms", map[string]string{"handler": "api"}, 10.0)
	r.ObserveLatency("http_latency_ms", map[string]string{"handler": "api"}, 50.0)
	r.ObserveLatency("http_latency_ms", map[string]string{"handler": "api"}, 500.0)

	text := r.Text()
	if !strings.Contains(text, "http_latency_ms_bucket") {
		t.Errorf("expected histogram buckets in output:\n%s", text)
	}
	if !strings.Contains(text, "http_latency_ms_sum") {
		t.Errorf("expected histogram sum in output:\n%s", text)
	}
	if !strings.Contains(text, "http_latency_ms_count") {
		t.Errorf("expected histogram count in output:\n%s", text)
	}
	if !strings.Contains(text, `le="+Inf"`) {
		t.Errorf("expected +Inf bucket in output:\n%s", text)
	}
}

func TestRegistry_HistogramCount(t *testing.T) {
	r := freshRegistry()
	for i := 0; i < 7; i++ {
		r.ObserveLatency("latency_ms", nil, float64(i*10))
	}
	text := r.Text()
	if !strings.Contains(text, "latency_ms_count 7") {
		t.Errorf("expected count=7 in output:\n%s", text)
	}
}

func TestRegistry_PrometheusTextFormat(t *testing.T) {
	r := freshRegistry()
	r.IncCounter("test_counter", map[string]string{"label": "val"})
	text := r.Text()

	// Each metric family must have a TYPE line.
	if !strings.Contains(text, "# TYPE test_counter counter") {
		t.Errorf("expected TYPE comment, got:\n%s", text)
	}
}

func TestRegistry_MultipleMetrics(t *testing.T) {
	r := freshRegistry()
	r.IncCounter("alpha_total", nil)
	r.IncCounter("beta_total", nil)
	r.ObserveLatency("gamma_ms", nil, 42.0)

	text := r.Text()
	for _, name := range []string{"alpha_total", "beta_total", "gamma_ms"} {
		if !strings.Contains(text, name) {
			t.Errorf("expected %q in output:\n%s", name, text)
		}
	}
}

func TestInstrumentedHandler_RecordsMetrics(t *testing.T) {
	r := freshRegistry()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := metrics.InstrumentedHandlerWithRegistry(r, "test_endpoint", inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rw := httptest.NewRecorder()
	wrapped.ServeHTTP(rw, req)

	text := r.Text()
	if !strings.Contains(text, "foundry_http_requests_total") {
		t.Errorf("expected request counter after request:\n%s", text)
	}
}

func TestInstrumentedHandler_TracksErrors(t *testing.T) {
	r := freshRegistry()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	wrapped := metrics.InstrumentedHandlerWithRegistry(r, "failing_endpoint", inner)
	req := httptest.NewRequest(http.MethodPost, "/fail", nil)
	rw := httptest.NewRecorder()
	wrapped.ServeHTTP(rw, req)

	text := r.Text()
	if !strings.Contains(text, `is_error="true"`) {
		t.Errorf("expected is_error=true label for 500 response:\n%s", text)
	}
}

func TestInstrumentedHandler_TracksLatency(t *testing.T) {
	r := freshRegistry()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	wrapped := metrics.InstrumentedHandlerWithRegistry(r, "slow_endpoint", inner)
	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rw := httptest.NewRecorder()
	wrapped.ServeHTTP(rw, req)

	text := r.Text()
	if !strings.Contains(text, "foundry_http_request_duration_ms_sum") {
		t.Errorf("expected latency histogram sum in output:\n%s", text)
	}
}

func TestMetricsServer_LivenessEndpoint(t *testing.T) {
	c := metrics.New()
	_ = c.Configure(spec.ComponentConfig{
		"bind":        ":0",
		"health_path": "/healthz",
	})
	_ = c.Register(spec.NewApplication(nil))

	// Test the handler directly without starting a server.
	mux := metrics.BuildMux(c)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("/healthz body = %q, want status:ok", body)
	}
}

func TestMetricsServer_ReadinessEndpoint_AllPassing(t *testing.T) {
	c := metrics.New()
	_ = c.Configure(spec.ComponentConfig{"ready_path": "/readyz"})
	c.AddReadinessCheck(func() error { return nil })
	c.AddReadinessCheck(func() error { return nil })
	_ = c.Register(spec.NewApplication(nil))

	mux := metrics.BuildMux(c)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("/readyz status = %d, want 200", rw.Code)
	}
}

func TestMetricsServer_ReadinessEndpoint_Failing(t *testing.T) {
	c := metrics.New()
	_ = c.Configure(spec.ComponentConfig{"ready_path": "/readyz"})
	c.AddReadinessCheck(func() error { return fmt.Errorf("db not ready") })
	_ = c.Register(spec.NewApplication(nil))

	mux := metrics.BuildMux(c)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	if rw.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz status = %d, want 503", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "db not ready") {
		t.Errorf("/readyz body = %q should contain error", body)
	}
}

func TestMetricsServer_MetricsEndpoint(t *testing.T) {
	r := freshRegistry()
	r.IncCounter("probe_counter", nil)

	c := metrics.NewWithRegistry(r)
	_ = c.Configure(spec.ComponentConfig{"path": "/metrics"})
	_ = c.Register(spec.NewApplication(nil))

	mux := metrics.BuildMux(c)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("/metrics status = %d, want 200", rw.Code)
	}
	ct := rw.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}

	body, _ := io.ReadAll(rw.Body)
	if !strings.Contains(string(body), "probe_counter") {
		t.Errorf("/metrics body should contain probe_counter:\n%s", string(body))
	}
}

func TestRegistry_LabelCardinality(t *testing.T) {
	r := freshRegistry()
	// Simulate high-cardinality label usage (e.g. per-endpoint).
	for i := 0; i < 10; i++ {
		r.IncCounter("endpoint_hits_total", map[string]string{
			"endpoint": fmt.Sprintf("/api/resource/%d", i),
		})
	}
	text := r.Text()
	if !strings.Contains(text, "endpoint_hits_total") {
		t.Errorf("expected endpoint_hits_total in output:\n%s", text)
	}
}
