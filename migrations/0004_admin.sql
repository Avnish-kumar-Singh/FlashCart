-- Phase 6: role-based admin access.
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'USER';
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('USER','ADMIN'));
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
