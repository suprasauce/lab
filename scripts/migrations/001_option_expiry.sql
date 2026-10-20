-- One-time migration of the original lab schema. Run with psql ON_ERROR_STOP=1.
-- Populate the classification input below before migrating existing contracts.
BEGIN;

-- Prevent writes from introducing unclassified contracts during the backfill.
LOCK TABLE lab.option_contracts IN ACCESS EXCLUSIVE MODE;

CREATE TYPE lab.expiry_type AS ENUM ('MONTHLY', 'WEEKLY');

CREATE TABLE lab.option_expiry (
    id BIGSERIAL PRIMARY KEY,
    underlying_id BIGINT NOT NULL REFERENCES lab.underlyings(id),
    expiry_date DATE NOT NULL,
    expiry_type lab.expiry_type NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (underlying_id, expiry_date)
);

CREATE TEMP TABLE option_expiry_classification (
    underlying_id BIGINT NOT NULL,
    expiry_date DATE NOT NULL,
    expiry_type lab.expiry_type NOT NULL,
    PRIMARY KEY (underlying_id, expiry_date)
) ON COMMIT DROP;

-- REQUIRED INPUT for populated databases: add explicitly verified classifications.
-- INSERT INTO pg_temp.option_expiry_classification
--     (underlying_id, expiry_date, expiry_type)
-- VALUES
--     (<underlying_id>, '<verified_expiry_date>', '<MONTHLY_or_WEEKLY>');
-- No default, weekday rule, or historical classification is inferred.

DO $$
DECLARE
    missing TEXT;
BEGIN
    SELECT string_agg(format('(%s, %s)', c.underlying_id, c.expiry), ', '
                      ORDER BY c.underlying_id, c.expiry)
    INTO missing
    FROM (SELECT DISTINCT underlying_id, expiry FROM lab.option_contracts) AS c
    LEFT JOIN pg_temp.option_expiry_classification AS input
        ON input.underlying_id = c.underlying_id AND input.expiry_date = c.expiry
    WHERE input.underlying_id IS NULL;

    IF missing IS NOT NULL THEN
        RAISE EXCEPTION 'Explicit expiry_type classification required for: %', missing
            USING HINT = 'Populate option_expiry_classification in this script and rerun the migration.';
    END IF;
END;
$$;

INSERT INTO lab.option_expiry (underlying_id, expiry_date, expiry_type)
SELECT c.underlying_id, c.expiry, input.expiry_type
FROM (SELECT DISTINCT underlying_id, expiry FROM lab.option_contracts) AS c
JOIN pg_temp.option_expiry_classification AS input
    ON input.underlying_id = c.underlying_id AND input.expiry_date = c.expiry;

ALTER TABLE lab.option_contracts ADD COLUMN expiry_id BIGINT;

UPDATE lab.option_contracts AS c
SET expiry_id = e.id
FROM lab.option_expiry AS e
WHERE e.underlying_id = c.underlying_id AND e.expiry_date = c.expiry;

ALTER TABLE lab.option_contracts
    ALTER COLUMN expiry_id SET NOT NULL,
    ADD CONSTRAINT option_contracts_expiry_id_fkey
        FOREIGN KEY (expiry_id) REFERENCES lab.option_expiry(id);

-- PostgreSQL removes table constraints and indexes depending on the old column,
-- including unique_option_contract. RESTRICT is intentional: external dependent
-- views/objects must be handled explicitly rather than silently cascaded away.
ALTER TABLE lab.option_contracts DROP COLUMN expiry;
ALTER TABLE lab.option_contracts
    ADD CONSTRAINT unique_option_contract UNIQUE (expiry_id, strike_price, option_type);

-- Contract primary keys and option_candles.option_contract_id remain unchanged.
COMMIT;
