-- Phase 4: identify where inventory was reserved so the saga can compensate
-- Redis-backed flash-sale reservations differently from normal DB reservations.
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'CART';

ALTER TABLE orders
    ADD CONSTRAINT orders_source_check CHECK (source IN ('CART', 'FLASH_SALE'));

CREATE INDEX IF NOT EXISTS idx_orders_source ON orders(source);
