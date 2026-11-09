-- One-time migration. Apply 004_strategies.sql first. Run with psql ON_ERROR_STOP=1.
BEGIN;

ALTER TABLE lab.strategies
ADD COLUMN description TEXT NOT NULL DEFAULT '';

COMMIT;
