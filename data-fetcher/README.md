# Historical data ingestion service

Independent Python 3.11+ service sharing the Go application's existing PostgreSQL
`lab` schema. Three synchronous application endpoints provide full option
history, one option day, and underlying history. Apply
`scripts/migrations/007_option_contract_synced.sql` before deploying this version.

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

## Breeze and database conventions

- The existing official SDK authentication and request parameters are preserved:
  underlying data uses NSE/cash; options use NFO/options and explicit expiry,
  right, and strike. Breeze requests use `5minute`; PostgreSQL stores `5m`.
- Requests use shared 13-calendar-day windows and inclusive-second Breeze bounds.
  The SDK documents a [1000-interval limit](https://github.com/Idirect-Tech/Breeze-Python-SDK).
  Responses reaching 1000 rows are rejected as potentially truncated.
- Existing pacing remains one request/second with a 5000-request daily process
  budget, respecting the documented [100/minute and 5000/day limits](https://api.icicidirect.com/breezeapi/documents/index.html).
  Options retry transient failures up to three attempts with 2/4-second backoff.
  The cash request path retains its existing immediate error propagation.
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
