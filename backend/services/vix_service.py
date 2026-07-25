"""India VIX curve builder backed by cached 5-minute market data."""

from __future__ import annotations

import logging
from datetime import date, datetime, time
from typing import Any

from backend.services import market_data_service

logger = logging.getLogger(__name__)

VIX_SYMBOL = "INDVIX"
VIX_EXCHANGE = "NSE"
VIX_CANDLE_TIME = time(15, 30)


def build_vix_curve(
    *,
    equity_curve: list[dict[str, Any]],
    trade_metrics: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    rows = []
    for curve_date in _curve_dates(equity_curve=equity_curve, trade_metrics=trade_metrics):
        try:
            candle = market_data_service.get_underlying_candle(
                symbol=VIX_SYMBOL,
                exchange=VIX_EXCHANGE,
                candle_date=curve_date,
                candle_time=VIX_CANDLE_TIME,
            )
        except Exception:
            logger.exception("India VIX fetch failed for %s", curve_date)
            continue

        close = _close(candle)
        if close is not None:
            rows.append({"date": curve_date.isoformat(), "vix": round(close, 2)})
    return rows


def _curve_dates(
    *,
    equity_curve: list[dict[str, Any]],
    trade_metrics: list[dict[str, Any]],
) -> list[date]:
    values = set()
    for row in equity_curve:
        parsed = _parse_date(row.get("date"))
        if parsed:
            values.add(parsed)
    for row in trade_metrics:
        parsed = _parse_date(row.get("expiry_date"))
        if parsed:
            values.add(parsed)
    return sorted(values)


def _parse_date(value: Any) -> date | None:
    if isinstance(value, date):
        return value
    if not value:
        return None
    try:
        return datetime.strptime(str(value), "%Y-%m-%d").date()
    except ValueError:
        return None


def _close(candle: dict | None) -> float | None:
    if not candle:
        return None
    try:
        return float(candle["close"])
    except (KeyError, TypeError, ValueError):
        return None
