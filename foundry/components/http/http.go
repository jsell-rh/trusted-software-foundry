// Package http provides the foundry-http trusted component — a self-contained HTTP
// server that routes requests to handlers registered by peer components.
//
// Configuration (spec api.rest block):
//
//	api:
//	  rest:
//	    bind: ":8000"          # listen address (default :8000)
//	    base_path: /api/v1    # all handler patterns are prefixed with this
//	    cors:
//	      allowed_origins: ["*"]
//	    version_header: true  # emit X-TSF-Version header
//	    tls: false
package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-http"
	componentVersion = "v1.0.0"
	// auditHash is the SHA-256 of this source tree at audit time.
	// Re-computed by the audit pipeline and verified by forge compile.
	auditHash = "0000000000000000000000000000000000000000000000000000000000000001"
)

// HTTPComponent implements spec.Component for the HTTP layer.
// It owns the net/http server and wires all registered HTTPHandlers into it.
type HTTPComponent struct {
	mu     sync.Mutex
	cfg    config
	app    *spec.Application
	server *http.Server
	mux    *http.ServeMux
}

type config struct {
	bind           string
	basePath       string
	versionHeader  bool
	allowedOrigins []string
	readTimeout    time.Duration
	writeTimeout   time.Duration
	idleTimeout    time.Duration
	maxBodyBytes   int64
}

// defaultReadTimeout and friends are conservative production-safe defaults.
const (
	defaultReadTimeout  = 30 * time.Second
	defaultWriteTimeout = 30 * time.Second
	defaultIdleTimeout  = 120 * time.Second
	// defaultMaxBodyBytes limits request bodies to 4 MiB to prevent OOM on malicious uploads.
	defaultMaxBodyBytes int64 = 4 << 20
)

// New returns a new HTTPComponent with defaults.
func New() *HTTPComponent {
	return &HTTPComponent{
		cfg: config{
			bind:         ":8000",
			basePath:     "",
			readTimeout:  defaultReadTimeout,
			writeTimeout: defaultWriteTimeout,
			idleTimeout:  defaultIdleTimeout,
			maxBodyBytes: defaultMaxBodyBytes,
		},
	}
}

func (c *HTTPComponent) Name() string      { return componentName }
func (c *HTTPComponent) Version() string   { return componentVersion }
func (c *HTTPComponent) AuditHash() string { return auditHash }

// Configure reads the api.rest section from the ComponentConfig.
func (c *HTTPComponent) Configure(cfg spec.ComponentConfig) error {
	if bind, ok := cfg["bind"].(string); ok && bind != "" {
		c.cfg.bind = bind
	}
	if basePath, ok := cfg["base_path"].(string); ok {
		c.cfg.basePath = strings.TrimRight(basePath, "/")
	}
	if vh, ok := cfg["version_header"].(bool); ok {
		c.cfg.versionHeader = vh
	}
	if cors, ok := cfg["cors"].(map[string]any); ok {
		if origins, ok := cors["allowed_origins"].([]any); ok {
			for _, o := range origins {
				if s, ok := o.(string); ok {
					c.cfg.allowedOrigins = append(c.cfg.allowedOrigins, s)
				}
			}
		}
	}
	// Timeout overrides (values in seconds).
	if v, ok := cfg["read_timeout_sec"].(int); ok && v > 0 {
		c.cfg.readTimeout = time.Duration(v) * time.Second
	}
	if v, ok := cfg["write_timeout_sec"].(int); ok && v > 0 {
		c.cfg.writeTimeout = time.Duration(v) * time.Second
	}
	if v, ok := cfg["idle_timeout_sec"].(int); ok && v > 0 {
		c.cfg.idleTimeout = time.Duration(v) * time.Second
	}
	if v, ok := cfg["max_body_bytes"].(int); ok && v > 0 {
		c.cfg.maxBodyBytes = int64(v)
	}
	return nil
}

// Register stores a reference to the application so Start can wire handlers.
func (c *HTTPComponent) Register(app *spec.Application) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.app = app
	return nil
}

// Start builds the HTTP server with all registered handlers and begins serving.
func (c *HTTPComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mux = http.NewServeMux()

	// Wire handlers registered by peer components; log registered routes.
	handlers := c.app.HTTPHandlers()
	for _, entry := range handlers {
		pattern := c.cfg.basePath + entry.Pattern
		h := entry.Handler
		// Capture loop variables.
		c.mux.HandleFunc(pattern, c.adapt(h))
	}

	// Build middleware chain for all routes.
	var handler http.Handler = c.mux
	for i := len(c.app.Middlewares()) - 1; i >= 0; i-- {
		mw := c.app.Middlewares()[i]
		handler = c.adaptMiddleware(mw, handler)
	}

	// Apply built-in middleware: request logging (innermost), CORS, version header.
	handler = c.requestLogMiddleware(handler)
	if len(c.cfg.allowedOrigins) > 0 {
		handler = c.corsMiddleware(handler)
	}
	if c.cfg.versionHeader {
		handler = c.versionHeaderMiddleware(handler)
	}

	c.server = &http.Server{
		Addr:         c.cfg.bind,
		Handler:      handler,
		ReadTimeout:  c.cfg.readTimeout,
		WriteTimeout: c.cfg.writeTimeout,
		IdleTimeout:  c.cfg.idleTimeout,
	}

	// Log registered routes on startup.
	fmt.Printf("foundry-http: listening on %s\n", c.cfg.bind)
	for _, entry := range handlers {
		fmt.Printf("  route: %s%s\n", c.cfg.basePath, entry.Pattern)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Check for immediate startup error.
	select {
	case err := <-errCh:
		return fmt.Errorf("foundry-http: listen %s: %w", c.cfg.bind, err)
	default:
	}
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (c *HTTPComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	srv := c.server
	c.mu.Unlock()
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("foundry-http: shutdown: %w", err)
	}
	return nil
}

// adapt converts a spec.HTTPHandler into a net/http.HandlerFunc.
func (c *HTTPComponent) adapt(h spec.HTTPHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, c.cfg.maxBodyBytes)
		body, _ := io.ReadAll(r.Body)
		req := &spec.Request{
			Method:  r.Method,
			URL:     r.URL.String(),
			Headers: map[string][]string(r.Header),
			Body:    body,
			Context: r.Context(),
		}
		rw := &responseWriterAdapter{w: w}
		h.ServeHTTP(rw, req)
	}
}

// adaptMiddleware wraps a spec.HTTPMiddleware around a net/http.Handler.
func (c *HTTPComponent) adaptMiddleware(mw spec.HTTPMiddleware, next http.Handler) http.Handler {
	specNext := &handlerAdapter{h: next}
	wrapped := mw(specNext)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, c.cfg.maxBodyBytes)
		body, _ := io.ReadAll(r.Body)
		req := &spec.Request{
			Method:  r.Method,
			URL:     r.URL.String(),
			Headers: map[string][]string(r.Header),
			Body:    body,
			Context: r.Context(),
		}
		wrapped.ServeHTTP(&responseWriterAdapter{w: w}, req)
	})
}

// requestLogMiddleware logs each HTTP request: method, path, status, and duration.
func (c *HTTPComponent) requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(lw, r)
		fmt.Printf("foundry-http: %s %s %d %s\n", r.Method, r.URL.Path, lw.code, time.Since(start))
	})
}

// loggingResponseWriter captures the HTTP status code for request logging.
type loggingResponseWriter struct {
	http.ResponseWriter
	code int
}

func (l *loggingResponseWriter) WriteHeader(code int) {
	l.code = code
	l.ResponseWriter.WriteHeader(code)
}

func (c *HTTPComponent) corsMiddleware(next http.Handler) http.Handler {
	// Build an O(1) lookup set; track whether wildcard is configured.
	wildcard := false
	allowed := make(map[string]bool, len(c.cfg.allowedOrigins))
	for _, o := range c.cfg.allowedOrigins {
		if o == "*" {
			wildcard = true
		} else {
			allowed[strings.ToLower(o)] = true
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		var matchedOrigin string
		if wildcard {
			// Access-Control-Allow-Origin: * cannot be used with credentials;
			// return "*" unconditionally — credentials must be excluded by callers.
			matchedOrigin = "*"
		} else if origin != "" && allowed[strings.ToLower(origin)] {
			// Reflect the specific matching origin so the browser trusts it.
			// Per-origin responses must vary by Origin for correct cache behaviour.
			matchedOrigin = origin
			w.Header().Add("Vary", "Origin")
		}

		if matchedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", matchedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (c *HTTPComponent) versionHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TSF-Version", componentVersion)
		next.ServeHTTP(w, r)
	})
}

// responseWriterAdapter adapts net/http.ResponseWriter to spec.ResponseWriter.
type responseWriterAdapter struct {
	w http.ResponseWriter
}

func (a *responseWriterAdapter) Header() map[string][]string {
	return map[string][]string(a.w.Header())
}

func (a *responseWriterAdapter) Write(b []byte) (int, error) {
	return a.w.Write(b)
}

func (a *responseWriterAdapter) WriteHeader(code int) {
	a.w.WriteHeader(code)
}

// handlerAdapter adapts a net/http.Handler to spec.HTTPHandler.
type handlerAdapter struct {
	h http.Handler
}

func (a *handlerAdapter) ServeHTTP(w spec.ResponseWriter, r *spec.Request) {
	req, _ := http.NewRequestWithContext(r.Context, r.Method, r.URL, nil)
	for k, vs := range r.Headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	a.h.ServeHTTP(&netHTTPResponseWriter{w: w}, req)
}

// netHTTPResponseWriter adapts spec.ResponseWriter back to net/http.ResponseWriter.
type netHTTPResponseWriter struct {
	w spec.ResponseWriter
}

func (n *netHTTPResponseWriter) Header() http.Header {
	return http.Header(n.w.Header())
}

func (n *netHTTPResponseWriter) Write(b []byte) (int, error) {
	return n.w.Write(b)
}

func (n *netHTTPResponseWriter) WriteHeader(code int) {
	n.w.WriteHeader(code)
}
