# Nifty Short Strangle Backtest

Backtest a Nifty 50 short strangle using ICICI Breeze API historical 5-minute data.

## Strategy Defaults

- Enter **45 DTE** before monthly expiry at **09:30** IST
- Exit on expiry day at **15:30** IST
- Strikes: **ATM +/- 6 strike steps** where each step is 50 points
- Monthly expiry: last **Thursday** before Sep 2025, last **Tuesday** from Sep 2025

## Setup

```bash
cd backtest
python -m venv .venv
.venv\Scripts\activate          # Windows
pip install -r requirements.txt
copy .env.example .env          # fill Breeze credentials
```

Get credentials from [ICICI Breeze API](https://api.icicidirect.com/). Session token must be refreshed daily after login.

## Run

```bash
python run_backtest.py --start-date 2024-01-01 --end-date 2025-12-31
```

Options: `--entry-dte`, `--entry-time`, `--exit-time`, `--strike-offset`, `--lot-size`, `--slippage`, `--brokerage`

## Output

Results land in `results/{timestamp}/`:

- `report.html` - summary, equity chart, trade tables

- `trades.csv`, `skipped.csv`, `equity_curve.csv`

5-minute candles are cached under `data/5min/` for reuse and future daily MTM.

## Tests

```bash
python -m pytest test_calendar.py -v
```
