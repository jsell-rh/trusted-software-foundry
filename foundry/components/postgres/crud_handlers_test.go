package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --- mock ResponseWriter ---

type mockResponseWriter struct {
	code    int
	headers map[string][]string
	body    bytes.Buffer
}

func newMockRW() *mockResponseWriter {
	return &mockResponseWriter{headers: make(map[string][]string)}
}

func (m *mockResponseWriter) Header() map[string][]string { return m.headers }
func (m *mockResponseWriter) Write(b []byte) (int, error) { return m.body.Write(b) }
func (m *mockResponseWriter) WriteHeader(code int)        { m.code = code }

func (m *mockResponseWriter) bodyMap() map[string]any {
	var v map[string]any
	json.Unmarshal(m.body.Bytes(), &v)
	return v
}

// --- helpers ---

func newTestDAO(t *testing.T) (*resourceDAO, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	res := spec.ResourceDefinition{
		Name:   "Dinosaur",
		Plural: "dinosaurs",
		Fields: []spec.FieldDefinition{
			{Name: "species", Type: "string", Required: true},
			{Name: "description", Type: "string"},
		},
		Operations: []string{"create", "read", "update", "delete", "list"},
	}
	return &resourceDAO{db: db, resource: res}, mock
}

func newRequest(method, url string, body []byte) *spec.Request {
	return &spec.Request{Method: method, URL: url, Body: body}
}

// --------------------------------------------------------------------------
// opsSet
// --------------------------------------------------------------------------

func TestOpsSet(t *testing.T) {
	ops := opsSet([]string{"create", "read", "list"})
	if !ops["create"] {
		t.Error("expected create in opsSet")
	}
	if ops["delete"] {
		t.Error("delete should not be in opsSet")
	}
}

// --------------------------------------------------------------------------
// extractID
// --------------------------------------------------------------------------

func TestExtractID(t *testing.T) {
	cases := []struct {
		url, prefix, want string
	}{
		{"/dinosaurs/abc-123", "dinosaurs", "abc-123"},
		{"/api/v1/dinosaurs/abc-123", "dinosaurs", "abc-123"},
		{"/dinosaurs/abc-123?page=1", "dinosaurs", "abc-123"},
		{"/dinosaurs/abc-123/", "dinosaurs", "abc-123"},
		{"/dinosaurs/", "dinosaurs", ""},
		{"/plants/abc", "dinosaurs", ""},
	}
	for _, tc := range cases {
		got := extractID(tc.url, tc.prefix)
		if got != tc.want {
			t.Errorf("extractID(%q, %q) = %q, want %q", tc.url, tc.prefix, got, tc.want)
		}
	}
}

// --------------------------------------------------------------------------
// queryParam
// --------------------------------------------------------------------------

func TestQueryParam(t *testing.T) {
	cases := []struct {
		url, key, want string
	}{
		{"/items?page=2&size=50", "page", "2"},
		{"/items?page=2&size=50", "size", "50"},
		{"/items?page=2", "size", ""},
		{"/items", "page", ""},
	}
	for _, tc := range cases {
		got := queryParam(tc.url, tc.key)
		if got != tc.want {
			t.Errorf("queryParam(%q, %q) = %q, want %q", tc.url, tc.key, got, tc.want)
		}
	}
}

// --------------------------------------------------------------------------
// writeJSON / writeError
// --------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	w := newMockRW()
	writeJSON(w, 200, map[string]any{"hello": "world"})
	if w.code != 200 {
		t.Errorf("code = %d, want 200", w.code)
	}
	if !strings.Contains(w.body.String(), "hello") {
		t.Errorf("body missing 'hello': %s", w.body.String())
	}
	if w.headers["Content-Type"][0] != "application/json" {
		t.Errorf("Content-Type = %v", w.headers["Content-Type"])
	}
}

func TestWriteJSON_MarshalError_DoesNotLeakGoTypeInfo(t *testing.T) {
	// Passing an unmarshalable value (channel) triggers the marshal error path.
	// The response must be 500 with a generic "internal server error" body —
	// not the raw Go type error string.
	w := newMockRW()
	writeJSON(w, 200, map[string]any{"bad": make(chan int)})
	if w.code != 500 {
		t.Errorf("expected 500 on marshal error, got %d", w.code)
	}
	body := w.body.String()
	if strings.Contains(body, "chan int") || strings.Contains(body, "unsupported type") {
		t.Errorf("response leaks Go type info: %s", body)
	}
	if !strings.Contains(body, "internal server error") {
		t.Errorf("expected generic error body, got: %s", body)
	}
}

func TestWriteError(t *testing.T) {
	w := newMockRW()
	writeError(w, 404, "not found")
	if w.code != 404 {
		t.Errorf("code = %d, want 404", w.code)
	}
	m := w.bodyMap()
	if m["error"] != "not found" {
		t.Errorf("error = %v, want 'not found'", m["error"])
	}
}

// --------------------------------------------------------------------------
// collectionHandler — list
// --------------------------------------------------------------------------

func TestCollectionHandler_List_Success(t *testing.T) {
	dao, mock := newTestDAO(t)
	// Count query runs before List query.
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"id", "species"}).
			AddRow("id-1", "T-Rex").AddRow("id-2", "Raptor"))

	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs", nil))

	if w.code != 200 {
		t.Errorf("code = %d, want 200", w.code)
	}
	m := w.bodyMap()
	if m["kind"] != "DinosaurList" {
		t.Errorf("kind = %v, want DinosaurList", m["kind"])
	}
	items, _ := m["items"].([]any)
	if len(items) != 2 {
		t.Errorf("items len = %d, want 2", len(items))
	}
	if m["total"] != float64(2) {
		t.Errorf("total = %v, want 2", m["total"])
	}
}

func TestCollectionHandler_List_Pagination(t *testing.T) {
	dao, mock := newTestDAO(t)
	// Count query runs before List query.
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(100))
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs?page=2&size=5", nil))

	if w.code != 200 {
		t.Errorf("code = %d, want 200", w.code)
	}
	m := w.bodyMap()
	if m["page"] != float64(2) {
		t.Errorf("page = %v, want 2", m["page"])
	}
	if m["size"] != float64(5) {
		t.Errorf("size = %v, want 5", m["size"])
	}
	if m["total"] != float64(100) {
		t.Errorf("total = %v, want 100", m["total"])
	}
}

func TestCollectionHandler_List_NotAllowed(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet(nil), resource: dao.resource}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs", nil))
	if w.code != 405 {
		t.Errorf("code = %d, want 405", w.code)
	}
}

func TestCollectionHandler_List_DBError(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE deleted_at IS NULL").
		WillReturnError(sql.ErrConnDone)

	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs", nil))
	if w.code != 500 {
		t.Errorf("code = %d, want 500", w.code)
	}
	// Response must NOT leak raw DB error details to the caller.
	body := w.body.String()
	if strings.Contains(body, sql.ErrConnDone.Error()) {
		t.Errorf("response body leaks internal DB error: %s", body)
	}
	if !strings.Contains(body, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", body)
	}
}

// --------------------------------------------------------------------------
// collectionHandler — create
// --------------------------------------------------------------------------

func TestCollectionHandler_Create_Success(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectQuery("INSERT INTO dinosaurs").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("new-id"))
	// handleCreate does a Get after Create
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WithArgs("new-id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "species"}).AddRow("new-id", "T-Rex"))

	h := &collectionHandler{dao: dao, ops: opsSet([]string{"create"}), resource: dao.resource}
	w := newMockRW()
	body, _ := json.Marshal(map[string]any{"species": "T-Rex"})
	h.ServeHTTP(w, newRequest("POST", "/dinosaurs", body))

	if w.code != 201 {
		t.Errorf("code = %d, want 201", w.code)
	}
	m := w.bodyMap()
	if m["id"] != "new-id" {
		t.Errorf("id = %v, want new-id", m["id"])
	}
}

func TestCollectionHandler_Create_MinimalResponseOnGetFail(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectQuery("INSERT INTO dinosaurs").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("new-id"))
	// Get fails after create
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WillReturnError(sql.ErrConnDone)

	h := &collectionHandler{dao: dao, ops: opsSet([]string{"create"}), resource: dao.resource}
	w := newMockRW()
	body, _ := json.Marshal(map[string]any{"species": "T-Rex"})
	h.ServeHTTP(w, newRequest("POST", "/dinosaurs", body))

	if w.code != 201 {
		t.Errorf("code = %d, want 201 (minimal response)", w.code)
	}
	m := w.bodyMap()
	if m["id"] != "new-id" {
		t.Errorf("id = %v, want new-id", m["id"])
	}
}

func TestCollectionHandler_Create_InvalidJSON(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"create"}), resource: dao.resource}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("POST", "/dinosaurs", []byte("not-json")))
	if w.code != 400 {
		t.Errorf("code = %d, want 400", w.code)
	}
}

func TestCollectionHandler_Create_MissingRequiredField(t *testing.T) {
	dao, _ := newTestDAO(t) // dinoResource has species: required=true
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"create"}), resource: dao.resource}
	w := newMockRW()
	// Send description but omit the required 'species' field.
	body, _ := json.Marshal(map[string]any{"description": "big"})
	h.ServeHTTP(w, newRequest("POST", "/dinosaurs", body))
	if w.code != 400 {
		t.Errorf("code = %d, want 400 (missing required field)", w.code)
	}
	m := w.bodyMap()
	errMsg, _ := m["error"].(string)
	if !strings.Contains(errMsg, "species") {
		t.Errorf("error = %q, should mention missing field 'species'", errMsg)
	}
}

func TestCollectionHandler_Create_NotAllowed(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet(nil), resource: dao.resource}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("POST", "/dinosaurs", []byte("{}")))
	if w.code != 405 {
		t.Errorf("code = %d, want 405", w.code)
	}
}

func TestCollectionHandler_Create_DBError(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectQuery("INSERT INTO dinosaurs").
		WillReturnError(sql.ErrConnDone)

	h := &collectionHandler{dao: dao, ops: opsSet([]string{"create"}), resource: dao.resource}
	w := newMockRW()
	body, _ := json.Marshal(map[string]any{"species": "Rex"})
	h.ServeHTTP(w, newRequest("POST", "/dinosaurs", body))
	if w.code != 500 {
		t.Errorf("code = %d, want 500", w.code)
	}
	respBody := w.body.String()
	if strings.Contains(respBody, sql.ErrConnDone.Error()) {
		t.Errorf("response body leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestCollectionHandler_UnknownMethod(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list", "create"}), resource: dao.resource}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("DELETE", "/dinosaurs", nil))
	if w.code != 405 {
		t.Errorf("code = %d, want 405", w.code)
	}
}

// --------------------------------------------------------------------------
// itemHandler — GET
// --------------------------------------------------------------------------

func TestItemHandler_Get_Success(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WithArgs("abc-123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "species"}).AddRow("abc-123", "T-Rex"))

	h := &itemHandler{dao: dao, ops: opsSet([]string{"read"}), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs/abc-123", nil))

	if w.code != 200 {
		t.Errorf("code = %d, want 200", w.code)
	}
	m := w.bodyMap()
	if m["id"] != "abc-123" {
		t.Errorf("id = %v, want abc-123", m["id"])
	}
}

func TestItemHandler_Get_NotFound(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty

	h := &itemHandler{dao: dao, ops: opsSet([]string{"read"}), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs/missing", nil))

	if w.code != 404 {
		t.Errorf("code = %d, want 404", w.code)
	}
}

func TestItemHandler_Get_DBError(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WillReturnError(sql.ErrConnDone)

	h := &itemHandler{dao: dao, ops: opsSet([]string{"read"}), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs/abc-123", nil))
	if w.code != 500 {
		t.Errorf("code = %d, want 500", w.code)
	}
	respBody := w.body.String()
	if strings.Contains(respBody, sql.ErrConnDone.Error()) {
		t.Errorf("response body leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestItemHandler_Get_NotAllowed(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &itemHandler{dao: dao, ops: opsSet(nil), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs/abc-123", nil))
	if w.code != 405 {
		t.Errorf("code = %d, want 405", w.code)
	}
}

func TestItemHandler_MissingID(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &itemHandler{dao: dao, ops: opsSet([]string{"read"}), plural: "dinosaurs"}
	w := newMockRW()
	// URL without id segment
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs/", nil))
	if w.code != 400 {
		t.Errorf("code = %d, want 400", w.code)
	}
}

// --------------------------------------------------------------------------
// itemHandler — PUT
// --------------------------------------------------------------------------

func TestItemHandler_Put_Success(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectExec("UPDATE dinosaurs SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE id").
		WithArgs("abc-123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "species"}).AddRow("abc-123", "Raptor"))

	h := &itemHandler{dao: dao, ops: opsSet([]string{"update"}), plural: "dinosaurs"}
	w := newMockRW()
	body, _ := json.Marshal(map[string]any{"species": "Raptor"})
	h.ServeHTTP(w, newRequest("PUT", "/dinosaurs/abc-123", body))

	if w.code != 200 {
		t.Errorf("code = %d, want 200", w.code)
	}
}

func TestItemHandler_Put_InvalidJSON(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &itemHandler{dao: dao, ops: opsSet([]string{"update"}), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("PUT", "/dinosaurs/abc-123", []byte("bad")))
	if w.code != 400 {
		t.Errorf("code = %d, want 400", w.code)
	}
}

func TestItemHandler_Put_NotAllowed(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &itemHandler{dao: dao, ops: opsSet(nil), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("PUT", "/dinosaurs/abc-123", []byte("{}")))
	if w.code != 405 {
		t.Errorf("code = %d, want 405", w.code)
	}
}

func TestItemHandler_Put_NoWritableFields(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &itemHandler{dao: dao, ops: opsSet([]string{"update"}), plural: "dinosaurs"}
	w := newMockRW()
	// Send only system columns — all will be filtered out.
	body, _ := json.Marshal(map[string]any{"id": "abc", "created_at": "2020-01-01"})
	h.ServeHTTP(w, newRequest("PUT", "/dinosaurs/abc-123", body))
	if w.code != 400 {
		t.Errorf("code = %d, want 400 (no writable fields)", w.code)
	}
}

func TestItemHandler_Put_DBError(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectExec("UPDATE dinosaurs SET").WillReturnError(sql.ErrConnDone)

	h := &itemHandler{dao: dao, ops: opsSet([]string{"update"}), plural: "dinosaurs"}
	w := newMockRW()
	body, _ := json.Marshal(map[string]any{"species": "Rex"})
	h.ServeHTTP(w, newRequest("PUT", "/dinosaurs/abc-123", body))
	if w.code != 500 {
		t.Errorf("code = %d, want 500", w.code)
	}
	respBody := w.body.String()
	if strings.Contains(respBody, sql.ErrConnDone.Error()) {
		t.Errorf("response body leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

// --------------------------------------------------------------------------
// itemHandler — DELETE
// --------------------------------------------------------------------------

func TestItemHandler_Delete_Success(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectExec("DELETE FROM dinosaurs WHERE id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := &itemHandler{dao: dao, ops: opsSet([]string{"delete"}), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("DELETE", "/dinosaurs/abc-123", nil))

	if w.code != 204 {
		t.Errorf("code = %d, want 204", w.code)
	}
}

func TestItemHandler_Delete_NotAllowed(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &itemHandler{dao: dao, ops: opsSet(nil), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("DELETE", "/dinosaurs/abc-123", nil))
	if w.code != 405 {
		t.Errorf("code = %d, want 405", w.code)
	}
}

func TestItemHandler_Delete_DBError(t *testing.T) {
	dao, mock := newTestDAO(t)
	mock.ExpectExec("DELETE FROM dinosaurs WHERE id").
		WillReturnError(sql.ErrConnDone)

	h := &itemHandler{dao: dao, ops: opsSet([]string{"delete"}), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("DELETE", "/dinosaurs/abc-123", nil))
	if w.code != 500 {
		t.Errorf("code = %d, want 500", w.code)
	}
	respBody := w.body.String()
	if strings.Contains(respBody, sql.ErrConnDone.Error()) {
		t.Errorf("response body leaks internal DB error: %s", respBody)
	}
	if !strings.Contains(respBody, "internal server error") {
		t.Errorf("expected generic error in body, got: %s", respBody)
	}
}

func TestItemHandler_UnknownMethod(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &itemHandler{dao: dao, ops: opsSet([]string{"read", "update", "delete"}), plural: "dinosaurs"}
	w := newMockRW()
	h.ServeHTTP(w, newRequest("PATCH", "/dinosaurs/abc-123", nil))
	if w.code != 405 {
		t.Errorf("code = %d, want 405", w.code)
	}
}

// --------------------------------------------------------------------------
// registerCRUDHandlers — integration smoke test via Register
// --------------------------------------------------------------------------

func TestRegister_RegistersCRUDHandlers(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	resources := []spec.ResourceDefinition{
		{Name: "Dinosaur", Plural: "dinosaurs", Operations: []string{"create", "read", "list"}},
		{Name: "Plant", Plural: "plants", Operations: []string{"read", "list"}},
	}
	app := spec.NewApplication(resources)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer c.db.db.Close()

	handlers := app.HTTPHandlers()
	// Expect 2 routes per resource: collection + item = 4 total
	if len(handlers) != 4 {
		t.Errorf("handler count = %d, want 4 (2 per resource)", len(handlers))
	}
	patterns := make(map[string]bool)
	for _, h := range handlers {
		patterns[h.Pattern] = true
	}
	for _, want := range []string{"/dinosaurs", "/dinosaurs/", "/plants", "/plants/"} {
		if !patterns[want] {
			t.Errorf("missing handler pattern %q; got: %v", want, handlers)
		}
	}
}

func TestRegisterCRUDHandlers_EmptyPluralFallback(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"dsn": badDSN}); err != nil {
		t.Fatal(err)
	}
	// Resource with empty Plural — should fall back to lowercase name + "s"
	resources := []spec.ResourceDefinition{
		{Name: "Widget", Operations: []string{"list"}},
	}
	app := spec.NewApplication(resources)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer c.db.db.Close()

	patterns := make(map[string]bool)
	for _, h := range app.HTTPHandlers() {
		patterns[h.Pattern] = true
	}
	if !patterns["/widgets"] {
		t.Errorf("expected /widgets route from fallback; got: %v", patterns)
	}
}

// --------------------------------------------------------------------------
// validateRequired
// --------------------------------------------------------------------------

func TestValidateRequired_AllPresent(t *testing.T) {
	fields := []spec.FieldDefinition{
		{Name: "species", Type: "string", Required: true},
		{Name: "description", Type: "string", Required: false},
	}
	missing := validateRequired(fields, map[string]any{"species": "T-Rex", "description": "big"})
	if len(missing) != 0 {
		t.Errorf("expected no missing fields, got: %v", missing)
	}
}

func TestValidateRequired_MissingOne(t *testing.T) {
	fields := []spec.FieldDefinition{
		{Name: "species", Type: "string", Required: true},
		{Name: "region", Type: "string", Required: true},
	}
	missing := validateRequired(fields, map[string]any{"species": "T-Rex"})
	if len(missing) != 1 || missing[0] != "region" {
		t.Errorf("expected [region], got %v", missing)
	}
}

func TestValidateRequired_AutoFieldExcluded(t *testing.T) {
	// Auto-managed fields should not be treated as required for input.
	fields := []spec.FieldDefinition{
		{Name: "species", Type: "string", Required: true},
		{Name: "created_at", Type: "timestamp", Required: true, Auto: "created"},
	}
	missing := validateRequired(fields, map[string]any{"species": "T-Rex"})
	if len(missing) != 0 {
		t.Errorf("auto fields should not be required in input, missing: %v", missing)
	}
}

// TestResourceDAO_List_PageOffset verifies that page=1 returns OFFSET 0,
// page=2 returns OFFSET size, etc. (1-based pagination).
func TestResourceDAO_List_PageOffset(t *testing.T) {
	cases := []struct {
		page, size int
		wantOffset int
	}{
		{1, 20, 0},
		{2, 20, 20},
		{3, 10, 20},
	}
	for _, tc := range cases {
		dao, mock := newTestDAO(t)
		mock.ExpectQuery("SELECT .* FROM dinosaurs WHERE deleted_at IS NULL ORDER BY id LIMIT").
			WithArgs(tc.size, tc.wantOffset).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		_, err := dao.List(context.Background(), tc.page, tc.size)
		if err != nil {
			t.Errorf("page=%d size=%d: %v", tc.page, tc.size, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("page=%d size=%d: expectations not met: %v", tc.page, tc.size, err)
		}
	}
}
