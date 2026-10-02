package payment

import (
	"context"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

type Status string

const (
	Success Status = "SUCCESS"
	Failed  Status = "FAILED"
)

// Simulator stands in for a real payment gateway. It never touches real
// money, but it deliberately models the two properties that make payment
// a distributed-systems problem in production:
//
//  1. Non-trivial latency (so callers can't assume it's instant).
//  2. A configurable failure rate (so the saga's compensation path
//     actually gets exercised, not just the happy path).
type Simulator struct {
	Redis       *redis.Client
	FailureRate float64 // 0.0–1.0, e.g. 0.1 = 10% of payments fail
}

func NewSimulator(redisClient *redis.Client, failureRate float64) *Simulator {
	return &Simulator{Redis: redisClient, FailureRate: failureRate}
}

// Charge processes a payment for orderID, idempotently. If this order has
// already been charged (the Redis key exists), the *original* result is
// returned rather than charging again — this is the
//
//	Idempotency-Key: abc123
//	Request 1 -> SUCCESS
//	Request 2 -> return existing result
//
// behavior from the spec, keyed on order ID since exactly one payment
// attempt should ever exist per order in this simulator.
func (s *Simulator) Charge(ctx context.Context, orderID string, amountPaise int64) (Status, error) {
	key := "payment:" + orderID

	existing, err := s.Redis.Get(ctx, key).Result()
	if err == nil {
		return Status(existing), nil
	}
	if err != redis.Nil {
		return "", err
	}

	// Simulate gateway latency.
	time.Sleep(50 * time.Millisecond)

	result := Success
	if rand.Float64() < s.FailureRate {
		result = Failed
	}

	if err := s.Redis.Set(ctx, key, string(result), 48*time.Hour).Err(); err != nil {
		return "", err
	}

	return result, nil
}
