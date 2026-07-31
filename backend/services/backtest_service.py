"""Generic multi-leg option backtest service."""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from datetime import date, datetime, time
import logging
from typing import Any

import pandas as pd

from backend.common.nse_calendar import entry_date_for_expiry, exit_date_for_expiry, iter_monthly_expiries
from backend.config.settings import lot_size_for_date
from backend.services import market_data_service
from backend.services.metrics_service import (
    build_average_mtm_by_expiry,
    build_backtest_metrics,
    build_equity_curve,
    build_expiry_pnl_curve,
    build_trade_metrics,
)
from backend.services.mtm_service import build_daily_mtm
from backend.services.result_service import save_run
from backend.services.strike_selection_service import StrikeSelectionContext, resolve_strike, round_atm
from backend.services.vix_service import build_vix_curve

logger = logging.getLogger(__name__)
MAX_EXPIRY_WORKERS = 3
CUSTOM_STRATEGY_ID = "custom_multi_leg"
CUSTOM_STRATEGY_NAME = "Custom Multi-Leg Strategy"


@dataclass(frozen=True)
class PositionConfig:
    leg_role: str
    option_type: str
    side: str
    quantity: int | None
    strike_selection: str
    strike_params: dict[str, float | None]
    entry_dte: int
    entry_time: time
    exit_dte: int
    exit_time: time


def list_strategies() -> list[dict]:
    return [
        {
            "id": CUSTOM_STRATEGY_ID,
            "name": CUSTOM_STRATEGY_NAME,
            "description": "Build a strategy by adding option legs with independent entry, exit, and strike rules.",
        }
    ]


def get_strategy(strategy_id: str) -> dict:
    strategy = list_strategies()[0]
    if strategy_id != CUSTOM_STRATEGY_ID:
        raise ValueError(f"Unknown strategy: {strategy_id}")
    return strategy


def run_backtest_for_strategy(
    *,
    strategy_id: str,
    start_date: date,
    end_date: date,
    positions: list[PositionConfig],
    include_mtm: bool = True,
) -> tuple[str, dict[str, Any]]:
    if not positions:
        raise ValueError("Add at least one leg.")

    expiries = iter_monthly_expiries(start_date, end_date)
    workers = min(MAX_EXPIRY_WORKERS, len(expiries)) if expiries else 0
    logger.info(
        "Backtest started strategy=%s start=%s end=%s expiries=%s workers=%s legs=%s",
        strategy_id,
        start_date,
        end_date,
        len(expiries),
        workers,
        len(positions),
    )

    results = run_backtest(start_date=start_date, end_date=end_date, positions=positions)
    if include_mtm:
        results["daily_mtm"] = build_daily_mtm(results["trades"])
    else:
        logger.info("MTM calculation skipped")
        results["daily_mtm"] = pd.DataFrame()

    results["metrics"] = build_backtest_metrics(
        trades=results["trades"],
        skipped_expiries=results["skipped_expiries"],
        daily_mtm=results["daily_mtm"],
        include_mtm=include_mtm,
    )
    results["equity_curve"] = build_equity_curve(results["trades"])
    results["expiry_pnl_curve"] = build_expiry_pnl_curve(results["trades"])
    results["average_mtm_by_expiry"] = (
        build_average_mtm_by_expiry(trades=results["trades"], daily_mtm=results["daily_mtm"])
        if include_mtm
        else []
    )
    results["trade_metrics"] = (
        build_trade_metrics(trades=results["trades"], daily_mtm=results["daily_mtm"])
        if include_mtm
        else []
    )
    results["vix_curve"] = (
        build_vix_curve(
            equity_curve=results["equity_curve"],
            trade_metrics=results["trade_metrics"],
        )
        if include_mtm
        else []
    )

    run_id = datetime.now().strftime("%Y%m%d_%H%M%S")
    metadata = {
        "run_id": run_id,
        "strategy_id": CUSTOM_STRATEGY_ID,
        "strategy_name": CUSTOM_STRATEGY_NAME,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "start_date": start_date.isoformat(),
        "end_date": end_date.isoformat(),
        "include_mtm": include_mtm,
        "positions": [_position_metadata(position) for position in positions],
        "trade_rows": len(results["trades"]),
        "skipped_expiries": len(results["skipped_expiries"]),
        "daily_mtm_rows": len(results["daily_mtm"]),
    }
    save_run(run_id, metadata, results)
    logger.info("Backtest completed run_id=%s", run_id)
    return run_id, results


def parse_date(value: str) -> date:
    return datetime.strptime(value, "%Y-%m-%d").date()


def parse_time(value: str) -> time:
    return datetime.strptime(value, "%H:%M").time()


def run_backtest(
    *,
    start_date: date,
    end_date: date,
    positions: list[PositionConfig],
) -> dict[str, pd.DataFrame]:
    expiries = iter_monthly_expiries(start_date, end_date)
    if not expiries:
        return {
            "trades": pd.DataFrame(columns=_trade_columns()),
            "skipped_expiries": pd.DataFrame(columns=_skipped_columns()),
        }

    results = []
    workers = min(MAX_EXPIRY_WORKERS, len(expiries))
    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = {
            executor.submit(_run_expiry, expiry, positions): expiry
            for expiry in expiries
        }
        for future in as_completed(futures):
            expiry = futures[future]
            try:
                results.append((expiry, future.result()))
                logger.info("Expiry completed expiry=%s", expiry)
            except Exception:
                logger.exception("Expiry failed expiry=%s", expiry)
                raise

    results.sort(key=lambda item: item[0])
    return {
        "trades": _concat([result["trades"] for _, result in results], _trade_columns()),
        "skipped_expiries": _concat(
            [result["skipped_expiries"] for _, result in results],
            _skipped_columns(),
        ),
    }


def _run_expiry(expiry: date, positions: list[PositionConfig]) -> dict[str, pd.DataFrame]:
    logger.info("Expiry started expiry=%s", expiry)
    trades = []
    skipped = []
    for position in positions:
        result = _run_position(expiry, position)
        if result["trade"] is not None:
            trades.append(result["trade"])
        else:
            skipped.append(result["skipped"])
    return {
        "trades": pd.DataFrame(trades, columns=_trade_columns()),
        "skipped_expiries": pd.DataFrame(skipped, columns=_skipped_columns()),
    }


def _run_position(expiry: date, position: PositionConfig) -> dict[str, dict | None]:
    entry_date = entry_date_for_expiry(expiry, position.entry_dte)
    exit_date = exit_date_for_expiry(expiry, position.exit_dte)
    base = _base_row(position, expiry, entry_date, exit_date)
    if entry_date >= expiry:
        return _skipped(base, "entry_on_or_after_expiry")
    if exit_date < entry_date:
        return _skipped(base, "exit_before_entry")

    spot = _close(
        market_data_service.get_underlying_candle(
            symbol="NIFTY",
            exchange="NSE",
            candle_date=entry_date,
            candle_time=position.entry_time,
        )
    )
    if spot is None:
        return _skipped(base, "spot_no_price")

    atm = round_atm(spot)
    strike = resolve_strike(
        StrikeSelectionContext(
            option_type=position.option_type,
            expiry=expiry,
            entry_date=entry_date,
            entry_time=position.entry_time,
            spot=spot,
            atm=atm,
            method=position.strike_selection,
            params=position.strike_params,
        )
    )
    if strike is None:
        return _skipped(base | {"spot_at_entry": round(spot, 2), "atm_strike": atm}, "strike_selection_failed")

    entry_price = _close(
        market_data_service.get_option_candle(
            symbol="NIFTY",
            exchange="NFO",
            expiry=expiry,
            strike=int(strike["strike"]),
            right=position.option_type,
            candle_date=entry_date,
            candle_time=position.entry_time,
        )
    )
    if entry_price is None:
        return _skipped(base | {"spot_at_entry": round(spot, 2), "atm_strike": atm}, "entry_price_missing")

    exit_price = _close(
        market_data_service.get_option_candle(
            symbol="NIFTY",
            exchange="NFO",
            expiry=expiry,
            strike=int(strike["strike"]),
            right=position.option_type,
            candle_date=exit_date,
            candle_time=position.exit_time,
        )
    )
    if exit_price is None:
        return _skipped(base | {"spot_at_entry": round(spot, 2), "atm_strike": atm}, "exit_price_missing")

    quantity = position.quantity if position.quantity is not None else lot_size_for_date(entry_date)
    trade = base | {
        "option_type": position.option_type,
        "side": position.side,
        "strike": int(strike["strike"]),
        "entry_price": round(entry_price, 2),
        "exit_price": round(exit_price, 2),
        "lot_size": quantity,
        "exit_reason": "scheduled_exit",
        "entry_delta": _round_or_none(strike.get("delta"), 4),
        "entry_iv": _round_or_none(strike.get("iv"), 4),
        "spot_at_entry": round(spot, 2),
        "atm_strike": atm,
        "strike_selection": position.strike_selection,
        "strike_params": _format_strike_params(position.strike_params),
    }
    return {"trade": trade, "skipped": None}


def _close(candle: dict | None) -> float | None:
    if candle is None or candle.get("close") is None:
        return None
    close = float(candle["close"])
    return close if close > 0 else None


def _base_row(position: PositionConfig, expiry: date, entry_date: date, exit_date: date) -> dict:
    return {
        "trade_id": f"{CUSTOM_STRATEGY_ID}_{expiry.isoformat()}",
        "expiry_date": expiry.isoformat(),
        "entry_date": entry_date.isoformat(),
        "exit_date": exit_date.isoformat(),
        "leg_role": position.leg_role,
        "strike_selection": position.strike_selection,
        "strike_params": _format_strike_params(position.strike_params),
        "entry_time": position.entry_time.strftime("%H:%M"),
        "exit_time": position.exit_time.strftime("%H:%M"),
    }


def _skipped(base: dict, reason: str) -> dict[str, dict | None]:
    row = {column: base.get(column) for column in _skipped_columns()}
    row["reason"] = reason
    return {"trade": None, "skipped": row}


def _position_metadata(position: PositionConfig) -> dict:
    return {
        "leg_role": position.leg_role,
        "option_type": position.option_type,
        "side": position.side,
        "quantity": position.quantity,
        "strike_selection": position.strike_selection,
        "strike_params": position.strike_params,
        "entry_dte": position.entry_dte,
        "entry_time": position.entry_time.strftime("%H:%M"),
        "exit_dte": position.exit_dte,
        "exit_time": position.exit_time.strftime("%H:%M"),
    }


def _round_or_none(value: Any, digits: int) -> float | None:
    return None if value is None else round(float(value), digits)


def _format_strike_params(params: dict[str, float | None]) -> str:
    values = [f"{key}={value}" for key, value in params.items() if value is not None]
    return ", ".join(values)


def _trade_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "entry_date",
        "exit_date",
        "leg_role",
        "option_type",
        "side",
        "strike",
        "entry_price",
        "exit_price",
        "lot_size",
        "exit_reason",
        "entry_delta",
        "entry_iv",
        "spot_at_entry",
        "atm_strike",
        "strike_selection",
        "strike_params",
        "entry_time",
        "exit_time",
    ]


def _skipped_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "entry_date",
        "exit_date",
        "leg_role",
        "reason",
        "spot_at_entry",
        "atm_strike",
        "strike_selection",
        "strike_params",
        "entry_time",
        "exit_time",
    ]


def _concat(frames: list[pd.DataFrame], columns: list[str]) -> pd.DataFrame:
    non_empty = [frame for frame in frames if not frame.empty]
    return pd.concat(non_empty, ignore_index=True) if non_empty else pd.DataFrame(columns=columns)
