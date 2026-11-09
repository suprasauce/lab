-- One-time migration. Apply 002_users.sql first. Run with psql ON_ERROR_STOP=1.
BEGIN;

CREATE TABLE lab.strategies (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES lab.users(id),
    name TEXT NOT NULL,
    script TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
