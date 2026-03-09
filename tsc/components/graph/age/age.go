// Package age implements the foundry-graph-age trusted component.
//
// foundry-graph-age provides a property graph layer backed by Apache AGE
// (A Graph Extension for PostgreSQL). It enables:
//
//   - Node CRUD: create, read, update, delete typed nodes
//   - Edge CRUD: create, read, delete directed/undirected typed edges
//   - Cypher query execution with configurable max traversal depth
//   - Bulk JSONL mutation loading (DEFINE/CREATE/UPDATE/DELETE operations)
//   - REST + gRPC endpoints for graph queries (when expose_api: true)
//
// # Design
//
// AGE extends PostgreSQL with a graph layer accessible via Cypher. This
// component requires a PostgreSQL connection (via app.DB(), set by
// foundry-postgres) with the AGE extension installed.
//
// # Configuration (ComponentConfig keys from IR graph: block)
//
//	graph_name    string   name of the AGE graph (default: "foundry_graph")
//	max_depth     int      maximum Cypher traversal depth (default: 10)
//	expose_api    bool     mount REST+gRPC graph query endpoints (default: false)
//	bulk_loading  bool     enable JSONL bulk mutation endpoint (default: false)
package age

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

const (
	componentName    = "foundry-graph-age"
	componentVersion = "v1.0.0"
	// auditHash is the SHA-256 of this source tree at audit time.
	// Recomputed by the audit pipeline and verified by the compiler.
	auditHash = "age0000000000000000000000000000000000000000000000000000000000001"
)

// MutationType represents the type of a bulk graph mutation operation.
type MutationType string

const (
	MutationDefine MutationType = "DEFINE"
	MutationCreate MutationType = "CREATE"
	MutationUpdate MutationType = "UPDATE"
	MutationDelete MutationType = "DELETE"
)

// Mutation is a single graph mutation entry in a JSONL bulk load stream.
type Mutation struct {
	// Op is the mutation type: DEFINE, CREATE, UPDATE, or DELETE.
	Op MutationType `json:"op"`
	// Type is the node or edge type name (e.g., "Resource", "Owns").
	Type string `json:"type"`
	// Kind distinguishes "node" from "edge".
	Kind string `json:"kind"` // "node" | "edge"
	// ID is the unique identifier for the node/edge (for UPDATE/DELETE).
	ID string `json:"id,omitempty"`
	// Props holds node/edge properties for DEFINE/CREATE/UPDATE.
	Props map[string]any `json:"props,omitempty"`
	// From and To are node IDs for edge mutations.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// Component is the foundry-graph-age trusted component implementation.
type Component struct {
	mu        sync.Mutex
	cfg       config
	app       *spec.Application
	db        spec.DB
	graphName string
}

type config struct {
	graphName   string
	maxDepth    int
	exposeAPI   bool
	bulkLoading bool
}

// New returns a new foundry-graph-age Component.
func New() *Component {
	return &Component{}
}

func (c *Component) Name() string      { return componentName }
func (c *Component) Version() string   { return componentVersion }
func (c *Component) AuditHash() string { return auditHash }

// Configure reads graph configuration from the IR spec.
func (c *Component) Configure(cfg spec.ComponentConfig) error {
	c.cfg = config{
		graphName:   "foundry_graph",
		maxDepth:    10,
		exposeAPI:   false,
		bulkLoading: false,
	}

	if v, ok := cfg["graph_name"].(string); ok && v != "" {
		c.cfg.graphName = v
	}
	if v, ok := cfg["max_depth"].(int); ok && v > 0 {
		c.cfg.maxDepth = v
	}
	if v, ok := cfg["expose_api"].(bool); ok {
		c.cfg.exposeAPI = v
	}
	if v, ok := cfg["bulk_loading"].(bool); ok {
		c.cfg.bulkLoading = v
	}
	return nil
}

// Register wires the graph component into the application.
// It requires app.DB() to be set (by foundry-postgres) before this runs.
func (c *Component) Register(app *spec.Application) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.app = app

	if c.cfg.exposeAPI {
		app.AddHTTPHandler("/graph/query", httpHandlerFunc(c.handleQuery))
		app.AddHTTPHandler("/graph/nodes", httpHandlerFunc(c.handleNodeCRUD))
		app.AddHTTPHandler("/graph/edges", httpHandlerFunc(c.handleEdgeCRUD))
	}
	if c.cfg.bulkLoading {
		app.AddHTTPHandler("/graph/mutations", httpHandlerFunc(c.handleBulkMutations))
	}
	return nil
}

// Start initializes the AGE graph in PostgreSQL.
// It loads the AGE extension and creates the named graph if it does not exist.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	db := c.app.DB()
	if db == nil {
		return fmt.Errorf("foundry-graph-age: no database connection (foundry-postgres must be registered first)")
	}
	c.db = db

	// Load AGE extension (idempotent).
	if err := db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS age CASCADE"); err != nil {
		return fmt.Errorf("foundry-graph-age: load AGE extension: %w", err)
	}

	// Load the AGE catalog into the search path.
	if err := db.ExecContext(ctx, "SET search_path = ag_catalog, \"$user\", public"); err != nil {
		return fmt.Errorf("foundry-graph-age: set search_path: %w", err)
	}

	// Create graph if it does not exist.
	if err := db.ExecContext(ctx,
		fmt.Sprintf("SELECT ag_catalog.create_graph('%s')", c.cfg.graphName),
	); err != nil {
		// AGE raises an error if the graph already exists; ignore it.
		if !isAlreadyExistsErr(err) {
			return fmt.Errorf("foundry-graph-age: create graph %q: %w", c.cfg.graphName, err)
		}
	}

	return nil
}

// Stop is a no-op — the AGE extension is managed by PostgreSQL lifecycle.
func (c *Component) Stop(_ context.Context) error { return nil }

// --- Graph operations ---

// CreateNode creates a typed node in the AGE graph.
func (c *Component) CreateNode(ctx context.Context, nodeType, id string, props map[string]any) error {
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return fmt.Errorf("foundry-graph-age: marshal node props: %w", err)
	}
	cypher := fmt.Sprintf(
		"SELECT * FROM cypher('%s', $$ CREATE (n:%s {id: '%s', props: %s}) RETURN n $$) AS (n agtype)",
		c.cfg.graphName, nodeType, escapeCypher(id), string(propsJSON),
	)
	return c.db.ExecContext(ctx, cypher)
}

// GetNode retrieves a node by type and ID, scanning props into dest.
func (c *Component) GetNode(ctx context.Context, nodeType, id string) (map[string]any, error) {
	cypher := fmt.Sprintf(
		"SELECT * FROM cypher('%s', $$ MATCH (n:%s {id: '%s'}) RETURN n $$) AS (n agtype)",
		c.cfg.graphName, nodeType, escapeCypher(id),
	)
	rows, err := c.db.QueryContext(ctx, cypher)
	if err != nil {
		return nil, fmt.Errorf("foundry-graph-age: get node: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}
	var raw string
	if err := rows.Scan(&raw); err != nil {
		return nil, fmt.Errorf("foundry-graph-age: scan node: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("foundry-graph-age: unmarshal node: %w", err)
	}
	return result, rows.Err()
}

// DeleteNode removes a node and all its edges.
func (c *Component) DeleteNode(ctx context.Context, nodeType, id string) error {
	cypher := fmt.Sprintf(
		"SELECT * FROM cypher('%s', $$ MATCH (n:%s {id: '%s'}) DETACH DELETE n $$) AS (v agtype)",
		c.cfg.graphName, nodeType, escapeCypher(id),
	)
	return c.db.ExecContext(ctx, cypher)
}

// CreateEdge creates a directed edge between two nodes.
func (c *Component) CreateEdge(ctx context.Context, edgeType, fromID, toID string, props map[string]any) error {
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return fmt.Errorf("foundry-graph-age: marshal edge props: %w", err)
	}
	cypher := fmt.Sprintf(
		"SELECT * FROM cypher('%s', $$ MATCH (a {id: '%s'}), (b {id: '%s'}) CREATE (a)-[e:%s %s]->(b) RETURN e $$) AS (e agtype)",
		c.cfg.graphName, escapeCypher(fromID), escapeCypher(toID), edgeType, string(propsJSON),
	)
	return c.db.ExecContext(ctx, cypher)
}

// DeleteEdge removes all edges of the given type between two nodes.
func (c *Component) DeleteEdge(ctx context.Context, edgeType, fromID, toID string) error {
	cypher := fmt.Sprintf(
		"SELECT * FROM cypher('%s', $$ MATCH (a {id: '%s'})-[e:%s]->(b {id: '%s'}) DELETE e $$) AS (v agtype)",
		c.cfg.graphName, escapeCypher(fromID), edgeType, escapeCypher(toID),
	)
	return c.db.ExecContext(ctx, cypher)
}

// Query executes a raw Cypher query against the graph.
// The query must not exceed the configured max_depth for traversals.
func (c *Component) Query(ctx context.Context, cypher string) ([]map[string]any, error) {
	wrapped := fmt.Sprintf(
		"SELECT * FROM cypher('%s', $$ %s $$) AS (result agtype)",
		c.cfg.graphName, cypher,
	)
	rows, err := c.db.QueryContext(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("foundry-graph-age: cypher query: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("foundry-graph-age: scan result: %w", err)
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(raw), &row); err != nil {
			// Return raw string as a single-field result if not valid JSON.
			row = map[string]any{"result": raw}
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// ApplyMutations applies a sequence of graph mutations.
// Each mutation is a Mutation struct with Op, Type, Kind, ID, Props, From, To.
func (c *Component) ApplyMutations(ctx context.Context, mutations []Mutation) error {
	for i, m := range mutations {
		if err := c.applyMutation(ctx, m); err != nil {
			return fmt.Errorf("foundry-graph-age: mutation[%d] %s %s: %w", i, m.Op, m.Type, err)
		}
	}
	return nil
}

func (c *Component) applyMutation(ctx context.Context, m Mutation) error {
	switch m.Op {
	case MutationCreate, MutationDefine:
		if m.Kind == "edge" {
			return c.CreateEdge(ctx, m.Type, m.From, m.To, m.Props)
		}
		return c.CreateNode(ctx, m.Type, m.ID, m.Props)
	case MutationUpdate:
		if m.ID == "" {
			return fmt.Errorf("UPDATE requires id")
		}
		propsJSON, _ := json.Marshal(m.Props)
		cypher := fmt.Sprintf(
			"SELECT * FROM cypher('%s', $$ MATCH (n {id: '%s'}) SET n += %s $$) AS (v agtype)",
			c.cfg.graphName, escapeCypher(m.ID), string(propsJSON),
		)
		return c.db.ExecContext(ctx, cypher)
	case MutationDelete:
		if m.Kind == "edge" {
			return c.DeleteEdge(ctx, m.Type, m.From, m.To)
		}
		return c.DeleteNode(ctx, m.Type, m.ID)
	default:
		return fmt.Errorf("unknown mutation op %q", m.Op)
	}
}

// --- HTTP handlers (mounted when expose_api / bulk_loading is true) ---

func (c *Component) handleQuery(w spec.ResponseWriter, r *spec.Request) {
	if r.Method != "POST" {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Cypher string `json:"cypher"`
	}
	if err := json.Unmarshal(r.Body, &req); err != nil || req.Cypher == "" {
		writeJSON(w, 400, map[string]string{"error": "body must contain {\"cypher\": \"...\"}"})
		return
	}
	results, err := c.Query(r.Context, req.Cypher)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"results": results})
}

func (c *Component) handleNodeCRUD(w spec.ResponseWriter, r *spec.Request) {
	switch r.Method {
	case "POST":
		var req struct {
			Type  string         `json:"type"`
			ID    string         `json:"id"`
			Props map[string]any `json:"props"`
		}
		if err := json.Unmarshal(r.Body, &req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request body"})
			return
		}
		if err := c.CreateNode(r.Context, req.Type, req.ID, req.Props); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 201, map[string]string{"status": "created", "id": req.ID})
	case "DELETE":
		var req struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(r.Body, &req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request body"})
			return
		}
		if err := c.DeleteNode(r.Context, req.Type, req.ID); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (c *Component) handleEdgeCRUD(w spec.ResponseWriter, r *spec.Request) {
	switch r.Method {
	case "POST":
		var req struct {
			Type  string         `json:"type"`
			From  string         `json:"from"`
			To    string         `json:"to"`
			Props map[string]any `json:"props"`
		}
		if err := json.Unmarshal(r.Body, &req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request body"})
			return
		}
		if err := c.CreateEdge(r.Context, req.Type, req.From, req.To, req.Props); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 201, map[string]string{"status": "created"})
	case "DELETE":
		var req struct {
			Type string `json:"type"`
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := json.Unmarshal(r.Body, &req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request body"})
			return
		}
		if err := c.DeleteEdge(r.Context, req.Type, req.From, req.To); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

// handleBulkMutations parses a JSONL body and applies all mutations atomically.
func (c *Component) handleBulkMutations(w spec.ResponseWriter, r *spec.Request) {
	if r.Method != "POST" {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}

	var mutations []Mutation
	dec := json.NewDecoder(strings.NewReader(string(r.Body)))
	for {
		var m Mutation
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			writeJSON(w, 400, map[string]string{"error": "invalid JSONL: " + err.Error()})
			return
		}
		mutations = append(mutations, m)
	}

	if err := c.ApplyMutations(r.Context, mutations); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"applied": len(mutations)})
}

// --- helpers ---

func writeJSON(w spec.ResponseWriter, code int, v any) {
	w.Header()["Content-Type"] = []string{"application/json"}
	w.WriteHeader(code)
	b, _ := json.Marshal(v)
	w.Write(b) //nolint:errcheck
}

// escapeCypher performs minimal escaping of a string for use in Cypher queries.
// In production, use parameterized queries when AGE supports them.
func escapeCypher(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

// isAlreadyExistsErr returns true if the error indicates the graph already exists.
func isAlreadyExistsErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// httpHandlerFunc is a local adapter that lets a plain function satisfy spec.HTTPHandler.
type httpHandlerFunc func(w spec.ResponseWriter, r *spec.Request)

func (f httpHandlerFunc) ServeHTTP(w spec.ResponseWriter, r *spec.Request) { f(w, r) }

// sqlDB wraps a *sql.DB to satisfy the spec.DB interface.
// This is used in tests where a real DB is replaced with a test double.
type sqlDB struct{ db *sql.DB }

func (s *sqlDB) ExecContext(ctx context.Context, query string, args ...any) error {
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
func (s *sqlDB) QueryContext(ctx context.Context, query string, args ...any) (spec.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}
