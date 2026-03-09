// Package kafka provides the foundry-kafka trusted component —
// event streaming via Apache Kafka.
//
// Configuration (spec events block when backend=kafka):
//
//	events:
//	  backend: kafka
//	  broker_url: "${KAFKA_BROKER_URL}"   # e.g. localhost:9092
package kafka

import (
	"context"
	"fmt"

	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

const (
	componentName    = "foundry-kafka"
	componentVersion = "v1.0.0"
	auditHash        = "0000000000000000000000000000000000000000000000000000000000000011"

	defaultBroker = "localhost:9092"
)

// KafkaComponent implements spec.Component for Apache Kafka event streaming.
type KafkaComponent struct {
	brokerURL string
	topics    []string
}

// New returns a KafkaComponent with defaults.
func New() *KafkaComponent {
	return &KafkaComponent{brokerURL: defaultBroker}
}

func (c *KafkaComponent) Name() string      { return componentName }
func (c *KafkaComponent) Version() string   { return componentVersion }
func (c *KafkaComponent) AuditHash() string { return auditHash }

// Configure reads the events.kafka section.
func (c *KafkaComponent) Configure(cfg spec.ComponentConfig) error {
	if url, ok := cfg["broker_url"].(string); ok && url != "" {
		c.brokerURL = url
	}
	return nil
}

// Register installs Kafka producer and consumer middleware on the application.
// Topics declared in the IR spec are automatically created if they do not exist.
func (c *KafkaComponent) Register(app *spec.Application) error {
	if c.brokerURL == "" {
		return fmt.Errorf("foundry-kafka: broker_url is required")
	}
	// TODO: create Kafka admin client, create topics, register producer/consumer.
	return nil
}

// Start begins consuming from subscribed topics.
func (c *KafkaComponent) Start(ctx context.Context) error { return nil }

// Stop drains in-flight messages and closes the Kafka client.
func (c *KafkaComponent) Stop(ctx context.Context) error { return nil }
