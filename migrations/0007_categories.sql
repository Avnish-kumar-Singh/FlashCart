CREATE TABLE IF NOT EXISTS categories (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    slug              TEXT NOT NULL UNIQUE,
    parent_id         UUID REFERENCES categories(id) ON DELETE RESTRICT,
    description       TEXT NOT NULL DEFAULT '',
    banner_image_url  TEXT NOT NULL DEFAULT '',
    seo_title         TEXT NOT NULL DEFAULT '',
    seo_description   TEXT NOT NULL DEFAULT '',
    sort_order        INTEGER NOT NULL DEFAULT 0,
    active            BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE INDEX IF NOT EXISTS idx_categories_parent_sort
    ON categories(parent_id, sort_order, name);

INSERT INTO categories (name, slug)
SELECT DISTINCT trim(category), trim(both '-' from regexp_replace(lower(trim(category)), '[^a-z0-9]+', '-', 'g'))
FROM products
WHERE trim(category) <> ''
ON CONFLICT (slug) DO NOTHING;