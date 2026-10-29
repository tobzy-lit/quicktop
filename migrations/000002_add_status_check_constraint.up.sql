ALTER TABLE transactions ADD CONSTRAINT chk_transactions_status CHECK (status IN ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED'));
