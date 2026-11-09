-- One-time migration. Apply 002_users.sql first. Run with psql ON_ERROR_STOP=1.
BEGIN;

-- Prevent writes between the conflict check and index creation.
LOCK TABLE lab.users IN SHARE MODE;

DO $$
DECLARE
    conflicts TEXT;
BEGIN
    SELECT string_agg(format('%L: %s', username, members), E'\n' ORDER BY username)
    INTO conflicts
    FROM (
        SELECT LOWER(name) AS username,
               string_agg(format('id=%s name=%L', id, name), ', ' ORDER BY id) AS members
        FROM lab.users
        GROUP BY LOWER(name)
        HAVING COUNT(*) > 1
    ) AS duplicates;

    IF conflicts IS NOT NULL THEN
        RAISE EXCEPTION USING
            MESSAGE = 'Cannot create users_name_unique_idx: case-insensitive duplicate usernames exist.',
            DETAIL = conflicts,
            HINT = 'Resolve the listed conflicts explicitly before rerunning this migration. No users were changed.';
    END IF;
END;
$$;

CREATE UNIQUE INDEX users_name_unique_idx ON lab.users (LOWER(name));

COMMIT;
