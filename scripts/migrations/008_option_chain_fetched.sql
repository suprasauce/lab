-- One-time migration. Apply 001_option_expiry.sql first.
BEGIN;
ALTER TABLE lab.option_expiry
    ADD COLUMN option_chain_fetched BOOLEAN NOT NULL DEFAULT FALSE;
COMMIT;
