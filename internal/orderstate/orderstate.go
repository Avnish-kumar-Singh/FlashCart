package orderstate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Status string

const (
	Created        Status = "CREATED"
	PaymentPending Status = "PAYMENT_PENDING"
	Paid           Status = "PAID"
	Confirmed      Status = "CONFIRMED"
	PaymentFailed  Status = "PAYMENT_FAILED"
	Cancelled      Status = "ORDER_CANCELLED"
)

// allowedTransitions is the single source of truth for the order
// lifecycle. Anything not listed here is rejected, so a bug that tries to
// e.g. jump straight from CREATED to CONFIRMED (skipping payment) fails
// loudly in code instead of silently corrupting order history.
//
//	CREATED -> PAYMENT_PENDING -> PAID -> CONFIRMED
//	                           \-> PAYMENT_FAILED -> ORDER_CANCELLED
var allowedTransitions = map[Status][]Status{
	Created:        {PaymentPending},
	PaymentPending: {Paid, PaymentFailed},
	Paid:           {Confirmed},
	PaymentFailed:  {Cancelled},
	Confirmed:      {}, // terminal
	Cancelled:      {}, // terminal
}

func CanTransition(from, to Status) bool {
	for _, allowed := range allowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// ErrInvalidTransition is returned by Transition when the DB's current
// status doesn't permit moving to the requested status.
type ErrInvalidTransition struct {
	From, To Status
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid order status transition: %s -> %s", e.From, e.To)
}

// Transition atomically reads the order's current status with a row lock,
// validates the transition, and writes the new status inside a single DB
// transaction. This prevents two concurrent workers from both successfully
// transitioning the same order — e.g. a redelivered Kafka message racing
// a fresh one both trying to move PAYMENT_PENDING -> PAID.
func Transition(ctx context.Context, db *pgxpool.Pool, orderID string, to Status) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var current Status
	if err := tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&current); err != nil {
		return err
	}

	if !CanTransition(current, to) {
		return &ErrInvalidTransition{From: current, To: to}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE orders
		SET status=$1,
		    invoice_eligible_at=CASE WHEN $1='CONFIRMED' THEN COALESCE(invoice_eligible_at, now()) ELSE invoice_eligible_at END
		WHERE id=$2`, to, orderID); err != nil {
		return err
	}
	if to == Confirmed {
		if _, err := tx.Exec(ctx, `UPDATE coupon_redemptions SET status='APPLIED',updated_at=now() WHERE order_id=$1 AND status='RESERVED'`, orderID); err != nil {
			return err
		}
	}
	if to == Cancelled {
		tag, err := tx.Exec(ctx, `UPDATE coupon_redemptions SET status='RELEASED',updated_at=now() WHERE order_id=$1 AND status='RESERVED'`, orderID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			if _, err := tx.Exec(ctx, `UPDATE coupons SET used_count=GREATEST(used_count-1,0) WHERE code=(SELECT coupon_code FROM orders WHERE id=$1)`, orderID); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

// Current reads the order's status without locking — used for redelivery
// checks where a worker just needs to know "has this already moved past
// where I'd act on it?" rather than intending to transition it itself.
func Current(ctx context.Context, db *pgxpool.Pool, orderID string) (Status, error) {
	var status Status
	err := db.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status)
	return status, err
}
