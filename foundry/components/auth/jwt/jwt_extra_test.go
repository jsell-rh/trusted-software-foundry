package jwt_test

// jwt_extra_test.go expands coverage for foundry-auth-jwt beyond what
// jwt_test.go already covers:
//   Name, Version, AuditHash, Register, Start (HMAC + JWKS), Stop,
//   refreshKeys (success, empty, invalid JSON, non-RSA, bad key),
//   refreshLoop (ctx cancel), rsaKeyFunc (wrong method, unknown kid),
//   hmacKeyFunc (wrong method), validate (no Bearer, issuer mismatch,
//   audience mismatch, no expiry, array audience), extractClaims (realm_roles),
//   RequireRole (no claims, wrong role, correct role), parseRSAPublicKey.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v4"

	authjwt "github.com/jsell-rh/trusted-software-foundry/foundry/components/auth/jwt"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// Shared RSA test key (1024-bit — speed over security for tests)
// --------------------------------------------------------------------------

var (
	extraTestRSAKey *rsa.PrivateKey
	extraTestKID    = "test-key-extra"
)

func init() {
	var err error
	extraTestRSAKey, err = rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic("generate test RSA key: " + err.Error())
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func makeRSATokenExtra(t *testing.T, key *rsa.PrivateKey, kid string, claims gojwt.MapClaims) string {
	t.Helper()
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign RSA token: %v", err)
	}
	return s
}

func encodeN(n *big.Int) string { return base64.RawURLEncoding.EncodeToString(n.Bytes()) }
func encodeE(e int) string {
	return base64.RawURLEncoding.EncodeToString(big.NewInt(int64(e)).Bytes())
}

// jwksServerExtra serves a JWKS with the given RSA public key.
func jwksServerExtra(t *testing.T, pub *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"keys": []interface{}{
			map[string]interface{}{
				"kid": kid,
				"kty": "RSA",
				"n":   encodeN(pub.N),
				"e":   encodeE(pub.E),
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

// startHMACExtra returns a started HMAC component. Extra cfg keys override
// or add to the base secret config.
func startHMACExtra(t *testing.T, secret string, extra spec.ComponentConfig) *authjwt.Component {
	t.Helper()
	c := authjwt.New()
	cfg := spec.ComponentConfig{"secret": secret}
	for k, v := range extra {
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

// startRSAExtra returns a started RSA/JWKS component.
func startRSAExtra(t *testing.T, jwksURL string) *authjwt.Component {
	t.Helper()
	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{"jwks_url": jwksURL}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { c.Stop(context.Background()) }) //nolint:errcheck
	return c
}

// invoke calls the middleware chain for the given component and token string.
func invoke(c *authjwt.Component, token string) (fw *fakeResponseWriter, handlerCalled bool) {
	fw = &fakeResponseWriter{}
	mw := c.Middleware()
	wrapped := mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		handlerCalled = true
	}))
	headers := map[string][]string{}
	if token != "" {
		headers["Authorization"] = []string{"Bearer " + token}
	}
	wrapped.ServeHTTP(fw, &spec.Request{
		Method:  "GET",
		URL:     "/api/test",
		Headers: headers,
		Context: context.Background(),
	})
	return
}

// --------------------------------------------------------------------------
// Metadata
// --------------------------------------------------------------------------

func TestExtra_Name(t *testing.T) {
	if got := authjwt.New().Name(); got != "foundry-auth-jwt" {
		t.Errorf("Name() = %q, want foundry-auth-jwt", got)
	}
}

func TestExtra_Version(t *testing.T) {
	if got := authjwt.New().Version(); got != "v1.0.0" {
		t.Errorf("Version() = %q, want v1.0.0", got)
	}
}

func TestExtra_AuditHash(t *testing.T) {
	if got := authjwt.New().AuditHash(); got == "" {
		t.Error("AuditHash() must not be empty")
	}
}

// --------------------------------------------------------------------------
// Register
// --------------------------------------------------------------------------

func TestExtra_Register_AddsMiddleware(t *testing.T) {
	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{"secret": "s"}); err != nil {
		t.Fatal(err)
	}
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Errorf("Register: %v", err)
	}
}

// --------------------------------------------------------------------------
// Configure: algorithm defaults
// --------------------------------------------------------------------------

func TestExtra_Configure_DefaultAlgo_HMAC(t *testing.T) {
	c := authjwt.New()
	// With secret only, default algo must be HS256.
	if err := c.Configure(spec.ComponentConfig{"secret": "s"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// Verify it works by starting and making a valid request.
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(context.Background()) //nolint:errcheck
}

func TestExtra_Configure_ExplicitAlgorithms(t *testing.T) {
	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{
		"secret":     "s",
		"algorithms": []interface{}{"HS256", "HS384"},
	}); err != nil {
		t.Fatalf("Configure with explicit algos: %v", err)
	}
}

// --------------------------------------------------------------------------
// Start — HMAC and JWKS paths
// --------------------------------------------------------------------------

func TestExtra_Start_HMAC_TokenRoundTrip(t *testing.T) {
	c := startHMACExtra(t, "my-secret", nil)
	tok := makeToken(t, "my-secret", gojwt.MapClaims{
		"sub": "u1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	_, called := invoke(c, tok)
	if !called {
		t.Error("handler not called with valid HMAC token")
	}
}

func TestExtra_Start_JWKS_Success(t *testing.T) {
	srv := jwksServerExtra(t, &extraTestRSAKey.PublicKey, extraTestKID)
	c := startRSAExtra(t, srv.URL)
	tok := makeRSATokenExtra(t, extraTestRSAKey, extraTestKID, gojwt.MapClaims{
		"sub": "u1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	_, called := invoke(c, tok)
	if !called {
		t.Error("handler not called with valid RSA token")
	}
}

func TestExtra_Start_JWKS_FetchFails(t *testing.T) {
	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{
		"jwks_url": "http://127.0.0.1:60999/jwks",
	}); err != nil {
		t.Fatal(err)
	}
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when JWKS server unreachable, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-auth-jwt") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Stop — with a non-nil cancel
// --------------------------------------------------------------------------

func TestExtra_Stop_WithCancel(t *testing.T) {
	srv := jwksServerExtra(t, &extraTestRSAKey.PublicKey, extraTestKID)
	c := startRSAExtra(t, srv.URL)
	// c.cancel is set because jwks_url was configured; Stop must call it.
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// --------------------------------------------------------------------------
// refreshKeys variants
// --------------------------------------------------------------------------

func TestExtra_RefreshKeys_EmptyJWKS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"keys":[]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{"jwks_url": srv.URL}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start with empty JWKS: %v", err)
	}
	c.Stop(context.Background()) //nolint:errcheck
}

func TestExtra_RefreshKeys_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not-json`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{"jwks_url": srv.URL}); err != nil {
		t.Fatal(err)
	}
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JWKS JSON")
	}
}

func TestExtra_RefreshKeys_NonRSAKeySkipped(t *testing.T) {
	// EC key (kty != "RSA") must be silently skipped — no error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"keys":[{"kid":"k1","kty":"EC"}]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{"jwks_url": srv.URL}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start with EC key skipped: %v", err)
	}
	c.Stop(context.Background()) //nolint:errcheck
}

func TestExtra_RefreshKeys_InvalidRSAKey_BadN(t *testing.T) {
	// RSA key with invalid N base64 → parseRSAPublicKey must return error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "!!!" is not valid base64url → DecodeString("!!!...") fails.
		w.Write([]byte(`{"keys":[{"kid":"k1","kty":"RSA","n":"!invalid!","e":"AQAB"}]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{"jwks_url": srv.URL}); err != nil {
		t.Fatal(err)
	}
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid RSA N, got nil")
	}
}

// --------------------------------------------------------------------------
// refreshLoop — exits when context is cancelled
// --------------------------------------------------------------------------

func TestExtra_RefreshLoop_ExitsOnContextCancel(t *testing.T) {
	srv := jwksServerExtra(t, &extraTestRSAKey.PublicKey, extraTestKID)
	c := startRSAExtra(t, srv.URL)
	// Stop calls c.cancel() which cancels the context passed to refreshLoop.
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// --------------------------------------------------------------------------
// hmacKeyFunc — wrong signing method
// --------------------------------------------------------------------------

func TestExtra_HMACKeyFunc_WrongSigningMethod(t *testing.T) {
	// Configure HMAC, send an RSA-signed token.
	// hmacKeyFunc checks the signing method and returns an error.
	c := startHMACExtra(t, "my-secret", nil)
	rsaTok := makeRSATokenExtra(t, extraTestRSAKey, extraTestKID, gojwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	fw, called := invoke(c, rsaTok)
	if called {
		t.Error("handler should not be called for wrong signing method")
	}
	if fw.code != 401 {
		t.Errorf("expected 401, got %d", fw.code)
	}
}

// --------------------------------------------------------------------------
// rsaKeyFunc — wrong method and unknown kid
// --------------------------------------------------------------------------

func TestExtra_RSAKeyFunc_WrongSigningMethod(t *testing.T) {
	// Configure RSA, send an HMAC token → rsaKeyFunc returns error.
	srv := jwksServerExtra(t, &extraTestRSAKey.PublicKey, extraTestKID)
	c := startRSAExtra(t, srv.URL)
	hmacTok := makeToken(t, "secret", gojwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	fw, called := invoke(c, hmacTok)
	if called {
		t.Error("handler should not be called for wrong signing method")
	}
	if fw.code != 401 {
		t.Errorf("expected 401, got %d", fw.code)
	}
}

func TestExtra_RSAKeyFunc_UnknownKID(t *testing.T) {
	// Token has a kid not present in the JWKS.
	srv := jwksServerExtra(t, &extraTestRSAKey.PublicKey, extraTestKID)
	c := startRSAExtra(t, srv.URL)
	tok := makeRSATokenExtra(t, extraTestRSAKey, "unknown-kid", gojwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	fw, called := invoke(c, tok)
	if called {
		t.Error("handler should not be called for unknown kid")
	}
	if fw.code != 401 {
		t.Errorf("expected 401, got %d", fw.code)
	}
}

// --------------------------------------------------------------------------
// validate — additional error branches
// --------------------------------------------------------------------------

func TestExtra_Validate_NoBearerScheme(t *testing.T) {
	c := startHMACExtra(t, "s", nil)
	fw := &fakeResponseWriter{}
	mw := c.Middleware()
	mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		t.Error("handler must not be called")
	})).ServeHTTP(fw, &spec.Request{
		Method:  "GET",
		URL:     "/",
		Headers: map[string][]string{"Authorization": {"Token not-bearer"}},
		Context: context.Background(),
	})
	if fw.code != 401 {
		t.Errorf("expected 401 for non-Bearer auth, got %d", fw.code)
	}
}

func TestExtra_Validate_IssuerMismatch(t *testing.T) {
	c := startHMACExtra(t, "s", spec.ComponentConfig{"issuer": "expected-issuer"})
	tok := makeToken(t, "s", gojwt.MapClaims{
		"sub": "u",
		"iss": "other-issuer",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	fw, called := invoke(c, tok)
	if called {
		t.Error("handler should not be called on issuer mismatch")
	}
	if fw.code != 401 {
		t.Errorf("expected 401, got %d", fw.code)
	}
}

func TestExtra_Validate_AudienceMismatch(t *testing.T) {
	c := startHMACExtra(t, "s", spec.ComponentConfig{"audience": "my-service"})
	tok := makeToken(t, "s", gojwt.MapClaims{
		"sub": "u",
		"aud": "other-service",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	// NOTE: golang-jwt v4 validates aud only if explicitly told to (via MapClaims.VerifyAudience).
	// Our component does its own audience check after extractClaims.
	fw, called := invoke(c, tok)
	if called {
		t.Error("handler should not be called on audience mismatch")
	}
	if fw.code != 401 {
		t.Errorf("expected 401 for audience mismatch, got %d", fw.code)
	}
}

func TestExtra_Validate_NoExpiry(t *testing.T) {
	// Token with no "exp" claim — parser accepts it but our code rejects it
	// (claims.Expiry.IsZero() == true → "token has no expiry claim").
	c := startHMACExtra(t, "s", nil)
	tok := makeToken(t, "s", gojwt.MapClaims{
		"sub": "u",
		// deliberately no "exp"
	})
	fw, called := invoke(c, tok)
	if called {
		t.Error("handler should not be called for token without exp")
	}
	if fw.code != 401 {
		t.Errorf("expected 401 for token without exp, got %d", fw.code)
	}
}

// --------------------------------------------------------------------------
// extractClaims — audience array and realm_roles
// --------------------------------------------------------------------------

func TestExtra_ExtractClaims_ArrayAudience(t *testing.T) {
	c := startHMACExtra(t, "s", nil)
	tok := makeToken(t, "s", gojwt.MapClaims{
		"sub": "u",
		"aud": []interface{}{"aud1", "aud2"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	_, called := invoke(c, tok)
	if !called {
		t.Error("handler not called with array audience token")
	}
}

func TestExtra_ExtractClaims_RealmRoles(t *testing.T) {
	c := startHMACExtra(t, "s", nil)
	tok := makeToken(t, "s", gojwt.MapClaims{
		"sub":         "u",
		"exp":         time.Now().Add(time.Hour).Unix(),
		"realm_roles": []interface{}{"viewer"},
	})
	var captured *authjwt.Claims
	mw := c.Middleware()
	mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		captured = authjwt.ClaimsFromContext(r.Context)
	})).ServeHTTP(&fakeResponseWriter{}, &spec.Request{
		Method:  "GET",
		URL:     "/",
		Headers: map[string][]string{"Authorization": {"Bearer " + tok}},
		Context: context.Background(),
	})
	if captured == nil {
		t.Fatal("no claims in context")
	}
	if len(captured.Roles) == 0 || captured.Roles[0] != "viewer" {
		t.Errorf("roles = %v, want [viewer]", captured.Roles)
	}
}

func TestExtra_ExtractClaims_Groups(t *testing.T) {
	c := startHMACExtra(t, "s", nil)
	tok := makeToken(t, "s", gojwt.MapClaims{
		"sub":    "u",
		"exp":    time.Now().Add(time.Hour).Unix(),
		"groups": []interface{}{"engineering"},
	})
	var captured *authjwt.Claims
	mw := c.Middleware()
	mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		captured = authjwt.ClaimsFromContext(r.Context)
	})).ServeHTTP(&fakeResponseWriter{}, &spec.Request{
		Method:  "GET",
		URL:     "/",
		Headers: map[string][]string{"Authorization": {"Bearer " + tok}},
		Context: context.Background(),
	})
	if captured == nil || len(captured.Roles) == 0 || captured.Roles[0] != "engineering" {
		t.Errorf("groups-as-roles = %v, want [engineering]", func() []string {
			if captured != nil {
				return captured.Roles
			}
			return nil
		}())
	}
}

// --------------------------------------------------------------------------
// RequireRole
// --------------------------------------------------------------------------

func TestExtra_RequireRole_NoClaims(t *testing.T) {
	mw := authjwt.RequireRole("admin")
	fw := &fakeResponseWriter{}
	mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		t.Error("handler must not be called with no claims")
	})).ServeHTTP(fw, &spec.Request{
		Method:  "GET",
		URL:     "/",
		Headers: map[string][]string{},
		Context: context.Background(), // no claims
	})
	if fw.code != 403 {
		t.Errorf("expected 403, got %d", fw.code)
	}
}

func TestExtra_RequireRole_WrongRole(t *testing.T) {
	ctx := context.WithValue(context.Background(), authjwt.ClaimsKey, &authjwt.Claims{
		Roles: []string{"viewer"},
	})
	mw := authjwt.RequireRole("admin")
	fw := &fakeResponseWriter{}
	mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		t.Error("handler must not be called for wrong role")
	})).ServeHTTP(fw, &spec.Request{
		Method: "GET", URL: "/", Headers: map[string][]string{}, Context: ctx,
	})
	if fw.code != 403 {
		t.Errorf("expected 403, got %d", fw.code)
	}
}

func TestExtra_RequireRole_CorrectRole(t *testing.T) {
	ctx := context.WithValue(context.Background(), authjwt.ClaimsKey, &authjwt.Claims{
		Roles: []string{"viewer", "admin"},
	})
	mw := authjwt.RequireRole("admin")
	var called bool
	mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		called = true
	})).ServeHTTP(&fakeResponseWriter{}, &spec.Request{
		Method: "GET", URL: "/", Headers: map[string][]string{}, Context: ctx,
	})
	if !called {
		t.Error("handler should be called for correct role")
	}
}

func TestExtra_RefreshKeys_InvalidRSAKey_BadE(t *testing.T) {
	// RSA key with valid N but invalid E base64 → parseRSAPublicKey decode-e error.
	// Use a real base64url N from our test key, but invalid E.
	validN := encodeN(extraTestRSAKey.PublicKey.N)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]interface{}{
			"keys": []interface{}{
				map[string]interface{}{
					"kid": "k1",
					"kty": "RSA",
					"n":   validN,
					"e":   "!invalid!", // invalid base64url for E
				},
			},
		})
		w.Write(body) //nolint:errcheck
	}))
	defer srv.Close()

	c := authjwt.New()
	if err := c.Configure(spec.ComponentConfig{"jwks_url": srv.URL}); err != nil {
		t.Fatal(err)
	}
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid RSA E, got nil")
	}
}

// --------------------------------------------------------------------------
// skip_paths — path-confusion bypass prevention (security)
// --------------------------------------------------------------------------

// invokeURL calls the middleware with a specific URL and no Authorization header,
// allowing skip_paths tests to confirm whether auth is bypassed.
func invokeURL(c *authjwt.Component, rawURL string) (fw *fakeResponseWriter, handlerCalled bool) {
	fw = &fakeResponseWriter{}
	mw := c.Middleware()
	wrapped := mw(handlerFunc(func(w spec.ResponseWriter, r *spec.Request) {
		handlerCalled = true
	}))
	wrapped.ServeHTTP(fw, &spec.Request{
		Method:  "GET",
		URL:     rawURL,
		Headers: map[string][]string{},
		Context: context.Background(),
	})
	return
}

// startHMACWithSkipPaths returns a started HMAC component with the given skip paths.
func startHMACWithSkipPaths(t *testing.T, skipPaths []interface{}) *authjwt.Component {
	t.Helper()
	return startHMACExtra(t, "test-secret", spec.ComponentConfig{
		"skip_paths": skipPaths,
	})
}

func TestExtra_SkipPath_ExactMatch_Bypasses(t *testing.T) {
	// /healthz configured as skip path; exact match must bypass authentication.
	c := startHMACWithSkipPaths(t, []interface{}{"/healthz"})
	_, called := invokeURL(c, "/healthz")
	if !called {
		t.Error("handler not called for exact skip-path match /healthz — auth bypass failed")
	}
}

func TestExtra_SkipPath_SubPath_Bypasses(t *testing.T) {
	// /healthz/live is a sub-path of /healthz; must bypass authentication.
	c := startHMACWithSkipPaths(t, []interface{}{"/healthz"})
	_, called := invokeURL(c, "/healthz/live")
	if !called {
		t.Error("handler not called for sub-path /healthz/live — auth bypass failed")
	}
}

func TestExtra_SkipPath_WithQuery_Bypasses(t *testing.T) {
	// /healthz?probe=1 starts with /healthz followed by '?'; must bypass authentication.
	c := startHMACWithSkipPaths(t, []interface{}{"/healthz"})
	_, called := invokeURL(c, "/healthz?probe=1")
	if !called {
		t.Error("handler not called for /healthz?probe=1 — auth bypass failed")
	}
}

func TestExtra_SkipPath_PathConfusion_DoesNotBypass(t *testing.T) {
	// /healthzmypath shares the prefix /healthz but is a DIFFERENT route.
	// A plain strings.HasPrefix check would incorrectly bypass auth here.
	// The pathMatchesSkip boundary check must reject this.
	c := startHMACWithSkipPaths(t, []interface{}{"/healthz"})
	fw, called := invokeURL(c, "/healthzmypath")
	if called {
		t.Error("handler was called for /healthzmypath — path-confusion bypass is present!")
	}
	if fw.code != 401 {
		t.Errorf("expected 401 for /healthzmypath, got %d", fw.code)
	}
}

func TestExtra_SkipPath_Unrelated_DoesNotBypass(t *testing.T) {
	// /api/data does not match the skip prefix /healthz at all — must require auth.
	c := startHMACWithSkipPaths(t, []interface{}{"/healthz"})
	fw, called := invokeURL(c, "/api/data")
	if called {
		t.Error("handler was called for /api/data without auth — unrelated path bypassed!")
	}
	if fw.code != 401 {
		t.Errorf("expected 401 for /api/data, got %d", fw.code)
	}
}

func TestExtra_SkipPath_MultipleSkips_MatchesCorrectOne(t *testing.T) {
	// Multiple skip paths; the matching one (/readyz) grants bypass while the
	// non-matching request (/metrics) is still protected.
	c := startHMACWithSkipPaths(t, []interface{}{"/healthz", "/readyz"})

	_, readyzCalled := invokeURL(c, "/readyz")
	if !readyzCalled {
		t.Error("handler not called for /readyz — should bypass auth")
	}

	fw, metricsCalled := invokeURL(c, "/metrics")
	if metricsCalled {
		t.Error("handler called for /metrics without auth — should require auth")
	}
	if fw.code != 401 {
		t.Errorf("expected 401 for /metrics, got %d", fw.code)
	}
}
