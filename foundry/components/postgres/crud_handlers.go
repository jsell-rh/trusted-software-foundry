package postgres

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// registerCRUDHandlers registers REST CRUD handlers for all resource operations
// declared in the IR spec. Called from Register() after DAOs are created.
//
// Routes registered per resource (example: resource "Dinosaur", plural "dinosaurs"):
//   GET    /<basePath>/dinosaurs          → list (paginated, ?page=1&size=20)
//   POST   /<basePath>/dinosaurs          → create
//   GET    /<basePath>/dinosaurs/<id>     → read by id
//   PUT    /<basePath>/dinosaurs/<id>     → update by id
//   DELETE /<basePath>/dinosaurs/<id>     → delete by id
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

	items, err := h.dao.List(context.Background(), page, size)
	if err != nil {
		writeError(w, 500, "list failed: "+err.Error())
		return
	}

	resp := map[string]any{
		"kind":  h.resource.Name + "List",
		"page":  page,
		"size":  size,
		"total": len(items),
		"items": items,
	}
	writeJSON(w, 200, resp)
}

func (h *collectionHandler) handleCreate(w spec.ResponseWriter, r *spec.Request) {
	var obj map[string]any
	if err := json.Unmarshal(r.Body, &obj); err != nil {
		writeError(w, 400, "invalid JSON: "+err.Error())
		return
	}

	id, err := h.dao.Create(context.Background(), obj)
	if err != nil {
		writeError(w, 500, "create failed: "+err.Error())
		return
	}

	// Return the created resource by id.
	created, err := h.dao.Get(context.Background(), id)
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
		obj, err := h.dao.Get(context.Background(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, 404, "not found")
			} else {
				writeError(w, 500, err.Error())
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
		if err := h.dao.Update(context.Background(), id, patch); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, 404, "not found")
			} else {
				writeError(w, 500, err.Error())
			}
			return
		}
		obj, err := h.dao.Get(context.Background(), id)
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
		if err := h.dao.Delete(context.Background(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, 404, "not found")
			} else {
				writeError(w, 500, err.Error())
			}
			return
		}
		w.WriteHeader(204)

	default:
		writeError(w, 405, "method not allowed")
	}
}

// --- helpers ---

func writeJSON(w spec.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		writeError(w, 500, "marshal error: "+err.Error())
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
