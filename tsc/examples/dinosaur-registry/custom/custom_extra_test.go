package custom

// custom_extra_test.go adds coverage for branches missed by custom_test.go:
//   enricher.Middleware: pass-through for non-dinosaur URL
//   enricher.enrich: empty body early return
//   capturingResponseWriter.Header: called when inner handler sets headers
//   validator.Middleware: malformed JSON pass-through, empty species pass-through
//   register.Register: invokes the /custom/version handler body

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

// --------------------------------------------------------------------------
// DescriptionEnricher.Middleware — pass-through for non-dinosaur URLs
// --------------------------------------------------------------------------

func TestDescriptionEnricher_PassesThroughNonDinosaurURL(t *testing.T) {
	e := NewDescriptionEnricher("svc", "v1")
	mw := e.Middleware()

	var called bool
	next := handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		called = true
		w.WriteHeader(200)
	})
	handler := mw(next)

	rw := &capturingResponseWriter{headers: make(map[string][]string), statusCode: 200}
	handler.ServeHTTP(rw, &spec.Request{Method: "GET", URL: "/api/v1/other-resource"})

	if !called {
		t.Error("next handler not called for non-dinosaur GET URL")
	}
}

func TestDescriptionEnricher_PassesThroughPostDinosaur(t *testing.T) {
	// POST to /dinosaurs — should pass through (not enriched).
	e := NewDescriptionEnricher("svc", "v1")
	mw := e.Middleware()

	var called bool
	next := handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		called = true
		w.WriteHeader(201)
	})
	handler := mw(next)

	rw := &capturingResponseWriter{headers: make(map[string][]string), statusCode: 200}
	handler.ServeHTTP(rw, &spec.Request{Method: "POST", URL: "/api/v1/dinosaurs"})

	if !called {
		t.Error("next handler not called for non-GET dinosaur request")
	}
}

// --------------------------------------------------------------------------
// enrich — empty body early return
// --------------------------------------------------------------------------

func TestEnrich_EmptyBody(t *testing.T) {
	e := NewDescriptionEnricher("svc", "v1")
	result := e.enrich(nil)
	if result != nil {
		t.Errorf("expected nil for nil body, got %v", result)
	}
	result = e.enrich([]byte{})
	if len(result) != 0 {
		t.Errorf("expected empty for empty body, got %v", result)
	}
}

// --------------------------------------------------------------------------
// capturingResponseWriter.Header — called when inner handler sets headers
// --------------------------------------------------------------------------

func TestDescriptionEnricher_InnerHandlerSetsHeader(t *testing.T) {
	e := NewDescriptionEnricher("svc", "v1")
	mw := e.Middleware()

	next := handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		// Calling w.Header() exercises capturingResponseWriter.Header()
		w.Header()["Content-Type"] = []string{"application/json"}
		w.WriteHeader(200)
		body, _ := json.Marshal(map[string]string{"id": "1"})
		w.Write(body) //nolint:errcheck
	})
	handler := mw(next)

	outer := &capturingResponseWriter{headers: make(map[string][]string), statusCode: 200}
	handler.ServeHTTP(outer, &spec.Request{Method: "GET", URL: "/api/v1/dinosaurs/1"})

	// Content-Type should have been forwarded.
	if outer.headers["Content-Type"] == nil {
		t.Error("expected Content-Type header to be forwarded to outer writer")
	}
}

// --------------------------------------------------------------------------
// SpeciesValidator.Middleware — malformed JSON pass-through
// --------------------------------------------------------------------------

func TestSpeciesValidator_MalformedJSON_PassesThrough(t *testing.T) {
	v := NewSpeciesValidator()
	mw := v.Middleware()

	var called bool
	next := handlerFunc(func(w spec.ResponseWriter, r *spec.Request) { called = true })
	handler := mw(next)

	// POST with malformed JSON body — validator lets it through.
	req := &spec.Request{Method: "POST", URL: "/api/v1/dinosaurs", Body: []byte("not-json{")}
	handler.ServeHTTP(&nopWriter{}, req)

	if !called {
		t.Error("expected next handler to be called when JSON is malformed (let foundry-postgres handle it)")
	}
}

// --------------------------------------------------------------------------
// SpeciesValidator.Middleware — empty species field pass-through
// --------------------------------------------------------------------------

func TestSpeciesValidator_EmptySpecies_PassesThrough(t *testing.T) {
	v := NewSpeciesValidator()
	mw := v.Middleware()

	var called bool
	next := handlerFunc(func(w spec.ResponseWriter, r *spec.Request) { called = true })
	handler := mw(next)

	// POST with JSON body but no species field → species == "" → pass through.
	body, _ := json.Marshal(map[string]string{"name": "something"})
	req := &spec.Request{Method: "POST", URL: "/api/v1/dinosaurs", Body: body}
	handler.ServeHTTP(&nopWriter{}, req)

	if !called {
		t.Error("expected next handler to be called when species field is absent")
	}
}

// --------------------------------------------------------------------------
// Register — exercise the /custom/version handler body
// --------------------------------------------------------------------------

func TestRegister_CustomVersionHandler(t *testing.T) {
	app := spec.NewApplication(nil)
	Register(app, "test-svc", "v0.2.0")

	// Find the /custom/version handler and invoke it.
	var versionHandler spec.HTTPHandler
	for _, h := range app.HTTPHandlers() {
		if strings.Contains(h.Pattern, "/custom/version") {
			versionHandler = h.Handler
		}
	}
	if versionHandler == nil {
		t.Fatal("/custom/version handler not registered")
	}

	rw := &nopWriter{}
	versionHandler.ServeHTTP(rw, &spec.Request{
		Method:  "GET",
		URL:     "/custom/version",
		Context: context.Background(),
	})

	if rw.statusCode != 200 {
		t.Errorf("expected 200, got %d", rw.statusCode)
	}
	var resp map[string]any
	if err := json.Unmarshal(rw.body, &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if resp["service"] != "test-svc" {
		t.Errorf("service = %v, want test-svc", resp["service"])
	}
	if resp["version"] != "v0.2.0" {
		t.Errorf("version = %v, want v0.2.0", resp["version"])
	}
}
