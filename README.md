# Nifty Short Strangle Backtest

Backtest strategies using ICICI Breeze API historical 5-minute data with on-demand DuckDB caching.

## Strategy Defaults

- Enter **45 DTE** before monthly expiry at **09:30** IST
- Exit on expiry day at **15:30** IST
- Strikes: **ATM +/- 6 strike steps** where each step is 50 points
- Monthly expiry: last **Thursday** before Sep 2025, last **Tuesday** from Sep 2025

## Architecture

- `market_data/store.py` stores 5-minute candles in `data/market_data.duckdb`.
- `market_data/data.py` checks DuckDB for requested candles, fetches full missing days from Breeze, and returns one candle dict at a time.
- `strategies/short_strangle.py` implements the current monthly short strangle.
- `backtest/engine.py` iterates expiries and calls `strategy.run(config, expiry, data)`.
- `analytics/mtm.py` derives daily MTM from completed trade rows and cached market data.
- `reporting/report.py` writes trades and skipped expiries as CSV files and simple HTML tables.
- `common/` contains shared settings, calendar helpers, candle utilities, and strike selection.
- `tests/` contains the test suite.

The engine stays deliberately small. It iterates expiries and combines the tables returned by the strategy. The current strategy computes its own entry and exit dates from the expiry, fetches only the exact candles it needs, returns one row per option leg in `trades`, and returns untraded expiries separately in `skipped_expiries`. Daily MTM is derived after the backtest from the trade rows.

DuckDB tables:

- `underlying_5m`
- `derivatives_5m`

## Setup

```bash
cd backtest
python -m venv .venv
.venv\Scripts\activate          # Windows
pip install -r requirements.txt
copy .env.example .env          # fill Breeze credentials
```

Get credentials from [ICICI Breeze API](https://api.icicidirect.com/). Session token must be refreshed daily after login.

## Breeze Session Token

Your `.env` needs:

```text
BREEZE_API_KEY=...
BREEZE_API_SECRET=...
BREEZE_SESSION_TOKEN=...
```

To create or refresh `BREEZE_SESSION_TOKEN`:

1. Open this URL after replacing `<API_KEY>` with your Breeze API key:

```text
https://api.icicidirect.com/apiuser/login?api_key=<API_KEY>
```

2. Log in with your ICICI Direct account and complete any required verification.
3. After login, copy the session value from the redirected URL. It is usually shown as an `apisession` query parameter.
4. Paste that value into `.env`:

```text
BREEZE_SESSION_TOKEN=<copied_apisession_value>
```

5. Run the backtest again.

The session token expires, so refresh it whenever Breeze returns `Session key is expired`.

## Run

```bash
python run_backtest.py --start-date 2024-01-01 --end-date 2025-12-31
```

Options: `--entry-dte`, `--entry-time`, `--exit-time`, `--strike-offset`, `--lot-size`

## Output

Results land in `results/{timestamp}/`:

- `report.html` - trades, skipped-expiries, and daily MTM tables

- `trades.csv`
- `skipped_expiries.csv`
- `daily_mtm.csv`

5-minute candles are cached in `data/market_data.duckdb` for reuse.

## Tests

```bash
python -m pytest
```
