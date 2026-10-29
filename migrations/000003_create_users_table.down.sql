-- 1. Drop index on transactions(user_id)
DROP INDEX IF EXISTS idx_transactions_user_id;

-- 2. Remove the foreign key column from transactions
ALTER TABLE transactions
    DROP COLUMN IF EXISTS user_id;

-- 3. Drop the users table
DROP TABLE IF EXISTS users;