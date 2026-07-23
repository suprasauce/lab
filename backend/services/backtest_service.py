"""Backtest use-case service."""

from __future__ import annotations

from datetime import date, datetime, time

import pandas as pd

from backend.common.nse_calendar import iter_monthly_expiries
from backend.config.settings import StrategyConfig
from backend.services.metrics_service import (
    build_backtest_metrics,
    build_equity_curve,
    build_trade_metrics,
)
from backend.services.mtm_service import build_daily_mtm
from backend.services.result_service import save_run
from backend.strategies.short_strangle import ShortStrangleStrategy

STRATEGIES = {
    ShortStrangleStrategy.name: {
        "id": ShortStrangleStrategy.name,
        "name": "Nifty Short Strangle",
        "description": "Monthly Nifty short strangle with configurable entry DTE and strike offset.",
    }
}


def list_strategies() -> list[dict]:
    return list(STRATEGIES.values())


def get_strategy(strategy_id: str) -> dict:
    if strategy_id not in STRATEGIES:
        raise ValueError(f"Unknown strategy: {strategy_id}")
    return STRATEGIES[strategy_id]


def run_backtest_for_strategy(
    *,
    strategy_id: str,
    start_date: date,
    end_date: date,
    entry_dte: int,
    entry_time: time,
    exit_time: time,
    strike_offset: int,
    lot_size: int | None,
) -> tuple[str, dict[str, pd.DataFrame]]:
    if strategy_id != ShortStrangleStrategy.name:
        raise ValueError(f"Unsupported strategy: {strategy_id}")

    config = StrategyConfig(
        entry_dte=entry_dte,
        entry_time=entry_time,
        exit_time=exit_time,
        strike_offset=strike_offset,
        lot_size=lot_size,
        start_date=start_date,
        end_date=end_date,
    )
    strategy = ShortStrangleStrategy()
    results = run_backtest(strategy, config)
    results["daily_mtm"] = build_daily_mtm(results["trades"])
    results["metrics"] = build_backtest_metrics(
        trades=results["trades"],
        skipped_expiries=results["skipped_expiries"],
        daily_mtm=results["daily_mtm"],
    )
    results["equity_curve"] = build_equity_curve(results["trades"])
    results["trade_metrics"] = build_trade_metrics(
        trades=results["trades"],
        daily_mtm=results["daily_mtm"],
    )

    run_id = datetime.now().strftime("%Y%m%d_%H%M%S")
    metadata = {
        "run_id": run_id,
        "strategy_id": strategy_id,
        "strategy_name": get_strategy(strategy_id)["name"],
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "start_date": start_date.isoformat(),
        "end_date": end_date.isoformat(),
        "entry_dte": entry_dte,
        "entry_time": entry_time.strftime("%H:%M"),
        "exit_time": exit_time.strftime("%H:%M"),
        "strike_offset": strike_offset,
        "lot_size": lot_size,
        "trade_rows": len(results["trades"]),
        "skipped_expiries": len(results["skipped_expiries"]),
        "daily_mtm_rows": len(results["daily_mtm"]),
    }
    save_run(run_id, metadata, results)
    return run_id, results


def parse_date(value: str) -> date:
    return datetime.strptime(value, "%Y-%m-%d").date()


def parse_time(value: str) -> time:
    return datetime.strptime(value, "%H:%M").time()


def run_backtest(strategy, config: StrategyConfig) -> dict[str, pd.DataFrame]:
    trades: list[pd.DataFrame] = []
    skipped_expiries: list[pd.DataFrame] = []

    for expiry in iter_monthly_expiries(config.start_date, config.end_date):
        result = strategy.run(config, expiry)
        trades.append(result["trades"])
        skipped_expiries.append(result["skipped_expiries"])

    return {
        "trades": _concat(trades),
        "skipped_expiries": _concat(skipped_expiries),
    }


def _concat(frames: list[pd.DataFrame]) -> pd.DataFrame:
    non_empty = [frame for frame in frames if not frame.empty]
    return pd.concat(non_empty, ignore_index=True) if non_empty else pd.DataFrame()
