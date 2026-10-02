-- Phase 2 moves the cart to Redis (see internal/handlers/cart.go).
-- The Postgres cart_items table from Phase 1 is no longer written to or
-- read from, so it's dropped rather than left as dead schema.
DROP TABLE IF EXISTS cart_items;
