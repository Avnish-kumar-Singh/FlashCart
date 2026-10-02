package kafka

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// Consumer wraps a kafka.Reader bound to one topic + consumer group.
// Using a consumer group (not a raw partition reader) means running
// multiple replicas of the same worker automatically shares the load —
// this is exactly the "Order Workers: 10 -> 100" horizontal scaling
// described in the architecture, with Kafka handling partition
// rebalancing for free.
type Consumer struct {
	reader *kafka.Reader
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,
			// StartOffset only matters for a brand new consumer group;
			// after that, committed offsets take over.
			StartOffset: kafka.FirstOffset,
		}),
	}
}

// Run blocks, invoking handler for each message and committing the offset
// only after handler returns nil. If handler returns an error, the offset
// is NOT committed — kafka-go's default reader will redeliver the message
// on the next poll after a restart, which is the correct behavior for a
// crashed worker (per "Worker fails -> job remains/retries -> another
// worker processes it" from the architecture notes). Handlers must
// therefore be idempotent-safe for redelivery; the order worker achieves
// this by re-checking order status before acting.
func (c *Consumer) Run(ctx context.Context, handler func(ctx context.Context, key, value []byte) error) {
	retryDelay := time.Second
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // context cancelled, shutting down
			}
			log.Printf("kafka: fetch error: %v", err)
			continue
		}

		if err := handler(ctx, msg.Key, msg.Value); err != nil {
			log.Printf("kafka: handler error, message will be redelivered: %v", err)
			timer := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if retryDelay < 30*time.Second {
				retryDelay *= 2
			}
			continue // don't commit; will be redelivered
		}
		retryDelay = time.Second

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("kafka: commit error: %v", err)
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
