# Historical data ingestion service

Independent Python 3.11+ service sharing the Go application's existing PostgreSQL
`lab` schema. Two synchronous application endpoints provide underlying history and a historical
option chain. Both endpoints always fetch 1-minute candles and store `1m`.
There is no timeframe request parameter. Apply migrations through
`scripts/migrations/009_drop_option_contract_synced.sql` before deploying this version.

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
thread (`main.py` already does this), so the Breeze session and request
budget cover all calls. Requests block until ingestion finishes; configure calling
clients with sufficiently long timeouts. There are no workers or scheduled jobs.

| Variable | Source | Purpose |
| --- | --- | --- |
| `MARKET_DATA_PROVIDER` | OS environment or `.env` | `breeze` (default) or `upstox`; selected once at startup |
| `UPSTOX_ACCESS_TOKEN` | OS environment or `.env` | Upstox bearer token for instrument search and historical candles |
| `DATABASE_URL` | OS environment only | PostgreSQL keyword/value DSN or URL |
| `BREEZE_API_KEY` | OS environment or `.env` | Breeze application key |
| `BREEZE_API_SECRET` | OS environment or `.env` | Breeze application secret |
| `BREEZE_SESSION_TOKEN` | OS environment or `.env` | Current API session token |

Existing OS values take precedence. The selected provider's credentials are loaded
from `.env`; `DATABASE_URL` remains OS-only. Credentials are never returned in responses.
Set `MARKET_DATA_PROVIDER=breeze` for Breeze ingestion, or `upstox` for Upstox
underlying and expired option-chain ingestion. Both providers expose the same two routes.
Unknown provider names fail startup. Restart the server to switch providers.
Breeze authentication remains lazy and its session is reused across requests.
Get a session token through the [Breeze portal](https://api.icicidirect.com/apiuser/home)
or the [SDK login flow](https://github.com/Idirect-Tech/Breeze-Python-SDK#api-usage).
Restart the service after updating the token; automatic renewal is not implemented.

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
fetches 1-minute cash/index candles and inserts them as `1m` into
`lab.underlying_candles`. Requests are split into two-calendar-day windows to
respect Breeze's candle limit. Supplying `timeframe` is rejected as an unknown field.
Overlapping requests skip existing `(underlying_id, timeframe, start_time)` rows.
It returns HTTP 200 with `status`, `fetched`, and newly `inserted` candle counts.
An empty underlying response is successful with zero inserts and can be retried.

These APIs return JSON errors: 400 for invalid input or missing catalog records,
502 for Breeze/runtime failures, and 500 for unexpected/database failures. A
failure must not be treated by callers as completed option ingestion.

### Upstox underlying ingestion

With `MARKET_DATA_PROVIDER=upstox`, the same underlying request uses the
[Instrument Search API](https://upstox.com/developer/api-documentation/instrument-search/)
to resolve an exact NSE index trading symbol or name. Searches are paginated;
missing or ambiguous exact matches return HTTP 400. Instrument keys are never
hardcoded, and partial matches such as other NIFTY indices are not substituted.

The [Historical V3 API](https://upstox.com/developer/api-documentation/v3/get-historical-candle-data/)
is queried at `minutes/1` using inclusive windows capped at each calendar month's end. Candles are sorted and
deduplicated, timestamps retain their actual IST starts, and Decimal conversion
preserves paise. The full result is fetched before batched inserts in one database
transaction. Reruns skip duplicates. Requests use the existing `requests` dependency,
a 30-second timeout, and one-request-per-second pacing. HTTP failures (including
expired tokens and rate limits) abort ingestion with HTTP 502; there are no
automatic retries or token renewal. Historical current-day data availability is
provider-defined; a separate intraday endpoint is not implemented.

## POST /option-chain/fetch

Fetch the historical strike universe for one already-classified expiry:

```sh
curl -X POST http://localhost:8000/option-chain/fetch \
  -H "Content-Type: application/json" \
  -d '{"underlying":"NIFTY","expiry":"2025-12-30"}'
```

Exactly `underlying` and `expiry` are required. Both catalog records must exist;
expiry must be before today. With Breeze, NIFTY uses a fixed Rs50 strike interval and BANKNIFTY
uses Rs100, stored internally as paise. The range is the underlying's minimum Low
rounded down and maximum High rounded up, extended by 40 intervals on each side.
Only positive strikes are generated. Both CALL and PUT contracts are fetched for
every strike, from expiry minus 45 calendar days through expiry inclusive.

Both underlying and option requests use `1minute`; stored candles use `1m`.
Only stored 1-minute underlying candles participate in strike-range calculation.
Breeze requests use two-calendar-day windows.
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
contracts and option candles, then sets `option_expiry.option_chain_fetched = true`.
The completion flag now represents the full 45-DTE **1-minute** chain. Existing `true` flags from
older 5-minute ingestion are not converted automatically. Those flags must be
reset explicitly before re-ingesting those expiries at 1 minute; otherwise they
continue to return `already_exists`. Existing candles are retained.
Conflicting candles are preserved.
Any failure rolls back all new ingestion data and flag changes. Successful empty
option windows are valid, including contracts with no available candles; these
contracts are still inserted because their entire historical range was checked. Empty underlying data with no stored candles is an error.

Success is HTTP 200, after commit, for example:

```json
{"status":"ingested","expiry_id":9,"contracts":166,"fetched":498,"inserted":498,"underlying_inserted":3}
```

Counts are illustrative. `contracts` counts generated contracts; `fetched` counts
option candles returned after per-contract deduplication; `inserted` counts new
option candles. `underlying_inserted` counts new underlying candles.
A request for an expiry already flagged complete returns without Breeze calls:

```json
{"status":"already_exists","expiry_id":9,"contracts":0,"fetched":0,"inserted":0}
```

The synchronous service remains occupied until completion. Large chains can take
many minutes and consume substantial portions of the existing daily API budget.
No partial progress is committed, so a failure requires fetching again on retry.
The initial completion check is sufficient for the service's single-process,
single-threaded request handling; the final write phase uses no explicit row lock.

### Upstox option-chain ingestion

Upstox resolves the underlying instrument key through search, then calls
[Expired Option Contracts](https://upstox.com/developer/api-documentation/get-expired-option-contracts/)
for the requested expiry. It uses every returned CE/PE contract and its instrument
key, rather than generating a strike range. The underlying and expiry must already
exist in PostgreSQL. Lot sizes remain supplied by the existing expiry record.

For each discovered contract, the
[Expired Historical Candle API](https://upstox.com/developer/api-documentation/get-expired-historical-candle-data/)
is requested at `1minute` from expiry minus 45 days through expiry inclusive,
split into inclusive windows capped at each calendar month's end. Underlying candles for the same range
are fetched through V3. Both paths reuse the same Upstox windowing, normalization,
timeout, and pacing logic.

All requests complete before one write transaction inserts the underlying candles,
contracts with Lab trading symbols, option candles, and the completion flag.
Duplicates are skipped. An empty contract list is an error and never marks the
chain complete. Successful empty histories for individual contracts are allowed,
as with Breeze. Missing historical coverage, authentication errors, or any candle
request failure abort ingestion; insertion or commit failures roll back everything.
The response fields are the same for both providers. No Upstox instrument keys
or broker trading symbols are written into Lab's trading-symbol column.

## Lab trading symbols

Contract creation reuses `trading_symbol.generate_option_trading_symbol()` for
full-chain ingestion. It produces
`UNDERLYING-YYYYMMDD-STRIKE-CE/PE`, for example `NIFTY-20261124-26000-CE` or
`NIFTY-20261124-26000-PE`. Monthly and weekly contracts use the same format;
no expiry-day rules are inferred.

Underlying names are uppercased. Strikes are converted from integer paise to
integer rupees without floating point: 2600000, 2600050, and 2600005 all become
`26000`; remaining paise are discarded. CALL maps to CE and PUT maps to PE. These are
Lab-specific symbols and are never passed as broker symbols to Breeze.

If `lab.option_contracts.tradingsymbol` exists, new contracts store their generated
symbol in that column. If absent, the existing schema is preserved and Go resolves
canonical symbols through their parsed contract attributes. Existing contracts
are reused without duplication; adding the column later requires a separate
backfill of existing records. No migration or backfill is performed automatically.
Breeze continues receiving structured underlying, expiry, strike, and option type
parameters, and candle windows and ingestion transactions are unchanged.

## Breeze and database conventions

- The existing official SDK authentication and request parameters are preserved:
  underlying data uses NSE/cash; options use NFO/options and explicit expiry,
  right, and strike. Both ingestion endpoints are fixed at `1minute` / `1m`.
- Requests use two-calendar-day windows and inclusive-second Breeze bounds.
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
- Fetching is synchronous and buffered in memory before writing. A failure before
  commit requires a fresh fetch on retry; SQLite checkpoints and bulk generation
  have been removed.

Go reads PostgreSQL independently and never calls this service. Ingest the
required data before running backtests. The option-chain completion flag remains;
individual-contract and day-only ingestion endpoints are not exposed.

## Files and tests

`main.py` contains provider-neutral routes and startup configuration. Each provider
implements `fetch_underlying(underlying, start_date, end_date)` and
`fetch_option_chain(underlying, expiry)`. Provider workflows own their request and
contract-discovery rules; database, trading-symbol, and `candles.py` date/price
conversion helpers remain shared.

```text
providers/
  __init__.py            # Provider selection
  breeze/
    __init__.py          # BreezeProvider: the two operations
    client.py            # Existing SDK authentication and rate limits
    ingestion.py         # Underlying ingestion and Breeze request windows
    option_chain.py      # Atomic chain ingestion
  upstox.py              # Underlying and atomic option-chain ingestion
  upstox_client.py       # Search, expired contracts, and historical requests
```

No provider API implementation is required by the routes themselves. No SDK or
new dependencies have been added for Upstox yet.

```powershell
.\.venv\Scripts\python.exe -m unittest discover -s tests -v
```

Tests use mocked Breeze and a transactional SQL double, including the actual
persistence helper functions. They cover chain ingestion, rollback, idempotency,
underlying overlap, request validation, and removal of the individual-contract route.
