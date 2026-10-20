# Underlying data fetcher

Independent Python 3.11+ command. It shares PostgreSQL with Go but imports no Go
code and has its own environment and dependencies. `main.py` ingests NIFTY index
candles; `ingest_options.py` independently ingests option contracts and candles.

From the repository root, on Windows PowerShell:

```powershell
cd data-fetcher
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
Copy-Item .env.example .env
```

Set the PostgreSQL connection string in the OS environment before running:

```powershell
$env:DATABASE_URL = "host=localhost port=5432 user=postgres password=replace_me dbname=optionsbacktest sslmode=disable"
```

`DATABASE_URL` is read only from the OS environment, never from `.env`.
Edit `.env` with your Breeze API key, secret, and current session token.
Existing process environment variables take precedence over `.env`.
Obtain a fresh token through the Breeze login/session flow when it
expires. Keep `.env` private; it and the local virtual environment are ignored.

### Get the Breeze session token

Open the [Breeze API login](https://api.icicidirect.com/apiuser/login?api_key=YOUR_API_KEY),
replacing `YOUR_API_KEY` with your URL-encoded Breeze API key. Complete the ICICI
Direct login and use the resulting API session token as `BREEZE_SESSION_TOKEN`
in `.env` (not your API secret).

If the API key contains special characters, generate the login URL locally:

```python
from urllib.parse import quote_plus

api_key = "YOUR_API_KEY"
print("https://api.icicidirect.com/apiuser/login?api_key=" + quote_plus(api_key))
```

Then update `.env`:

```dotenv
BREEZE_SESSION_TOKEN=YOUR_API_SESSION_TOKEN
```

See the official [Breeze SDK authentication example](https://github.com/Idirect-Tech/Breeze-Python-SDK#api-usage).

The database must already contain `lab.underlyings` with `symbol = 'NIFTY'` and
`lab.underlying_candles`, as used by the Go queries. The schema script lists the
columns and constraints but does not qualify a schema; this fetcher explicitly
uses the application's `lab` tables. It never creates tables or underlying rows.

```powershell
.\.venv\Scripts\python.exe main.py
```

Defaults: NIFTY, September 1–30, 2026 inclusive, 5-minute candles. Override them:

```powershell
.\.venv\Scripts\python.exe main.py --underlying NIFTY --start-date 2026-09-01 --end-date 2026-09-30 --timeframe 5minute
```

Direct callers can import `fetch_underlying` from `pipelines.underlying`, set the
environment variables, and call:

```python
from pipelines.underlying import fetch_underlying

fetch_underlying(underlying="NIFTY", start_date="2026-09-01", end_date="2026-09-30", timeframe="5minute")
```

## Input examples

Run the following commands from `data-fetcher`. Breeze credentials belong in `.env`
and `DATABASE_URL` belongs in the OS environment;
underlying, date range, and timeframe are command-line inputs.

| Input | Default | Accepted values |
| --- | --- | --- |
| `--underlying` | `NIFTY` | `NIFTY` |
| `--start-date` | `2026-09-01` | Calendar date in `YYYY-MM-DD` format |
| `--end-date` | `2026-09-30` | Calendar date on or after start date, inclusive |
| `--timeframe` | `5minute` | `1minute`, `5minute`, `1day` |

Fetch one day of 1-minute candles:

```powershell
.\.venv\Scripts\python.exe main.py --start-date 2026-09-01 --end-date 2026-09-01 --timeframe 1minute
```

Fetch one week of 5-minute candles, with each input shown separately:

```powershell
.\.venv\Scripts\python.exe main.py `
    --underlying NIFTY `
    --start-date 2026-09-01 `
    --end-date 2026-09-07 `
    --timeframe 5minute
```

Fetch daily candles for September:

```powershell
.\.venv\Scripts\python.exe main.py --start-date 2026-09-01 --end-date 2026-09-30 --timeframe 1day
```

Show available arguments without fetching data:

```powershell
.\.venv\Scripts\python.exe main.py --help
```

To change defaults permanently, edit the `default=` values in `main.py`'s
`parser.add_argument(...)` calls. Command-line arguments override those defaults.

For a standalone Python script inside `data-fetcher`, load `.env` explicitly:

```python
import os
from dotenv import dotenv_values
from pipelines.underlying import fetch_underlying

values = dotenv_values(".env")
for name in ("BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN"):
    if values.get(name) is not None:
        os.environ.setdefault(name, values[name])
result = fetch_underlying(
    underlying="NIFTY",
    start_date="2026-09-01",
    end_date="2026-09-07",
    timeframe="5minute",
)
print(result)  # {"fetched": ..., "inserted": ...}
```

Rerun any command to fetch the same range again; existing candles are skipped.

The official [Breeze SDK](https://github.com/Idirect-Tech/Breeze-Python-SDK)
provides historical V2 data. Requests use `stock_code=NIFTY`, `exchange_code=NSE`,
and `product_type=cash`, without option expiry, strike, or right parameters.
Following its historical examples, request timestamps use market wall-clock
values with the API's literal `.000Z` suffix. Naive response timestamps are
interpreted as Asia/Kolkata; explicitly offset response timestamps are converted
to that zone. PostgreSQL receives timezone-aware datetime values.

Intraday requests use nonoverlapping 12-hour calendar windows, including weekends
and holidays, keeping even 1-minute requests below the documented 1,000-candle
limit. Daily requests cover one calendar day. No trading-calendar assumptions,
resampling, or missing-candle generation are applied. A response reaching the
limit is rejected instead of silently accepting possibly truncated data.

Timeframe mapping: `1minute → 1m`, `5minute → 5m`, `1day → 1d`. Breeze V2 does
not offer `15minute`; it is rejected rather than aggregated or mislabeled.
OHLC values pass through `Decimal(str(value))` and are stored as integer paise;
sub-paise, negative, nonfinite, and out-of-BIGINT values are rejected. Index volume
is stored as returned, including zero.

Inserts use batches of up to 500 rows and the existing unique key
`(underlying_id, timeframe, start_time)` with `ON CONFLICT DO NOTHING`. Each window
commits independently; on a later failure, rerunning safely skips committed rows.
Existing candles are not overwritten. Logs include fetched and newly inserted
counts, so reruns can report fetched rows but zero inserts.

The [Breeze API limits](https://api.icicidirect.com/breezeapi/documents/index.html)
are 100 requests/minute and 5,000/day. This client spaces historical calls at
least one second apart and caps its own daily request count, including its
session request. It cannot count calls made by other processes or earlier runs:
run one fetcher per account and account for other Breeze usage. API failures stop
the underlying run. The options command retries transient failures as described
below. Neither command uses background workers.

Run offline tests (no authentication or database access):

```powershell
.\.venv\Scripts\python.exe -m unittest discover -s tests -v
```

## Independent options ingestion

The underlying and expiry must already exist in `lab.underlyings` and
`lab.option_expiry`. Apply the Go project's expiry relationship migration first
if the database still uses `option_contracts.expiry`. This command never creates
or classifies expiry rows, and never invokes the underlying pipeline.

Underlying candles for the full requested range must already be available.
The command queries their overall minimum low and maximum high once, across
stored timeframes. It fails if no underlying candles exist; it does not fetch
missing underlying data or assert that every day is present.

Supply the strike interval and symmetric padding as command-line inputs.
`--strike-interval-paise 5000` means INR 50; `--strike-padding 20` adds 20
intervals below the rounded minimum and above the rounded maximum. Neither
value is read from an environment variable or hardcoded in the pipeline.

With `DATABASE_URL` already set in the OS and Breeze credentials configured:

```powershell
.\.venv\Scripts\python.exe ingest_options.py `
    --underlying NIFTY `
    --expiry 2026-09-29 `
    --start-date 2026-09-01 `
    --end-date 2026-09-30 `
    --strike-interval-paise 5000 `
    --strike-padding 20
```

All six arguments are required. The interval must be a positive integer in
paise; padding must be a nonnegative integer (zero adds no extra intervals). Dates are inclusive, in Asia/Kolkata. The
interval `[start_date, end_date + 1 day)` may span at most one calendar month
from the start date (with month-end clamping). September 1-30 and September 15
through October 14 are valid; September 1 through October 1 is too long.

The range is calculated in paise:

```text
lower = floor(overall_low / interval) * interval - padding * interval
upper = ceil(overall_high / interval) * interval + padding * interval
```

Every interval-aligned strike from lower through upper produces a CALL and PUT.
Contracts use the existing `expiry_id`, skip duplicates, and resolve both new
and existing IDs. Invalid negative expanded strikes are rejected, not clamped.

Every contract uses the same supplied start/end range, even if end_date is after
expiry. No expiry clipping or market-hours filter is applied. Twelve-hour
requests use `5minute`, exchange `NFO`, product `options`, the explicit expiry,
`call`/`put`, and a rupee strike string derived with Decimal from integer paise.
Successful responses are inserted in batches into `lab.option_candles` as `5m`.
All returned candles within the requested windows are retained, including those
outside normal market hours. Existing candle rows are not overwritten.

Supported stock-code mappings are NIFTY to `NIFTY` and BANKNIFTY to `CNXBAN`.
For BANKNIFTY, supply its interval using `--strike-interval-paise`. Other symbols
fail rather than guessing their Breeze code or exchange.

Transient network errors, HTTP/API 429, and selected 5xx errors receive at most
three attempts, with 2- and 4-second backoff plus normal request pacing.
Permanent API errors and exhausted retries stop the run with contract/window
context. Logs show the strike range, contract count, request progress, fetched
rows, inserted rows, failures, and skipped completed windows.

Successful historical windows are checkpointed in local SQLite metadata at
`.state/options.sqlite3`, **after PostgreSQL commits**. This is ingestion progress,
not a new PostgreSQL table. Existing candle counts alone never imply completion.
Empty successful historical responses are also recorded as completed; failed
requests are not. Windows ending less than five minutes before request time are
not checkpointed, so recent or future ranges can be fetched again later.

Rerun the identical command after a failure. Completed contract/windows for that
database are skipped; unfinished windows are fetched again, and duplicate
inserts are harmless. An already completed rerun does not authenticate to Breeze.
Keep the local progress file to retain this benefit. If you restore/delete
PostgreSQL candle data or want to recheck successful empty responses, remove the
local `.state/options.sqlite3` file before rerunning. Run one options ingestion
process at a time; account-wide quotas across other processes remain your
responsibility. A large strike range may need multiple daily runs.
