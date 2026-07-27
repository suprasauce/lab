"""Request-scoped in-memory cache helpers."""

from __future__ import annotations

from datetime import date, time
from typing import Any

import pandas as pd

from backend.common.utils import bar_start_for_end_time
from backend.services import market_data_service


def create_context() -> dict[str, Any]:
    return {
        "underlying_candles": {},
        "option_candles": {},
        "option_ranges": {},
    }


def get_underlying_candle(
    context: dict | None,
    *,
    symbol: str,
    exchange: str,
    candle_date: date,
    candle_time: time,
) -> dict | None:
    if context is None:
        return market_data_service.get_underlying_candle(
            symbol=symbol,
            exchange=exchange,
            candle_date=candle_date,
            candle_time=candle_time,
        )

    cache = context.setdefault("underlying_candles", {})
    key = (symbol, exchange, candle_date, candle_time)
    if key not in cache:
        cache[key] = market_data_service.get_underlying_candle(
            symbol=symbol,
            exchange=exchange,
            candle_date=candle_date,
            candle_time=candle_time,
        )
    return cache[key]


def get_option_candle(
    context: dict | None,
    *,
    symbol: str,
    exchange: str,
    expiry: date,
    strike: int,
    right: str,
    candle_date: date,
    candle_time: time,
) -> dict | None:
    right = right.lower()
    if context is None:
        return market_data_service.get_option_candle(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            candle_date=candle_date,
            candle_time=candle_time,
        )

    cache = context.setdefault("option_candles", {})
    key = (symbol, exchange, expiry, strike, right, candle_date, candle_time)
    if key not in cache:
        cache[key] = _option_candle_from_ranges(
            context,
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            candle_date=candle_date,
            candle_time=candle_time,
        )
        if key not in cache or cache[key] is None:
            cache[key] = market_data_service.get_option_candle(
                symbol=symbol,
                exchange=exchange,
                expiry=expiry,
                strike=strike,
                right=right,
                candle_date=candle_date,
                candle_time=candle_time,
            )
    return cache[key]


def get_option_5m_range(
    context: dict | None,
    *,
    symbol: str,
    exchange: str,
    expiry: date,
    strike: int,
    right: str,
    start: date,
    end: date,
) -> pd.DataFrame:
    right = right.lower()
    if context is None:
        return market_data_service.get_option_5m_range(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            start=start,
            end=end,
        )

    cache = context.setdefault("option_ranges", {})
    key = (symbol, exchange, expiry, strike, right, start, end)
    if key not in cache:
        cache[key] = market_data_service.get_option_5m_range(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            start=start,
            end=end,
        )
    return cache[key]


def _option_candle_from_ranges(
    context: dict,
    *,
    symbol: str,
    exchange: str,
    expiry: date,
    strike: int,
    right: str,
    candle_date: date,
    candle_time: time,
) -> dict | None:
    candle_start = bar_start_for_end_time(candle_date, candle_time)
    for key, df in context.setdefault("option_ranges", {}).items():
        (
            cached_symbol,
            cached_exchange,
            cached_expiry,
            cached_strike,
            cached_right,
            cached_start,
            cached_end,
        ) = key
        if (
            cached_symbol != symbol
            or cached_exchange != exchange
            or cached_expiry != expiry
            or cached_strike != strike
            or cached_right != right
            or not (cached_start <= candle_date <= cached_end)
            or df.empty
        ):
            continue
        candle = _row_at_start(df, candle_start)
        if candle is not None:
            return candle
    return None


def _row_at_start(df: pd.DataFrame, candle_start) -> dict | None:
    for _, row in df.iterrows():
        value = pd.Timestamp(row["datetime"]).to_pydatetime().replace(tzinfo=None)
        if value == candle_start:
            return row.to_dict()
    return None
