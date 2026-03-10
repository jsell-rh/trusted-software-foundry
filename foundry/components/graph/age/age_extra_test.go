package age

// age_extra_test.go expands coverage for the foundry-graph-age component
// beyond what age_test.go already covers:
//   Start (nil DB, load AGE error, set search_path error, create graph
//   already-exists, create graph other error, success), Stop,
//   CreateNode (marshal error, exec error), GetNode (query error, no rows,
//   scan error, unmarshal error, success), DeleteNode, CreateEdge (marshal
//   error, exec error), DeleteEdge, Query (query error, scan error, non-JSON
//   row, rows.Err, success), ApplyMutations (error wrapping), applyMutation
//   (all ops), handleQuery (all branches), handleNodeCRUD (all branches),
//   handleEdgeCRUD (all branches), handleBulkMutations (405, invalid JSONL,
//   mutation error), isAlreadyExistsErr, httpHandlerFunc.ServeHTTP,
//   sqlDB.ExecContext, sqlDB.QueryContext.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// Additional test doubles
// --------------------------------------------------------------------------

// orderedExecDB returns successive errors from execErrs on each ExecContext call.
type orderedExecDB struct {
	execErrs []error
	call     int
	queryErr error
	rows     []string
}

func (o *orderedExecDB) ExecContext(_ context.Context, _ string, _ ...any) error {
	if o.call < len(o.execErrs) {
		err := o.execErrs[o.call]
		o.call++
		return err
	}
	o.call++
	return nil
}

func (o *orderedExecDB) QueryContext(_ context.Context, _ string, _ ...any) (spec.Rows, error) {
	if o.queryErr != nil {
		return nil, o.queryErr
	}
	return &stubRows{rows: o.rows}, nil
}

// errorScanRows always returns error from Scan (Next returns true once).
type errorScanRows struct{ done bool }

func (r *errorScanRows) Next() bool        { b := !r.done; r.done = true; return b }
func (r *errorScanRows) Scan(...any) error { return errors.New("scan error") }
func (r *errorScanRows) Close() error      { return nil }
func (r *errorScanRows) Err() error        { return nil }

// errorScanDB returns an errorScanRows from QueryContext.
type errorScanDB struct{}

func (d *errorScanDB) ExecContext(_ context.Context, _ string, _ ...any) error { return nil }
func (d *errorScanDB) QueryContext(_ context.Context, _ string, _ ...any) (spec.Rows, error) {
	return &errorScanRows{}, nil
}

// errAfterRows returns rows that Err() with an error after iteration.
type errAfterRows struct {
	rows  []string
	index int
	err   error
}

func (r *errAfterRows) Next() bool { r.index++; return r.index <= len(r.rows) }
func (r *errAfterRows) Scan(dest ...any) error {
	if sp, ok := dest[0].(*string); ok {
		*sp = r.rows[r.index-1]
	}
	return nil
}
func (r *errAfterRows) Close() error { return nil }
func (r *errAfterRows) Err() error   { return r.err }

type errAfterDB struct {
	rows []string
	err  error
}

func (d *errAfterDB) ExecContext(_ context.Context, _ string, _ ...any) error { return nil }
func (d *errAfterDB) QueryContext(_ context.Context, _ string, _ ...any) (spec.Rows, error) {
	return &errAfterRows{rows: d.rows, err: d.err}, nil
}

// newTestWriter creates a fresh testWriter (reuse type from age_test.go).
func newTestWriter() *testWriter {
	return &testWriter{headers: make(map[string][]string)}
}

// --------------------------------------------------------------------------
// Start
// --------------------------------------------------------------------------

func TestStart_NilDB(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.app = spec.NewApplication(nil)
	// app.DB() returns nil → error
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when app.DB() is nil")
	}
	if !strings.Contains(err.Error(), "foundry-graph-age") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_LoadAGEExtension_Error(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.app = spec.NewApplication(nil)
	c.app.SetDB(&orderedExecDB{execErrs: []error{errors.New("ext install failed")}})
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on CREATE EXTENSION failure")
	}
	if !strings.Contains(err.Error(), "load AGE extension") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_SetSearchPath_Error(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.app = spec.NewApplication(nil)
	c.app.SetDB(&orderedExecDB{
		execErrs: []error{nil, errors.New("search_path error")},
	})
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on SET search_path failure")
	}
	if !strings.Contains(err.Error(), "set search_path") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_CreateGraph_AlreadyExists(t *testing.T) {
	// Third ExecContext returns "already exists" → success (ignored).
	c := New()
	c.cfg.graphName = "test"
	c.app = spec.NewApplication(nil)
	c.app.SetDB(&orderedExecDB{
		execErrs: []error{nil, nil, errors.New("graph already exists")},
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start should succeed when graph already exists: %v", err)
	}
}

func TestStart_CreateGraph_OtherError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.app = spec.NewApplication(nil)
	c.app.SetDB(&orderedExecDB{
		execErrs: []error{nil, nil, errors.New("unexpected DB error")},
	})
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on create graph failure")
	}
	if !strings.Contains(err.Error(), "create graph") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "foundry_graph"
	c.app = spec.NewApplication(nil)
	c.app.SetDB(&stubDB{}) // all ExecContext calls return nil
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// --------------------------------------------------------------------------
// Stop
// --------------------------------------------------------------------------

func TestStop(t *testing.T) {
	c := New()
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// --------------------------------------------------------------------------
// CreateNode
// --------------------------------------------------------------------------

func TestCreateNode_MarshalError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	// json.Marshal fails on a channel value.
	props := map[string]any{"bad": make(chan int)}
	err := c.CreateNode(context.Background(), "Node", "id1", props)
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if !strings.Contains(err.Error(), "marshal node props") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateNode_ExecError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("db exec error")}
	err := c.CreateNode(context.Background(), "Node", "id1", map[string]any{"k": "v"})
	if err == nil {
		t.Fatal("expected exec error, got nil")
	}
}

func TestCreateNode_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	if err := c.CreateNode(context.Background(), "Resource", "r1", map[string]any{"slug": "a"}); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
}

// --------------------------------------------------------------------------
// GetNode
// --------------------------------------------------------------------------

func TestGetNode_QueryError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{queryErr: errors.New("query failed")}
	_, err := c.GetNode(context.Background(), "Node", "id1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "get node") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetNode_NoRows(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{rows: nil}
	result, err := c.GetNode(context.Background(), "Node", "id1")
	if err != nil {
		t.Fatalf("GetNode with no rows: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for no rows, got %v", result)
	}
}

func TestGetNode_ScanError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &errorScanDB{}
	_, err := c.GetNode(context.Background(), "Node", "id1")
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
	if !strings.Contains(err.Error(), "scan node") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetNode_UnmarshalError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{rows: []string{"not-valid-json{{{"}}
	_, err := c.GetNode(context.Background(), "Node", "id1")
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal node") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetNode_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	raw, _ := json.Marshal(map[string]any{"id": "r1", "slug": "alpha"})
	c.db = &stubDB{rows: []string{string(raw)}}
	result, err := c.GetNode(context.Background(), "Resource", "r1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["id"] != "r1" {
		t.Errorf("id = %v, want r1", result["id"])
	}
}

// --------------------------------------------------------------------------
// DeleteNode
// --------------------------------------------------------------------------

func TestDeleteNode_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	if err := c.DeleteNode(context.Background(), "Resource", "r1"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
}

func TestDeleteNode_Error(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("delete failed")}
	if err := c.DeleteNode(context.Background(), "Resource", "r1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --------------------------------------------------------------------------
// CreateEdge
// --------------------------------------------------------------------------

func TestCreateEdge_MarshalError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	props := map[string]any{"bad": make(chan int)}
	err := c.CreateEdge(context.Background(), "Owns", "a", "b", props)
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if !strings.Contains(err.Error(), "marshal edge props") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateEdge_ExecError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("exec failed")}
	err := c.CreateEdge(context.Background(), "Owns", "a", "b", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreateEdge_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	if err := c.CreateEdge(context.Background(), "Owns", "a", "b", map[string]any{"weight": 1}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}
}

// --------------------------------------------------------------------------
// DeleteEdge
// --------------------------------------------------------------------------

func TestDeleteEdge_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	if err := c.DeleteEdge(context.Background(), "Owns", "a", "b"); err != nil {
		t.Fatalf("DeleteEdge: %v", err)
	}
}

func TestDeleteEdge_Error(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("delete edge failed")}
	if err := c.DeleteEdge(context.Background(), "Owns", "a", "b"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --------------------------------------------------------------------------
// Query
// --------------------------------------------------------------------------

func TestQuery_QueryError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{queryErr: errors.New("query failed")}
	_, err := c.Query(context.Background(), "MATCH (n) RETURN n")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cypher query") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQuery_ScanError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &errorScanDB{}
	_, err := c.Query(context.Background(), "MATCH (n) RETURN n")
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
	if !strings.Contains(err.Error(), "scan result") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQuery_NonJSONRow(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	// Non-JSON raw string falls back to {"result": raw}.
	c.db = &stubDB{rows: []string{"not-json"}}
	results, err := c.Query(context.Background(), "MATCH (n) RETURN n")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["result"] != "not-json" {
		t.Errorf("result = %v, want 'not-json' fallback", results[0])
	}
}

func TestQuery_RowsErr(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	raw, _ := json.Marshal(map[string]any{"k": "v"})
	c.db = &errAfterDB{rows: []string{string(raw)}, err: errors.New("rows error")}
	_, err := c.Query(context.Background(), "MATCH (n) RETURN n")
	if err == nil {
		t.Fatal("expected rows.Err(), got nil")
	}
}

func TestQuery_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	row1, _ := json.Marshal(map[string]any{"id": "n1"})
	row2, _ := json.Marshal(map[string]any{"id": "n2"})
	c.db = &stubDB{rows: []string{string(row1), string(row2)}}
	results, err := c.Query(context.Background(), "MATCH (n) RETURN n")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

// --------------------------------------------------------------------------
// ApplyMutations — error wrapping
// --------------------------------------------------------------------------

func TestApplyMutations_ErrorWrapped(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("db error")}
	err := c.ApplyMutations(context.Background(), []Mutation{
		{Op: MutationCreate, Kind: "node", Type: "Resource", ID: "r1"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mutation[0]") {
		t.Errorf("error should mention mutation index: %v", err)
	}
}

func TestApplyMutations_Empty(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	if err := c.ApplyMutations(context.Background(), nil); err != nil {
		t.Fatalf("ApplyMutations(nil): %v", err)
	}
}

// --------------------------------------------------------------------------
// applyMutation — remaining op/kind branches
// --------------------------------------------------------------------------

func TestApplyMutation_Create_Edge(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.applyMutation(context.Background(), Mutation{
		Op: MutationCreate, Kind: "edge", Type: "Owns", From: "a", To: "b",
	})
	if err != nil {
		t.Fatalf("applyMutation CREATE edge: %v", err)
	}
}

func TestApplyMutation_Define_Node(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.applyMutation(context.Background(), Mutation{
		Op: MutationDefine, Kind: "node", Type: "Resource", ID: "r1",
	})
	if err != nil {
		t.Fatalf("applyMutation DEFINE node: %v", err)
	}
}

func TestApplyMutation_Define_Edge(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.applyMutation(context.Background(), Mutation{
		Op: MutationDefine, Kind: "edge", Type: "Owns", From: "a", To: "b",
	})
	if err != nil {
		t.Fatalf("applyMutation DEFINE edge: %v", err)
	}
}

func TestApplyMutation_Update_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.applyMutation(context.Background(), Mutation{
		Op: MutationUpdate, Kind: "node", Type: "Resource", ID: "r1",
		Props: map[string]any{"slug": "updated"},
	})
	if err != nil {
		t.Fatalf("applyMutation UPDATE: %v", err)
	}
}

func TestApplyMutation_Delete_Node(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.applyMutation(context.Background(), Mutation{
		Op: MutationDelete, Kind: "node", Type: "Resource", ID: "r1",
	})
	if err != nil {
		t.Fatalf("applyMutation DELETE node: %v", err)
	}
}

func TestApplyMutation_Delete_Edge(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.applyMutation(context.Background(), Mutation{
		Op: MutationDelete, Kind: "edge", Type: "Owns", From: "a", To: "b",
	})
	if err != nil {
		t.Fatalf("applyMutation DELETE edge: %v", err)
	}
}

// --------------------------------------------------------------------------
// handleQuery
// --------------------------------------------------------------------------

func TestHandleQuery_MethodNotAllowed(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleQuery(rw, &spec.Request{Method: "GET", URL: "/graph/query", Context: context.Background()})
	if rw.statusCode != 405 {
		t.Errorf("expected 405, got %d", rw.statusCode)
	}
}

func TestHandleQuery_InvalidJSON(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleQuery(rw, &spec.Request{
		Method: "POST", URL: "/graph/query",
		Body:    []byte("not-json"),
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400, got %d", rw.statusCode)
	}
}

func TestHandleQuery_EmptyCypher(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"cypher": ""})
	c.handleQuery(rw, &spec.Request{
		Method: "POST", URL: "/graph/query",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400 for empty cypher, got %d", rw.statusCode)
	}
}

func TestHandleQuery_DBError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{queryErr: errors.New("db error")}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"cypher": "MATCH (n) RETURN n"})
	c.handleQuery(rw, &spec.Request{
		Method: "POST", URL: "/graph/query",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 500 {
		t.Errorf("expected 500, got %d", rw.statusCode)
	}
	respBody := string(rw.body)
	if strings.Contains(respBody, "db error") {
		t.Errorf("response leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestHandleQuery_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{rows: nil} // no rows → empty results
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"cypher": "MATCH (n) RETURN n"})
	c.handleQuery(rw, &spec.Request{
		Method: "POST", URL: "/graph/query",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 200 {
		t.Errorf("expected 200, got %d; body: %s", rw.statusCode, rw.body)
	}
}

// --------------------------------------------------------------------------
// handleNodeCRUD
// --------------------------------------------------------------------------

func TestHandleNodeCRUD_POST_InvalidJSON(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/nodes",
		Body:    []byte("not-json"),
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400, got %d", rw.statusCode)
	}
}

func TestHandleNodeCRUD_POST_DBError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("db error")}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]any{"type": "Resource", "id": "r1", "props": nil})
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/nodes",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 500 {
		t.Errorf("expected 500, got %d", rw.statusCode)
	}
	respBody := string(rw.body)
	if strings.Contains(respBody, "db error") {
		t.Errorf("response leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestHandleNodeCRUD_POST_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]any{"type": "Resource", "id": "r1", "props": map[string]any{"slug": "a"}})
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/nodes",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 201 {
		t.Errorf("expected 201, got %d; body: %s", rw.statusCode, rw.body)
	}
}

func TestHandleNodeCRUD_DELETE_InvalidJSON(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/nodes",
		Body:    []byte("not-json"),
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400, got %d", rw.statusCode)
	}
}

func TestHandleNodeCRUD_DELETE_DBError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("db error")}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"type": "Resource", "id": "r1"})
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/nodes",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 500 {
		t.Errorf("expected 500, got %d", rw.statusCode)
	}
	respBody := string(rw.body)
	if strings.Contains(respBody, "db error") {
		t.Errorf("response leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestHandleNodeCRUD_DELETE_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"type": "Resource", "id": "r1"})
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/nodes",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 200 {
		t.Errorf("expected 200, got %d", rw.statusCode)
	}
}

func TestHandleNodeCRUD_MethodNotAllowed(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "GET", URL: "/graph/nodes", Context: context.Background(),
	})
	if rw.statusCode != 405 {
		t.Errorf("expected 405, got %d", rw.statusCode)
	}
}

// --------------------------------------------------------------------------
// handleEdgeCRUD
// --------------------------------------------------------------------------

func TestHandleEdgeCRUD_POST_InvalidJSON(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/edges",
		Body:    []byte("not-json"),
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400, got %d", rw.statusCode)
	}
}

func TestHandleEdgeCRUD_POST_DBError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("db error")}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]any{"type": "Owns", "from": "a", "to": "b", "props": nil})
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/edges",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 500 {
		t.Errorf("expected 500, got %d", rw.statusCode)
	}
	respBody := string(rw.body)
	if strings.Contains(respBody, "db error") {
		t.Errorf("response leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestHandleEdgeCRUD_POST_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]any{"type": "Owns", "from": "a", "to": "b", "props": map[string]any{}})
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/edges",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 201 {
		t.Errorf("expected 201, got %d; body: %s", rw.statusCode, rw.body)
	}
}

func TestHandleEdgeCRUD_DELETE_InvalidJSON(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/edges",
		Body:    []byte("not-json"),
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400, got %d", rw.statusCode)
	}
}

func TestHandleEdgeCRUD_DELETE_DBError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("db error")}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"type": "Owns", "from": "a", "to": "b"})
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/edges",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 500 {
		t.Errorf("expected 500, got %d", rw.statusCode)
	}
	respBody := string(rw.body)
	if strings.Contains(respBody, "db error") {
		t.Errorf("response leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestHandleEdgeCRUD_DELETE_Success(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"type": "Owns", "from": "a", "to": "b"})
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/edges",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 200 {
		t.Errorf("expected 200, got %d", rw.statusCode)
	}
}

func TestHandleEdgeCRUD_MethodNotAllowed(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "PUT", URL: "/graph/edges", Context: context.Background(),
	})
	if rw.statusCode != 405 {
		t.Errorf("expected 405, got %d", rw.statusCode)
	}
}

// --------------------------------------------------------------------------
// handleBulkMutations — additional branches
// --------------------------------------------------------------------------

func TestHandleBulkMutations_MethodNotAllowed(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleBulkMutations(rw, &spec.Request{
		Method: "GET", URL: "/graph/mutations", Context: context.Background(),
	})
	if rw.statusCode != 405 {
		t.Errorf("expected 405, got %d", rw.statusCode)
	}
}

func TestHandleBulkMutations_InvalidJSONL(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	c.handleBulkMutations(rw, &spec.Request{
		Method:  "POST",
		URL:     "/graph/mutations",
		Body:    []byte("not-json-at-all{{{{"),
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400 for invalid JSONL, got %d", rw.statusCode)
	}
}

func TestHandleBulkMutations_ApplyError(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{execErr: errors.New("db error")}
	rw := newTestWriter()
	m := Mutation{Op: MutationCreate, Kind: "node", Type: "Resource", ID: "r1"}
	b, _ := json.Marshal(m)
	c.handleBulkMutations(rw, &spec.Request{
		Method:  "POST",
		URL:     "/graph/mutations",
		Body:    b,
		Context: context.Background(),
	})
	if rw.statusCode != 500 {
		t.Errorf("expected 500, got %d", rw.statusCode)
	}
	respBody := string(rw.body)
	if strings.Contains(respBody, "db error") {
		t.Errorf("response leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

// --------------------------------------------------------------------------
// isAlreadyExistsErr
// --------------------------------------------------------------------------

func TestIsAlreadyExistsErr(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("graph already exists"), true},
		{errors.New("already exists in catalog"), true},
		{errors.New("some other error"), false},
	}
	for _, tc := range tests {
		got := isAlreadyExistsErr(tc.err)
		if got != tc.want {
			t.Errorf("isAlreadyExistsErr(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// --------------------------------------------------------------------------
// httpHandlerFunc.ServeHTTP
// --------------------------------------------------------------------------

func TestHTTPHandlerFunc_ServeHTTP(t *testing.T) {
	called := false
	f := httpHandlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		called = true
	})
	rw := newTestWriter()
	f.ServeHTTP(rw, &spec.Request{Method: "GET", URL: "/", Context: context.Background()})
	if !called {
		t.Error("httpHandlerFunc.ServeHTTP did not call the underlying function")
	}
}

// --------------------------------------------------------------------------
// sqlDB.ExecContext and sqlDB.QueryContext
// --------------------------------------------------------------------------

func TestSqlDB_ExecContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(1, 1))

	s := &sqlDB{db: db}
	if err := s.ExecContext(context.Background(), "SELECT 1"); err != nil {
		t.Fatalf("ExecContext: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSqlDB_ExecContext_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("SELECT 1").WillReturnError(errors.New("exec error"))

	s := &sqlDB{db: db}
	if err := s.ExecContext(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSqlDB_QueryContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"col"}).AddRow("val"))

	s := &sqlDB{db: db}
	rows, err := s.QueryContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	rows.Close()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSqlDB_QueryContext_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT 1").WillReturnError(errors.New("query error"))

	s := &sqlDB{db: db}
	_, err = s.QueryContext(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --------------------------------------------------------------------------
// validateLabel — Cypher injection prevention
// --------------------------------------------------------------------------

func TestValidateLabel_ValidLabels(t *testing.T) {
	valid := []string{
		"Node", "Resource", "DataSource", "MyType123", "A", "z", "Type_Name",
		"Owns", "RELATES_TO", "abc123",
	}
	for _, label := range valid {
		if err := validateLabel(label); err != nil {
			t.Errorf("validateLabel(%q) returned unexpected error: %v", label, err)
		}
	}
}

func TestValidateLabel_InvalidLabels(t *testing.T) {
	invalid := []string{
		"",
		"123Node",       // starts with digit
		"Node Type",     // contains space
		"Node; DROP",    // semicolon injection
		"Node$",         // dollar sign
		"Node\nLabel",   // newline
		"'quoted'",      // single quotes
		"Node--comment", // double-dash
	}
	for _, label := range invalid {
		if err := validateLabel(label); err == nil {
			t.Errorf("validateLabel(%q) expected error, got nil", label)
		}
	}
}

func TestCreateNode_InvalidLabel(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.CreateNode(context.Background(), "Bad Label!", "id1", nil)
	if err == nil {
		t.Fatal("expected label validation error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid graph label") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetNode_InvalidLabel(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	_, err := c.GetNode(context.Background(), "Bad; DROP", "id1")
	if err == nil {
		t.Fatal("expected label validation error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid graph label") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeleteNode_InvalidLabel(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.DeleteNode(context.Background(), "'; DROP GRAPH foundry_graph; --", "id1")
	if err == nil {
		t.Fatal("expected label validation error, got nil")
	}
}

func TestCreateEdge_InvalidLabel(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.CreateEdge(context.Background(), "Bad Edge!", "a", "b", nil)
	if err == nil {
		t.Fatal("expected label validation error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid graph label") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeleteEdge_InvalidLabel(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	err := c.DeleteEdge(context.Background(), "'; --", "a", "b")
	if err == nil {
		t.Fatal("expected label validation error, got nil")
	}
}

func TestHandleNodeCRUD_POST_InvalidLabel_Returns400(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]any{"type": "Bad Label!", "id": "r1"})
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/nodes",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400 for invalid label, got %d; body: %s", rw.statusCode, rw.body)
	}
}

func TestHandleNodeCRUD_DELETE_InvalidLabel_Returns400(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"type": "'; DROP GRAPH --", "id": "r1"})
	c.handleNodeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/nodes",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400 for invalid label, got %d; body: %s", rw.statusCode, rw.body)
	}
}

func TestHandleEdgeCRUD_POST_InvalidLabel_Returns400(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]any{"type": "Bad; DROP", "from": "a", "to": "b"})
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "POST", URL: "/graph/edges",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400 for invalid edge label, got %d; body: %s", rw.statusCode, rw.body)
	}
}

func TestHandleEdgeCRUD_DELETE_InvalidLabel_Returns400(t *testing.T) {
	c := New()
	c.cfg.graphName = "test"
	c.db = &stubDB{}
	rw := newTestWriter()
	body, _ := json.Marshal(map[string]string{"type": "'; DROP --", "from": "a", "to": "b"})
	c.handleEdgeCRUD(rw, &spec.Request{
		Method: "DELETE", URL: "/graph/edges",
		Body:    body,
		Context: context.Background(),
	})
	if rw.statusCode != 400 {
		t.Errorf("expected 400 for invalid edge label, got %d; body: %s", rw.statusCode, rw.body)
	}
}

// --------------------------------------------------------------------------
// Configure — graph_name validation
// --------------------------------------------------------------------------

func TestConfigure_ValidGraphName(t *testing.T) {
	for _, name := range []string{"foundry_graph", "MyGraph", "graph123", "A"} {
		c := New()
		if err := c.Configure(spec.ComponentConfig{"graph_name": name}); err != nil {
			t.Errorf("Configure(%q) unexpected error: %v", name, err)
		}
		if c.cfg.graphName != name {
			t.Errorf("graphName = %q, want %q", c.cfg.graphName, name)
		}
	}
}

func TestConfigure_InvalidGraphName_RejectsInjection(t *testing.T) {
	// graph_name is interpolated directly into every Cypher query; any character
	// outside [A-Za-z][A-Za-z0-9_]* must be rejected to prevent injection.
	invalid := []string{
		"'; DROP GRAPH foundry_graph; --",
		"graph name",  // space
		"123graph",    // starts with digit
		"graph-name",  // hyphen
		"graph.name",  // dot
		"graph\nname", // newline
	}
	for _, name := range invalid {
		c := New()
		err := c.Configure(spec.ComponentConfig{"graph_name": name})
		if err == nil {
			t.Errorf("Configure(%q) expected error for invalid graph_name, got nil", name)
		}
	}
}
