ALTER TABLE transactions
    DROP COLUMN IF EXISTS service_type,
    DROP COLUMN IF EXISTS plan_code;

DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS wallets;
