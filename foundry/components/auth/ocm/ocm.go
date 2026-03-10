// Package ocm provides the foundry-auth-ocm trusted component — an optional
// OpenShift Cluster Management (OCM) authentication layer for TSF applications.
//
// This component validates OCM/Red Hat SSO bearer tokens by fetching the OIDC
// JWKS from the configured auth URL and verifying JWT signatures locally.
// It never imports the OCM SDK, keeping the TSF platform core stdlib-only.
// OCM coupling is strictly contained to this optional component.
//
// Configuration (spec auth block when type is "ocm"):
//
//	auth:
//	  type: ocm
//	  ocm_url: "https://sso.redhat.com/auth/realms/redhat-external"
//	  jwks_url: ""          # optional override (derived from ocm_url if empty)
//	  allowed_orgs: []      # list of permitted OCM org IDs (empty = allow all)
//	  cache_ttl: "5m"       # token validation cache TTL (default: 5m)
//	  required: true        # reject unauthenticated requests (default: true)
package ocm

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-auth-ocm"
	componentVersion = "v1.0.0"
	// auditHash is the SHA-256 of this source tree at audit time.
	auditHash = "0000000000000000000000000000000000000000000000000000000000000020"

	defaultOCMURL    = "https://sso.redhat.com/auth/realms/redhat-external"
	defaultCacheTTL  = 5 * time.Minute
	jwksRefreshEvery = 10 * time.Minute
)

// Component implements spec.Component for OCM bearer token validation.
// All methods are safe for concurrent use.
type Component struct {
	mu          sync.RWMutex
	ocmURL      string
	jwksURL     string
	allowedOrgs map[string]struct{}
	cacheTTL    time.Duration
	required    bool
	httpClient  *http.Client

	// JWKS cache
	jwksMu     sync.RWMutex
	jwksKeys   map[string]*rsa.PublicKey // kid → public key
	jwksExpiry time.Time

	// Token validation cache: token → cacheEntry
	tokenMu    sync.RWMutex
	tokenCache map[string]cacheEntry
}

type cacheEntry struct {
	claims OCMClaims
	expiry time.Time
}

// OCMClaims holds the parsed fields from an OCM/RH-SSO JWT.
type OCMClaims struct {
	Subject    string   `json:"sub"`
	Email      string   `json:"email"`
	Username   string   `json:"preferred_username"`
	OrgID      string   `json:"org_id"`
	AccountID  string   `json:"account_id"`
	IsOrgAdmin bool     `json:"is_org_admin"`
	Issuer     string   `json:"iss"`
	Audience   []string `json:"aud"`
	ExpiresAt  int64    `json:"exp"`
	IssuedAt   int64    `json:"iat"`
}

// claimsKey is the context key for OCM claims.
type claimsKey struct{}

// ClaimsFromContext retrieves OCM claims stored by the auth middleware.
// Returns (zero-value, false) when no claims are present.
func ClaimsFromContext(ctx context.Context) (OCMClaims, bool) {
	v, ok := ctx.Value(claimsKey{}).(OCMClaims)
	return v, ok
}

// New returns a Component with secure defaults.
func New() *Component {
	return &Component{
		ocmURL:      defaultOCMURL,
		cacheTTL:    defaultCacheTTL,
		required:    true,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		jwksKeys:    make(map[string]*rsa.PublicKey),
		tokenCache:  make(map[string]cacheEntry),
		allowedOrgs: make(map[string]struct{}),
	}
}

func (c *Component) Name() string      { return componentName }
func (c *Component) Version() string   { return componentVersion }
func (c *Component) AuditHash() string { return auditHash }

// Configure applies the IR spec auth section.
func (c *Component) Configure(cfg spec.ComponentConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := cfg["ocm_url"].(string); ok && v != "" {
		c.ocmURL = strings.TrimRight(v, "/")
	}
	if v, ok := cfg["jwks_url"].(string); ok && v != "" {
		c.jwksURL = v
	}
	if v, ok := cfg["cache_ttl"].(string); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("foundry-auth-ocm: invalid cache_ttl %q: %w", v, err)
		}
		c.cacheTTL = d
	}
	if v, ok := cfg["required"].(bool); ok {
		c.required = v
	}

	// allowed_orgs may be []interface{} or []string depending on YAML decoder.
	if orgs, ok := cfg["allowed_orgs"]; ok {
		c.allowedOrgs = make(map[string]struct{})
		switch v := orgs.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					c.allowedOrgs[s] = struct{}{}
				}
			}
		case []string:
			for _, s := range v {
				if s != "" {
					c.allowedOrgs[s] = struct{}{}
				}
			}
		}
	}

	return nil
}

// Register installs the OCM auth middleware on the application.
func (c *Component) Register(app *spec.Application) error {
	if app == nil {
		return nil
	}
	app.AddMiddleware(spec.HTTPMiddleware(func(next spec.HTTPHandler) spec.HTTPHandler {
		return &authHandler{comp: c, next: next}
	}))
	return nil
}

// Start fetches the initial JWKS and begins the background refresh loop.
func (c *Component) Start(ctx context.Context) error {
	if err := c.refreshJWKS(ctx); err != nil {
		return fmt.Errorf("foundry-auth-ocm: initial JWKS fetch failed: %w", err)
	}
	go c.refreshLoop(ctx)
	return nil
}

// Stop is a no-op; the refresh loop exits when its context is cancelled.
func (c *Component) Stop(ctx context.Context) error { return nil }

// --- JWKS management ---

func (c *Component) jwksEndpoint() string {
	if c.jwksURL != "" {
		return c.jwksURL
	}
	return c.ocmURL + "/protocol/openid-connect/certs"
}

func (c *Component) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(jwksRefreshEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.refreshJWKS(ctx)
		}
	}
}

func (c *Component) refreshJWKS(ctx context.Context) error {
	endpoint := c.jwksEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	c.jwksMu.Lock()
	c.jwksKeys = keys
	c.jwksExpiry = time.Now().Add(jwksRefreshEvery)
	c.jwksMu.Unlock()
	return nil
}

func (c *Component) rsaKeyForKID(kid string) (*rsa.PublicKey, bool) {
	c.jwksMu.RLock()
	defer c.jwksMu.RUnlock()
	k, ok := c.jwksKeys[kid]
	return k, ok
}

// --- Token validation ---

// ValidateToken validates the raw JWT and returns OCMClaims.
// Results are cached for cacheTTL to avoid repeated crypto operations.
func (c *Component) ValidateToken(ctx context.Context, rawToken string) (OCMClaims, error) {
	// Check token cache first.
	c.tokenMu.RLock()
	if entry, ok := c.tokenCache[rawToken]; ok && time.Now().Before(entry.expiry) {
		c.tokenMu.RUnlock()
		return entry.claims, nil
	}
	c.tokenMu.RUnlock()

	claims, err := c.validateJWT(ctx, rawToken)
	if err != nil {
		return OCMClaims{}, err
	}

	// Check org membership if allowedOrgs is configured.
	c.mu.RLock()
	orgCount := len(c.allowedOrgs)
	_, orgAllowed := c.allowedOrgs[claims.OrgID]
	ttl := c.cacheTTL
	c.mu.RUnlock()

	if orgCount > 0 && !orgAllowed {
		return OCMClaims{}, fmt.Errorf("org %q is not in allowed_orgs", claims.OrgID)
	}

	// Cache the validated result.
	c.tokenMu.Lock()
	c.tokenCache[rawToken] = cacheEntry{claims: claims, expiry: time.Now().Add(ttl)}
	c.tokenMu.Unlock()

	return claims, nil
}

// validateJWT parses and verifies a JWT against the cached JWKS.
func (c *Component) validateJWT(ctx context.Context, rawToken string) (OCMClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return OCMClaims{}, fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	// Decode header to get kid and alg.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return OCMClaims{}, fmt.Errorf("decode JWT header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return OCMClaims{}, fmt.Errorf("parse JWT header: %w", err)
	}
	if header.Alg != "RS256" {
		return OCMClaims{}, fmt.Errorf("unsupported signing algorithm %q (want RS256)", header.Alg)
	}
	if header.Kid == "" {
		return OCMClaims{}, fmt.Errorf("JWT header missing kid")
	}

	// Look up public key; refresh JWKS once if key is unknown.
	pubKey, ok := c.rsaKeyForKID(header.Kid)
	if !ok {
		if err := c.refreshJWKS(ctx); err != nil {
			return OCMClaims{}, fmt.Errorf("JWKS refresh failed: %w", err)
		}
		pubKey, ok = c.rsaKeyForKID(header.Kid)
		if !ok {
			return OCMClaims{}, fmt.Errorf("unknown key ID %q", header.Kid)
		}
	}

	// Verify RS256 signature.
	if err := verifyRS256(parts[0]+"."+parts[1], parts[2], pubKey); err != nil {
		return OCMClaims{}, fmt.Errorf("JWT signature invalid: %w", err)
	}

	// Decode claims payload.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return OCMClaims{}, fmt.Errorf("decode JWT claims: %w", err)
	}

	// audience can be a string or []string — handle both.
	var rawClaims struct {
		Sub        string      `json:"sub"`
		Email      string      `json:"email"`
		Username   string      `json:"preferred_username"`
		OrgID      string      `json:"org_id"`
		AccountID  string      `json:"account_id"`
		IsOrgAdmin bool        `json:"is_org_admin"`
		Issuer     string      `json:"iss"`
		Audience   interface{} `json:"aud"`
		ExpiresAt  int64       `json:"exp"`
		IssuedAt   int64       `json:"iat"`
	}
	if err := json.Unmarshal(claimsJSON, &rawClaims); err != nil {
		return OCMClaims{}, fmt.Errorf("parse JWT claims: %w", err)
	}

	if rawClaims.ExpiresAt > 0 && time.Now().Unix() > rawClaims.ExpiresAt {
		return OCMClaims{}, fmt.Errorf("token expired")
	}

	var audience []string
	switch v := rawClaims.Audience.(type) {
	case string:
		audience = []string{v}
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok {
				audience = append(audience, s)
			}
		}
	}

	return OCMClaims{
		Subject:    rawClaims.Sub,
		Email:      rawClaims.Email,
		Username:   rawClaims.Username,
		OrgID:      rawClaims.OrgID,
		AccountID:  rawClaims.AccountID,
		IsOrgAdmin: rawClaims.IsOrgAdmin,
		Issuer:     rawClaims.Issuer,
		Audience:   audience,
		ExpiresAt:  rawClaims.ExpiresAt,
		IssuedAt:   rawClaims.IssuedAt,
	}, nil
}

// --- HTTP middleware ---

type authHandler struct {
	comp *Component
	next spec.HTTPHandler
}

func (h *authHandler) ServeHTTP(w spec.ResponseWriter, r *spec.Request) {
	authHeader := ""
	if vals, ok := r.Headers["Authorization"]; ok && len(vals) > 0 {
		authHeader = vals[0]
	}
	if authHeader == "" {
		if vals, ok := r.Headers["authorization"]; ok && len(vals) > 0 {
			authHeader = vals[0]
		}
	}

	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		h.comp.mu.RLock()
		required := h.comp.required
		h.comp.mu.RUnlock()
		if required {
			writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}
		h.next.ServeHTTP(w, r)
		return
	}

	rawToken := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := h.comp.ValidateToken(r.Context, rawToken)
	if err != nil {
		h.comp.mu.RLock()
		required := h.comp.required
		h.comp.mu.RUnlock()
		if required {
			writeError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}
	}

	// Attach claims to context for downstream handlers.
	newCtx := context.WithValue(r.Context, claimsKey{}, claims)
	r = &spec.Request{
		Method:  r.Method,
		URL:     r.URL,
		Headers: r.Headers,
		Body:    r.Body,
		Context: newCtx,
	}
	h.next.ServeHTTP(w, r)
}

// --- crypto helpers ---

func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	if e == 0 {
		return nil, fmt.Errorf("zero exponent")
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}

func writeError(w spec.ResponseWriter, status int, msg string) {
	body, _ := json.Marshal(map[string]any{"error": msg, "status": status})
	w.Header()["Content-Type"] = []string{"application/json"}
	w.WriteHeader(status)
	w.Write(body) //nolint:errcheck
}
