CREATE TABLE IF NOT EXISTS transactions (
    id BIGSERIAL PRIMARY KEY,
    reference VARCHAR(64) NOT NULL UNIQUE,
    network VARCHAR(16) NOT NULL,
    phone_number VARCHAR(16) NOT NULL,
    amount_kobo BIGINT NOT NULL CHECK (amount_kobo > 0),
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast lookup by reference during verification/callbacks
CREATE INDEX IF NOT EXISTS idx_transactions_reference ON transactions(reference);

-- Index for querying transaction history by phone number
CREATE INDEX IF NOT EXISTS idx_transactions_phone ON transactions(phone_number);