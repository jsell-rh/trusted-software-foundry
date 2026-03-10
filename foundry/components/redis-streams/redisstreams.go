// Package redisstreams provides the foundry-redis-streams trusted component —
// persistent event streaming via Redis Streams.
//
// Configuration (spec events block when backend=redis-streams):
//
//	events:
//	  backend: redis-streams
//	  broker_url: "${REDIS_URL}"   # default: redis://localhost:6379
package redisstreams

import (
	"context"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

const (
	componentName    = "foundry-redis-streams"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000013"

	defaultURL = "redis://localhost:6379"
)

// RedisStreamsComponent implements spec.Component for Redis Streams event bus.
type RedisStreamsComponent struct {
	url       string
	maxLen    int64  // max stream length (MAXLEN ~) before trimming
	groupName string // consumer group name
}

// New returns a RedisStreamsComponent with defaults.
func New() *RedisStreamsComponent {
	return &RedisStreamsComponent{
		url:       defaultURL,
		maxLen:    100_000,
		groupName: "foundry-consumers",
	}
}

func (c *RedisStreamsComponent) Name() string      { return componentName }
func (c *RedisStreamsComponent) Version() string   { return componentVersion }
func (c *RedisStreamsComponent) AuditHash() string { return auditHash }

// Configure reads the events.redis-streams section.
func (c *RedisStreamsComponent) Configure(cfg spec.ComponentConfig) error {
	if url, ok := cfg["broker_url"].(string); ok && url != "" {
		c.url = url
	}
	if group, ok := cfg["consumer_group"].(string); ok && group != "" {
		c.groupName = group
	}
	return nil
}

// Register installs the Redis Streams producer and consumer group.
func (c *RedisStreamsComponent) Register(app *spec.Application) error {
	if c.url == "" {
		return fmt.Errorf("foundry-redis-streams: broker_url is required")
	}
	// TODO: connect to Redis, create streams and consumer groups, register helpers.
	return nil
}

// Start begins the consumer group read loop.
func (c *RedisStreamsComponent) Start(ctx context.Context) error { return nil }

// Stop drains pending messages and closes Redis connection.
func (c *RedisStreamsComponent) Stop(ctx context.Context) error { return nil }
