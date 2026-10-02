-- Phase 8: marketplace coupons.
ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_code TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS discount_paise BIGINT NOT NULL DEFAULT 0 CHECK (discount_paise >= 0);

CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    discount_type TEXT NOT NULL CHECK (discount_type IN ('PERCENT','FIXED')),
    discount_value BIGINT NOT NULL CHECK (discount_value > 0),
    min_order_paise BIGINT NOT NULL DEFAULT 0 CHECK (min_order_paise >= 0),
    max_discount_paise BIGINT,
    max_uses INTEGER NOT NULL DEFAULT 0 CHECK (max_uses >= 0),
    used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons(active, expires_at);

INSERT INTO coupons (code, discount_type, discount_value, min_order_paise, max_discount_paise, max_uses, active)
VALUES ('FLASH10','PERCENT',10,50000,100000,1000,true)
ON CONFLICT (code) DO NOTHING;
