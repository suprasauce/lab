# Options Backtesting UI

Backtest strategies using ICICI Breeze API historical 5-minute data with on-demand DuckDB caching.

## Strategy Builder

- Build a custom multi-leg option strategy from the UI.
- Each leg has its own option type, side, quantity, strike rule, entry DTE/time, and exit DTE/time.
- Supported strike selection methods: Delta, Offset in points, Premium, Fixed Strike, and ATM.
- All positions in one expiry cycle use the same monthly expiry.
- Monthly expiry: last **Thursday** before Sep 2025, last **Tuesday** from Sep 2025

## Architecture

- `backend/controllers/` contains FastAPI page routes.
- `backend/services/` contains API use cases: running backtests, loading results, fetching market data, and building MTM.
- `backend/dao/market_data_dao.py` stores and loads 5-minute candles in `data/market_data.duckdb`.
- `backend/client/breeze_client.py` wraps the Breeze SDK.
- `backend/common/` contains shared calendar, candle, and strike-selection helpers.
- `frontend/templates/` contains the HTML templates.
- `tests/` contains the test suite.

The web controller handles HTTP/form input, services orchestrate the use case, the DAO owns DuckDB SQL, and the client owns Breeze API calls.

DuckDB tables:

- `underlying_5m`
- `derivatives_5m`

## Setup

```bash
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

## UI

```bash
python -m uvicorn backend.app:app --reload
```

Open `http://127.0.0.1:8000` to configure legs, run a backtest, view trades, and open a trade's daily MTM table.

## Output

Results land in `results/{timestamp}/`:

- `metadata.json`
- `metrics.json`
- `equity_curve.json`
- `expiry_pnl_curve.json`
- `trade_metrics.json`
- `vix_curve.json`
- `trades.csv`
- `skipped_expiries.csv`
- `daily_mtm.csv`

5-minute candles are cached in `data/market_data.duckdb` for reuse.

## Tests

```bash
python -m pytest
```
