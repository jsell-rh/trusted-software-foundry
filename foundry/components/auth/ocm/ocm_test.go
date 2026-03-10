package ocm_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	autcocm "github.com/jsell-rh/trusted-software-foundry/foundry/components/auth/ocm"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// Test RSA key (1024-bit — speed over security for tests)
// --------------------------------------------------------------------------

var (
	testKey *rsa.PrivateKey
	testKID = "test-kid-1"
)

func init() {
	var err error
	testKey, err = rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic("generate test RSA key: " + err.Error())
	}
}

// --------------------------------------------------------------------------
// JWT construction helpers
// --------------------------------------------------------------------------

// buildJWT creates a signed RS256 JWT from the given claims.
func buildJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]interface{}) string {
	t.Helper()

	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	pay, _ := json.Marshal(claims)

	h := base64.RawURLEncoding.EncodeToString(hdr)
	p := base64.RawURLEncoding.EncodeToString(pay)
	signingInput := h + "." + p

	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// validClaims returns a set of claims that expire in the future.
func validClaims(expiresIn time.Duration) map[string]interface{} {
	return map[string]interface{}{
		"sub":                "user-123",
		"preferred_username": "alice",
		"email":              "alice@example.com",
		"org_id":             "org-abc",
		"account_id":         "acct-456",
		"is_org_admin":       true,
		"iss":                "https://sso.redhat.com/auth/realms/redhat-external",
		"aud":                "openshift",
		"exp":                time.Now().Add(expiresIn).Unix(),
		"iat":                time.Now().Unix(),
	}
}

// --------------------------------------------------------------------------
// Mock JWKS server
// --------------------------------------------------------------------------

func jwksServer(t *testing.T, pub *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"keys": []interface{}{
			map[string]interface{}{
				"kid": kid,
				"kty": "RSA",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --------------------------------------------------------------------------
// Component factory
// --------------------------------------------------------------------------

func startedComponent(t *testing.T, jwksURL string, extraCfg spec.ComponentConfig) *autcocm.Component {
	t.Helper()
	c := autcocm.New()
	cfg := spec.ComponentConfig{"jwks_url": jwksURL}
	for k, v := range extraCfg {
		cfg[k] = v
	}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { c.Stop(context.Background()) }) //nolint:errcheck
	return c
}

// --------------------------------------------------------------------------
// Test doubles
// --------------------------------------------------------------------------

type fakeResponseWriter struct {
	header     map[string][]string
	statusCode int
	body       []byte
}

func (w *fakeResponseWriter) Header() map[string][]string { return w.header }
func (w *fakeResponseWriter) WriteHeader(code int)        { w.statusCode = code }
func (w *fakeResponseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return len(b), nil
}

type captureHandler struct {
	called bool
	ctx    context.Context
}

func (h *captureHandler) ServeHTTP(w spec.ResponseWriter, r *spec.Request) {
	h.called = true
	h.ctx = r.Context
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestComponent_ImplementsSpec(t *testing.T) {
	var _ spec.Component = autcocm.New()
}

func TestComponent_Identity(t *testing.T) {
	c := autcocm.New()
	if c.Name() != "foundry-auth-ocm" {
		t.Errorf("Name() = %q, want foundry-auth-ocm", c.Name())
	}
	if c.Version() != "v1.0.0" {
		t.Errorf("Version() = %q, want v1.0.0", c.Version())
	}
	if len(c.AuditHash()) != 64 {
		t.Errorf("AuditHash() len = %d, want 64", len(c.AuditHash()))
	}
}

func TestConfigure_EmptyConfig(t *testing.T) {
	c := autcocm.New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure empty cfg: %v", err)
	}
}

func TestConfigure_InvalidCacheTTL(t *testing.T) {
	c := autcocm.New()
	err := c.Configure(spec.ComponentConfig{"cache_ttl": "not-a-duration"})
	if err == nil {
		t.Fatal("expected error for invalid cache_ttl, got nil")
	}
}

func TestRegister_NilApp(t *testing.T) {
	c := autcocm.New()
	if err := c.Register(nil); err != nil {
		t.Errorf("Register(nil): %v", err)
	}
}

func TestStop_IsNoop(t *testing.T) {
	c := autcocm.New()
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestValidateToken_ValidToken(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	tok := buildJWT(t, testKey, testKID, validClaims(time.Hour))
	claims, err := c.ValidateToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Username != "alice" {
		t.Errorf("Username = %q, want alice", claims.Username)
	}
	if claims.OrgID != "org-abc" {
		t.Errorf("OrgID = %q, want org-abc", claims.OrgID)
	}
	if !claims.IsOrgAdmin {
		t.Error("IsOrgAdmin = false, want true")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	tok := buildJWT(t, testKey, testKID, validClaims(-time.Minute))
	_, err := c.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateToken_TamperedPayload(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	tok := buildJWT(t, testKey, testKID, validClaims(time.Hour))
	parts := strings.Split(tok, ".")
	badPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"hacker","exp":9999999999}`))
	tampered := parts[0] + "." + badPayload + "." + parts[2]

	_, err := c.ValidateToken(context.Background(), tampered)
	if err == nil {
		t.Fatal("expected signature error for tampered payload, got nil")
	}
}

func TestValidateToken_UnknownKID(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	tok := buildJWT(t, testKey, "unknown-kid", validClaims(time.Hour))
	_, err := c.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for unknown kid, got nil")
	}
}

func TestValidateToken_MalformedJWT(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	_, err := c.ValidateToken(context.Background(), "not.a.jwt.at.all.with.too.many.dots")
	if err == nil {
		t.Fatal("expected error for malformed JWT, got nil")
	}
}

func TestValidateToken_WrongAlgorithm(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	hdr, _ := json.Marshal(map[string]string{"alg": "HS256", "kid": testKID})
	pay, _ := json.Marshal(validClaims(time.Hour))
	tok := base64.RawURLEncoding.EncodeToString(hdr) + "." +
		base64.RawURLEncoding.EncodeToString(pay) + ".fakesig"

	_, err := c.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected alg error, got nil")
	}
}

func TestValidateToken_AllowedOrgs_Permit(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, spec.ComponentConfig{
		"allowed_orgs": []interface{}{"org-abc", "org-xyz"},
	})

	tok := buildJWT(t, testKey, testKID, validClaims(time.Hour))
	if _, err := c.ValidateToken(context.Background(), tok); err != nil {
		t.Fatalf("expected org-abc to be allowed: %v", err)
	}
}

func TestValidateToken_AllowedOrgs_Reject(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, spec.ComponentConfig{
		"allowed_orgs": []interface{}{"org-abc"},
	})

	bad := validClaims(time.Hour)
	bad["org_id"] = "org-unknown"
	tok := buildJWT(t, testKey, testKID, bad)
	_, err := c.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected org rejection, got nil")
	}
}

func TestValidateToken_CachesResult(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := json.Marshal(map[string]interface{}{
			"keys": []interface{}{
				map[string]interface{}{
					"kid": testKID,
					"kty": "RSA",
					"n":   base64.RawURLEncoding.EncodeToString(testKey.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testKey.PublicKey.E)).Bytes()),
				},
			},
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write(body) //nolint:errcheck
	}))
	defer srv.Close()

	c := autcocm.New()
	if err := c.Configure(spec.ComponentConfig{"jwks_url": srv.URL, "cache_ttl": "1m"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(context.Background()) //nolint:errcheck

	callsAfterStart := callCount
	tok := buildJWT(t, testKey, testKID, validClaims(time.Hour))

	for i := 0; i < 5; i++ {
		if _, err := c.ValidateToken(context.Background(), tok); err != nil {
			t.Fatalf("ValidateToken iteration %d: %v", i, err)
		}
	}

	// JWKS server should only be hit once more (for first token validation).
	// Subsequent calls must use the token cache.
	extraCalls := callCount - callsAfterStart
	if extraCalls > 1 {
		t.Errorf("token cache not effective: JWKS hit %d times after first validate (want ≤1)", extraCalls)
	}
}

func TestMiddleware_ValidToken_InjectsContext(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	mws := app.Middlewares()
	if len(mws) == 0 {
		t.Fatal("no middlewares registered")
	}

	sink := &captureHandler{}
	handler := mws[0](sink)

	tok := buildJWT(t, testKey, testKID, validClaims(time.Hour))
	req := &spec.Request{
		Method:  "GET",
		URL:     "/api/v1/test",
		Headers: map[string][]string{"Authorization": {"Bearer " + tok}},
		Context: context.Background(),
	}
	w := &fakeResponseWriter{header: make(map[string][]string)}
	handler.ServeHTTP(w, req)

	if !sink.called {
		t.Fatal("next handler was not called")
	}
	claims, ok := autcocm.ClaimsFromContext(sink.ctx)
	if !ok {
		t.Fatal("ClaimsFromContext: not found in context")
	}
	if claims.Username != "alice" {
		t.Errorf("Username = %q, want alice", claims.Username)
	}
	if claims.OrgID != "org-abc" {
		t.Errorf("OrgID = %q, want org-abc", claims.OrgID)
	}
}

func TestMiddleware_MissingToken_Required(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil) // required=true by default

	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mws := app.Middlewares()

	sink := &captureHandler{}
	handler := mws[0](sink)
	req := &spec.Request{
		Method:  "GET",
		URL:     "/",
		Headers: map[string][]string{},
		Context: context.Background(),
	}
	w := &fakeResponseWriter{header: make(map[string][]string)}
	handler.ServeHTTP(w, req)

	if sink.called {
		t.Error("next handler must not be called when auth is required and header is missing")
	}
	if w.statusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.statusCode)
	}
}

func TestMiddleware_MissingToken_Optional(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, spec.ComponentConfig{"required": false})

	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mws := app.Middlewares()

	sink := &captureHandler{}
	handler := mws[0](sink)
	req := &spec.Request{
		Method:  "GET",
		URL:     "/",
		Headers: map[string][]string{},
		Context: context.Background(),
	}
	w := &fakeResponseWriter{header: make(map[string][]string)}
	handler.ServeHTTP(w, req)

	if !sink.called {
		t.Error("next handler must be called for optional auth with missing header")
	}
}

func TestMiddleware_InvalidToken_Required(t *testing.T) {
	srv := jwksServer(t, &testKey.PublicKey, testKID)
	c := startedComponent(t, srv.URL, nil)

	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mws := app.Middlewares()

	sink := &captureHandler{}
	handler := mws[0](sink)
	req := &spec.Request{
		Method:  "GET",
		URL:     "/",
		Headers: map[string][]string{"Authorization": {"Bearer invalid.jwt.token"}},
		Context: context.Background(),
	}
	w := &fakeResponseWriter{header: make(map[string][]string)}
	handler.ServeHTTP(w, req)

	if sink.called {
		t.Error("next handler must not be called for invalid token with required=true")
	}
	if w.statusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.statusCode)
	}
}

func TestClaimsFromContext_Empty(t *testing.T) {
	_, ok := autcocm.ClaimsFromContext(context.Background())
	if ok {
		t.Error("expected no claims in empty context")
	}
}
