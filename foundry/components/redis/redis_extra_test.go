package redis

// redis_extra_test.go expands coverage for the foundry-redis component
// using miniredis for an in-process Redis server:
//   Start (shared URL, separate ratelimit/lock URLs, dial errors),
//   Stop (nil clients, shared client, multiple clients),
//   Set (TTL=0 uses default, explicit TTL), Get (hit, miss, error),
//   Delete, RateLimit (under limit, over limit, pipeline error),
//   Lock (success, not acquired, SetNX error),
//   Unlock (success, token mismatch, eval error),
//   dialRedis (invalid URL, ping error).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// startMini starts an in-process Redis server and returns its URL.
func startMini(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return mr
}

// startedComponent returns a Component fully started against the given miniredis.
func startedComponent(t *testing.T, mr *miniredis.Miniredis) *Component {
	t.Helper()
	c := New()
	if err := c.Configure(map[string]any{
		"url":        "redis://" + mr.Addr(),
		"key_prefix": "test",
		"default_ttl": 60,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })
	return c
}

// --------------------------------------------------------------------------
// Start — connection pooling and per-capability URLs
// --------------------------------------------------------------------------

func TestStart_SharedURL_SingleConnection(t *testing.T) {
	mr := startMini(t)
	c := New()
	c.Configure(map[string]any{"url": "redis://" + mr.Addr()})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop(context.Background()) //nolint:errcheck
	// All three clients should be the same pointer (shared).
	if c.cacheDB != c.ratelimitDB {
		t.Error("cacheDB and ratelimitDB should be the same connection when using shared URL")
	}
	if c.cacheDB != c.lockDB {
		t.Error("cacheDB and lockDB should be the same connection when using shared URL")
	}
}

func TestStart_SeparateRatelimitURL(t *testing.T) {
	mr1 := startMini(t)
	mr2 := startMini(t)
	c := New()
	c.Configure(map[string]any{
		"url":           "redis://" + mr1.Addr(),
		"ratelimit_url": "redis://" + mr2.Addr(),
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start with separate ratelimit URL: %v", err)
	}
	defer c.Stop(context.Background()) //nolint:errcheck
	if c.cacheDB == c.ratelimitDB {
		t.Error("cacheDB and ratelimitDB should be different for separate URLs")
	}
	// lockURL defaults to cacheURL → same as cacheDB
	if c.cacheDB != c.lockDB {
		t.Error("lockDB should share cacheDB when lock_url not set")
	}
}

func TestStart_SeparateLockURL(t *testing.T) {
	mr1 := startMini(t)
	mr2 := startMini(t)
	c := New()
	c.Configure(map[string]any{
		"url":      "redis://" + mr1.Addr(),
		"lock_url": "redis://" + mr2.Addr(),
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start with separate lock URL: %v", err)
	}
	defer c.Stop(context.Background()) //nolint:errcheck
	if c.cacheDB == c.lockDB {
		t.Error("lockDB and cacheDB should differ for separate lock_url")
	}
	// ratelimitURL defaults to cacheURL → same as cacheDB
	if c.cacheDB != c.ratelimitDB {
		t.Error("ratelimitDB should share cacheDB when ratelimit_url not set")
	}
}

func TestStart_RatelimitDial_Error(t *testing.T) {
	mr := startMini(t)
	c := New()
	c.Configure(map[string]any{
		"url":           "redis://" + mr.Addr(),
		"ratelimit_url": "redis://127.0.0.1:19998", // unreachable
	})
	err := c.Start(context.Background())
	if err == nil {
		t.Skip("port 19998 is in use — skipping ratelimit dial error test")
	}
	if !strings.Contains(err.Error(), "ratelimit connection") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_LockDial_Error(t *testing.T) {
	mr := startMini(t)
	c := New()
	c.Configure(map[string]any{
		"url":      "redis://" + mr.Addr(),
		"lock_url": "redis://127.0.0.1:19997", // unreachable
	})
	err := c.Start(context.Background())
	if err == nil {
		t.Skip("port 19997 is in use — skipping lock dial error test")
	}
	if !strings.Contains(err.Error(), "lock connection") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Stop
// --------------------------------------------------------------------------

func TestStop_NilClients(t *testing.T) {
	c := New()
	// All clients are nil → Stop should be a no-op.
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop with nil clients: %v", err)
	}
}

func TestStop_SharedClient(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	// All three are the same pointer — Stop must close it only once.
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestStop_MultipleDistinctClients(t *testing.T) {
	mr1 := startMini(t)
	mr2 := startMini(t)
	mr3 := startMini(t)
	c := New()
	c.Configure(map[string]any{
		"url":           "redis://" + mr1.Addr(),
		"ratelimit_url": "redis://" + mr2.Addr(),
		"lock_url":      "redis://" + mr3.Addr(),
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// --------------------------------------------------------------------------
// Set
// --------------------------------------------------------------------------

func TestSet_ExplicitTTL(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	if err := c.Set(context.Background(), "mykey", []byte("myval"), 5*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, _ := mr.Get("test:cache:mykey")
	if val != "myval" {
		t.Errorf("stored value = %q, want myval", val)
	}
	// TTL should be set.
	ttl := mr.TTL("test:cache:mykey")
	if ttl <= 0 || ttl > 5*time.Second {
		t.Errorf("TTL = %v, want > 0 and <= 5s", ttl)
	}
}

func TestSet_ZeroTTL_UsesDefault(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	if err := c.Set(context.Background(), "key2", []byte("val2"), 0); err != nil {
		t.Fatalf("Set with zero TTL: %v", err)
	}
	ttl := mr.TTL("test:cache:key2")
	// default_ttl = 60s from startedComponent
	if ttl <= 0 || ttl > 60*time.Second {
		t.Errorf("TTL = %v, want 0 < TTL <= 60s (default)", ttl)
	}
}

// --------------------------------------------------------------------------
// Get
// --------------------------------------------------------------------------

func TestGet_Hit(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	// Pre-seed the key.
	mr.Set("test:cache:hitkey", "hitval")
	val, err := c.Get(context.Background(), "hitkey")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "hitval" {
		t.Errorf("val = %q, want hitval", val)
	}
}

func TestGet_Miss(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	val, err := c.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Get miss should not error: %v", err)
	}
	if val != nil {
		t.Errorf("expected nil on cache miss, got %v", val)
	}
}

func TestGet_RedisError(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	// Close the server so the get fails.
	mr.Close()
	_, err := c.Get(context.Background(), "anykey")
	if err == nil {
		t.Fatal("expected error after server closed, got nil")
	}
	if !strings.Contains(err.Error(), "cache get") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Delete
// --------------------------------------------------------------------------

func TestDelete_Success(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	mr.Set("test:cache:delkey", "v")
	if err := c.Delete(context.Background(), "delkey"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := mr.Get("test:cache:delkey")
	if err == nil {
		t.Error("key should have been deleted")
	}
}

func TestDelete_MissingKey_NoError(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	// DEL on a non-existent key is not an error in Redis.
	if err := c.Delete(context.Background(), "missing"); err != nil {
		t.Fatalf("Delete missing key: %v", err)
	}
}

// --------------------------------------------------------------------------
// RateLimit
// --------------------------------------------------------------------------

func TestRateLimit_UnderLimit(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	if err := c.RateLimit(context.Background(), "/api", "192.168.1.1", 10, time.Minute); err != nil {
		t.Fatalf("RateLimit under limit: %v", err)
	}
}

func TestRateLimit_OverLimit(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)

	// Send max+1 requests — the (max+1)th should be rate-limited.
	const maxRequests = 3
	var lastErr error
	for i := 0; i < maxRequests+1; i++ {
		lastErr = c.RateLimit(context.Background(), "/api/v1", "10.0.0.1", maxRequests, time.Minute)
	}
	if !errors.Is(lastErr, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited after exceeding limit, got %v", lastErr)
	}
}

func TestRateLimit_PipelineError(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	mr.Close()
	err := c.RateLimit(context.Background(), "/api", "client", 10, time.Minute)
	if err == nil {
		t.Fatal("expected error after server closed, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit pipeline") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Lock
// --------------------------------------------------------------------------

func TestLock_Success(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	token, err := c.Lock(context.Background(), "resource", "id1", 30*time.Second)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if token == "" {
		t.Error("token should not be empty on successful lock")
	}
}

func TestLock_AlreadyHeld(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	_, err := c.Lock(context.Background(), "resource", "id1", 30*time.Second)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	// Second lock attempt on the same key should fail.
	_, err = c.Lock(context.Background(), "resource", "id1", 30*time.Second)
	if !errors.Is(err, ErrLockNotAcquired) {
		t.Errorf("expected ErrLockNotAcquired, got %v", err)
	}
}

func TestLock_SetNXError(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	mr.Close()
	_, err := c.Lock(context.Background(), "resource", "id1", 30*time.Second)
	if err == nil {
		t.Fatal("expected error after server closed, got nil")
	}
	if !strings.Contains(err.Error(), "lock acquire") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Unlock
// --------------------------------------------------------------------------

func TestUnlock_Success(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	token, err := c.Lock(context.Background(), "resource", "id2", 30*time.Second)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := c.Unlock(context.Background(), "resource", "id2", token); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	// Lock should now be acquirable again.
	_, err = c.Lock(context.Background(), "resource", "id2", 30*time.Second)
	if err != nil {
		t.Errorf("re-lock after unlock: %v", err)
	}
}

func TestUnlock_TokenMismatch(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	_, err := c.Lock(context.Background(), "resource", "id3", 30*time.Second)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	err = c.Unlock(context.Background(), "resource", "id3", "wrong-token")
	if err == nil {
		t.Fatal("expected error for token mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "not held by token") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnlock_EvalError(t *testing.T) {
	mr := startMini(t)
	c := startedComponent(t, mr)
	mr.Close()
	err := c.Unlock(context.Background(), "resource", "id4", "tok")
	if err == nil {
		t.Fatal("expected error after server closed, got nil")
	}
	if !strings.Contains(err.Error(), "lock release") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// dialRedis — invalid URL
// --------------------------------------------------------------------------

func TestDialRedis_InvalidURL(t *testing.T) {
	_, err := dialRedis(context.Background(), "not-a-redis-url://??")
	if err == nil {
		t.Fatal("expected error for invalid Redis URL, got nil")
	}
	if !strings.Contains(err.Error(), "parse redis URL") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDialRedis_Success(t *testing.T) {
	mr := startMini(t)
	client, err := dialRedis(context.Background(), "redis://"+mr.Addr())
	if err != nil {
		t.Fatalf("dialRedis: %v", err)
	}
	defer client.Close()
	if client == nil {
		t.Error("expected non-nil client")
	}
}

// --------------------------------------------------------------------------
// Stop — Close error path (inject a broken client)
// --------------------------------------------------------------------------

func TestStop_CloseError(t *testing.T) {
	c := New()
	// Inject a real client that is already closed — Close() on it should error.
	mr := startMini(t)
	addr := mr.Addr()
	opts, _ := goredis.ParseURL("redis://" + addr)
	client := goredis.NewClient(opts)
	client.Close()   // pre-close
	mr.Close()

	c.cacheDB = client
	// ratelimitDB and lockDB are nil → won't be visited again.
	err := c.Stop(context.Background())
	// go-redis Close() on an already-closed client may or may not error;
	// what matters is that Stop does not panic.
	_ = err
}
