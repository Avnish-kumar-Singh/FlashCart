CREATE TABLE IF NOT EXISTS festival_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_code TEXT NOT NULL DEFAULT '',
    country_code TEXT NOT NULL,
    holiday_date DATE NOT NULL,
    name TEXT NOT NULL,
    local_name TEXT NOT NULL DEFAULT '',
    counties JSONB NOT NULL DEFAULT '[]'::jsonb,
    global_holiday BOOLEAN NOT NULL DEFAULT false,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'SCHEDULED'
        CHECK (status IN ('SCHEDULED','ACTIVATING','ACTIVE','ENDING','ENDED','FAILED')),
    discount_percent INTEGER NOT NULL DEFAULT 10 CHECK (discount_percent BETWEEN 1 AND 50),
    worker_lease_until TIMESTAMPTZ,
    reminder_queued_at TIMESTAMPTZ,
    start_notice_queued_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(country_code, holiday_date, name),
    CHECK (ends_at > starts_at)
);
CREATE INDEX IF NOT EXISTS idx_festival_events_schedule ON festival_events(status, starts_at, ends_at);

CREATE TABLE IF NOT EXISTS festival_sales (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    festival_event_id UUID NOT NULL REFERENCES festival_events(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id),
    sale_id UUID NOT NULL UNIQUE,
    stock_allocated INTEGER NOT NULL CHECK (stock_allocated > 0),
    sale_price_paise BIGINT NOT NULL CHECK (sale_price_paise >= 0),
    discount_percent INTEGER NOT NULL CHECK (discount_percent BETWEEN 1 AND 50),
    remaining_to_restore INTEGER NOT NULL DEFAULT 0 CHECK (remaining_to_restore >= 0),
    status TEXT NOT NULL DEFAULT 'ALLOCATED'
        CHECK (status IN ('ALLOCATED','ACTIVE','ENDING','ENDED','FAILED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(festival_event_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_festival_sales_event_status ON festival_sales(festival_event_id, status);

CREATE TABLE IF NOT EXISTS festival_activity_log (
    id BIGSERIAL PRIMARY KEY,
    festival_event_id UUID NOT NULL REFERENCES festival_events(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
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