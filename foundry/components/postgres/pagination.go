package postgres

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// handleListCursor handles GET /<plural>?cursor=<token>&size=<n>.
//
// Cursor encoding: base64(id) of the last seen item from the previous page.
// The cursor is opaque to callers — they must not parse or construct it.
//
// Response shape:
//
//	{
//	  "kind":        "<Name>List",
//	  "size":        <n>,
//	  "items":       [...],
//	  "next_cursor": "<base64-encoded-id>"  // omitted on last page
//	}
//
// Backwards compatibility: if no `cursor` query param is present, the handler
// falls through to the standard offset-based handleList.
func (h *collectionHandler) handleListCursor(w spec.ResponseWriter, r *spec.Request) {
	size := 20
	if q := queryParam(r.URL, "size"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 200 {
			size = v
		}
	}

	// Decode the cursor (base64-encoded last-seen id).
	afterID := ""
	if encoded := queryParam(r.URL, "cursor"); encoded != "" {
		dec, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			spec.NewBadRequestError("invalid cursor: must be a value returned by a previous list call").WriteHTTP(w)
			return
		}
		afterID = string(dec)
	}

	c := ctx(r)
	rows, err := h.dao.ListCursor(c, afterID, size)
	if err != nil {
		fmt.Fprintf(os.Stderr, "foundry-postgres: list-cursor %s: %v\n", h.resource.Plural, err)
		spec.NewInternalError(err).WriteHTTP(w)
		return
	}

	// Detect whether there is a next page.
	var nextCursor string
	if len(rows) > size {
		rows = rows[:size]                           // truncate to requested size
		lastID := fmt.Sprintf("%v", rows[len(rows)-1]["id"])
		nextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
	}

	resp := map[string]any{
		"kind":  h.resource.Name + "List",
		"size":  size,
		"items": rows,
	}
	if nextCursor != "" {
		resp["next_cursor"] = nextCursor
	}
	writeJSON(w, 200, resp)
}
