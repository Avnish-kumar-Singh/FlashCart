-- Phase 7: Razorpay test-mode payment records and customer addresses.
-- Razorpay secrets stay in environment variables; only payment identifiers
-- and server-side status are persisted here.

CREATE TABLE IF NOT EXISTS payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    razorpay_order_id   TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT,
    razorpay_signature  TEXT,
    amount_paise        BIGINT NOT NULL CHECK (amount_paise >= 0),
    currency            TEXT NOT NULL DEFAULT 'INR',
    method              TEXT NOT NULL DEFAULT 'RAZORPAY',
    status              TEXT NOT NULL DEFAULT 'CREATED'
                        CHECK (status IN ('CREATED','AUTHORIZED','CAPTURED','FAILED','REFUNDED')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments(created_at DESC);

CREATE TABLE IF NOT EXISTS user_addresses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label       TEXT NOT NULL DEFAULT 'Home',
    full_name   TEXT NOT NULL,
    phone       TEXT NOT NULL,
    line1       TEXT NOT NULL,
    line2       TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT '',
    pincode     TEXT NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_user_addresses_user ON user_addresses(user_id, created_at DESC);
