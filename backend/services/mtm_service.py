"""Daily mark-to-market builders."""

from __future__ import annotations

from datetime import date, datetime, time

import pandas as pd

from backend.common.nse_calendar import trading_days_between

DAILY_MTM_TIME = time(15, 30)


def build_daily_mtm(trades: pd.DataFrame, data) -> pd.DataFrame:
    if trades.empty:
        return pd.DataFrame(columns=_daily_mtm_columns())

    rows: list[dict] = []
    for _, trade in trades.iterrows():
        rows.extend(_build_leg_daily_mtm(trade, data))
    return pd.DataFrame(rows, columns=_daily_mtm_columns())


def _build_leg_daily_mtm(trade: pd.Series, data) -> list[dict]:
    entry_date = _parse_date(trade["entry_date"])
    exit_date = _parse_date(trade["exit_date"])
    expiry = _parse_date(trade["expiry_date"])
    entry_time = _parse_time(trade["entry_time"])
    exit_time = _parse_time(trade["exit_time"])

    rows = []
    for mtm_date in trading_days_between(entry_date, exit_date):
        mtm_time = _mtm_time_for_day(
            mtm_date=mtm_date,
            entry_date=entry_date,
            exit_date=exit_date,
            entry_time=entry_time,
            exit_time=exit_time,
        )
        candle = data.get_option_candle(
            symbol="NIFTY",
            exchange="NFO",
            expiry=expiry,
            strike=int(trade["strike"]),
            right=str(trade["option_type"]),
            candle_date=mtm_date,
            candle_time=mtm_time,
        )
        current_price = _close(candle)
        rows.append(_mtm_row(trade, mtm_date, mtm_time, current_price))
    return rows


def _mtm_time_for_day(
    *,
    mtm_date: date,
    entry_date: date,
    exit_date: date,
    entry_time: time,
    exit_time: time,
) -> time:
    if mtm_date == entry_date:
        return entry_time
    if mtm_date == exit_date:
        return exit_time
    return DAILY_MTM_TIME


def _mtm_row(trade: pd.Series, mtm_date: date, mtm_time: time, current_price: float | None) -> dict:
    entry_price = float(trade["entry_price"])
    lot_size = int(trade["lot_size"])
    mtm = None if current_price is None else round((entry_price - current_price) * lot_size, 2)
    return {
        "trade_id": trade["trade_id"],
        "expiry_date": trade["expiry_date"],
        "mtm_date": mtm_date.isoformat(),
        "mtm_time": mtm_time.strftime("%H:%M"),
        "leg_role": trade["leg_role"],
        "option_type": trade["option_type"],
        "strike": int(trade["strike"]),
        "entry_price": round(entry_price, 2),
        "current_price": None if current_price is None else round(current_price, 2),
        "lot_size": lot_size,
        "mtm": mtm,
        "reason": "price_missing" if current_price is None else "ok",
    }


def _close(candle: dict | None) -> float | None:
    if candle is None or candle.get("close") is None:
        return None
    close = float(candle["close"])
    return close if close > 0 else None


def _parse_date(value) -> date:
    if isinstance(value, date):
        return value
    return datetime.strptime(str(value), "%Y-%m-%d").date()


def _parse_time(value) -> time:
    if isinstance(value, time):
        return value
    return datetime.strptime(str(value), "%H:%M").time()


def _daily_mtm_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "mtm_date",
        "mtm_time",
        "leg_role",
        "option_type",
        "strike",
        "entry_price",
        "current_price",
        "lot_size",
        "mtm",
        "reason",
    ]
