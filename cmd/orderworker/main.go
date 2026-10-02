// Command orderworker is the saga orchestrator described in the FlashCart
// architecture: it consumes order.created, calls the payment simulator,
// and drives each order to a terminal state — CONFIRMED on success, or
// ORDER_CANCELLED (with inventory released) on failure.
//
//	Create Order -> Reserve Inventory -> Process Payment -> Confirm Order
//	                                          |
//	                                          v (failure)
//	                              Release Inventory -> Cancel Order
//
// Run several replicas of this binary and Kafka's consumer-group
// rebalancing shares the order.created partitions between them — this is
// the "Order Workers: 10 -> 100" horizontal scaling from the architecture
// notes, achieved without any code change here.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/flashcart/internal/cache"
	"github.com/yourname/flashcart/internal/config"
	"github.com/yourname/flashcart/internal/db"
	"github.com/yourname/flashcart/internal/events"
	"github.com/yourname/flashcart/internal/inventory"
	"github.com/yourname/flashcart/internal/kafka"
	"github.com/yourname/flashcart/internal/metrics"
	"github.com/yourname/flashcart/internal/orderstate"
	"github.com/yourname/flashcart/internal/payment"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	redisClient, err := cache.NewClient(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}
	defer redisClient.Close()

	simulator := payment.NewSimulator(redisClient, cfg.PaymentFailureRate)
	producer := kafka.NewProducer(cfg.KafkaBrokers)
	defer producer.Close()

	consumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicOrderCreated, "order-worker")
	defer consumer.Close()

	go func() {
		if err := http.ListenAndServe(":9090", metrics.Handler()); err != nil {
			log.Printf("metrics server stopped: %v", err)
		}
	}()

	log.Println("order worker started, waiting for order.created events")

	consumer.Run(ctx, func(ctx context.Context, key, value []byte) error {
		var evt events.OrderCreated
		if err := json.Unmarshal(value, &evt); err != nil {
			metrics.WorkerMessages.WithLabelValues("order", "invalid").Inc()
			log.Printf("order worker: bad message, dropping: %v", err)
			return nil // don't redeliver a message that will never parse
		}
		err := processOrder(ctx, pool, redisClient, simulator, producer, evt)
		if err != nil {
			metrics.WorkerMessages.WithLabelValues("order", "error").Inc()
		} else {
			metrics.WorkerMessages.WithLabelValues("order", "success").Inc()
		}
		return err
	})
}

// processOrder runs one pass of the saga for a single order. It's written
// to be safe under Kafka at-least-once redelivery: before doing anything,
// it checks the order's current status and no-ops if a previous run
// already moved it past PAYMENT_PENDING — so a redelivered message never
// double-charges or double-releases inventory.
func processOrder(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, simulator *payment.Simulator, producer *kafka.Producer, evt events.OrderCreated) error {
	current, err := orderstate.Current(ctx, pool, evt.OrderID)
	if err != nil {
		return err
	}
	if current != orderstate.PaymentPending {
		log.Printf("order %s already past PAYMENT_PENDING (status=%s), skipping redelivered event", evt.OrderID, current)
		return nil
	}

	// Razorpay is completed through Checkout + server-side signature
	// verification. The worker must not auto-charge it with the simulator.
	var paymentMethod string
	if err := pool.QueryRow(ctx, `SELECT payment_method FROM orders WHERE id=$1`, evt.OrderID).Scan(&paymentMethod); err != nil {
		return err
	}
	if paymentMethod == "RAZORPAY" {
		log.Printf("order %s awaiting Razorpay Checkout verification", evt.OrderID)
		return nil
	}

	status, err := simulator.Charge(ctx, evt.OrderID, evt.TotalPaise)
	if err != nil {
		metrics.PaymentAttempts.WithLabelValues("error").Inc()
		return err // Kafka will redeliver; order stays PAYMENT_PENDING, which is correct
	}
	if status == payment.Success {
		metrics.PaymentAttempts.WithLabelValues("success").Inc()
		return confirmOrder(ctx, pool, redisClient, producer, evt)
	}
	metrics.PaymentAttempts.WithLabelValues("failed").Inc()
	return cancelOrder(ctx, pool, redisClient, producer, evt, "payment failed")
}

// confirmOrder walks PAYMENT_PENDING -> PAID -> CONFIRMED and announces it.
func confirmOrder(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, producer *kafka.Producer, evt events.OrderCreated) error {
	if evt.Source == events.SourceFlashSale {
		if err := inventory.Consume(ctx, redisClient, evt.FlashSaleID, evt.UserID, evt.ReservationID); err != nil {
			return err
		}
	}
	if err := orderstate.Transition(ctx, pool, evt.OrderID, orderstate.Paid); err != nil {
		return err
	}
	if err := orderstate.Transition(ctx, pool, evt.OrderID, orderstate.Confirmed); err != nil {
		return err
	}
	log.Printf("order %s CONFIRMED", evt.OrderID)

	return producer.Publish(ctx, events.TopicOrderConfirmed, evt.OrderID, events.OrderConfirmed{
		OrderID: evt.OrderID,
		UserID:  evt.UserID,
	})
}

// cancelOrder is the compensating transaction: walk PAYMENT_PENDING ->
// PAYMENT_FAILED -> ORDER_CANCELLED, release every unit of inventory that
// was reserved at checkout, and announce the cancellation.
//
//	Payment FAILED -> Release Inventory -> Cancel Order
//
// Inventory is released inside its own DB transaction, separate from the
// status transition, deliberately: even if publishing the cancellation
// event later fails, stock has already been safely returned rather than
// left reserved forever.
func cancelOrder(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, producer *kafka.Producer, evt events.OrderCreated, reason string) error {
	if err := orderstate.Transition(ctx, pool, evt.OrderID, orderstate.PaymentFailed); err != nil {
		return err
	}

	if evt.Source == events.SourceFlashSale {
		if err := inventory.Release(ctx, redisClient, evt.FlashSaleID, evt.UserID, evt.ReservationID); err != nil {
			return err
		}
	} else {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		for _, item := range evt.Items {
			if _, err := tx.Exec(ctx, `UPDATE products SET stock = stock + $1 WHERE id = $2`, item.Quantity, item.ProductID); err != nil {
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	if err := orderstate.Transition(ctx, pool, evt.OrderID, orderstate.Cancelled); err != nil {
		return err
	}
	log.Printf("order %s CANCELLED (%s), inventory released", evt.OrderID, reason)

	return producer.Publish(ctx, events.TopicOrderCancelled, evt.OrderID, events.OrderCancelled{
		OrderID: evt.OrderID,
		UserID:  evt.UserID,
		Reason:  reason,
	})
}
