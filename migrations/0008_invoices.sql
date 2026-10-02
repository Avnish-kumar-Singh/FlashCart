ALTER TABLE products
    ADD COLUMN IF NOT EXISTS tax_rate_bps INTEGER NOT NULL DEFAULT 0
    CHECK (tax_rate_bps BETWEEN 0 AND 10000);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS invoice_eligible_at TIMESTAMPTZ;

ALTER TABLE order_items
    ADD COLUMN IF NOT EXISTS product_name_snapshot TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tax_rate_bps INTEGER NOT NULL DEFAULT 0
    CHECK (tax_rate_bps BETWEEN 0 AND 10000);

UPDATE order_items oi
SET product_name_snapshot = p.name,
    tax_rate_bps = p.tax_rate_bps
FROM products p
WHERE p.id = oi.product_id AND oi.product_name_snapshot = '';

CREATE SEQUENCE IF NOT EXISTS invoice_number_seq;

CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL UNIQUE REFERENCES orders(id),
    invoice_number TEXT NOT NULL UNIQUE,
    issued_at TIMESTAMPTZ NOT NULL,
    customer_name TEXT NOT NULL,
    customer_email TEXT NOT NULL,
    subtotal_paise BIGINT NOT NULL CHECK (subtotal_paise >= 0),
    discount_paise BIGINT NOT NULL CHECK (discount_paise >= 0),
    tax_paise BIGINT NOT NULL CHECK (tax_paise >= 0),
    total_paise BIGINT NOT NULL CHECK (total_paise >= 0),
    payment_method TEXT NOT NULL,
    payment_reference TEXT NOT NULL DEFAULT '',
    email_status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (email_status IN ('PENDING','SENDING','SENT','FAILED')),
    email_attempted_at TIMESTAMPTZ,
    email_sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invoices_customer_email ON invoices(customer_email, issued_at DESC);

CREATE TABLE IF NOT EXISTS invoice_email_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id UUID NOT NULL UNIQUE REFERENCES invoices(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PROCESSING','SENT')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invoice_email_outbox_pending
    ON invoice_email_outbox(next_attempt_at, created_at) WHERE status <> 'SENT';

INSERT INTO invoice_email_outbox(invoice_id)
SELECT i.id
FROM invoices i
JOIN orders o ON o.id=i.order_id
WHERE o.invoice_eligible_at IS NOT NULL AND i.email_status <> 'SENT'
ON CONFLICT(invoice_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS invoice_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    product_name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_paise BIGINT NOT NULL CHECK (unit_price_paise >= 0),
    discount_paise BIGINT NOT NULL DEFAULT 0 CHECK (discount_paise >= 0),
    tax_rate_bps INTEGER NOT NULL CHECK (tax_rate_bps BETWEEN 0 AND 10000),
    tax_paise BIGINT NOT NULL CHECK (tax_paise >= 0),
    line_total_paise BIGINT NOT NULL CHECK (line_total_paise >= 0)
);
CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice ON invoice_items(invoice_id);