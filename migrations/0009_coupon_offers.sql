ALTER TABLE coupons ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS offer_type TEXT NOT NULL DEFAULT 'CART';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS eligible_category TEXT NOT NULL DEFAULT '';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS required_payment_method TEXT NOT NULL DEFAULT '';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS required_issuer TEXT NOT NULL DEFAULT '';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS new_user_only BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_instrument TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_issuer TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS coupon_redemptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id UUID NOT NULL REFERENCES coupons(id),
    order_id UUID NOT NULL UNIQUE REFERENCES orders(id),
    user_id UUID NOT NULL REFERENCES users(id),
    discount_paise BIGINT NOT NULL CHECK (discount_paise >= 0),
    status TEXT NOT NULL DEFAULT 'RESERVED' CHECK (status IN ('RESERVED','APPLIED','RELEASED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_coupon_redemption_active_user
    ON coupon_redemptions(coupon_id, user_id) WHERE status IN ('RESERVED','APPLIED');
CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_coupon ON coupon_redemptions(coupon_id, created_at DESC);

INSERT INTO coupons (code, discount_type, discount_value, min_order_paise, max_discount_paise,
                     max_uses, active, description, offer_type, eligible_category,
                     required_payment_method, required_issuer, new_user_only)
VALUES
    ('SBI10','PERCENT',10,50000,100000,10000,true,'10% off with eligible SBI credit cards','BANK','','CARD','SBI',false),
    ('HDFC12','PERCENT',12,50000,150000,10000,true,'12% off with eligible HDFC credit cards','BANK','','CARD','HDFC',false),
    ('ICICI8','PERCENT',8,50000,100000,10000,true,'8% off with eligible ICICI credit cards','BANK','','CARD','ICICI',false),
    ('AXIS10','PERCENT',10,50000,100000,10000,true,'10% off with eligible Axis Bank credit cards','BANK','','CARD','AXIS',false),
    ('PHONEPE50','FIXED',5000,50000,5000,10000,true,'Rs 50 PhonePe wallet offer','WALLET','','WALLET','PHONEPE',false),
    ('PAYTM30','FIXED',3000,50000,3000,10000,true,'Rs 30 Paytm wallet offer','WALLET','','WALLET','PAYTM',false),
    ('FESTIVE10','PERCENT',10,50000,150000,10000,true,'10% seasonal discount','SEASONAL','','','',false),
    ('NEWUSER15','PERCENT',15,50000,200000,10000,true,'15% for first-time customers','NEW_USER','','','',true),
    ('GROCERY5','FIXED',500,50000,500,10000,true,'Rs 5 off eligible grocery items','CATEGORY','Grocery','','',false),
    ('SNACKS10','PERCENT',10,50000,100000,10000,true,'10% off eligible snacks','CATEGORY','Snacks','','',false),
    ('SAVE50','FIXED',5000,50000,5000,10000,true,'Rs 50 off carts above Rs 500','CART','','','',false),
    ('SAVE100','FIXED',10000,100000,10000,10000,true,'Rs 100 off carts above Rs 1,000','CART','','','',false),
    ('FLASH10','PERCENT',10,50000,100000,10000,true,'10% off eligible carts above Rs 500','CART','','','',false)
ON CONFLICT (code) DO UPDATE SET
    description=EXCLUDED.description,
    offer_type=EXCLUDED.offer_type,
    eligible_category=EXCLUDED.eligible_category,
    required_payment_method=EXCLUDED.required_payment_method,
    required_issuer=EXCLUDED.required_issuer,
    new_user_only=EXCLUDED.new_user_only;