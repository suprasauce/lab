# Option expiry migration

`../lab_schema.sql` is the complete schema for a fresh database. Do not run it
against an existing installation. `001_option_expiry.sql` migrates the original
`lab.option_contracts.expiry` design once; it is not a repeatable migration.

Before migrating a populated database, list the pairs requiring classification:

```sql
SELECT DISTINCT underlying_id, expiry
FROM lab.option_contracts
ORDER BY underlying_id, expiry;
```

Verify each expiry's MONTHLY/WEEKLY classification against your authoritative
contract data. Fill in the commented `INSERT INTO pg_temp.option_expiry_classification`
input block in the migration, one row per pair. These are migration inputs, not
application configuration. Do not substitute a weekday or month-end heuristic.
An empty contracts table requires no classification rows.

Run during a maintenance window; the transaction locks `option_contracts`:

```powershell
psql --dbname "$env:DATABASE_URL" --set ON_ERROR_STOP=1 --file scripts/migrations/001_option_expiry.sql
```

Every existing contract is linked to a newly migrated expiry. Contract IDs,
underlying IDs, prices, types, timestamps, and option candle references are
preserved. Missing classifications abort the transaction without changing the
schema or data. The old expiry column and its dependent table constraints and
indexes are removed; unrelated external dependencies cause a rollback rather
than a cascading deletion. Stop old application writers and deploy the updated
Go queries together with this migration.

`GetExpiries` retains its date-list API and now reads the expiry catalog, including
dates with no contracts yet. `GetOptionContract` retains its lookup signature and
joins the catalog to return `Option.ExpiryID` and its resolved `Option.Expiry` date.
Expiry types are stored without adding an implicit monthly/weekly filter.
