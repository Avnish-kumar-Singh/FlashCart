package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a Redis client used for three distinct purposes in Phase 2:
//   - Cart storage (fast read/write, no need for durability guarantees Postgres gives)
//   - Idempotency-Key -> Order ID mapping (checkout dedup)
//   - Rate limiting counters (login, checkout)
//
// All three share one client for now; if load profiles diverge later
// (e.g. rate limiting needs its own low-latency instance), split them.
func NewClient(addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("unable to connect to redis: %w", err)
	}

	return client, nil
}
