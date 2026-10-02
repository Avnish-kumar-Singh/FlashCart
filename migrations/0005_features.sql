-- Feature pass: product images, payment method selection, flash-sale
-- analytics linkage, admin announcements ("Boom" banner), broadcast
-- messages (pre-festival announcements via email/WhatsApp), wishlist,
-- and flash-sale alert subscriptions.

ALTER TABLE users ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT '';

ALTER TABLE products ADD COLUMN IF NOT EXISTS image_url TEXT NOT NULL DEFAULT '';

ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_method TEXT NOT NULL DEFAULT 'COD';
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_payment_method_check;
ALTER TABLE orders ADD CONSTRAINT orders_payment_method_check
    CHECK (payment_method IN ('COD', 'CARD', 'UPI', 'RAZORPAY'));

-- Nullable, only set for flash-sale orders — lets the analytics dashboard
-- join orders back to the specific sale that produced them, since a
-- product can run through many flash sales over its lifetime.
ALTER TABLE orders ADD COLUMN IF NOT EXISTS flash_sale_id UUID;
CREATE INDEX IF NOT EXISTS idx_orders_flash_sale_id ON orders(flash_sale_id) WHERE flash_sale_id IS NOT NULL;

-- One row = one banner. Doubles as the "admin message" (e.g. "Diwali Sale
-- Begins!") and the "boom link" (a festival/promo URL) — a banner without
-- a message wouldn't make sense, so they're one entity, not two.
CREATE TABLE IF NOT EXISTS announcements (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message    TEXT NOT NULL,
    link       TEXT NOT NULL DEFAULT '',
    active     BOOLEAN NOT NULL DEFAULT true,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_announcements_active ON announcements(active, created_at DESC);

-- Pre-festival broadcast messages. FlashCart has no real email/WhatsApp
-- provider wired in (that needs real credentials — see
-- internal/notify/simulator.go), so `status` tracks the simulated send;
-- swapping in a real provider only touches that one file.
CREATE TABLE IF NOT EXISTS broadcast_messages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel          TEXT NOT NULL CHECK (channel IN ('EMAIL', 'WHATSAPP', 'BOTH')),
    message          TEXT NOT NULL,
    recipient_count  INTEGER NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'QUEUED' CHECK (status IN ('QUEUED', 'SENT', 'FAILED')),
    created_by       UUID REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS wishlist_items (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, product_id)
);

-- Flash-sale alert subscriptions. product_id NULL means "notify me about
-- any new flash sale"; a specific product_id means "notify me only when
-- this product goes on sale". Either a logged-in user_id or a bare email
-- can subscribe, so the storefront can offer this without forcing signup.
CREATE TABLE IF NOT EXISTS flashsale_subscriptions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID REFERENCES users(id) ON DELETE CASCADE,
    email      TEXT,
    product_id UUID REFERENCES products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT flashsale_subscriptions_identity_check CHECK (user_id IS NOT NULL OR email IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_flashsale_subscriptions_product ON flashsale_subscriptions(product_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_flashsale_sub_user_product
    ON flashsale_subscriptions(user_id, product_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_flashsale_sub_email_product
    ON flashsale_subscriptions(email, product_id) WHERE email IS NOT NULL;
