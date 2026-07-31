"""Mark-to-market builders."""

from __future__ import annotations

import logging
from datetime import date, datetime, time

import pandas as pd

from backend.common.utils import bar_end_time, bar_start_for_end_time
from backend.services import market_data_service

logger = logging.getLogger(__name__)


def build_daily_mtm(
    trades: pd.DataFrame,
) -> pd.DataFrame:
    if trades.empty:
        return pd.DataFrame(columns=_daily_mtm_columns())

    logger.info("MTM calculation started trades=%s expiries=%s", len(trades), trades["expiry_date"].nunique())
    rows: list[dict] = []
    for expiry, expiry_trades in trades.groupby("expiry_date", sort=True):
        logger.info("MTM expiry started expiry=%s legs=%s", expiry, len(expiry_trades))
        before = len(rows)
        for _, trade in expiry_trades.iterrows():
            rows.extend(_build_leg_5m_mtm(trade))
        logger.info("MTM expiry completed expiry=%s rows=%s", expiry, len(rows) - before)
    logger.info("MTM calculation completed rows=%s", len(rows))
    return pd.DataFrame(rows, columns=_daily_mtm_columns())


def _build_leg_5m_mtm(trade: pd.Series) -> list[dict]:
    entry_date = _parse_date(trade["entry_date"])
    exit_date = _parse_date(trade["exit_date"])
    expiry = _parse_date(trade["expiry_date"])
    entry_time = _parse_time(trade["entry_time"])
    exit_time = _parse_time(trade["exit_time"])
    candles = market_data_service.get_option_5m_range(
        symbol="NIFTY",
        exchange="NFO",
        expiry=expiry,
        strike=int(trade["strike"]),
        right=str(trade["option_type"]),
        start=entry_date,
        end=exit_date,
    )
    candle_closes = _candle_closes(candles)
    entry_bar_start = bar_start_for_end_time(entry_date, entry_time)
    exit_bar_start = bar_start_for_end_time(exit_date, exit_time)

    rows = []
    for candle_time, current_price in candle_closes:
        if candle_time < entry_bar_start or candle_time > exit_bar_start:
            continue
        rows.append(_mtm_row(trade, candle_time.date(), bar_end_time(candle_time), current_price))
    return rows


def _mtm_row(trade: pd.Series, mtm_date: date, mtm_time: time, current_price: float | None) -> dict:
    entry_price = float(trade["entry_price"])
    lot_size = int(trade["lot_size"])
    side = str(trade.get("side", "sell")).lower()
    if current_price is None:
        mtm = None
    elif side == "buy":
        mtm = round((current_price - entry_price) * lot_size, 2)
    else:
        mtm = round((entry_price - current_price) * lot_size, 2)
    return {
        "trade_id": trade["trade_id"],
        "expiry_date": trade["expiry_date"],
        "mtm_date": mtm_date.isoformat(),
        "mtm_time": mtm_time.strftime("%H:%M"),
        "leg_role": trade["leg_role"],
        "option_type": trade["option_type"],
        "side": side,
        "strike": int(trade["strike"]),
        "entry_price": round(entry_price, 2),
        "current_price": None if current_price is None else round(current_price, 2),
        "lot_size": lot_size,
        "mtm": mtm,
        "reason": "price_missing" if current_price is None else "ok",
    }


def _candle_closes(candles: pd.DataFrame) -> list[tuple[datetime, float]]:
    if candles is None or candles.empty or "datetime" not in candles.columns or "close" not in candles.columns:
        return []

    df = candles[["datetime", "close"]].copy()
    df["datetime"] = pd.to_datetime(df["datetime"], errors="coerce")
    if getattr(df["datetime"].dt, "tz", None) is not None:
        df["datetime"] = df["datetime"].dt.tz_localize(None)
    df["close"] = pd.to_numeric(df["close"], errors="coerce")
    df = df.dropna(subset=["datetime", "close"])
    df = df[df["close"] > 0]
    df = df.sort_values("datetime")
    return list(zip(df["datetime"].dt.to_pydatetime(), df["close"].astype(float)))


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
        "side",
        "strike",
        "entry_price",
        "current_price",
        "lot_size",
        "mtm",
        "reason",
    ]
