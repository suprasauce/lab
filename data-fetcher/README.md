# Historical data ingestion service

Independent Python 3.11+ service sharing the Go application's existing PostgreSQL
`lab` schema. Four synchronous application endpoints provide full option
history, one option day, underlying history, and a historical option chain. Apply
`scripts/migrations/007_option_contract_synced.sql` and
`scripts/migrations/008_option_chain_fetched.sql` before deploying this version.

## Install and start

From the repository root, in PowerShell:

```powershell
cd data-fetcher
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
Copy-Item .env.example .env
$env:DATABASE_URL = "host=localhost port=5432 user=postgres password=replace_me dbname=optionsbacktest sslmode=disable"
.\.venv\Scripts\python.exe main.py
```

The service listens on `http://127.0.0.1:8000`. Keep one process and one request
thread (`main.py` already does this), so the shared Breeze session and request
budget cover all calls. Requests block until ingestion finishes; configure calling
clients with sufficiently long timeouts. There are no workers or scheduled jobs.

| Variable | Source | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | OS environment only | PostgreSQL keyword/value DSN or URL |
| `BREEZE_API_KEY` | OS environment or `.env` | Breeze application key |
| `BREEZE_API_SECRET` | OS environment or `.env` | Breeze application secret |
| `BREEZE_SESSION_TOKEN` | OS environment or `.env` | Current API session token |

Existing OS values take precedence. `.env` is read by `python main.py` and only
its three Breeze fields are loaded. Credentials are never returned in responses.
Get a session token through the [Breeze portal](https://api.icicidirect.com/apiuser/home)
or the [SDK login flow](https://github.com/Idirect-Tech/Breeze-Python-SDK#api-usage).
Restart the service after updating the token; automatic renewal is not implemented.

## POST /options/fetch

```json
{
  "underlying": "NIFTY",
  "expiry": "2025-12-30",
  "strike": 25000,
  "option_type": "CALL"
}
```

`strike` is in **rupees** at the HTTP boundary (`25000` becomes `2500000` paise).
`option_type` is `CALL` or `PUT`. Dates use `YYYY-MM-DD`. Supported symbol mappings
are NIFTY -> NIFTY and BANKNIFTY -> CNXBAN; the underlying must already exist in
`lab.underlyings`. The matching `lab.option_expiry` row must also already exist,
with its explicit MONTHLY/WEEKLY classification. The service does not guess it.

PowerShell curl example:

```powershell
@'
{"underlying":"NIFTY","expiry":"2025-12-30","strike":25000,"option_type":"CALL"}
'@ | curl.exe -X POST http://127.0.0.1:8000/options/fetch -H "Content-Type: application/json" --data-binary "@-"
```

A contract with `synced = true` returns HTTP 200 immediately, without
authenticating with Breeze or fetching candles:

```json
{"status":"already_exists","contract_id":123,"fetched":0,"inserted":0}
```

For a missing or unsynced contract, `MAX_DTE = 45` and `5minute` are fixed. The example fetches
November 15 through December 30, 2025, inclusive (46 calendar dates). There are no
DTE, timeframe, strike-range, or padding request fields. Unknown fields are rejected.

All Breeze windows are fetched and validated before the write transaction opens.
Then one transaction resolves or inserts the contract, inserts all candles in
batches, and sets `synced = true`. Any insertion or commit failure prevents
success and rolls back the changes, preserving previously stored partial data
and its unsynced flag. Unique constraints prevent duplicate
contracts/candles; a concurrent winner is reused after its transaction commits.
A new successful ingestion returns HTTP 200 only after commit:

```json
{"status":"ingested","contract_id":123,"fetched":3400,"inserted":3400}
```

Full synchronization with an expiry today or later is rejected because historical
data is incomplete. Empty full histories fail without inserting a contract or
setting `synced`. The migration marks existing contracts true, since they were
created by the previous 45-DTE pipeline; new contracts default to false.

## POST /options/fetch-day

```json
{
  "underlying": "NIFTY",
  "expiry": "2025-12-30",
  "strike": 25000,
  "option_type": "CALL",
  "day": "2025-12-01"
}
```

Fetches only the requested IST calendar day's 5-minute candles. The day must be
before today and no later than expiry. Existing data for that day avoids another
Breeze call. This endpoint atomically creates a missing contract and inserts day
candles; it leaves `synced` unchanged (a new contract remains false). Repeated
and overlapping day/full ingestions skip duplicate candles.

```powershell
@'
{"underlying":"NIFTY","expiry":"2025-12-30","strike":25000,"option_type":"CALL","day":"2025-12-01"}
'@ | curl.exe -X POST http://127.0.0.1:8000/options/fetch-day -H "Content-Type: application/json" --data-binary "@-"
```

Returns HTTP 200 with `status` (`ingested`, `already_exists`, or `no_data`),
`contract_id`, `fetched`, and `inserted`. With `no_data`, no contract or candles
are inserted; `contract_id` may be null. Go treats `no_data` as an unavailable
candidate. Failed requests roll back all new writes.

## POST /underlyings/fetch

```json
{
  "underlying": "NIFTY",
  "start_date": "2025-01-01",
  "end_date": "2025-12-31"
}
```

```powershell
@'
{"underlying":"NIFTY","start_date":"2025-01-01","end_date":"2025-12-31"}
'@ | curl.exe -X POST http://127.0.0.1:8000/underlyings/fetch -H "Content-Type: application/json" --data-binary "@-"
```

Dates are inclusive. The underlying must exist. This manually invoked endpoint
fetches only 5-minute cash/index data and inserts into `lab.underlying_candles`.
Overlapping requests skip existing `(underlying_id, timeframe, start_time)` rows.
It returns HTTP 200 with `status`, `fetched`, and newly `inserted` candle counts.
An empty underlying response is successful with zero inserts and can be retried.

These APIs return JSON errors: 400 for invalid input or missing catalog records,
502 for Breeze/runtime failures, and 500 for unexpected/database failures. A
failure must not be treated by callers as completed option ingestion.

## POST /option-chain/fetch

Fetch the historical strike universe for one already-classified expiry:

```sh
curl -X POST http://localhost:8000/option-chain/fetch \
  -H "Content-Type: application/json" \
  -d '{"underlying":"NIFTY","expiry":"2025-12-30"}'
```

Only `underlying` and `expiry` are accepted. Both catalog records must exist;
expiry must be before today. NIFTY uses a fixed Rs50 strike interval and BANKNIFTY
uses Rs100, stored internally as paise. The range is the underlying's minimum Low
rounded down and maximum High rounded up, extended by 40 intervals on each side.
Only positive strikes are generated. Both CALL and PUT contracts are fetched for
every strike, from expiry minus 45 calendar days through expiry inclusive.

All historical requests use 5-minute candles and at most 15-calendar-day windows.
Transient network, rate-limit, and server errors have at most three attempts total
(initial attempt plus two retries), with existing pacing and 2/4-second backoff.
Permanent errors fail immediately. A terminal option failure identifies the
underlying, expiry, strike in paise, option type, failed window, and reason.

Stored underlying candles are reused and their values preserved. Because there
is no underlying coverage marker, the full underlying range is also fetched
before deriving the strike universe, avoiding assumptions that partial stored
data is complete. Silent gaps in successful Breeze responses cannot be detected
independently. No market calendar or synthetic candles are introduced.

All fetched data is buffered in memory. Only after **every request succeeds** does
one database transaction insert any new underlying candles, all required
contracts and option candles, set contract `synced = true`, and set
`option_expiry.option_chain_fetched = true`. Conflicting candles are preserved.
Any failure rolls back all new ingestion data and flag changes. Successful empty
option windows are valid, including contracts with no available candles; these
contracts are still marked synced because their entire historical range was
checked. Empty underlying data with no stored candles is an error.

Success is HTTP 200, after commit, for example:

```json
{"status":"ingested","expiry_id":9,"contracts":166,"fetched":664,"inserted":664,"underlying_inserted":4}
```

Counts are illustrative. `contracts` counts generated contracts; `fetched` counts
option candles returned after per-contract deduplication; `inserted` counts new
option candles. `underlying_inserted` counts new underlying candles.
An expiry already flagged complete returns without Breeze calls:

```json
{"status":"already_exists","expiry_id":9,"contracts":0,"fetched":0,"inserted":0}
```

The synchronous service remains occupied until completion. Large chains can take
many minutes and consume substantial portions of the existing daily API budget.
No partial progress is committed, so a failure requires fetching again on retry.
The initial completion check is sufficient for the service's single-process,
single-threaded request handling; the final write phase uses no explicit row lock.

## Breeze and database conventions

- The existing official SDK authentication and request parameters are preserved:
  underlying data uses NSE/cash; options use NFO/options and explicit expiry,
  right, and strike. Breeze requests use `5minute`; PostgreSQL stores `5m`.
- Requests use shared 15-calendar-day windows and inclusive-second Breeze bounds.
  The SDK documents a [1000-interval limit](https://github.com/Idirect-Tech/Breeze-Python-SDK).
  Responses reaching 1000 rows are rejected as potentially truncated.
- Existing pacing remains one request/second with a 5000-request daily process
  budget, respecting the documented [100/minute and 5000/day limits](https://api.icicidirect.com/breezeapi/documents/index.html).
  Both options and underlying requests retry transient failures with at most
  three total attempts and 2/4-second backoff.
- Budget accounting is process-local, resets on restart, and cannot account for
  other applications using the same credentials. Run a single service process.
- Naive Breeze timestamps are interpreted as Asia/Kolkata. Aware timestamps are
  converted to that zone. Candle timestamps are preserved, never shifted by five
  minutes. The existing SDK convention formats local clock values with `.000Z`
  in requests; this refactor intentionally preserves that working behavior.
- Decimal conversion produces integer-paise BIGINT OHLC/strike values and
  validates integral BIGINT volumes, preserving Breeze's negative values for now.
  No float-based rupee rounding,
  resampling, synthetic candles, market-hour filters, or holiday calendar is added.
- Completeness means every requested window succeeded; availability is determined
  by Breeze. Empty successful windows are allowed, but API errors are not silently
  treated as holidays. The API cannot independently detect silent upstream gaps.
- **Existing data:** migration 007 marks all existing contracts synced, as
  requested. Subsequent full-history requests trust that flag; day-only
  contracts are distinguished by `synced = false`.
- Fetching is synchronous and buffered in memory before writing. A failure before
  commit requires a fresh fetch on retry; SQLite checkpoints and bulk generation
  have been removed.

Go queries PostgreSQL first, calls `/options/fetch` for missing or unsynced full
history, waits for completion, then queries again. Delta candidates use the
day-only endpoint instead.

## Files and tests

`main.py` contains HTTP/configuration; `ingestion.py` contains the three flows and
candle conversion; `database.py` contains SQL; `breeze_client.py` owns SDK access
and pacing. There is no generic pipeline framework.

```powershell
.\.venv\Scripts\python.exe -m unittest discover -s tests -v
```

Tests use mocked Breeze and a transactional SQL double, including the actual
persistence helper functions. They cover existing contracts, fetching before a
transaction, rollback after a later insert fails, commit failure, duplicate/racing
requests, overlapping ranges, paise/IST conversion, and fixed 45-DTE/5m behavior.
No live Breeze session or PostgreSQL credentials are required by these tests.
