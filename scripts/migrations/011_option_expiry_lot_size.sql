-- One-time migration. Existing NIFTY MONTHLY expiry rows only.
-- NSE FAOP/56233, FAOP/61415, FAOP/64625 and FAOP/70616.
-- Agreed simplification: May 2024 uses 25 for its entire simulation,
-- including dates before the April 26, 2024 intra-contract lot-size change.
BEGIN;

ALTER TABLE lab.option_expiry ADD COLUMN lot_size INTEGER;

UPDATE lab.option_expiry AS e
SET lot_size = CASE
    WHEN e.expiry_date < DATE '2024-05-01' THEN 50
    WHEN e.expiry_date < DATE '2025-02-01' THEN 25
    WHEN e.expiry_date < DATE '2026-01-01' THEN 75
    ELSE 65
END
FROM lab.underlyings AS u
WHERE u.id = e.underlying_id
  AND u.symbol = 'NIFTY'
  AND e.expiry_type = 'MONTHLY'
  AND e.expiry_date BETWEEN DATE '2023-01-01' AND DATE '2026-10-31';

-- Any unclassified expiry causes a rollback instead of guessing its lot size.
ALTER TABLE lab.option_expiry
    ALTER COLUMN lot_size SET NOT NULL,
    ADD CONSTRAINT option_expiry_lot_size_positive CHECK (lot_size > 0);

COMMIT;
