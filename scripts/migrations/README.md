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

## Option history synchronization (historical, superseded by 009)

`007_option_contract_synced.sql` adds `lab.option_contracts.synced`. All existing
rows become `true`, since the previous pipeline ingested full 45-DTE history.
Future rows default to `false`; day-only ingestion leaves that flag unchanged.
Full-history ingestion sets it to true only in the transaction that commits the
historical candles. Apply this migration before deploying the updated Go and
Python services:

```powershell
psql --dbname "$env:DATABASE_URL" --set ON_ERROR_STOP=1 --file scripts/migrations/007_option_contract_synced.sql
```

## Historical option-chain completion

`008_option_chain_fetched.sql` adds `lab.option_expiry.option_chain_fetched` as
`BOOLEAN NOT NULL DEFAULT FALSE`. The flag was absent from the checked-in schema
and live database when this endpoint was introduced. Existing expiries remain
false: individual synced contracts do not prove a complete chain was ingested.
Apply this migration before using Python's `/option-chain/fetch` endpoint:

```powershell
psql --dbname "$env:DATABASE_URL" --set ON_ERROR_STOP=1 --file scripts/migrations/008_option_chain_fetched.sql
```

The endpoint updates the flag only in the transaction that commits the complete
historical strike universe and candles. No migration is applied automatically.

## Remove per-contract synchronization

`009_drop_option_contract_synced.sql` removes only `lab.option_contracts.synced`.
The fresh schema no longer includes it. The day-only ingestion endpoint and Go
repository method have also been removed. Full-chain ingestion still uses
`option_expiry.option_chain_fetched`, and commits all contracts/candles together.
Existing contracts and candles are preserved. Old partial contracts should be
completed using full-chain ingestion before relying on contract existence as a
full-history indicator. Stop older application writers before applying:

```powershell
psql --dbname "$env:DATABASE_URL" --set ON_ERROR_STOP=1 --file scripts/migrations/009_drop_option_contract_synced.sql
```

## Lab option trading symbols

`010_option_contract_tradingsymbol.sql` adds `tradingsymbol TEXT`, backfills all
existing contracts using `UNDERLYING-YYYYMMDD-RUPEE_STRIKE-CE/PE`, then enforces
NOT NULL and uniqueness. Fractional strikes are converted from integer paise
without floating point or redundant decimal zeros. Contract IDs and candles are
preserved. The entire migration is transactional; inconsistent underlying/expiry
relationships or duplicate generated symbols cause it to fail rather than leave
partially migrated data.

Stop older contract writers and deploy the symbol-aware data fetcher with this
migration. The fresh schema already includes the column and constraint.

```powershell
psql --dbname "$env:DATABASE_URL" --set ON_ERROR_STOP=1 --file scripts/migrations/010_option_contract_tradingsymbol.sql
```
