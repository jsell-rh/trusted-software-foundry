package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// --- ListCursor DAO tests ---

func TestListCursor_FirstPage(t *testing.T) {
	dao, mock := newTestDAO(t)

	// Expect: size+1 = 21 rows fetched, return 21 to signal a next page exists.
	rows := sqlmock.NewRows([]string{"id", "species"})
	for i := 1; i <= 21; i++ {
		rows.AddRow(fmt.Sprintf("id-%02d", i), fmt.Sprintf("Species%d", i))
	}
	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs(21). // size+1
		WillReturnRows(rows)

	results, err := dao.ListCursor(context.Background(), "", 20)
	if err != nil {
		t.Fatalf("ListCursor: %v", err)
	}
	if len(results) != 21 {
		t.Errorf("got %d rows, want 21 (including lookahead)", len(results))
	}
}

func TestListCursor_WithAfterID(t *testing.T) {
	dao, mock := newTestDAO(t)

	rows := sqlmock.NewRows([]string{"id", "species"}).AddRow("id-05", "Raptor")
	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs("id-04", 6). // afterID, size+1
		WillReturnRows(rows)

	results, err := dao.ListCursor(context.Background(), "id-04", 5)
	if err != nil {
		t.Fatalf("ListCursor with afterID: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d rows, want 1", len(results))
	}
}

func TestListCursor_Empty(t *testing.T) {
	dao, mock := newTestDAO(t)

	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs(11).
		WillReturnRows(sqlmock.NewRows([]string{"id", "species"}))

	results, err := dao.ListCursor(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("ListCursor empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d rows, want 0", len(results))
	}
}

func TestListCursor_DBError(t *testing.T) {
	dao, mock := newTestDAO(t)

	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs(11).
		WillReturnError(fmt.Errorf("db timeout"))

	_, err := dao.ListCursor(context.Background(), "", 10)
	if err == nil {
		t.Error("expected error from db")
	}
}

// --- handleListCursor handler tests ---

func TestHandleListCursor_FirstPage_HasNextCursor(t *testing.T) {
	dao, mock := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}

	// Return size+1=6 rows to indicate next page
	rows := sqlmock.NewRows([]string{"id", "species"})
	for i := 1; i <= 6; i++ {
		rows.AddRow(fmt.Sprintf("id-%02d", i), fmt.Sprintf("Species%d", i))
	}
	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs(6).
		WillReturnRows(rows)

	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs?cursor=&size=5", nil))

	if w.code != 200 {
		t.Fatalf("status = %d, want 200", w.code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items := resp["items"].([]any)
	if len(items) != 5 {
		t.Errorf("items count = %d, want 5 (truncated to size)", len(items))
	}
	nc, ok := resp["next_cursor"].(string)
	if !ok || nc == "" {
		t.Error("next_cursor should be present when more rows available")
	}
	// next_cursor should decode to the last item's id
	dec, err := base64.StdEncoding.DecodeString(nc)
	if err != nil {
		t.Fatalf("next_cursor base64 decode: %v", err)
	}
	if string(dec) != "id-05" {
		t.Errorf("decoded cursor = %q, want 'id-05'", string(dec))
	}
}

func TestHandleListCursor_LastPage_NoNextCursor(t *testing.T) {
	dao, mock := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}

	// Return only 3 rows (< size=5), no next page
	rows := sqlmock.NewRows([]string{"id", "species"}).
		AddRow("id-01", "T-Rex").
		AddRow("id-02", "Raptor").
		AddRow("id-03", "Triceratops")
	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs(6).
		WillReturnRows(rows)

	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs?cursor=&size=5", nil))

	if w.code != 200 {
		t.Fatalf("status = %d, want 200", w.code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, hasNextCursor := resp["next_cursor"]; hasNextCursor {
		t.Error("next_cursor should not be present on last page")
	}
	items := resp["items"].([]any)
	if len(items) != 3 {
		t.Errorf("items = %d, want 3", len(items))
	}
}

func TestHandleListCursor_WithCursor_DecodesAfterID(t *testing.T) {
	dao, mock := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}

	encoded := base64.StdEncoding.EncodeToString([]byte("id-10"))

	rows := sqlmock.NewRows([]string{"id", "species"}).AddRow("id-11", "Allosaurus")
	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs("id-10", 6).
		WillReturnRows(rows)

	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs?cursor="+encoded+"&size=5", nil))

	if w.code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.code, w.body.String())
	}
}

func TestHandleListCursor_InvalidCursor(t *testing.T) {
	dao, _ := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}

	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs?cursor=!!!notbase64", nil))

	if w.code != 400 {
		t.Errorf("status = %d, want 400 for invalid cursor", w.code)
	}
	if !strings.Contains(w.body.String(), "cursor") {
		t.Errorf("error body should mention 'cursor', got: %s", w.body.String())
	}
}

func TestHandleListCursor_DBError(t *testing.T) {
	dao, mock := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}

	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).WillReturnError(fmt.Errorf("db fail"))

	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs?cursor=&size=5", nil))

	if w.code != 500 {
		t.Errorf("status = %d, want 500", w.code)
	}
}

func TestHandleListCursor_DefaultSize(t *testing.T) {
	dao, mock := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}

	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs(21). // default size=20, fetch 21
		WillReturnRows(sqlmock.NewRows([]string{"id", "species"}))

	w := newMockRW()
	// cursor= present but size omitted → default 20
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs?cursor=", nil))

	if w.code != 200 {
		t.Fatalf("status = %d, want 200", w.code)
	}
}

func TestHandleListCursor_OffsetFallback(t *testing.T) {
	// Without cursor param, regular offset pagination still works
	dao, mock := newTestDAO(t)
	h := &collectionHandler{dao: dao, ops: opsSet([]string{"list"}), resource: dao.resource}

	// Count + list (offset mode)
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(`SELECT \* FROM dinosaurs`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("id-01").AddRow("id-02").AddRow("id-03"))

	w := newMockRW()
	h.ServeHTTP(w, newRequest("GET", "/dinosaurs", nil)) // no cursor param

	if w.code != 200 {
		t.Fatalf("status = %d, want 200", w.code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Offset mode response has "page" and "total" fields
	if _, ok := resp["total"]; !ok {
		t.Error("offset mode response should have 'total' field")
	}
}
