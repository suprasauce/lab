BEGIN;

-- Existing contracts were ingested through the full 45-DTE pipeline.
ALTER TABLE lab.option_contracts ADD COLUMN synced BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE lab.option_contracts ALTER COLUMN synced SET DEFAULT FALSE;

COMMIT;
