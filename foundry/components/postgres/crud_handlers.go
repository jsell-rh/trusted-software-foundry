package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jsell-rh/trusted-software-foundry/foundry/components/postgres/filter"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// ctx returns r.Context if non-nil, otherwise context.Background().
// Protects against requests created without a context (e.g. in tests).
func ctx(r *spec.Request) context.Context {
	if c := r.Context; c != nil {
		return c
	}
	return context.Background()
}

// registerCRUDHandlers registers REST CRUD handlers for all resource operations
// declared in the IR spec. Called from Register() after DAOs are created.
//
// Routes registered per resource (example: resource "Dinosaur", plural "dinosaurs"):
//
//	GET    /<basePath>/dinosaurs          → list (paginated, ?page=1&size=20)
//	POST   /<basePath>/dinosaurs          → create
//	GET    /<basePath>/dinosaurs/<id>     → read by id
//	PUT    /<basePath>/dinosaurs/<id>     → update by id
//	DELETE /<basePath>/dinosaurs/<id>     → delete by id
//
// Routes are registered on the spec.Application; foundry-http wires them into
// the HTTP server with the configured base_path prefix.
func (c *Component) registerCRUDHandlers(app *spec.Application) {
	for _, res := range c.cfg.Resources {
		dao := c.daos[res.Name]
		if dao == nil {
			continue
		}
		ops := opsSet(res.Operations)
		plural := res.Plural
		if plural == "" {
			plural = strings.ToLower(res.Name) + "s"
		}

		// Collection endpoint: GET /<plural> and POST /<plural>
		app.AddHTTPHandler("/"+plural, &collectionHandler{dao: dao, ops: ops, resource: res})

		// Item endpoint: GET /<plural>/<id>, PUT /<plural>/<id>, DELETE /<plural>/<id>
		app.AddHTTPHandler("/"+plural+"/", &itemHandler{dao: dao, ops: ops, plural: plural})
	}
}

// opsSet converts an operations slice into a fast lookup set.
func opsSet(ops []string) map[string]bool {
	s := make(map[string]bool, len(ops))
	for _, op := range ops {
		s[op] = true
	}
	return s
}

// collectionHandler handles POST (create) and GET (list) on the collection path.
type collectionHandler struct {
	dao      *resourceDAO
	ops      map[string]bool
	resource spec.ResourceDefinition
}

func (h *collectionHandler) ServeHTTP(w spec.ResponseWriter, r *spec.Request) {
	switch r.Method {
	case "GET":
		if !h.ops["list"] {
			writeError(w, 405, "list not allowed for this resource")
			return
		}
		h.handleList(w, r)
	case "POST":
		if !h.ops["create"] {
			writeError(w, 405, "create not allowed for this resource")
			return
		}
		h.handleCreate(w, r)
	default:
		writeError(w, 405, "method not allowed")
	}
}

func (h *collectionHandler) handleList(w spec.ResponseWriter, r *spec.Request) {
	page, size := 1, 20
	if q := queryParam(r.URL, "page"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			page = v
		}
	}
	if q := queryParam(r.URL, "size"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 200 {
			size = v
		}
	}

	// Parse optional ?search= filter parameter.
	searchParam := queryParam(r.URL, "search")
	var (
		whereSQL  string
		whereArgs []any
	)
	if searchParam != "" {
		// URL-decode the search value (query params may be percent-encoded).
		decoded := urlDecodeSimple(searchParam)
		var err error
		whereSQL, whereArgs, err = filter.BuildWhere(decoded, h.dao.allowedFilterFields())
		if err != nil {
			writeError(w, 400, "invalid search filter: "+err.Error())
			return
		}
	}

	c := ctx(r)
	var (
		total int64
		items []map[string]any
		err   error
	)

	if whereSQL != "" {
		total, err = h.dao.CountSearch(c, whereSQL, whereArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "foundry-postgres: count search %s: %v\n", h.resource.Plural, err)
			writeError(w, 500, "internal server error")
			return
		}
		items, err = h.dao.Search(c, whereSQL, whereArgs, page, size)
	} else {
		total, err = h.dao.Count(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "foundry-postgres: count %s: %v\n", h.resource.Plural, err)
			writeError(w, 500, "internal server error")
			return
		}
		items, err = h.dao.List(c, page, size)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "foundry-postgres: list %s: %v\n", h.resource.Plural, err)
		writeError(w, 500, "internal server error")
		return
	}

	resp := map[string]any{
		"kind":  h.resource.Name + "List",
		"page":  page,
		"size":  size,
		"total": total,
		"items": items,
	}
	writeJSON(w, 200, resp)
}

// urlDecodeSimple percent-decodes a query param value (replaces + with space,
// and %XX with the corresponding byte). Non-decodable sequences are left as-is.
func urlDecodeSimple(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '+' {
			b.WriteByte(' ')
			i++
			continue
		}
		if s[i] == '%' && i+2 < len(s) {
			hi := hexNibble(s[i+1])
			lo := hexNibble(s[i+2])
			if hi >= 0 && lo >= 0 {
				b.WriteByte(byte(hi<<4 | lo))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func (h *collectionHandler) handleCreate(w spec.ResponseWriter, r *spec.Request) {
	var obj map[string]any
	if err := json.Unmarshal(r.Body, &obj); err != nil {
		writeError(w, 400, "invalid JSON: "+err.Error())
		return
	}

	// Normalise keys to lowercase before validation so field name comparisons are case-insensitive.
	normalised := make(map[string]any, len(obj))
	for k, v := range obj {
		normalised[strings.ToLower(k)] = v
	}

	if missing := validateRequired(h.resource.Fields, normalised); len(missing) > 0 {
		writeError(w, 400, "missing required fields: "+strings.Join(missing, ", "))
		return
	}

	if errs := validateFieldLengths(h.resource.Fields, normalised); len(errs) > 0 {
		writeError(w, 400, "field validation failed: "+strings.Join(errs, "; "))
		return
	}

	id, err := h.dao.Create(ctx(r), normalised)
	if err != nil {
		fmt.Fprintf(os.Stderr, "foundry-postgres: create %s: %v\n", h.resource.Name, err)
		writeError(w, 500, "internal server error")
		return
	}

	// Return the created resource by id.
	created, err := h.dao.Get(ctx(r), id)
	if err != nil {
		// Return minimal response if re-fetch fails.
		writeJSON(w, 201, map[string]any{"id": id})
		return
	}
	writeJSON(w, 201, created)
}

// itemHandler handles GET/PUT/DELETE on /<plural>/<id>.
type itemHandler struct {
	dao    *resourceDAO
	ops    map[string]bool
	plural string
}

func (h *itemHandler) ServeHTTP(w spec.ResponseWriter, r *spec.Request) {
	id := extractID(r.URL, h.plural)
	if id == "" {
		writeError(w, 400, "missing resource id")
		return
	}

	switch r.Method {
	case "GET":
		if !h.ops["read"] {
			writeError(w, 405, "read not allowed for this resource")
			return
		}
		obj, err := h.dao.Get(ctx(r), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, 404, "not found")
			} else {
				fmt.Fprintf(os.Stderr, "foundry-postgres: get %s %s: %v\n", h.plural, id, err)
				writeError(w, 500, "internal server error")
			}
			return
		}
		writeJSON(w, 200, obj)

	case "PUT":
		if !h.ops["update"] {
			writeError(w, 405, "update not allowed for this resource")
			return
		}
		var patch map[string]any
		if err := json.Unmarshal(r.Body, &patch); err != nil {
			writeError(w, 400, "invalid JSON: "+err.Error())
			return
		}
		if err := h.dao.Update(ctx(r), id, patch); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, 404, "not found")
			} else if strings.Contains(err.Error(), "no writable fields") {
				writeError(w, 400, err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "foundry-postgres: update %s %s: %v\n", h.plural, id, err)
				writeError(w, 500, "internal server error")
			}
			return
		}
		obj, err := h.dao.Get(ctx(r), id)
		if err != nil {
			writeJSON(w, 200, map[string]any{"id": id})
			return
		}
		writeJSON(w, 200, obj)

	case "DELETE":
		if !h.ops["delete"] {
			writeError(w, 405, "delete not allowed for this resource")
			return
		}
		if err := h.dao.Delete(ctx(r), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, 404, "not found")
			} else {
				fmt.Fprintf(os.Stderr, "foundry-postgres: delete %s %s: %v\n", h.plural, id, err)
				writeError(w, 500, "internal server error")
			}
			return
		}
		w.WriteHeader(204)

	default:
		writeError(w, 405, "method not allowed")
	}
}

// --- helpers ---

// validateRequired checks that all required fields declared in the resource definition
// are present (with a non-nil value) in the input map. Returns a list of missing field
// names; an empty slice means the input is valid.
func validateRequired(fields []spec.FieldDefinition, input map[string]any) []string {
	var missing []string
	for _, f := range fields {
		if !f.Required || f.Auto != "" || f.SoftDelete {
			continue
		}
		name := strings.ToLower(f.Name)
		if v, ok := input[name]; !ok || v == nil {
			missing = append(missing, f.Name)
		}
	}
	return missing
}

// validateFieldLengths checks string fields against their declared max_length constraint.
// Returns a slice of error messages (field: value too long, max N); empty slice = valid.
func validateFieldLengths(fields []spec.FieldDefinition, input map[string]any) []string {
	var errs []string
	for _, f := range fields {
		if f.MaxLength <= 0 || f.Type != "string" {
			continue
		}
		name := strings.ToLower(f.Name)
		v, ok := input[name]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if len(s) > f.MaxLength {
			errs = append(errs, fmt.Sprintf("%s: value too long (%d chars, max %d)", f.Name, len(s), f.MaxLength))
		}
	}
	return errs
}

func writeJSON(w spec.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		// Log internally; write a static error body to avoid calling back into
		// writeJSON (which would recurse) and to avoid leaking Go type details.
		fmt.Fprintf(os.Stderr, "foundry-postgres: marshal response: %v\n", err)
		w.Header()["Content-Type"] = []string{"application/json"}
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal server error","status":500}`)) //nolint:errcheck
		return
	}
	w.Header()["Content-Type"] = []string{"application/json"}
	w.WriteHeader(status)
	w.Write(data) //nolint:errcheck
}

func writeError(w spec.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "status": status})
}

// extractID pulls the id segment from a URL like /dinosaurs/abc-123.
// prefix is the plural resource name (e.g. "dinosaurs").
func extractID(rawURL, prefix string) string {
	// Strip query string.
	path := rawURL
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	// Pattern: /<prefix>/<id>
	seg := "/" + prefix + "/"
	idx := strings.Index(path, seg)
	if idx < 0 {
		return ""
	}
	id := path[idx+len(seg):]
	// Remove any trailing slashes.
	id = strings.Trim(id, "/")
	return id
}

// queryParam extracts a query parameter value from a raw URL string.
func queryParam(rawURL, key string) string {
	i := strings.Index(rawURL, "?")
	if i < 0 {
		return ""
	}
	query := rawURL[i+1:]
	for _, part := range strings.Split(query, "&") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && kv[0] == key {
			return kv[1]
		}
	}
	return ""
}
