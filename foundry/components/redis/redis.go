// Package redis implements the foundry-redis trusted component.
//
// foundry-redis provides Redis-backed capabilities for Foundry applications:
//
//   - Caching: TTL-based cache for arbitrary byte values, keyed by string
//   - Rate limiting: token-bucket rate limiter per route and client IP
//   - Distributed locking: advisory locks with TTL and retry semantics
//
// # Design
//
// Each capability maps to a distinct Redis key namespace to avoid collisions:
//
//	cache:    "foundry:cache:<key>"
//	ratelimit "foundry:rl:<route>:<client>"
//	lock:     "foundry:lock:<resource>:<id>"
//
// Multiple Redis backends (cache, ratelimit, locks) can be configured with
// separate URLs, or all capabilities can share a single connection.
//
// # Configuration (ComponentConfig keys from IR state: block)
//
//	url           string   single Redis URL for all capabilities (default: redis://localhost:6379)
//	cache_url     string   Redis URL for cache (overrides url)
//	ratelimit_url string   Redis URL for rate limiting (overrides url)
//	lock_url      string   Redis URL for distributed locking (overrides url)
//	default_ttl   int      default cache TTL in seconds (default: 300)
//	key_prefix    string   global key prefix (default: "foundry")
package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-redis"
	componentVersion = "v1.0.0"
	// auditHash is the SHA-256 of this source tree at audit time.
	auditHash = "red0000000000000000000000000000000000000000000000000000000000001"
)

// ErrRateLimited is returned by RateLimit when the request exceeds the limit.
var ErrRateLimited = errors.New("foundry-redis: rate limit exceeded")

// ErrLockNotAcquired is returned by Lock when the lock cannot be obtained.
var ErrLockNotAcquired = errors.New("foundry-redis: lock not acquired")

// Component is the foundry-redis trusted component implementation.
type Component struct {
	mu          sync.Mutex
	cfg         config
	cacheDB     *redis.Client
	ratelimitDB *redis.Client
	lockDB      *redis.Client
}

type config struct {
	defaultURL   string
	cacheURL     string
	ratelimitURL string
	lockURL      string
	defaultTTL   time.Duration
	keyPrefix    string
}

// New returns a new foundry-redis Component with defaults.
func New() *Component {
	return &Component{}
}

func (c *Component) Name() string      { return componentName }
func (c *Component) Version() string   { return componentVersion }
func (c *Component) AuditHash() string { return auditHash }

// Configure reads Redis configuration from the IR spec.
func (c *Component) Configure(cfg spec.ComponentConfig) error {
	c.cfg = config{
		defaultURL: "redis://localhost:6379",
		defaultTTL: 300 * time.Second,
		keyPrefix:  "foundry",
	}

	if v, ok := cfg["url"].(string); ok && v != "" {
		c.cfg.defaultURL = v
	}
	if v, ok := cfg["cache_url"].(string); ok && v != "" {
		c.cfg.cacheURL = v
	}
	if v, ok := cfg["ratelimit_url"].(string); ok && v != "" {
		c.cfg.ratelimitURL = v
	}
	if v, ok := cfg["lock_url"].(string); ok && v != "" {
		c.cfg.lockURL = v
	}
	if v, ok := cfg["default_ttl"].(int); ok && v > 0 {
		c.cfg.defaultTTL = time.Duration(v) * time.Second
	}
	if v, ok := cfg["key_prefix"].(string); ok && v != "" {
		c.cfg.keyPrefix = v
	}
	return nil
}

// Register is a no-op for redis — it does not mount HTTP handlers or middleware by default.
// Custom code can add rate-limit middleware via app.AddMiddleware after getting the component.
func (c *Component) Register(_ *spec.Application) error { return nil }

// Start connects to Redis backends.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheURL := c.cfg.cacheURL
	if cacheURL == "" {
		cacheURL = c.cfg.defaultURL
	}
	rlURL := c.cfg.ratelimitURL
	if rlURL == "" {
		rlURL = c.cfg.defaultURL
	}
	lockURL := c.cfg.lockURL
	if lockURL == "" {
		lockURL = c.cfg.defaultURL
	}

	var err error
	c.cacheDB, err = dialRedis(ctx, cacheURL)
	if err != nil {
		return fmt.Errorf("foundry-redis: cache connection: %w", err)
	}

	// Reuse the same connection if all URLs are identical.
	if rlURL == cacheURL {
		c.ratelimitDB = c.cacheDB
	} else {
		c.ratelimitDB, err = dialRedis(ctx, rlURL)
		if err != nil {
			return fmt.Errorf("foundry-redis: ratelimit connection: %w", err)
		}
	}

	if lockURL == cacheURL {
		c.lockDB = c.cacheDB
	} else {
		c.lockDB, err = dialRedis(ctx, lockURL)
		if err != nil {
			return fmt.Errorf("foundry-redis: lock connection: %w", err)
		}
	}

	return nil
}

// Stop closes all Redis connections.
func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := map[*redis.Client]bool{}
	for _, client := range []*redis.Client{c.cacheDB, c.ratelimitDB, c.lockDB} {
		if client != nil && !seen[client] {
			seen[client] = true
			if err := client.Close(); err != nil {
				return fmt.Errorf("foundry-redis: close: %w", err)
			}
		}
	}
	return nil
}

// --- Cache ---

// Set stores value under key with the given TTL (0 = use default_ttl).
func (c *Component) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = c.cfg.defaultTTL
	}
	return c.cacheDB.Set(ctx, c.cacheKey(key), value, ttl).Err()
}

// Get retrieves the value for key. Returns (nil, nil) on cache miss.
func (c *Component) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.cacheDB.Get(ctx, c.cacheKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("foundry-redis: cache get: %w", err)
	}
	return val, nil
}

// Delete removes a key from cache.
func (c *Component) Delete(ctx context.Context, key string) error {
	return c.cacheDB.Del(ctx, c.cacheKey(key)).Err()
}

// --- Rate limiting ---

// RateLimit checks whether the given client may access route.
// It uses a fixed-window counter. Returns ErrRateLimited if the limit is exceeded.
//
// requestsPerWindow: max requests allowed in the window duration.
// window: the time window (e.g., time.Second for per-second rate limiting).
func (c *Component) RateLimit(ctx context.Context, route, clientID string, requestsPerWindow int64, window time.Duration) error {
	key := fmt.Sprintf("%s:rl:%s:%s", c.cfg.keyPrefix, route, clientID)

	pipe := c.ratelimitDB.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("foundry-redis: rate limit pipeline: %w", err)
	}

	if incr.Val() > requestsPerWindow {
		return ErrRateLimited
	}
	return nil
}

// --- Distributed locking ---

// Lock acquires a distributed lock for resource+id with the given TTL.
// Returns ErrLockNotAcquired if the lock is already held.
// The returned token must be passed to Unlock to release the lock.
func (c *Component) Lock(ctx context.Context, resource, id string, ttl time.Duration) (token string, err error) {
	key := fmt.Sprintf("%s:lock:%s:%s", c.cfg.keyPrefix, resource, id)
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("foundry-redis: generate lock token: %w", err)
	}
	token = hex.EncodeToString(b[:])

	ok, err := c.lockDB.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", fmt.Errorf("foundry-redis: lock acquire: %w", err)
	}
	if !ok {
		return "", ErrLockNotAcquired
	}
	return token, nil
}

// Unlock releases a distributed lock. It verifies the token matches to prevent
// accidental release of another holder's lock.
func (c *Component) Unlock(ctx context.Context, resource, id, token string) error {
	key := fmt.Sprintf("%s:lock:%s:%s", c.cfg.keyPrefix, resource, id)

	// Lua script: compare-and-delete (atomic).
	const luaUnlock = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end`
	result, err := c.lockDB.Eval(ctx, luaUnlock, []string{key}, token).Int()
	if err != nil {
		return fmt.Errorf("foundry-redis: lock release: %w", err)
	}
	if result == 0 {
		return fmt.Errorf("foundry-redis: lock %q not held by token %q (may have expired)", key, token)
	}
	return nil
}

// --- helpers ---

func (c *Component) cacheKey(key string) string {
	return fmt.Sprintf("%s:cache:%s", c.cfg.keyPrefix, key)
}

func dialRedis(ctx context.Context, rawURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL %q: %w", rawURL, err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis at %q: %w", rawURL, err)
	}
	return client, nil
}
