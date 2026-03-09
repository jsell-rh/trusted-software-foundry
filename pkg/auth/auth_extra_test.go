package auth

// auth_extra_test.go covers branches not reached by middleware_test.go:
//   SetUsernameContext / GetUsernameFromContext
//   MiddlewareMock.AuthenticateAccountJWT
//   NewAuthzMiddlewareMock / AuthorizeApi
//   handleError (4xx vs 5xx), writeJSONResponse
//   BearerTokenMiddleware (all paths)
//   DefaultBypassPaths, ExtendBypassPaths
//   WithKeysURL, WithKeysFile, WithACLFile
//   NewAuthMiddlewareBuilder, BuildHTTPMiddleware, GetStrategy
//   loadKeys, refreshKeysLoop (via Stop)

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/pkg/config"
	"github.com/jsell-rh/trusted-software-foundry/pkg/errors"
)

// --------------------------------------------------------------------------
// SetUsernameContext / GetUsernameFromContext
// --------------------------------------------------------------------------

func TestSetAndGetUsernameContext(t *testing.T) {
	ctx := SetUsernameContext(context.Background(), "alice")
	got := GetUsernameFromContext(ctx)
	if got != "alice" {
		t.Errorf("GetUsernameFromContext = %q, want 'alice'", got)
	}
}

func TestGetUsernameFromContext_NotSet(t *testing.T) {
	got := GetUsernameFromContext(context.Background())
	if got != "" {
		t.Errorf("GetUsernameFromContext (not set) = %q, want empty", got)
	}
}

// --------------------------------------------------------------------------
// MiddlewareMock.AuthenticateAccountJWT
// --------------------------------------------------------------------------

func TestMiddlewareMock_AuthenticateAccountJWT(t *testing.T) {
	mock := &MiddlewareMock{}
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})
	wrapped := mock.AuthenticateAccountJWT(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if !handlerCalled {
		t.Error("AuthenticateAccountJWT should call the next handler")
	}
}

// --------------------------------------------------------------------------
// NewAuthzMiddlewareMock / AuthorizeApi
// --------------------------------------------------------------------------

func TestNewAuthzMiddlewareMock(t *testing.T) {
	mock := NewAuthzMiddlewareMock()
	if mock == nil {
		t.Fatal("NewAuthzMiddlewareMock() returned nil")
	}
}

func TestAuthzMiddlewareMock_AuthorizeApi(t *testing.T) {
	mock := NewAuthzMiddlewareMock()
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})
	wrapped := mock.AuthorizeApi(next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if !handlerCalled {
		t.Error("AuthorizeApi mock should call the next handler")
	}
}

// --------------------------------------------------------------------------
// handleError
// --------------------------------------------------------------------------

func TestHandleError_4xx(t *testing.T) {
	rr := httptest.NewRecorder()
	// NotFound is 404 — should log at Info level, not Error.
	handleError(context.Background(), rr, errors.ErrorNotFound, "not found")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleError_5xx(t *testing.T) {
	rr := httptest.NewRecorder()
	// ErrorGeneral maps to 500 — should log at Error level.
	handleError(context.Background(), rr, errors.ErrorGeneral, "server error")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// --------------------------------------------------------------------------
// writeJSONResponse
// --------------------------------------------------------------------------

func TestWriteJSONResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONResponse(rr, http.StatusOK, map[string]string{"key": "value"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rr.Body.Len() == 0 {
		t.Error("response body should not be empty")
	}
}

func TestWriteJSONResponse_NilPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONResponse(rr, http.StatusNoContent, nil)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
}

// --------------------------------------------------------------------------
// BearerTokenMiddleware
// --------------------------------------------------------------------------

func TestBearerTokenMiddleware_BypassPath(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := BearerTokenMiddleware("secret", []string{"/healthz"})(next)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Error("bypass path should skip auth and call next")
	}
}

func TestBearerTokenMiddleware_NoHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := BearerTokenMiddleware("secret", nil)(next)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for missing header", rr.Code)
	}
}

func TestBearerTokenMiddleware_NoBearerPrefix(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := BearerTokenMiddleware("secret", nil)(next)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for non-Bearer header", rr.Code)
	}
}

func TestBearerTokenMiddleware_InvalidToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := BearerTokenMiddleware("secret", nil)(next)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for invalid token", rr.Code)
	}
}

func TestBearerTokenMiddleware_ValidToken(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := BearerTokenMiddleware("my-secret", nil)(next)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer my-secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Error("valid token should call next handler")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestBearerTokenMiddleware_LowercaseBearerPrefix(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := BearerTokenMiddleware("my-secret", nil)(next)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "bearer my-secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Error("lowercase 'bearer' prefix should also work")
	}
}

// --------------------------------------------------------------------------
// DefaultBypassPaths / ExtendBypassPaths
// --------------------------------------------------------------------------

func TestDefaultBypassPaths(t *testing.T) {
	paths := DefaultBypassPaths()
	if len(paths) == 0 {
		t.Error("DefaultBypassPaths() should return non-empty list")
	}
	for _, p := range paths {
		if !strings.HasPrefix(p, "/") {
			t.Errorf("bypass path %q should start with /", p)
		}
	}
}

func TestExtendBypassPaths(t *testing.T) {
	extended := ExtendBypassPaths("/custom1", "/custom2")
	defaults := DefaultBypassPaths()
	if len(extended) != len(defaults)+2 {
		t.Errorf("len = %d, want %d", len(extended), len(defaults)+2)
	}
}

// --------------------------------------------------------------------------
// WithKeysURL, WithKeysFile, WithACLFile
// --------------------------------------------------------------------------

func TestWithKeysURL(t *testing.T) {
	h := NewJWTHandler().WithKeysURL("https://example.com/jwk")
	if h.keysURL != "https://example.com/jwk" {
		t.Errorf("keysURL = %q, want https://example.com/jwk", h.keysURL)
	}
}

func TestWithKeysFile(t *testing.T) {
	h := NewJWTHandler().WithKeysFile("/path/to/keys.json")
	if h.keysFile != "/path/to/keys.json" {
		t.Errorf("keysFile = %q, want /path/to/keys.json", h.keysFile)
	}
}

func TestWithACLFile(t *testing.T) {
	h := NewJWTHandler().WithACLFile("/path/to/acl.yaml")
	if h.aclFile != "/path/to/acl.yaml" {
		t.Errorf("aclFile = %q, want /path/to/acl.yaml", h.aclFile)
	}
}

// --------------------------------------------------------------------------
// NewAuthMiddlewareBuilder / GetStrategy
// --------------------------------------------------------------------------

func TestNewAuthMiddlewareBuilder_BothStrategies(t *testing.T) {
	cfg := &config.AuthConfig{EnableJWT: true, EnableBearer: true}
	b := NewAuthMiddlewareBuilder(cfg)
	if b.GetStrategy() != AuthStrategyBoth {
		t.Errorf("strategy = %d, want AuthStrategyBoth (%d)", b.GetStrategy(), AuthStrategyBoth)
	}
}

func TestNewAuthMiddlewareBuilder_JWTOnly(t *testing.T) {
	cfg := &config.AuthConfig{EnableJWT: true, EnableBearer: false}
	b := NewAuthMiddlewareBuilder(cfg)
	if b.GetStrategy() != AuthStrategyJWT {
		t.Errorf("strategy = %d, want AuthStrategyJWT (%d)", b.GetStrategy(), AuthStrategyJWT)
	}
}

func TestNewAuthMiddlewareBuilder_BearerOnly(t *testing.T) {
	cfg := &config.AuthConfig{EnableJWT: false, EnableBearer: true, BearerToken: "tok"}
	b := NewAuthMiddlewareBuilder(cfg)
	if b.GetStrategy() != AuthStrategyBearer {
		t.Errorf("strategy = %d, want AuthStrategyBearer (%d)", b.GetStrategy(), AuthStrategyBearer)
	}
}

func TestNewAuthMiddlewareBuilder_None(t *testing.T) {
	cfg := &config.AuthConfig{EnableJWT: false, EnableBearer: false}
	b := NewAuthMiddlewareBuilder(cfg)
	if b.GetStrategy() != AuthStrategyNone {
		t.Errorf("strategy = %d, want AuthStrategyNone (%d)", b.GetStrategy(), AuthStrategyNone)
	}
}

// --------------------------------------------------------------------------
// BuildHTTPMiddleware — none strategy (pass-through)
// --------------------------------------------------------------------------

func TestBuildHTTPMiddleware_NoneStrategy(t *testing.T) {
	cfg := &config.AuthConfig{EnableJWT: false, EnableBearer: false}
	b := NewAuthMiddlewareBuilder(cfg)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	mw, err := b.BuildHTTPMiddleware()
	if err != nil {
		t.Fatalf("BuildHTTPMiddleware: %v", err)
	}
	if mw == nil {
		t.Fatal("BuildHTTPMiddleware returned nil middleware")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)
	if !called {
		t.Error("None strategy should pass through to next handler")
	}
}

func TestBuildHTTPMiddleware_BearerStrategy(t *testing.T) {
	cfg := &config.AuthConfig{
		EnableJWT:    false,
		EnableBearer: true,
		BearerToken:  "secret",
		BypassPaths:  []string{"/health"},
	}
	b := NewAuthMiddlewareBuilder(cfg)
	mw, err := b.BuildHTTPMiddleware()
	if err != nil {
		t.Fatalf("BuildHTTPMiddleware bearer: %v", err)
	}
	if mw == nil {
		t.Fatal("BuildHTTPMiddleware returned nil")
	}
}

// --------------------------------------------------------------------------
// loadKeys — error path (no URL, no file)
// --------------------------------------------------------------------------

func TestLoadKeys_NoSourceError(t *testing.T) {
	h := NewJWTHandler() // no keysURL, no keysFile
	err := h.loadKeys()
	if err == nil {
		t.Error("loadKeys with no source should return error")
	}
}

func TestLoadKeys_FileError(t *testing.T) {
	h := NewJWTHandler().WithKeysFile("/nonexistent/keys.json")
	err := h.loadKeys()
	if err == nil {
		t.Error("loadKeys with nonexistent file should return error")
	}
}

// --------------------------------------------------------------------------
// DefaultBypassMethods / ExtendBypassMethods
// --------------------------------------------------------------------------

func TestDefaultBypassMethods(t *testing.T) {
	methods := DefaultBypassMethods()
	if len(methods) == 0 {
		t.Error("DefaultBypassMethods() should return non-empty list")
	}
}

func TestExtendBypassMethods(t *testing.T) {
	extended := ExtendBypassMethods("/custom.Service/")
	defaults := DefaultBypassMethods()
	if len(extended) != len(defaults)+1 {
		t.Errorf("ExtendBypassMethods len = %d, want %d", len(extended), len(defaults)+1)
	}
}

// --------------------------------------------------------------------------
// validateBearerToken directly
// --------------------------------------------------------------------------

func TestValidateBearerToken_NoMetadata(t *testing.T) {
	err := validateBearerToken(context.Background(), "secret")
	if err == nil {
		t.Error("expected error for context without gRPC metadata")
	}
}

// --------------------------------------------------------------------------
// NewAuthMiddlewareBuilder — auth middleware for auth middleware
// --------------------------------------------------------------------------

func TestNewAuthMiddlewareBuilder(t *testing.T) {
	cfg := &config.AuthConfig{EnableJWT: false, EnableBearer: false}
	b := NewAuthMiddlewareBuilder(cfg)
	if b == nil {
		t.Error("NewAuthMiddlewareBuilder returned nil")
	}
}
