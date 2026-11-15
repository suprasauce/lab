-- One-time migration. Stop old contract writers before applying.
BEGIN;

ALTER TABLE lab.option_contracts ADD COLUMN tradingsymbol TEXT;

-- Backfill Lab symbols using exact integer paise conversion.
UPDATE lab.option_contracts AS c
SET tradingsymbol = UPPER(u.symbol) || '-' || TO_CHAR(e.expiry_date, 'YYYYMMDD') || '-' ||
    (c.strike_price / 100)::TEXT ||
    CASE WHEN c.strike_price % 100 = 0 THEN ''
         ELSE '.' || RTRIM(LPAD((c.strike_price % 100)::TEXT, 2, '0'), '0') END ||
    CASE c.option_type WHEN 'CALL' THEN '-CE' WHEN 'PUT' THEN '-PE' END
FROM lab.underlyings AS u, lab.option_expiry AS e
WHERE u.id = c.underlying_id
  AND e.id = c.expiry_id
  AND e.underlying_id = c.underlying_id;

ALTER TABLE lab.option_contracts
    ALTER COLUMN tradingsymbol SET NOT NULL,
    ADD CONSTRAINT unique_option_contract_tradingsymbol UNIQUE (tradingsymbol);

COMMIT;
