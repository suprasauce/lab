-- One-time migration. Apply 002_users.sql first. Run with psql ON_ERROR_STOP=1.
BEGIN;

CREATE TYPE lab.backtest_status AS ENUM (
    'PENDING',
    'COMPLETED',
    'FAILED'
);

CREATE TABLE lab.backtests (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES lab.users(id),
    status lab.backtest_status NOT NULL DEFAULT 'PENDING',
    script TEXT NOT NULL,
    results JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
