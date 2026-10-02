package db

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a connection pool to PostgreSQL.
// Kept as a thin wrapper now so Phase 2 can add pool tuning
// (max conns, health checks) without touching call sites.
func NewPool(databaseURL string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return pool, nil
}

// EnsureAdmin creates the configured development admin if it does not exist
// and promotes the matching email to ADMIN. This keeps a fresh Docker volume
// and an existing Phase 1-5 volume both usable without manual SQL.
func EnsureAdmin(pool *pgxpool.Pool, email, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'USER'`); err != nil {
		return fmt.Errorf("ensure user role column: %w", err)
	}
	if _, err := pool.Exec(ctx, marketplaceSchemaSQL); err != nil {
		return fmt.Errorf("ensure marketplace schema: %w", err)
	}

	if err := ensureDemoCatalog(ctx, pool); err != nil {
		return fmt.Errorf("ensure demo catalog: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO categories (name, slug)
		SELECT DISTINCT trim(category), trim(both '-' from regexp_replace(lower(trim(category)), '[^a-z0-9]+', '-', 'g'))
		FROM products
		WHERE trim(category) <> ''
		ON CONFLICT (slug) DO NOTHING`); err != nil {
		return fmt.Errorf("ensure product categories: %w", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check`); err != nil {
		return fmt.Errorf("ensure user role constraint: %w", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('USER','ADMIN'))`); err != nil {
		return fmt.Errorf("ensure user role constraint: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ('FlashCart Admin', $1, $2, 'ADMIN')
		ON CONFLICT (email) DO UPDATE SET role = 'ADMIN'
	`, email, string(hash))
	if err != nil {
		return fmt.Errorf("ensure admin: %w", err)
	}
	return nil
}

const marketplaceSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
ALTER TABLE users ADD COLUMN IF NOT EXISTS theme TEXT NOT NULL DEFAULT 'light' CHECK (theme IN ('light','dark','gradient','aurora','neon','particles'));
CREATE TABLE IF NOT EXISTS festival_events (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	provider_code TEXT NOT NULL DEFAULT '', country_code TEXT NOT NULL, holiday_date DATE NOT NULL,
	name TEXT NOT NULL, local_name TEXT NOT NULL DEFAULT '', counties JSONB NOT NULL DEFAULT '[]'::jsonb,
	global_holiday BOOLEAN NOT NULL DEFAULT false, starts_at TIMESTAMPTZ NOT NULL, ends_at TIMESTAMPTZ NOT NULL,
	status TEXT NOT NULL DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED','ACTIVATING','ACTIVE','ENDING','ENDED','FAILED')),
	discount_percent INTEGER NOT NULL DEFAULT 10 CHECK (discount_percent BETWEEN 1 AND 50),
	worker_lease_until TIMESTAMPTZ, reminder_queued_at TIMESTAMPTZ, start_notice_queued_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE(country_code, holiday_date, name), CHECK (ends_at > starts_at)
);
CREATE INDEX IF NOT EXISTS idx_festival_events_schedule ON festival_events(status, starts_at, ends_at);
CREATE TABLE IF NOT EXISTS festival_sales (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(), festival_event_id UUID NOT NULL REFERENCES festival_events(id) ON DELETE CASCADE,
	product_id UUID NOT NULL REFERENCES products(id), sale_id UUID NOT NULL UNIQUE,
	stock_allocated INTEGER NOT NULL CHECK (stock_allocated > 0), sale_price_paise BIGINT NOT NULL CHECK (sale_price_paise >= 0),
	discount_percent INTEGER NOT NULL CHECK (discount_percent BETWEEN 1 AND 50), remaining_to_restore INTEGER NOT NULL DEFAULT 0 CHECK (remaining_to_restore >= 0),
	status TEXT NOT NULL DEFAULT 'ALLOCATED' CHECK (status IN ('ALLOCATED','ACTIVE','ENDING','ENDED','FAILED')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE(festival_event_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_festival_sales_event_status ON festival_sales(festival_event_id, status);
CREATE TABLE IF NOT EXISTS festival_activity_log (
	id BIGSERIAL PRIMARY KEY, festival_event_id UUID NOT NULL REFERENCES festival_events(id) ON DELETE CASCADE,
	action TEXT NOT NULL, details JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_festival_activity_event ON festival_activity_log(festival_event_id, created_at DESC);
CREATE TABLE IF NOT EXISTS web_push_subscriptions (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	endpoint TEXT NOT NULL UNIQUE,
	p256dh TEXT NOT NULL,
	auth_secret TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_web_push_user ON web_push_subscriptions(user_id);
CREATE TABLE IF NOT EXISTS categories (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	name TEXT NOT NULL,
	slug TEXT NOT NULL UNIQUE,
	parent_id UUID REFERENCES categories(id) ON DELETE RESTRICT,
	description TEXT NOT NULL DEFAULT '',
	banner_image_url TEXT NOT NULL DEFAULT '',
	seo_title TEXT NOT NULL DEFAULT '',
	seo_description TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
	active BOOLEAN NOT NULL DEFAULT true,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK (parent_id IS NULL OR parent_id <> id)
);
CREATE INDEX IF NOT EXISTS idx_categories_parent_sort ON categories(parent_id, sort_order, name);
ALTER TABLE products ADD COLUMN IF NOT EXISTS tax_rate_bps INTEGER NOT NULL DEFAULT 0 CHECK (tax_rate_bps BETWEEN 0 AND 10000);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS invoice_eligible_at TIMESTAMPTZ;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS product_name_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS tax_rate_bps INTEGER NOT NULL DEFAULT 0 CHECK (tax_rate_bps BETWEEN 0 AND 10000);
UPDATE order_items oi SET product_name_snapshot=p.name, tax_rate_bps=p.tax_rate_bps FROM products p WHERE p.id=oi.product_id AND oi.product_name_snapshot='';
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
	email_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (email_status IN ('PENDING','SENDING','SENT','FAILED')),
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
CREATE INDEX IF NOT EXISTS idx_invoice_email_outbox_pending ON invoice_email_outbox(next_attempt_at, created_at) WHERE status <> 'SENT';
INSERT INTO invoice_email_outbox(invoice_id)
SELECT i.id FROM invoices i JOIN orders o ON o.id=i.order_id
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
ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_method TEXT NOT NULL DEFAULT 'COD';
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_payment_method_check;
ALTER TABLE orders ADD CONSTRAINT orders_payment_method_check CHECK (payment_method IN ('COD','CARD','QR','UPI','NETBANKING','WALLET','RAZORPAY'));
ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_code TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS discount_paise BIGINT NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    razorpay_order_id TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT,
    razorpay_signature TEXT,
    amount_paise BIGINT NOT NULL CHECK (amount_paise >= 0),
    currency TEXT NOT NULL DEFAULT 'INR',
    method TEXT NOT NULL DEFAULT 'RAZORPAY',
    status TEXT NOT NULL DEFAULT 'CREATED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS user_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label TEXT NOT NULL DEFAULT 'Home',
    full_name TEXT NOT NULL,
    phone TEXT NOT NULL,
    line1 TEXT NOT NULL,
    line2 TEXT NOT NULL DEFAULT '',
    city TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT '',
    pincode TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    discount_type TEXT NOT NULL CHECK (discount_type IN ('PERCENT','FIXED')),
    discount_value BIGINT NOT NULL CHECK (discount_value > 0),
    min_order_paise BIGINT NOT NULL DEFAULT 0,
    max_discount_paise BIGINT,
    max_uses INTEGER NOT NULL DEFAULT 0,
    used_count INTEGER NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
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
CREATE UNIQUE INDEX IF NOT EXISTS uq_coupon_redemption_active_user ON coupon_redemptions(coupon_id,user_id) WHERE status IN ('RESERVED','APPLIED');
CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_coupon ON coupon_redemptions(coupon_id,created_at DESC);
CREATE TABLE IF NOT EXISTS order_returns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'REQUESTED' CHECK (status IN ('REQUESTED','APPROVED','REJECTED','REFUNDED','CANCELLED')),
    amount_paise BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS support_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL,
    subject TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','IN_PROGRESS','RESOLVED','CLOSED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
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
ON CONFLICT (code) DO UPDATE SET description=EXCLUDED.description, offer_type=EXCLUDED.offer_type,
	eligible_category=EXCLUDED.eligible_category, required_payment_method=EXCLUDED.required_payment_method,
	required_issuer=EXCLUDED.required_issuer, new_user_only=EXCLUDED.new_user_only;
`

func ensureDemoCatalog(ctx context.Context, pool *pgxpool.Pool) error {
	type product struct {
		name, desc, brand, category, image string
		price                              int64
		stock                              int
	}
	products := []product{
		{"Galaxy Book 6 Laptop", "16GB RAM, 512GB SSD and premium WUXGA display.", "Samsung", "Electronics", "https://images.unsplash.com/photo-1496181133206-80ce9b88a853?auto=format&fit=crop&w=900&q=80", 12159000, 18},
		{"Pixel Pro Smartphone", "Flagship camera, fast OLED display and all-day battery.", "Google", "Mobiles", "https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?auto=format&fit=crop&w=900&q=80", 7499000, 32},
		{"Wireless ANC Headphones", "Immersive sound with active noise cancellation.", "Sony", "Electronics", "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?auto=format&fit=crop&w=900&q=80", 1299000, 45},
		{"Air Max Everyday Sneakers", "Lightweight sneakers for everyday comfort.", "Nike", "Fashion", "https://images.unsplash.com/photo-1542291026-7eec264c27ff?auto=format&fit=crop&w=900&q=80", 699900, 60},
		{"Smart LED TV 55 inch", "4K HDR smart television with cinematic sound.", "LG", "Electronics", "https://images.unsplash.com/photo-1593784991095-a205069470b6?auto=format&fit=crop&w=900&q=80", 4299000, 12},
		{"Cotton Casual Shirt", "Breathable regular-fit cotton shirt.", "Levis", "Fashion", "https://images.unsplash.com/photo-1603252110481-7ba873bf42ab?auto=format&fit=crop&w=900&q=80", 149900, 75},
		{"Air Fryer 5L", "Crispy meals with less oil and simple digital controls.", "Philips", "Appliances", "https://images.unsplash.com/photo-1585515320310-259814833e62?auto=format&fit=crop&w=900&q=80", 649900, 24},
		{"Modern Table Lamp", "Warm ambient lighting for desk and bedside.", "Ikea", "Home", "https://images.unsplash.com/photo-1507473885765-e6ed057f782c?auto=format&fit=crop&w=900&q=80", 189900, 40},
		{"Everyday Skincare Kit", "Cleanser, moisturizer and SPF for daily care.", "Minimalist", "Beauty", "https://images.unsplash.com/photo-1556228720-195a672e8a03?auto=format&fit=crop&w=900&q=80", 79900, 80},
		{"Premium Coffee Beans", "Freshly roasted Arabica beans, 500g.", "Blue Tokai", "Grocery", "https://images.unsplash.com/photo-1495474472287-4d71bcdd2085?auto=format&fit=crop&w=900&q=80", 54900, 100},
		{"Bluetooth Speaker", "Portable speaker with rich bass and 12-hour battery.", "JBL", "Electronics", "https://images.unsplash.com/photo-1608043152269-423dbba4e7e1?auto=format&fit=crop&w=900&q=80", 349900, 37},
		{"Robot Vacuum Cleaner", "Smart mapping, scheduled cleaning and app control.", "Ecovacs", "Home", "https://images.unsplash.com/photo-1567690187548-f07b1d7bf5a9?auto=format&fit=crop&w=900&q=80", 2199900, 9},

		// --- Electronics ---
		{"Ultrabook Pro 14", "Lightweight aluminium laptop with all-day battery life.", "Dell", "Electronics", "https://images.unsplash.com/photo-1531297484001-80022131f5a1?auto=format&fit=crop&w=900&q=80", 8999000, 22},
		{"Mechanical Gaming Keyboard", "RGB backlit keyboard with hot-swappable switches.", "Logitech", "Electronics", "https://images.unsplash.com/photo-1587829741301-dc798b83add3?auto=format&fit=crop&w=900&q=80", 649900, 55},
		{"Wireless Gaming Mouse", "Ultra-light mouse with 25k DPI sensor.", "Logitech", "Electronics", "https://images.unsplash.com/photo-1527814050087-3793815479db?auto=format&fit=crop&w=900&q=80", 449900, 60},
		{"27-inch 2K Monitor", "IPS panel with 165Hz refresh rate for work and play.", "LG", "Electronics", "https://images.unsplash.com/photo-1527443224154-c4a3942d3acf?auto=format&fit=crop&w=900&q=80", 2399900, 20},
		{"True Wireless Earbuds", "Active noise cancellation with 30-hour battery case.", "Boat", "Electronics", "https://images.unsplash.com/photo-1590658268037-6bf12165a8df?auto=format&fit=crop&w=900&q=80", 299900, 90},
		{"Smartwatch Series 5", "Heart-rate, SpO2 and sleep tracking with AMOLED display.", "Noise", "Electronics", "https://images.unsplash.com/photo-1544117519-31a4b719223d?auto=format&fit=crop&w=900&q=80", 549900, 48},
		{"Mirrorless Camera Kit", "24MP APS-C sensor with 18-55mm kit lens.", "Sony", "Electronics", "https://images.unsplash.com/photo-1516035069371-29a1b244cc32?auto=format&fit=crop&w=900&q=80", 5499000, 10},
		{"Portable Power Bank 20000mAh", "Fast charging power bank with dual USB-C ports.", "Anker", "Electronics", "https://images.unsplash.com/photo-1609091839311-d5365f9ff1c5?auto=format&fit=crop&w=900&q=80", 189900, 100},
		{"4K Action Camera", "Waterproof action cam with image stabilization.", "GoPro", "Electronics", "https://images.unsplash.com/photo-1519183071298-a2962be90b8e?auto=format&fit=crop&w=900&q=80", 3199900, 15},
		{"Home Wi-Fi Router AX3000", "Dual-band Wi-Fi 6 router for whole-home coverage.", "TP-Link", "Electronics", "https://images.unsplash.com/photo-1544197150-b99a580bb7a8?auto=format&fit=crop&w=900&q=80", 449900, 40},
		{"Gaming Console Pro", "4K gaming console with 1TB storage and controller.", "Sony", "Electronics", "https://images.unsplash.com/photo-1486401899868-0e435ed85128?auto=format&fit=crop&w=900&q=80", 4999900, 8},
		{"Tablet 10.9 inch", "All-day battery tablet great for study and entertainment.", "Samsung", "Electronics", "https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?auto=format&fit=crop&w=900&q=80", 3299900, 18},

		// --- Mobiles ---
		{"Nova X50 Smartphone", "6.7-inch AMOLED display with 108MP triple camera.", "Samsung", "Mobiles", "https://images.unsplash.com/photo-1598327105666-5b89351aff97?auto=format&fit=crop&w=900&q=80", 2699900, 40},
		{"Budget 5G Smartphone", "Great everyday performance with 5G connectivity.", "Xiaomi", "Mobiles", "https://images.unsplash.com/photo-1598965675045-45c6c7e4f0c8?auto=format&fit=crop&w=900&q=80", 1299900, 65},
		{"iView Pro Smartphone", "Pro-grade camera system and titanium frame.", "Apple", "Mobiles", "https://images.unsplash.com/photo-1592286927505-1def25115558?auto=format&fit=crop&w=900&q=80", 13999900, 14},
		{"Rugged Outdoor Phone", "Shockproof, dustproof phone built for the outdoors.", "Motorola", "Mobiles", "https://images.unsplash.com/photo-1601784551446-20c9e07cdbdb?auto=format&fit=crop&w=900&q=80", 1899900, 25},
		{"Phone Case & Screen Guard Combo", "Shock-absorbing case with tempered glass protector.", "Spigen", "Mobiles", "https://images.unsplash.com/photo-1601593346740-925612772716?auto=format&fit=crop&w=900&q=80", 79900, 150},
		{"Fast Charger 65W", "GaN fast charger compatible with most smartphones.", "Anker", "Mobiles", "https://images.unsplash.com/photo-1583863788434-e58a36330cf0?auto=format&fit=crop&w=900&q=80", 129900, 120},
		{"Foldable Smartphone", "Book-style foldable display with multitasking mode.", "Samsung", "Mobiles", "https://images.unsplash.com/photo-1610792516307-ea5acd9c3b00?auto=format&fit=crop&w=900&q=80", 15999900, 6},
		{"Feature Phone Classic", "Long standby battery, dual SIM basic phone.", "Nokia", "Mobiles", "https://images.unsplash.com/photo-1585060544812-6b45742d762f?auto=format&fit=crop&w=900&q=80", 189900, 80},

		// --- Fashion ---
		{"Running Shoes Flex", "Breathable mesh running shoes with cushioned sole.", "Adidas", "Fashion", "https://images.unsplash.com/photo-1595950653106-6c9ebd614d3a?auto=format&fit=crop&w=900&q=80", 549900, 70},
		{"Slim Fit Denim Jeans", "Stretchable slim-fit denim for everyday wear.", "Levis", "Fashion", "https://images.unsplash.com/photo-1542272604-787c3835535d?auto=format&fit=crop&w=900&q=80", 229900, 90},
		{"Bomber Jacket", "Water-resistant bomber jacket for cool evenings.", "H&M", "Fashion", "https://images.unsplash.com/photo-1551028719-00167b16eac5?auto=format&fit=crop&w=900&q=80", 349900, 45},
		{"Floral Summer Dress", "Lightweight floral dress perfect for summer outings.", "Zara", "Fashion", "https://images.unsplash.com/photo-1595777457583-95e059d581b8?auto=format&fit=crop&w=900&q=80", 279900, 50},
		{"Structured Handbag", "Faux-leather handbag with adjustable strap.", "Caprese", "Fashion", "https://images.unsplash.com/photo-1584917865442-de89df76afd3?auto=format&fit=crop&w=900&q=80", 199900, 55},
		{"Aviator Sunglasses", "UV-protected polarized aviator sunglasses.", "Ray-Ban", "Fashion", "https://images.unsplash.com/photo-1572635196237-14b3f281503f?auto=format&fit=crop&w=900&q=80", 399900, 65},
		{"Analog Wrist Watch", "Stainless steel strap with sapphire-coated glass.", "Fossil", "Fashion", "https://images.unsplash.com/photo-1524805444758-089113d48a6d?auto=format&fit=crop&w=900&q=80", 649900, 35},
		{"Leather Belt", "Genuine leather belt with reversible buckle.", "Woodland", "Fashion", "https://images.unsplash.com/photo-1553062407-98eeb64c6a62?auto=format&fit=crop&w=900&q=80", 99900, 100},
		{"Formal Blazer", "Tailored-fit blazer for office and formal occasions.", "Van Heusen", "Fashion", "https://images.unsplash.com/photo-1594938298603-c8148c4dae35?auto=format&fit=crop&w=900&q=80", 449900, 30},
		{"Ethnic Kurta Set", "Cotton kurta set with matching bottoms.", "Fabindia", "Fashion", "https://images.unsplash.com/photo-1610030181087-540c3c9e5717?auto=format&fit=crop&w=900&q=80", 189900, 60},

		// --- Appliances ---
		{"Stand Mixer Grinder", "750W motor with 3 stainless steel jars.", "Preethi", "Appliances", "https://images.unsplash.com/photo-1570222094114-d054a817e56b?auto=format&fit=crop&w=900&q=80", 349900, 40},
		{"Front Load Washing Machine 7kg", "Energy-efficient washer with 12 wash programs.", "Bosch", "Appliances", "https://images.unsplash.com/photo-1626806787461-102c1bfaaea1?auto=format&fit=crop&w=900&q=80", 2899900, 12},
		{"Double Door Refrigerator 340L", "Frost-free refrigerator with convertible freezer.", "Whirlpool", "Appliances", "https://images.unsplash.com/photo-1571175443880-49e1d25b2bc5?auto=format&fit=crop&w=900&q=80", 3199900, 10},
		{"Steam Iron", "Ceramic soleplate steam iron with anti-drip.", "Philips", "Appliances", "https://images.unsplash.com/photo-1517677129300-07b130802f46?auto=format&fit=crop&w=900&q=80", 129900, 70},
		{"RO+UV Water Purifier", "7-stage purification for safe drinking water.", "Kent", "Appliances", "https://images.unsplash.com/photo-1523362628745-0c100150b504?auto=format&fit=crop&w=900&q=80", 1499900, 22},
		{"Microwave Oven 25L", "Convection microwave with grill and pre-set menus.", "IFB", "Appliances", "https://images.unsplash.com/photo-1585659722983-3a675dabf23d?auto=format&fit=crop&w=900&q=80", 999900, 25},
		{"Ceiling Fan Aerodynamic", "High air-delivery ceiling fan with anti-dust coating.", "Havells", "Appliances", "https://images.unsplash.com/photo-1587825140708-dfaf72ae4b04?auto=format&fit=crop&w=900&q=80", 249900, 60},
		{"Electric Kettle 1.5L", "Auto shut-off kettle with boil-dry protection.", "Prestige", "Appliances", "https://images.unsplash.com/photo-1594213586454-a1a2a4d0a99a?auto=format&fit=crop&w=900&q=80", 89900, 90},

		// --- Home ---
		{"Decorative Throw Cushion Set", "Set of 2 soft cushions with removable covers.", "Ikea", "Home", "https://images.unsplash.com/photo-1584100936595-c0654b55a2e2?auto=format&fit=crop&w=900&q=80", 129900, 80},
		{"Blackout Curtains (Pair)", "Thermal-insulated blackout curtains, 7ft.", "Home Centre", "Home", "https://images.unsplash.com/photo-1513694203232-719a280e022f?auto=format&fit=crop&w=900&q=80", 179900, 55},
		{"Framed Wall Art Set", "Set of 3 abstract canvas prints, ready to hang.", "Wallmantra", "Home", "https://images.unsplash.com/photo-1513519245088-0e12902e5a38?auto=format&fit=crop&w=900&q=80", 249900, 40},
		{"Area Rug 5x7ft", "Soft-pile area rug for living room and bedroom.", "Ikea", "Home", "https://images.unsplash.com/photo-1567016432779-094069958ea5?auto=format&fit=crop&w=900&q=80", 349900, 30},
		{"Storage Organizer Box Set", "Set of 3 stackable fabric storage bins.", "Cello", "Home", "https://images.unsplash.com/photo-1594026112284-02bb6f3352fe?auto=format&fit=crop&w=900&q=80", 99900, 100},
		{"Ceramic Planter Set", "Set of 3 minimalist ceramic planters with drainage.", "Ugaoo", "Home", "https://images.unsplash.com/photo-1485955900006-10f4d324d411?auto=format&fit=crop&w=900&q=80", 79900, 90},
		{"Cotton Bedsheet Set (King)", "300 thread-count cotton bedsheet with 2 pillow covers.", "Bombay Dyeing", "Home", "https://images.unsplash.com/photo-1522771739844-6a9f6d5f14af?auto=format&fit=crop&w=900&q=80", 199900, 65},
		{"Non-Stick Cookware Set", "5-piece induction-friendly non-stick cookware set.", "Prestige", "Home", "https://images.unsplash.com/photo-1556909212-d5b604d0c90d?auto=format&fit=crop&w=900&q=80", 349900, 45},

		// --- Beauty ---
		{"Matte Liquid Lipstick", "Long-wearing, transfer-proof matte lipstick.", "Sugar", "Beauty", "https://images.unsplash.com/photo-1512496015851-a90fb38ba796?auto=format&fit=crop&w=900&q=80", 59900, 120},
		{"Eau De Parfum 100ml", "Long-lasting fragrance with woody-floral notes.", "Fogg", "Beauty", "https://images.unsplash.com/photo-1541643600914-78b084683601?auto=format&fit=crop&w=900&q=80", 149900, 60},
		{"Hair Care Combo", "Shampoo and conditioner set for damaged hair repair.", "L'Oreal", "Beauty", "https://images.unsplash.com/photo-1522337360788-8b13dee7a37e?auto=format&fit=crop&w=900&q=80", 89900, 100},
		{"Gentle Foaming Face Wash", "Sulphate-free face wash for all skin types.", "Cetaphil", "Beauty", "https://images.unsplash.com/photo-1556228453-efd6c1ff04f6?auto=format&fit=crop&w=900&q=80", 49900, 130},
		{"SPF 50 Sunscreen Gel", "Non-greasy, broad-spectrum sunscreen gel.", "Minimalist", "Beauty", "https://images.unsplash.com/photo-1556228578-8c89e6adf883?auto=format&fit=crop&w=900&q=80", 39900, 140},
		{"Electric Hair Dryer", "1800W dryer with cool-shot and multiple heat settings.", "Philips", "Beauty", "https://images.unsplash.com/photo-1522338242992-e1a54906a8da?auto=format&fit=crop&w=900&q=80", 129900, 70},

		// --- Grocery ---
		{"Assam CTC Tea 1kg", "Strong and aromatic everyday CTC tea leaves.", "Tata Tea", "Grocery", "https://images.unsplash.com/photo-1544787219-7f47ccb76574?auto=format&fit=crop&w=900&q=80", 42900, 150},
		{"Raw Forest Honey 500g", "100% pure, unprocessed forest honey.", "Dabur", "Grocery", "https://images.unsplash.com/photo-1587049352846-4a222e784d38?auto=format&fit=crop&w=900&q=80", 32900, 120},
		{"Extra Virgin Olive Oil 1L", "Cold-pressed extra virgin olive oil.", "Figaro", "Grocery", "https://images.unsplash.com/photo-1474979266404-7eaacbcd87c5?auto=format&fit=crop&w=900&q=80", 74900, 90},
		{"Whole Spices Combo Pack", "Set of 6 essential whole spices for Indian cooking.", "Everest", "Grocery", "https://images.unsplash.com/photo-1596040033229-a9821ebd058d?auto=format&fit=crop&w=900&q=80", 29900, 160},
		{"Mixed Dry Fruits 1kg", "Premium almonds, cashews, raisins and pistachios.", "Happilo", "Grocery", "https://images.unsplash.com/photo-1508061253366-f7da158b6d46?auto=format&fit=crop&w=900&q=80", 89900, 100},
		{"Breakfast Cereal Combo", "High-fibre breakfast cereal, family pack.", "Kelloggs", "Grocery", "https://images.unsplash.com/photo-1521483451981-77e63ba05f79?auto=format&fit=crop&w=900&q=80", 34900, 140},
		{"Assorted Namkeen Snacks Pack", "Pack of 4 traditional Indian savoury snacks.", "Haldirams", "Grocery", "https://images.unsplash.com/photo-1621939514649-280e2ee25f60?auto=format&fit=crop&w=900&q=80", 24900, 180},
		{"Basmati Rice 5kg", "Aged long-grain basmati rice for everyday meals.", "India Gate", "Grocery", "https://images.unsplash.com/photo-1586201375761-83865001e31c?auto=format&fit=crop&w=900&q=80", 64900, 110},

		// --- Additional Electronics ---
		{"Smart Home Hub", "Voice-controlled hub for compatible smart home devices.", "Amazon", "Electronics", "https://images.unsplash.com/photo-1558089687-f282ffcbc126?auto=format&fit=crop&w=900&q=80", 899900, 24},
		{"USB-C Docking Station", "Multiport dock with HDMI, Ethernet and USB expansion.", "Anker", "Electronics", "https://images.unsplash.com/photo-1625842268584-8f3296236761?auto=format&fit=crop&w=900&q=80", 1199900, 30},
		{"External SSD 1TB", "Compact solid-state drive with fast USB-C transfer speeds.", "Samsung", "Electronics", "https://images.unsplash.com/photo-1597872200969-2b65d56bd16b?auto=format&fit=crop&w=900&q=80", 799900, 36},
		{"Laser Printer Wi-Fi", "Wireless monochrome printer for home and small offices.", "Brother", "Electronics", "https://images.unsplash.com/photo-1612815154858-60aa4c59eaa6?auto=format&fit=crop&w=900&q=80", 1299900, 14},
		{"4K USB Webcam", "Wide-angle webcam with autofocus and dual microphones.", "Logitech", "Electronics", "https://images.unsplash.com/photo-1587826080692-f439cd0b70da?auto=format&fit=crop&w=900&q=80", 599900, 28},
		{"USB Desktop Microphone", "Clear condenser microphone for calls, streaming and recording.", "Rode", "Electronics", "https://images.unsplash.com/photo-1590602847861-f357a9332fc8?auto=format&fit=crop&w=900&q=80", 749900, 20},
		{"Wireless Gaming Headset", "Low-latency audio with a detachable noise-cancelling mic.", "HyperX", "Electronics", "https://images.unsplash.com/photo-1599669454699-248893623440?auto=format&fit=crop&w=900&q=80", 899900, 32},
		{"Portable Projector Full HD", "Compact projector with built-in speakers and HDMI input.", "Epson", "Electronics", "https://images.unsplash.com/photo-1535016120720-40c646be5580?auto=format&fit=crop&w=900&q=80", 3499900, 9},
		{"E-reader 6 inch", "Glare-free reading display with adjustable warm light.", "Amazon", "Electronics", "https://images.unsplash.com/photo-1544716278-ca5e3f4abd8c?auto=format&fit=crop&w=900&q=80", 1199900, 26},
		{"Wi-Fi Mesh System 2-Pack", "Whole-home mesh coverage with simple app setup.", "TP-Link", "Electronics", "https://images.unsplash.com/photo-1544197150-b99a580bb7a8?auto=format&fit=crop&w=900&q=80", 999900, 18},
		{"Surge Protector 8-Outlet", "Eight protected outlets with individual safety shutters.", "Belkin", "Electronics", "https://images.unsplash.com/photo-1558618666-fcd25c85cd64?auto=format&fit=crop&w=900&q=80", 199900, 55},
		{"Drawing Tablet 10 inch", "Pressure-sensitive pen tablet for digital illustration.", "Wacom", "Electronics", "https://images.unsplash.com/photo-1583394838336-acd977736f90?auto=format&fit=crop&w=900&q=80", 649900, 17},
		{"Camera Tripod Aluminum", "Lightweight adjustable tripod with quick-release plate.", "Amazon Basics", "Electronics", "https://images.unsplash.com/photo-1516035069371-29a1b244cc32?auto=format&fit=crop&w=900&q=80", 249900, 33},
		{"Smart Video Doorbell", "Wi-Fi doorbell camera with motion alerts and two-way audio.", "Qubo", "Electronics", "https://images.unsplash.com/photo-1558002038-1055907df827?auto=format&fit=crop&w=900&q=80", 499900, 21},

		// --- Additional Mobiles ---
		{"5G Smartphone 128GB", "Bright AMOLED screen with dependable all-day performance.", "OnePlus", "Mobiles", "https://images.unsplash.com/photo-1598327105666-5b89351aff97?auto=format&fit=crop&w=900&q=80", 2499900, 35},
		{"Compact Smartphone 256GB", "Pocket-friendly phone with a sharp display and fast charging.", "Google", "Mobiles", "https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?auto=format&fit=crop&w=900&q=80", 5999900, 19},
		{"Smartphone Gimbal Stabilizer", "Three-axis stabilizer for smooth mobile video recording.", "DJI", "Mobiles", "https://images.unsplash.com/photo-1601593346740-925612772716?auto=format&fit=crop&w=900&q=80", 899900, 16},
		{"Magnetic Wireless Charger", "Fast magnetic charging stand with non-slip base.", "Belkin", "Mobiles", "https://images.unsplash.com/photo-1583863788434-e58a36330cf0?auto=format&fit=crop&w=900&q=80", 299900, 48},
		{"USB-C Earphones", "Wired in-ear headphones with clear calls and built-in controls.", "OnePlus", "Mobiles", "https://images.unsplash.com/photo-1590658268037-6bf12165a8df?auto=format&fit=crop&w=900&q=80", 99900, 85},
		{"Rugged Phone Case", "Raised-edge protective case with reinforced corners.", "Spigen", "Mobiles", "https://images.unsplash.com/photo-1601593346740-925612772716?auto=format&fit=crop&w=900&q=80", 129900, 110},
		{"Car Phone Mount", "Adjustable dashboard mount with one-handed release.", "Portronics", "Mobiles", "https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?auto=format&fit=crop&w=900&q=80", 79900, 70},
		{"Smartphone Camera Lens Kit", "Clip-on wide-angle and macro lenses for mobile photography.", "Apexel", "Mobiles", "https://images.unsplash.com/photo-1516035069371-29a1b244cc32?auto=format&fit=crop&w=900&q=80", 149900, 25},
		{"Dual USB-C Wall Charger", "Compact dual-port charger with power delivery support.", "Spigen", "Mobiles", "https://images.unsplash.com/photo-1583863788434-e58a36330cf0?auto=format&fit=crop&w=900&q=80", 179900, 95},
		{"Smartphone Ring Light", "Portable clip-on ring light with three brightness levels.", "Digitek", "Mobiles", "https://images.unsplash.com/photo-1522337360788-8b13dee7a37e?auto=format&fit=crop&w=900&q=80", 99900, 42},
		{"Foldable Phone Stand", "Pocket-sized adjustable stand for hands-free viewing.", "Portronics", "Mobiles", "https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?auto=format&fit=crop&w=900&q=80", 59900, 130},
		{"Bluetooth Selfie Stick", "Extendable selfie stick with detachable wireless remote.", "Xiaomi", "Mobiles", "https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?auto=format&fit=crop&w=900&q=80", 119900, 50},

		// --- Additional Fashion ---
		{"Everyday Cotton T-shirt", "Soft cotton crew-neck T-shirt for daily wear.", "Bewakoof", "Fashion", "https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?auto=format&fit=crop&w=900&q=80", 79900, 100},
		{"Classic Polo Shirt", "Regular-fit polo with breathable pique cotton fabric.", "US Polo", "Fashion", "https://images.unsplash.com/photo-1625910513413-5fc8d3b5c3b5?auto=format&fit=crop&w=900&q=80", 179900, 68},
		{"Straight Fit Chinos", "Stretch cotton chinos with a clean straight-leg cut.", "Jack and Jones", "Fashion", "https://images.unsplash.com/photo-1473966968600-fa801b869a1a?auto=format&fit=crop&w=900&q=80", 249900, 44},
		{"Lightweight Puffer Vest", "Packable insulated vest for cool-weather layering.", "Decathlon", "Fashion", "https://images.unsplash.com/photo-1544923246-77307dd654cb?auto=format&fit=crop&w=900&q=80", 299900, 27},
		{"Canvas Everyday Backpack", "Water-resistant backpack with padded laptop sleeve.", "Wildcraft", "Fashion", "https://images.unsplash.com/photo-1553062407-98eeb64c6a62?auto=format&fit=crop&w=900&q=80", 199900, 58},
		{"Running Cap Lightweight", "Quick-dry cap with an adjustable back strap.", "Adidas", "Fashion", "https://images.unsplash.com/photo-1588850561407-ed78c282e89b?auto=format&fit=crop&w=900&q=80", 89900, 74},
		{"Leather Wallet Slim", "Compact bifold wallet with multiple card slots.", "Hidesign", "Fashion", "https://images.unsplash.com/photo-1627123424574-724758594e93?auto=format&fit=crop&w=900&q=80", 149900, 39},
		{"Women's Walking Sandals", "Cushioned everyday sandals with adjustable straps.", "Bata", "Fashion", "https://images.unsplash.com/photo-1603487742131-4160ec999306?auto=format&fit=crop&w=900&q=80", 129900, 62},
		{"Men's Formal Oxford Shoes", "Polished lace-up shoes for work and special occasions.", "Red Tape", "Fashion", "https://images.unsplash.com/photo-1614252369475-531eba835eb1?auto=format&fit=crop&w=900&q=80", 399900, 23},
		{"Knitted Winter Scarf", "Soft, warm scarf with a classic textured knit.", "Monte Carlo", "Fashion", "https://images.unsplash.com/photo-1520903920243-00d872a2d1c9?auto=format&fit=crop&w=900&q=80", 99900, 51},
		{"Sports Ankle Socks 5-Pack", "Moisture-wicking cushioned socks for training.", "Puma", "Fashion", "https://images.unsplash.com/photo-1542291026-7eec264c27ff?auto=format&fit=crop&w=900&q=80", 69900, 140},
		{"Polarized Wayfarer Sunglasses", "Lightweight polarized lenses with full UV protection.", "Fastrack", "Fashion", "https://images.unsplash.com/photo-1511499767150-a48a237f0083?auto=format&fit=crop&w=900&q=80", 249900, 34},

		// --- Additional Appliances ---
		{"Mixer Grinder 750W", "Three-speed mixer grinder with stainless steel jars.", "Bajaj", "Appliances", "https://images.unsplash.com/photo-1570222094114-d054a817e56b?auto=format&fit=crop&w=900&q=80", 279900, 32},
		{"Induction Cooktop 2000W", "Portable induction cooktop with preset cooking modes.", "Prestige", "Appliances", "https://images.unsplash.com/photo-1556909212-d5b604d0c90d?auto=format&fit=crop&w=900&q=80", 229900, 25},
		{"Electric Rice Cooker 1.8L", "Family-size rice cooker with keep-warm function.", "Panasonic", "Appliances", "https://images.unsplash.com/photo-1585515320310-259814833e62?auto=format&fit=crop&w=900&q=80", 189900, 37},
		{"Pop-Up Toaster 2-Slice", "Variable browning toaster with removable crumb tray.", "Morphy Richards", "Appliances", "https://images.unsplash.com/photo-1520201163981-8cc95007dd2a?auto=format&fit=crop&w=900&q=80", 159900, 29},
		{"Handheld Garment Steamer", "Quick-heating fabric steamer for home and travel.", "Philips", "Appliances", "https://images.unsplash.com/photo-1517677129300-07b130802f46?auto=format&fit=crop&w=900&q=80", 249900, 21},
		{"Air Cooler 35L", "Room air cooler with adjustable fan speed and castor wheels.", "Symphony", "Appliances", "https://images.unsplash.com/photo-1585515320310-259814833e62?auto=format&fit=crop&w=900&q=80", 699900, 11},
		{"Tower Fan Remote Control", "Slim oscillating fan with timer and remote control.", "Havells", "Appliances", "https://images.unsplash.com/photo-1587825140708-dfaf72ae4b04?auto=format&fit=crop&w=900&q=80", 499900, 16},
		{"Cordless Vacuum Cleaner", "Lightweight stick vacuum with washable filter.", "Eureka Forbes", "Appliances", "https://images.unsplash.com/photo-1558317374-067fb5f30001?auto=format&fit=crop&w=900&q=80", 899900, 13},
		{"Dishwasher 12 Place Settings", "Water-efficient dishwasher with multiple wash programs.", "Bosch", "Appliances", "https://images.unsplash.com/photo-1571175443880-49e1d25b2bc5?auto=format&fit=crop&w=900&q=80", 3799900, 7},
		{"OTG Oven 28L", "Countertop oven with bake, toast and grill functions.", "Borosil", "Appliances", "https://images.unsplash.com/photo-1585659722983-3a675dabf23d?auto=format&fit=crop&w=900&q=80", 499900, 18},
		{"Digital Bathroom Scale", "Accurate digital scale with large easy-read display.", "HealthSense", "Appliances", "https://images.unsplash.com/photo-1576091160399-112ba8d25d1d?auto=format&fit=crop&w=900&q=80", 99900, 46},
		{"Electric Water Heater 15L", "Energy-saving storage water heater with safety valve.", "Racold", "Appliances", "https://images.unsplash.com/photo-1600210492486-724fe5c67fb0?auto=format&fit=crop&w=900&q=80", 749900, 10},

		// --- Additional Home ---
		{"Memory Foam Pillow Pair", "Supportive memory foam pillows with washable covers.", "Wakefit", "Home", "https://images.unsplash.com/photo-1631049307264-da0ec9d70304?auto=format&fit=crop&w=900&q=80", 149900, 52},
		{"Queen Comforter Set", "Soft all-season comforter with matching pillow shams.", "Spaces", "Home", "https://images.unsplash.com/photo-1505693416388-ac5ce068fe85?auto=format&fit=crop&w=900&q=80", 499900, 26},
		{"LED Desk Lamp Dimmable", "Adjustable desk lamp with touch controls and USB port.", "Wipro", "Home", "https://images.unsplash.com/photo-1507473885765-e6ed057f782c?auto=format&fit=crop&w=900&q=80", 129900, 41},
		{"Wall Clock Silent Sweep", "Minimal wall clock with a quiet, non-ticking movement.", "Ajanta", "Home", "https://images.unsplash.com/photo-1563861826100-9cb868fdbe1c?auto=format&fit=crop&w=900&q=80", 99900, 35},
		{"Stainless Steel Dinner Set", "Dinner set for six with plates, bowls and spoons.", "Bergner", "Home", "https://images.unsplash.com/photo-1603199506016-b9a594b593c0?auto=format&fit=crop&w=900&q=80", 799900, 19},
		{"Glass Food Storage Set", "Airtight glass containers in assorted kitchen sizes.", "Cello", "Home", "https://images.unsplash.com/photo-1608686207856-001b95cf60ca?auto=format&fit=crop&w=900&q=80", 199900, 64},
		{"Bamboo Laundry Hamper", "Foldable laundry hamper with breathable fabric liner.", "Home Centre", "Home", "https://images.unsplash.com/photo-1584622650111-993a426fbf0a?auto=format&fit=crop&w=900&q=80", 179900, 28},
		{"Bath Towel Set 4-Piece", "Absorbent cotton towels for everyday bathroom use.", "Bombay Dyeing", "Home", "https://images.unsplash.com/photo-1620626011761-996317b8d101?auto=format&fit=crop&w=900&q=80", 249900, 47},
		{"Floating Wall Shelf Set", "Set of three space-saving shelves with concealed mounts.", "Ikea", "Home", "https://images.unsplash.com/photo-1594620302200-9a762244a156?auto=format&fit=crop&w=900&q=80", 229900, 23},
		{"Indoor Money Plant", "Easy-care indoor plant delivered in a nursery pot.", "Ugaoo", "Home", "https://images.unsplash.com/photo-1485955900006-10f4d324d411?auto=format&fit=crop&w=900&q=80", 49900, 75},
		{"Door Mat Anti-Slip", "Washable coir-look entrance mat with non-slip backing.", "Status", "Home", "https://images.unsplash.com/photo-1600210492486-724fe5c67fb0?auto=format&fit=crop&w=900&q=80", 69900, 58},
		{"Reusable Water Bottle 1L", "Leakproof stainless steel bottle for home and travel.", "Milton", "Home", "https://images.unsplash.com/photo-1602143407151-7111542de6e8?auto=format&fit=crop&w=900&q=80", 89900, 90},

		// --- Additional Beauty ---
		{"Vitamin C Face Serum", "Lightweight daily serum for a brighter-looking complexion.", "Plum", "Beauty", "https://images.unsplash.com/photo-1608248543803-ba4f8c70ae0b?auto=format&fit=crop&w=900&q=80", 49900, 95},
		{"Hydrating Moisturizer 100ml", "Daily moisturizer for soft, comfortable skin.", "Cetaphil", "Beauty", "https://images.unsplash.com/photo-1556228720-195a672e8a03?auto=format&fit=crop&w=900&q=80", 39900, 105},
		{"Cleansing Micellar Water", "Gentle no-rinse cleanser for face and eye makeup.", "Garnier", "Beauty", "https://images.unsplash.com/photo-1556228453-efd6c1ff04f6?auto=format&fit=crop&w=900&q=80", 29900, 84},
		{"Nourishing Body Lotion", "Fast-absorbing body lotion for everyday hydration.", "Nivea", "Beauty", "https://images.unsplash.com/photo-1608248543803-ba4f8c70ae0b?auto=format&fit=crop&w=900&q=80", 24900, 120},
		{"Herbal Shampoo 400ml", "Everyday shampoo with a gentle plant-based formula.", "Himalaya", "Beauty", "https://images.unsplash.com/photo-1522337360788-8b13dee7a37e?auto=format&fit=crop&w=900&q=80", 19900, 115},
		{"Nourishing Hair Oil", "Lightweight hair oil for a weekly scalp-care routine.", "Parachute", "Beauty", "https://images.unsplash.com/photo-1608571423902-eed4a5ad8108?auto=format&fit=crop&w=900&q=80", 14900, 135},
		{"Kajal Eyeliner Waterproof", "Smooth kajal pencil with long-wear waterproof color.", "Maybelline", "Beauty", "https://images.unsplash.com/photo-1512496015851-a90fb38ba796?auto=format&fit=crop&w=900&q=80", 29900, 88},
		{"Nail Polish Color Trio", "Three glossy nail colors for everyday styling.", "Lakme", "Beauty", "https://images.unsplash.com/photo-1604654894610-df63bc536371?auto=format&fit=crop&w=900&q=80", 19900, 76},
		{"Makeup Brush Set 8-Piece", "Soft synthetic brushes for blending and detail work.", "Colorbar", "Beauty", "https://images.unsplash.com/photo-1522335789203-aabd1fc54bc9?auto=format&fit=crop&w=900&q=80", 39900, 42},
		{"Beard Grooming Kit", "Beard wash, oil and comb for a simple grooming routine.", "Beardo", "Beauty", "https://images.unsplash.com/photo-1621607512214-68297480165e?auto=format&fit=crop&w=900&q=80", 59900, 39},
		{"Electric Toothbrush", "Rechargeable toothbrush with two-minute timer.", "Oral-B", "Beauty", "https://images.unsplash.com/photo-1609840114035-3c981b782dfe?auto=format&fit=crop&w=900&q=80", 99900, 31},
		{"Aloe Vera Skin Gel", "Cooling multi-purpose gel for face and body.", "Patanjali", "Beauty", "https://images.unsplash.com/photo-1556228578-8c89e6adf883?auto=format&fit=crop&w=900&q=80", 12900, 145},

		// --- Additional Grocery ---
		{"Organic Rolled Oats 1kg", "Whole-grain rolled oats for breakfast and baking.", "True Elements", "Grocery", "https://images.unsplash.com/photo-1517673132405-a56a62b18caf?auto=format&fit=crop&w=900&q=80", 24900, 90},
		{"Whole Wheat Atta 5kg", "Stone-ground whole wheat flour for everyday cooking.", "Aashirvaad", "Grocery", "https://images.unsplash.com/photo-1627485937980-221c88ac04f9?auto=format&fit=crop&w=900&q=80", 29900, 105},
		{"Cold Pressed Coconut Oil 1L", "Pure coconut oil for cooking and pantry use.", "KLF", "Grocery", "https://images.unsplash.com/photo-1474979266404-7eaacbcd87c5?auto=format&fit=crop&w=900&q=80", 39900, 73},
		{"Green Tea Bags 100 Count", "Light, refreshing green tea bags for daily brewing.", "Tetley", "Grocery", "https://images.unsplash.com/photo-1544787219-7f47ccb76574?auto=format&fit=crop&w=900&q=80", 27900, 82},
		{"Peanut Butter Creamy 1kg", "Creamy peanut butter made for toast and smoothies.", "Pintola", "Grocery", "https://images.unsplash.com/photo-1505576399274-565b52d4ac71?auto=format&fit=crop&w=900&q=80", 34900, 69},
		{"Dark Chocolate Almond Bar", "Dark chocolate bar with roasted almond pieces.", "Amul", "Grocery", "https://images.unsplash.com/photo-1495474472287-4d71bcdd2085?auto=format&fit=crop&w=900&q=80", 9990, 125},
		{"Instant Noodles Family Pack", "Quick-cooking noodles in a convenient family pack.", "Maggi", "Grocery", "https://images.unsplash.com/photo-1612929633738-8fe44f7ec841?auto=format&fit=crop&w=900&q=80", 14900, 160},
		{"Organic Quinoa 500g", "Protein-rich quinoa, rinsed and ready to cook.", "Conscious Food", "Grocery", "https://images.unsplash.com/photo-1515543904379-3d757afe72e4?auto=format&fit=crop&w=900&q=80", 29900, 56},
		{"Roasted Masala Peanuts", "Crunchy roasted peanuts with a mild spice blend.", "Haldirams", "Grocery", "https://images.unsplash.com/photo-1508061253366-f7da158b6d46?auto=format&fit=crop&w=900&q=80", 9900, 130},
		{"Tomato Ketchup 1kg", "Classic tomato ketchup for meals and snacks.", "Kissan", "Grocery", "https://images.unsplash.com/photo-1472476443507-c7a5948772fc?auto=format&fit=crop&w=900&q=80", 12900, 97},
		{"Organic Jaggery Powder 1kg", "Unrefined cane jaggery powder for sweetening.", "Organic Tattva", "Grocery", "https://images.unsplash.com/photo-1586201375761-83865001e31c?auto=format&fit=crop&w=900&q=80", 16900, 64},
		{"Filter Coffee Powder 500g", "South Indian filter coffee blend with rich aroma.", "Cothas", "Grocery", "https://images.unsplash.com/photo-1495474472287-4d71bcdd2085?auto=format&fit=crop&w=900&q=80", 22900, 71},
		{"Pasta Variety Pack", "Three pasta shapes for easy weeknight meals.", "Del Monte", "Grocery", "https://images.unsplash.com/photo-1551462147-ff29053bfc14?auto=format&fit=crop&w=900&q=80", 19900, 88},
		{"Mixed Berry Jam 500g", "Fruit-forward mixed berry jam for breakfast.", "Mapro", "Grocery", "https://images.unsplash.com/photo-1563805042-7684c019e1cb?auto=format&fit=crop&w=900&q=80", 17900, 59},
	}
	for _, p := range products {
		description := demoProductDescription(p.name, p.desc, p.brand, p.category)
		if _, err := pool.Exec(ctx, `INSERT INTO products(name,description,brand,category,price_paise,stock,image_url)
			SELECT $1,$2,$3,$4,$5,$6,$7
			WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = $1 AND brand = $3)`, p.name, description, p.brand, p.category, p.price, p.stock, p.image); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `UPDATE products SET description = $4
			WHERE name = $1 AND brand = $2 AND (description = $3 OR position($5 IN description) > 0 OR position($6 IN description) > 0 OR position($7 IN description) > 0)`, p.name, p.brand, p.desc, description, "Details below distinguish stated listing facts from information that still needs supplier confirmation.", "specifications not present in the listing are identified rather than assumed.", "Product-specific details not supplied in the source listing are clearly marked for seller confirmation."); err != nil {
			return err
		}
	}
	return nil
}

type demoProductProfile struct {
	kind    string
	usage   string
	benefit string
	care    string
}

var demoProductProfiles = []struct {
	terms []string
	value demoProductProfile
}{
	{[]string{"laptop", "ultrabook"}, demoProductProfile{"laptop computer", "Use on a stable, dry surface. Check the operating-system, software, port, and charger requirements against your intended workload and existing accessories.", "A portable computer can support work, study, and media tasks; the processor, memory, storage, screen, and battery details determine suitability and are only stated where included in the listing.", "Keep vents clear, avoid liquid exposure, and follow the manufacturer's cleaning and battery-care instructions."}},
	{[]string{"smartphone", "feature phone", "outdoor phone", "phone"}, demoProductProfile{"mobile phone", "Before purchase, verify carrier and network-band support, SIM format, storage needs, and compatibility with your charger and accessories. Follow the device setup and charging instructions.", "A mobile phone combines communication and the capabilities described in the listing. Network support, storage, camera configuration, and included accessories vary by model; verify those details before ordering.", "Use a compatible charger, protect the screen from impact, and follow the supplied battery and cleaning guidance."}},
	{[]string{"earbud", "headphone", "headset", "earphone"}, demoProductProfile{"personal audio device", "Check wired or wireless connection type and device compatibility before use. Start at a low volume and adjust gradually; use noise cancellation only where it is safe to do so.", "Personal audio can support calls, focused listening, gaming, or travel depending on the listed connection and microphone features. Battery runtime and included tips or cables are not stated unless shown in the listing.", "Keep speakers and charging contacts dry; clean only as directed by the manufacturer."}},
	{[]string{"speaker"}, demoProductProfile{"audio speaker", "Pair or connect to a compatible audio source using the connection method supported by the model. Keep the device dry unless its product label confirms water resistance.", "A speaker makes compatible audio easier to share or listen to. Output power, codecs, connectivity, and water-resistance ratings are not provided unless explicitly listed.", "Keep ports clear and follow the manufacturer's charging and cleaning instructions."}},
	{[]string{"monitor", "smart led tv", "television", " tv"}, demoProductProfile{"display", "Confirm screen size, resolution, refresh rate, input ports, mounting pattern, and available space before setup. Use compatible cables and source devices.", "A display can improve viewing for work or entertainment. Picture quality and compatibility depend on the exact panel and supported inputs; use only specifications stated in this listing.", "Use a stable stand or compatible mount and clean the screen with a method approved for its surface."}},
	{[]string{"keyboard"}, demoProductProfile{"computer keyboard", "Confirm the connection type, operating-system support, layout, and switch style suit your setup. Connect it using the supplied instructions.", "A dedicated keyboard can make typing and control more comfortable. Layout, switch type, key rollover, and lighting details should be verified against the listing and your preferences.", "Keep liquids and debris away from the keys; clean according to the manufacturer's guidance."}},
	{[]string{"mouse"}, demoProductProfile{"computer mouse", "Check connection type, operating-system support, hand fit, and desk surface before use. Install any required software only from the manufacturer's official source.", "A mouse provides pointer control for compatible computers. Sensor resolution, buttons, polling rate, and battery details vary and are not implied unless stated in the listing.", "Clean the sensor and contact surfaces gently; replace or recharge batteries as directed."}},
	{[]string{"smartwatch", "wrist watch", "analog watch"}, demoProductProfile{"wrist-worn watch", "Check wrist fit, phone compatibility for connected features, and the stated water-resistance rating before wearing. Do not assume a watch is suitable for swimming unless its rating confirms it.", "A watch provides timekeeping and, where listed, health or notification functions. Sensor accuracy, supported apps, and water resistance depend on the specific model.", "Keep the strap and case clean and dry as directed; service the battery or movement through an appropriate provider."}},
	{[]string{"camera", "webcam"}, demoProductProfile{"camera", "Check the supported resolution, lens or field of view, storage media, battery, and connection requirements. Use a compatible card or computer if required.", "A camera can capture photos or video for the uses described in the listing. Actual recording time and image quality depend on settings, lighting, storage, and compatible accessories.", "Protect the lens from dust and moisture, and use a suitable case when carrying the device."}},
	{[]string{"power bank"}, demoProductProfile{"portable battery", "Check the output ports, charging protocol, and capacity compatibility with your devices. Recharge the power bank using a supported adapter and cable.", "A portable battery can extend device use away from a wall outlet. Real-world charging depends on output, cable, device capacity, and conversion losses; do not interpret capacity as guaranteed full charges.", "Keep the battery away from heat, moisture, and physical damage; discontinue use if it swells or is damaged."}},
	{[]string{"charger", "charging"}, demoProductProfile{"power adapter or charging accessory", "Verify connector, supported charging standard, and required wattage against the device before connecting. Use a suitable cable and wall outlet.", "A compatible charger can provide convenient power delivery. Charging speed depends on the adapter, cable, and receiving device, and is not guaranteed by wattage alone.", "Keep the adapter dry and unplug it by the plug, not the cable."}},
	{[]string{"router", "wi-fi", "wifi", "mesh system"}, demoProductProfile{"networking device", "Check broadband compatibility, supported Wi-Fi standard, modem requirements, and coverage needs. Place the device in an open, central location and follow the secure setup instructions.", "Networking equipment can extend or organize home connectivity. Actual coverage and throughput depend on walls, interference, internet service, and connected devices.", "Keep firmware updated through the manufacturer's supported method and use a unique administrator password."}},
	{[]string{"console"}, demoProductProfile{"gaming console", "Check display, account, network, controller, and game compatibility before setup. A display and internet connection may be required for some features.", "A console provides a dedicated platform for compatible games and media. Storage, resolution, subscriptions, and included games depend on the exact bundle and are not included unless listed.", "Allow ventilation around the console and keep it on a stable surface."}},
	{[]string{"tablet", "e-reader", "reader"}, demoProductProfile{"portable reading or tablet device", "Check screen size, storage, connectivity, supported apps or file formats, and charger requirements against your intended use.", "A portable screen can support reading, study, browsing, or media use depending on the model. App availability, network options, and accessory compatibility vary.", "Use a compatible case and charger; clean the screen with a suitable soft cloth."}},
	{[]string{"projector"}, demoProductProfile{"projector", "Confirm throw distance, screen or wall space, input compatibility, room lighting, and audio needs before installation. Follow the specified ventilation clearances.", "Projection can create a large shared viewing area. Brightness, image size, focus, and perceived contrast depend on the room and exact model specifications.", "Keep air vents unobstructed and clean the lens only with an appropriate lens cloth."}},
	{[]string{"microphone"}, demoProductProfile{"microphone", "Check connector, device compatibility, mounting, and any required audio interface or software. Position it as directed and test input levels before recording.", "A dedicated microphone can improve voice capture for calls or recording. Pickup pattern, frequency response, and included accessories are not stated unless specified in the listing.", "Protect the capsule from moisture and dust and store it safely between uses."}},
	{[]string{"printer"}, demoProductProfile{"printer", "Verify computer and operating-system compatibility, connection method, paper size, and consumable type. Install supplies and drivers using the manufacturer's instructions.", "A printer supports document output for home or office tasks. Print speed, page yield, duplex support, and included toner or ink depend on the exact model and package.", "Use supported paper and consumables and follow the manufacturer's cleaning and maintenance schedule."}},
	{[]string{"ssd", "storage drive"}, demoProductProfile{"data-storage device", "Confirm interface, connector, device support, and required formatting before use. Keep a separate backup of important files; a new drive is not a backup by itself.", "Additional storage can help keep files portable or expand a compatible computer. Actual transfer speed depends on the drive, port, cable, and workload.", "Avoid impact, liquid exposure, and disconnecting during active transfers."}},
	{[]string{"dock", "hub"}, demoProductProfile{"connectivity accessory", "Verify host-device compatibility, connector type, supported display resolution, power delivery, and port requirements before connecting peripherals.", "A compatible dock or hub can consolidate connections and reduce cable changes. Port count alone does not guarantee simultaneous power or full-speed operation.", "Disconnect by the connector and avoid exceeding stated power or port limits."}},
	{[]string{"tripod", "gimbal", "selfie stick"}, demoProductProfile{"camera support accessory", "Check device mounting dimensions and supported weight before use. Lock joints and clamps securely, then test stability before mounting valuable equipment.", "A support accessory can help frame or steady photos and video. Compatibility and stability depend on the device size, mount, and operating conditions.", "Store folded and dry; inspect locks and clamps before each use."}},
	{[]string{"doorbell"}, demoProductProfile{"doorbell camera", "Check Wi-Fi coverage, power or battery requirements, mounting surface, and mobile-app support before installation. Review privacy settings and local recording rules.", "A connected doorbell may help monitor an entry and provide visitor notifications. Recording, cloud storage, and alert features depend on the model, network, and any required subscription.", "Mount securely and clean the camera lens gently; do not expose the unit to conditions beyond its stated rating."}},
	{[]string{"sneaker", "shoes", "sandals", "footwear"}, demoProductProfile{"footwear", "Compare the seller's size chart with your foot measurements and intended sock thickness. Break in new footwear gradually and use it on suitable surfaces.", "Footwear can provide comfort and support for its described activity when the fit and sole suit the wearer and surface. Fit varies by brand and style.", "Follow the care instructions for the upper and sole materials; allow footwear to dry naturally if damp."}},
	{[]string{"shirt", "t-shirt", "tshirt", "jeans", "chinos", "jacket", "dress", "blazer", "kurta", "scarf", "vest"}, demoProductProfile{"apparel", "Use the seller's size chart and garment measurements to select a fit. Check closure, length, and layering needs; follow the sewn-in care label after purchase.", "Apparel can add a practical or style layer for the use described in the listing. Fit and feel depend on cut, construction, and fiber blend, which should be confirmed from the garment label.", "Fiber percentages, colorfastness, and wash instructions are not supplied here; follow the garment's care label."}},
	{[]string{"backpack", "handbag", "wallet", "belt"}, demoProductProfile{"fashion accessory", "Check dimensions, capacity, closure type, strap adjustment, and the intended items before ordering. Avoid loading beyond the seller's stated limit.", "An accessory can organize or carry everyday essentials. Capacity, pocket layout, and durability depend on construction details not fully supplied in this demo listing.", "Clean according to the material label and store away from prolonged moisture or direct heat."}},
	{[]string{"sunglasses", "eyewear"}, demoProductProfile{"eyewear", "Check frame fit and lens information before use. Do not rely on sunglasses as protective eyewear unless the product is specifically rated for that purpose.", "Eyewear can reduce glare or provide UV protection only when the lens rating confirms it. Verify the protection standard and fit before purchase.", "Store in a protective case and clean with a lens-safe cloth."}},
	{[]string{"mixer", "grinder"}, demoProductProfile{"food-preparation appliance", "Check jar or attachment fit, power requirements, and the manual's recommended load. Secure lids before operating and unplug before cleaning blades or attachments.", "A mixer or grinder can help prepare ingredients for cooking. Results depend on batch size, ingredients, and the specific attachments supplied.", "Wash removable parts according to the manual; keep the motor base dry."}},
	{[]string{"washing machine", "dishwasher"}, demoProductProfile{"cleaning appliance", "Confirm installation space, water and drain connections, electrical requirements, and load capacity before purchase. Level the appliance and follow detergent guidance in the manual.", "A cleaning appliance can reduce manual household work. Cycle duration, resource use, and load results depend on the selected program and installation.", "Clean filters and seals regularly and follow the manufacturer's maintenance schedule."}},
	{[]string{"refrigerator", "fridge"}, demoProductProfile{"refrigeration appliance", "Measure the installation space, door clearance, ventilation gap, and route to the final location. Allow the unit to settle and follow startup and temperature-setting instructions.", "Refrigeration helps store food at controlled temperatures. Usable capacity and energy use depend on model configuration, placement, and usage.", "Keep ventilation clear and clean interior surfaces according to the manual."}},
	{[]string{"air fryer", "rice cooker", "kettle", "toaster", "cooktop", "microwave", "oven"}, demoProductProfile{"kitchen appliance", "Check capacity, cookware or container compatibility, power requirements, and clearance around the appliance. Follow the manual's fill limits and food-safety guidance.", "A kitchen appliance can simplify the preparation task named in the listing. Cooking time and results vary with quantity, ingredients, settings, and cookware.", "Unplug before cleaning and allow hot surfaces to cool. Clean removable parts only as directed."}},
	{[]string{"fan", "air cooler"}, demoProductProfile{"air-circulation appliance", "Place on a stable surface with clear airflow and a suitable power outlet. Keep fingers and objects away from moving parts and follow the manual for assembly.", "Air circulation can improve perceived comfort in a suitable room. It does not replace cooling or ventilation performance beyond the device's stated capability.", "Disconnect power before cleaning and keep motor and electrical parts dry."}},
	{[]string{"vacuum"}, demoProductProfile{"floor-care appliance", "Check floor-surface compatibility and dust-bin or bag requirements. Clear large debris first and keep the intake free of tangled material.", "A vacuum can help collect dust and debris from compatible surfaces. Runtime and pickup depend on battery, filter condition, floor type, and cleaning mode.", "Empty the bin and clean or replace filters as instructed; unplug before clearing a blockage."}},
	{[]string{"iron", "steamer"}, demoProductProfile{"garment-care appliance", "Check that the fabric is suitable for the selected heat or steam setting. Test an inconspicuous area and keep hot surfaces away from skin and children.", "An iron or steamer can help reduce creases on compatible fabrics. Results depend on fabric, temperature, steam output, and technique.", "Empty water as directed and allow the appliance to cool fully before storage."}},
	{[]string{"water purifier"}, demoProductProfile{"water-treatment appliance", "Confirm water source, installation requirements, filter availability, and service access before purchase. Follow the manufacturer's commissioning and replacement schedule.", "A purifier is designed to treat water using its specified process. Do not assume contaminant removal or drinking-water suitability without the model's certification and source-water guidance.", "Replace filters on schedule and sanitize only as directed by the manufacturer."}},
	{[]string{"lamp", "light"}, demoProductProfile{"lighting product", "Place or mount securely, check bulb or power compatibility, and keep away from damp areas unless the product is rated for them.", "Lighting can add task or ambient illumination to a suitable space. Brightness, color temperature, and dimming support are only known when specified.", "Switch off before cleaning and use only compatible bulbs or power supplies."}},
	{[]string{"bedsheet", "comforter", "pillow", "curtain", "towel", "cushion"}, demoProductProfile{"home textile", "Measure the bed, window, or furniture before ordering and check the seller's fit dimensions. Follow the supplied wash and drying instructions.", "A textile can add comfort, coverage, or privacy in the intended room. Feel, warmth, and fit depend on fiber composition and dimensions, which are not fully supplied here.", "Exact fiber blend and care instructions are not provided; follow the product label and wash separately when colorfastness is unknown."}},
	{[]string{"shelf", "organizer", "hamper", "storage"}, demoProductProfile{"home organization item", "Measure the available area and confirm mounting, assembly, and load requirements before use. Secure wall-mounted items into suitable supports.", "Organizing products can make household items easier to group and access. Usable capacity and safe load depend on dimensions and installation details not stated here.", "Keep within the seller's load guidance and clean using a method suitable for the item's material."}},
	{[]string{"plant", "planter"}, demoProductProfile{"plant or gardening item", "Check whether the listing is for a live plant, planter, or both. Match drainage, light, and watering to the plant species and local conditions.", "A plant or planter can support indoor greenery when its light and care requirements fit the space. Species, pot dimensions, and included accessories should be confirmed with the seller.", "Protect live plants from temperature extremes and check soil moisture before watering."}},
	{[]string{"serum", "moisturizer", "sunscreen", "face wash", "cleanser", "skin gel", "lotion", "lipstick", "kajal", "nail polish", "shampoo", "hair oil", "hair care", "beard", "toothbrush", "makeup"}, demoProductProfile{"personal-care product", "Read the complete package label, directions, ingredients, warnings, and expiry date before use. For topical products, patch-test first and stop if irritation occurs; use sun protection as directed on sunscreen labels.", "The product is intended for the personal-care purpose named in the listing. Suitability depends on ingredients, skin or hair type, and directions; no clinical result is guaranteed.", "Full ingredients, allergens, and storage conditions are not included here. Follow the packaging and keep away from children unless the label says otherwise."}},
	{[]string{"tea", "coffee", "beans", "rice", "atta", "oats", "honey", "oil", "spices", "dry fruits", "cereal", "snacks", "namkeen", "noodles", "quinoa", "pasta", "jam", "peanuts", "chocolate", "ketchup", "jaggery"}, demoProductProfile{"food or pantry item", "Check the sealed package, full ingredient and allergen declaration, nutrition panel, and best-before date before consumption. Follow the package directions for preparation and storage.", "This pantry product can be used for the food or preparation purpose named in the listing. Dietary suitability and nutrition cannot be confirmed without the full package label.", "The complete ingredient list, allergen cross-contact information, and nutrition values are not supplied here; use the package label as the source of truth."}},
}

var demoListedSizePattern = regexp.MustCompile(`(?i)\b(?:\d+(?:\.\d+)?\s?(?:inches?|in|kg|g|l|ml|mah|w|mp|tb|gb|mm|ft|place settings|pieces?|count)|\d+\s?-\s?(?:piece|pack|count)s?)\b`)

func demoProductProfileFor(name, category string) demoProductProfile {
	profile := demoProductProfile{
		kind:    strings.ToLower(category) + " product",
		usage:   "Check fit, compatibility, installation, and included contents against the item you need. Follow the seller or manufacturer instructions supplied with the product.",
		benefit: "The listing's stated purpose can help you decide whether this item fits your needs. Verify compatibility and any requirements before ordering.",
		care:    "Care and storage directions were not included in this demo listing. Follow the instructions supplied with the product.",
	}

	switch category {
	case "Electronics", "Mobiles":
		profile.usage = "Check connection, power, device compatibility, and included accessories before setup. Use and charge according to the manufacturer's instructions."
		profile.benefit = "The listed function can support compatible work, communication, or entertainment uses. Confirm model compatibility and included accessories before ordering."
		profile.care = "Keep ports and vents clear, protect from liquid and impact, and follow the manufacturer's cleaning and battery-care instructions."
	case "Fashion":
		profile.usage = "Compare the seller's size chart and fit measurements before ordering. Follow the sewn-in care label after purchase."
		profile.benefit = "The described style and intended use can help you compare this item with your wardrobe. Fit varies by cut and brand."
		profile.care = "Fiber composition and care-label instructions are not supplied; check the garment label before washing."
	case "Appliances":
		profile.usage = "Confirm capacity, available space, power, ventilation, and installation requirements before setup. Operate according to the manufacturer's manual."
		profile.benefit = "The listed function can help with the household task described. Check measurements, capacity, utilities, and installation needs first."
		profile.care = "Disconnect power before cleaning and follow the manufacturer's cleaning and maintenance schedule."
	case "Home":
		profile.usage = "Measure the intended space and check assembly or mounting requirements before ordering. Follow supplied setup and care instructions."
		profile.benefit = "The listed item is intended for the room or household use described. Confirm dimensions, finish, and installation needs against your space."
		profile.care = "Material, finish, and care requirements are not fully supplied; follow the label or seller instructions."
	case "Beauty":
		profile.usage = "Read the product label, directions, ingredients, warnings, and expiry information before use. Patch-test topical products first and stop if irritation occurs."
		profile.benefit = "The item is intended for the personal-care use named in its listing. Suitability depends on individual needs and the product's complete label."
		profile.care = "Ingredients, allergens, and storage conditions are not fully supplied; use the package label as the source of truth."
	case "Grocery":
		profile.usage = "Check the package seal, full ingredient and allergen declarations, nutrition information, and best-before date. Follow package preparation and storage directions."
		profile.benefit = "The item is intended for the pantry or food use named in the listing. Dietary suitability cannot be confirmed without the full package label."
		profile.care = "Store according to the package directions and check the seal and best-before date on delivery."
	}

	text := strings.ToLower(name)
	for _, candidate := range demoProductProfiles {
		for _, term := range candidate.terms {
			if strings.Contains(text, term) {
				return candidate.value
			}
		}
	}
	return profile
}

func demoProductDescription(name, summary, brand, category string) string {
	profile := demoProductProfileFor(name, category)
	feature := strings.TrimSpace(strings.TrimRight(summary, ". "))
	if feature == "" {
		feature = "No feature summary was supplied for this demo listing."
	}

	sizeMatches := demoListedSizePattern.FindAllString(name+" "+summary, -1)
	sizes := make([]string, 0, len(sizeMatches))
	seenSizes := make(map[string]bool)
	for _, value := range sizeMatches {
		value = strings.TrimSpace(value)
		if !seenSizes[strings.ToLower(value)] {
			seenSizes[strings.ToLower(value)] = true
			sizes = append(sizes, value)
		}
	}
	sizeDetails := "No size, capacity, or weight measurement is stated in the current listing. Confirm exact dimensions and shipping weight with the seller if they affect fit or installation."
	if len(sizes) > 0 {
		sizeDetails = fmt.Sprintf("Measurements or quantities explicitly stated in the listing: %s. These are product specifications, not shipping dimensions; confirm exact dimensions and shipping weight with the seller if needed.", strings.Join(sizes, ", "))
	}

	mentionedMaterials := make([]string, 0)
	materialTerms := []string{"cotton", "denim", "leather", "faux-leather", "aluminium", "aluminum", "stainless steel", "steel", "glass", "ceramic", "bamboo", "wood", "fabric", "mesh", "rubber", "silicone", "polyester", "Arabica", "almonds", "cashews", "raisins", "pistachios", "honey", "coconut", "olive oil", "quinoa", "peanuts", "spices", "rice", "wheat"}
	listingText := strings.ToLower(name + " " + summary)
	for _, material := range materialTerms {
		if strings.Contains(listingText, strings.ToLower(material)) {
			mentionedMaterials = append(mentionedMaterials, material)
		}
	}
	materialDetails := "No material composition or complete ingredient list was supplied in this demo listing. Check the product label or confirm the exact composition with the seller before purchase."
	if len(mentionedMaterials) > 0 {
		materialDetails = fmt.Sprintf("The listing mentions: %s. This is not a complete material or ingredient declaration; confirm composition, percentages, allergens, and package contents with the seller or product label.", strings.Join(mentionedMaterials, ", "))
	}

	warranty := "No warranty duration, coverage, or service provider is stated in this demo listing. Confirm written warranty terms with the seller before purchase."
	if category == "Grocery" {
		warranty = "Warranty coverage is generally not applicable to grocery products. Check package seal, expiry or best-before date, and seller return terms on delivery."
	}

	return strings.Join([]string{
		fmt.Sprintf("Overview\n%s by %s is listed in the %s collection. The details below summarize its stated use, available specifications, and seller confirmations needed before purchase.", name, brand, category),
		fmt.Sprintf("Key features\n- %s\n- Product type: %s", feature, profile.kind),
		fmt.Sprintf("Specifications\n- Product: %s\n- Brand: %s\n- Category: %s\n- Additional model, variant, compatibility, and package-content details were not supplied unless stated above.", name, brand, category),
		fmt.Sprintf("How to use\n%s", profile.usage),
		fmt.Sprintf("Benefits\n%s", profile.benefit),
		fmt.Sprintf("Materials / ingredients\n%s", materialDetails),
		fmt.Sprintf("Size / weight\n%s", sizeDetails),
		fmt.Sprintf("Care / storage\n%s", profile.care),
		fmt.Sprintf("Warranty\n%s", warranty),
	}, "\n\n")
}
