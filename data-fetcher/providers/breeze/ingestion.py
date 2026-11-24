"""Shared candle helpers and synchronous ingestion, fixed at 1-minute candles."""

import logging
from datetime import datetime, time, timedelta
from decimal import InvalidOperation

import database
from candles import BATCH_SIZE, calendar_date, candle_row, to_paise
from .client import MARKET_ZONE, get_client

logger = logging.getLogger(__name__)
MAX_DTE = 45
REQUEST_WINDOW = timedelta(days=2)
BREEZE_CODES = {"NIFTY": "NIFTY", "BANKNIFTY": "CNXBAN"}


def stock_code(underlying):
    if not isinstance(underlying, str) or underlying not in BREEZE_CODES:
        raise ValueError("Supported underlying symbols are NIFTY and BANKNIFTY")
    return BREEZE_CODES[underlying]


def fetch_candles(start, end, request_window):
    """Collect and validate all windows before any write transaction starts."""
    current = datetime.combine(start, time.min, MARKET_ZONE)
    stop = datetime.combine(end + timedelta(days=1), time.min, MARKET_ZONE)
    rows = {}
    while current < stop:
        window_stop = min(current + REQUEST_WINDOW, stop)
        try:
            candles = request_window(current, window_stop - timedelta(seconds=1))
        except RuntimeError as exc:
            raise RuntimeError(f"Historical window {current.isoformat()} through {(window_stop - timedelta(seconds=1)).isoformat()}: {exc}") from exc
        for candle in candles:
            try:
                row = candle_row(candle)
            except (KeyError, TypeError, ValueError, InvalidOperation) as exc:
                raise RuntimeError(f"Breeze returned invalid candle data in window {current.isoformat()} through {window_stop.isoformat()}") from exc
            if not current <= row[0] < window_stop:
                raise RuntimeError(f"Breeze returned a candle outside the requested local-time window {current.isoformat()} through {window_stop.isoformat()}")
            rows[row[0]] = row
        current = window_stop
    return [rows[timestamp] for timestamp in sorted(rows)]


def fetch_underlying(underlying, start_date, end_date):
    symbol = stock_code(underlying)
    start, end = calendar_date(start_date), calendar_date(end_date)
    if start > end:
        raise ValueError("start_date must not be after end_date")
    with database.connect() as connection:
        underlying_id = database.get_underlying_id(connection, underlying)
    client = get_client()
    rows = fetch_candles(start, end, lambda first, last: client.historical_underlying(symbol, first, last))
    inserted = 0
    if rows:
        with database.connect() as connection:
            with connection.transaction():
                for offset in range(0, len(rows), BATCH_SIZE):
                    batch = [(underlying_id, *row) for row in rows[offset:offset + BATCH_SIZE]]
                    inserted += database.insert_underlying_candles(connection, batch)
    logger.info("underlying=%s fetched=%d inserted=%d", underlying, len(rows), inserted)
    return {"status": "ingested", "fetched": len(rows), "inserted": inserted}
