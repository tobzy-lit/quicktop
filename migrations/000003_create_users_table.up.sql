
-- 1. Create the users table
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Link existing transactions table to users
-- user_id is nullable so existing anonymous records remain valid
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

-- 3. Index user_id on transactions for efficient user history queries
CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions(user_id);