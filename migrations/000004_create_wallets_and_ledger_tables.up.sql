-- 1. Create Wallets Table
CREATE TABLE IF NOT EXISTS wallets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    balance_kobo BIGINT NOT NULL DEFAULT 0 CHECK (balance_kobo >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index wallets by user_id
CREATE INDEX IF NOT EXISTS idx_wallets_user_id ON wallets(user_id);

-- 2. Create Immutable Ledger Entries Table
CREATE TABLE IF NOT EXISTS ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
    amount_kobo BIGINT NOT NULL CHECK (amount_kobo <> 0),
    balance_after_kobo BIGINT NOT NULL CHECK (balance_after_kobo >= 0),
    entry_type VARCHAR(50) NOT NULL,
    reference VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index ledger entries by wallet_id and entry_type
CREATE INDEX IF NOT EXISTS idx_ledger_wallet_id ON ledger_entries(wallet_id);
CREATE INDEX IF NOT EXISTS idx_ledger_reference ON ledger_entries(reference);

-- 3. Extend Transactions Table for Multi-Service Support
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS service_type VARCHAR(20) NOT NULL DEFAULT 'AIRTIME',
    ADD COLUMN IF NOT EXISTS plan_code VARCHAR(50) NULL;

-- Automatically initialize a wallet for all existing users
INSERT INTO wallets (user_id, balance_kobo)
SELECT id, 0 FROM users
ON CONFLICT (user_id) DO NOTHING;
