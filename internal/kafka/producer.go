package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer wraps a per-topic kafka.Writer. One Producer instance can be
// reused for many topics — kafka-go internally routes based on the topic
// passed to WriteMessages, so we don't need a writer per topic.
type Producer struct {
	writer *kafka.Writer
}

func NewProducer(brokers []string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireAll, // wait for all in-sync replicas before acking the write
			BatchTimeout: 10 * time.Millisecond,
		},
	}
}

// Publish JSON-encodes the event and writes it to the given topic, keyed
// by `key` (typically the order ID) so all events for one order land on
// the same partition and are processed in order by a single consumer.
func (p *Producer) Publish(ctx context.Context, topic, key string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
		Time:  time.Now(),
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
