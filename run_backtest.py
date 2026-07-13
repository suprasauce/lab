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

from breeze_client import BreezeClient
from data_fetcher import DataFetcher
from engine import BacktestEngine
from report import write_report
from settings import StrategyConfig, load_credentials

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
    p.add_argument("--slippage", type=float, default=0.0)
    p.add_argument("--brokerage", type=float, default=0.0)
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
        slippage_per_leg=args.slippage,
        brokerage_per_trade=args.brokerage,
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
    fetcher = DataFetcher(client)
    engine = BacktestEngine(config, fetcher)
    trades, skipped = engine.run()
    out_dir = write_report(trades, skipped, config)

    total_pnl = sum(t.pnl for t in trades)
    win_rate = 100 * sum(1 for t in trades if t.pnl > 0) / len(trades) if trades else 0
    print(f"\nBacktest complete -> {out_dir}")
    print(
        f"  Trades: {len(trades)} | Skipped: {len(skipped)} | "
        f"Total PnL: Rs {total_pnl:,.0f} | Win rate: {win_rate:.1f}%"
    )
    print(f"  Open {out_dir / 'report.html'} in your browser to view full results.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
