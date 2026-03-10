package age

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --- Component interface tests ---

func TestComponent_ImplementsSpecComponent(t *testing.T) {
	var _ spec.Component = New()
}

func TestComponent_Identity(t *testing.T) {
	c := New()
	if c.Name() != "foundry-graph-age" {
		t.Errorf("Name() = %q, want foundry-graph-age", c.Name())
	}
	if c.Version() != "v1.0.0" {
		t.Errorf("Version() = %q, want v1.0.0", c.Version())
	}
	if c.AuditHash() == "" {
		t.Error("AuditHash() must not be empty")
	}
}

func TestComponent_Configure_Defaults(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure with empty config: %v", err)
	}
	if c.cfg.graphName != "foundry_graph" {
		t.Errorf("default graphName = %q, want foundry_graph", c.cfg.graphName)
	}
	if c.cfg.maxDepth != 10 {
		t.Errorf("default maxDepth = %d, want 10", c.cfg.maxDepth)
	}
	if c.cfg.exposeAPI {
		t.Error("exposeAPI should default to false")
	}
	if c.cfg.bulkLoading {
		t.Error("bulkLoading should default to false")
	}
}

func TestComponent_Configure_Custom(t *testing.T) {
	c := New()
	err := c.Configure(spec.ComponentConfig{
		"graph_name":   "test_graph",
		"max_depth":    5,
		"expose_api":   true,
		"bulk_loading": true,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.cfg.graphName != "test_graph" {
		t.Errorf("graphName = %q, want test_graph", c.cfg.graphName)
	}
	if c.cfg.maxDepth != 5 {
		t.Errorf("maxDepth = %d, want 5", c.cfg.maxDepth)
	}
	if !c.cfg.exposeAPI {
		t.Error("exposeAPI should be true")
	}
	if !c.cfg.bulkLoading {
		t.Error("bulkLoading should be true")
	}
}

func TestComponent_Register_MountsHandlers_WhenEnabled(t *testing.T) {
	c := New()
	c.Configure(spec.ComponentConfig{"expose_api": true, "bulk_loading": true})

	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	handlers := app.HTTPHandlers()
	patterns := make(map[string]bool)
	for _, h := range handlers {
		patterns[h.Pattern] = true
	}

	for _, want := range []string{"/graph/query", "/graph/nodes", "/graph/edges", "/graph/mutations"} {
		if !patterns[want] {
			t.Errorf("expected handler %q to be mounted", want)
		}
	}
}

func TestComponent_Register_NoHandlers_WhenDisabled(t *testing.T) {
	c := New()
	c.Configure(spec.ComponentConfig{"expose_api": false, "bulk_loading": false})

	app := spec.NewApplication(nil)
	c.Register(app)

	if len(app.HTTPHandlers()) != 0 {
		t.Errorf("expected no handlers when expose_api and bulk_loading are false, got %d", len(app.HTTPHandlers()))
	}
}

// --- Mutation parsing tests ---

func TestApplyMutation_UnknownOp(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: nil}

	err := c.applyMutation(context.Background(), Mutation{Op: "BOGUS", Type: "Node", Kind: "node", ID: "1"})
	if err == nil {
		t.Error("expected error for unknown mutation op")
	}
	if !strings.Contains(err.Error(), "unknown mutation op") {
		t.Errorf("error %q should mention 'unknown mutation op'", err.Error())
	}
}

func TestApplyMutation_UpdateRequiresID(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}

	err := c.applyMutation(context.Background(), Mutation{Op: MutationUpdate, Type: "Node", Kind: "node"})
	if err == nil {
		t.Error("expected error for UPDATE without id")
	}
}

// --- Bulk mutation handler test ---

func TestHandleBulkMutations_ParsesJSONL(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}

	// Build a JSONL body with 3 mutations.
	mutations := []Mutation{
		{Op: MutationCreate, Kind: "node", Type: "Resource", ID: "r1", Props: map[string]any{"slug": "a"}},
		{Op: MutationCreate, Kind: "node", Type: "DataSource", ID: "ds1"},
		{Op: MutationCreate, Kind: "edge", Type: "Owns", From: "ds1", To: "r1"},
	}
	var lines []string
	for _, m := range mutations {
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	body := []byte(strings.Join(lines, "\n"))

	rw := &testWriter{headers: make(map[string][]string)}
	req := &spec.Request{
		Method:  "POST",
		URL:     "/graph/mutations",
		Body:    body,
		Context: context.Background(),
	}
	c.handleBulkMutations(rw, req)

	if rw.statusCode != 200 {
		t.Errorf("expected 200, got %d; body: %s", rw.statusCode, rw.body)
	}

	var resp map[string]any
	json.Unmarshal(rw.body, &resp)
	if applied, _ := resp["applied"].(float64); int(applied) != 3 {
		t.Errorf("expected applied=3, got %v", resp["applied"])
	}
}

func TestEscapeCypher(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"plain", "plain"},
		{"with'quote", `with\'quote`},
		{"multi'ple'quotes", `multi\'ple\'quotes`},
		{"", ""},
	}
	for _, tc := range tests {
		got := escapeCypher(tc.input)
		if got != tc.want {
			t.Errorf("escapeCypher(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- test doubles ---

type stubDB struct {
	execErr  error
	queryErr error
	rows     []string
}

func (s *stubDB) ExecContext(_ context.Context, _ string, _ ...any) error { return s.execErr }
func (s *stubDB) QueryContext(_ context.Context, _ string, _ ...any) (spec.Rows, error) {
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	return &stubRows{rows: s.rows}, nil
}

type stubRows struct {
	rows  []string
	index int
}

func (r *stubRows) Next() bool { r.index++; return r.index <= len(r.rows) }
func (r *stubRows) Scan(dest ...any) error {
	if len(dest) > 0 {
		if sp, ok := dest[0].(*string); ok {
			*sp = r.rows[r.index-1]
		}
	}
	return nil
}
func (r *stubRows) Close() error { return nil }
func (r *stubRows) Err() error   { return nil }

type testWriter struct {
	headers    map[string][]string
	statusCode int
	body       []byte
}

func (w *testWriter) Header() map[string][]string { return w.headers }
func (w *testWriter) Write(b []byte) (int, error) { w.body = append(w.body, b...); return len(b), nil }
func (w *testWriter) WriteHeader(code int)        { w.statusCode = code }

// suppress "io" import warning — io is used in the main file.
var _ = io.EOF
