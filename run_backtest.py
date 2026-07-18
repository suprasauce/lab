#!/usr/bin/env python3
"""Run Nifty short strangle backtest."""

from __future__ import annotations

import argparse
import logging
import sys
from datetime import date, datetime, time
from pathlib import Path

# Allow running as script from repo root or backtest folder
sys.path.insert(0, str(Path(__file__).resolve().parent))

from backtest.engine import BacktestEngine
from breeze_client import BreezeClient
from market_data.data import MarketData
from market_data.store import DuckDBMarketDataStore
from reporting.report import write_report
from settings import StrategyConfig, load_credentials
from strategies.short_strangle import ShortStrangleStrategy

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
)
logger = logging.getLogger(__name__)


def parse_time(s: str) -> time:
    h, m = map(int, s.split(":"))
    return time(h, m)


def parse_date(s: str) -> date:
    return datetime.strptime(s, "%Y-%m-%d").date()


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description="Nifty short strangle backtest (Breeze API)")
    p.add_argument("--entry-dte", type=int, default=45)
    p.add_argument("--entry-time", type=str, default="09:30", help="HH:MM IST")
    p.add_argument("--exit-time", type=str, default="15:30", help="HH:MM IST")
    p.add_argument("--strike-offset", type=int, default=6)
    p.add_argument("--lot-size", type=int, default=None, help="Override auto lot size")
    p.add_argument("--start-date", type=str, default=None, help="YYYY-MM-DD")
    p.add_argument("--end-date", type=str, default=None, help="YYYY-MM-DD")
    return p


def main() -> int:
    args = build_parser().parse_args()
    today = date.today()
    config = StrategyConfig(
        entry_dte=args.entry_dte,
        entry_time=parse_time(args.entry_time),
        exit_time=parse_time(args.exit_time),
        strike_offset=args.strike_offset,
        lot_size=args.lot_size,
        start_date=parse_date(args.start_date) if args.start_date else date(today.year - 1, today.month, 1),
        end_date=parse_date(args.end_date) if args.end_date else today,
    )

    logger.info(
        "Backtest %s -> %s | DTE=%d offset=+/- %d entry=%s exit=%s",
        config.start_date,
        config.end_date,
        config.entry_dte,
        config.strike_offset,
        config.entry_time,
        config.exit_time,
    )

    try:
        creds = load_credentials()
    except ValueError as e:
        logger.error("%s", e)
        return 1

    client = BreezeClient(creds)
    store = DuckDBMarketDataStore()
    market_data = MarketData(client, store)
    strategy = ShortStrangleStrategy()
    engine = BacktestEngine(market_data)
    results = engine.run(strategy, config)
    out_dir = write_report(results, strategy_name=strategy.name)

    trades = results["trades"]
    skipped_expiries = results["skipped_expiries"]
    print(f"\nBacktest complete -> {out_dir}")
    print(f"  Trade rows: {len(trades)} | Skipped expiries: {len(skipped_expiries)}")
    print(f"  Open {out_dir / 'report.html'} in your browser to view full results.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
