-- One-time migration. Stop old application writers before applying.
-- Removes only the obsolete flag; contracts and candles are preserved.
BEGIN;
ALTER TABLE lab.option_contracts DROP COLUMN synced;
COMMIT;
