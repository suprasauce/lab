"""Ingest one expiry's contracts and 5-minute candles, independently of underlying ingestion."""

import calendar
from contextlib import closing
from datetime import date, datetime, time, timedelta
import hashlib
import logging
from pathlib import Path
import sqlite3

from breeze_client import BreezeClient, MARKET_ZONE
from database import (
    connect, ensure_option_contracts, get_option_expiry_id,
    get_underlying_id, get_underlying_range, insert_option_candles,
)
from pipelines.underlying import BATCH_SIZE, REQUEST_WINDOW, candle_row


logger = logging.getLogger(__name__)
PROGRESS_PATH = Path(__file__).resolve().parents[1] / ".state" / "options.sqlite3"
BREEZE_CODES = {"NIFTY": "NIFTY", "BANKNIFTY": "CNXBAN"}


def input_dates(expiry, start_date, end_date):
    expiry = date.fromisoformat(expiry)
    start = date.fromisoformat(start_date)
    end = date.fromisoformat(end_date)
    if start > end:
        raise ValueError("start_date must not be after end_date")
    year, month = (start.year + 1, 1) if start.month == 12 else (start.year, start.month + 1)
    month_stop = date(year, month, min(start.day, calendar.monthrange(year, month)[1]))
    if end + timedelta(days=1) > month_stop:
        raise ValueError("Inclusive date range must not exceed one calendar month")
    return expiry, datetime.combine(start, time.min, MARKET_ZONE), datetime.combine(end + timedelta(days=1), time.min, MARKET_ZONE)


def strike_bounds(low, high, interval, padding):
    if type(interval) is not int or interval <= 0:
        raise ValueError("Strike interval must be positive integer paise")
    if type(padding) is not int or padding < 0:
        raise ValueError("Strike padding must be a nonnegative integer number of intervals")
    lower = (low // interval - padding) * interval
    upper = ((high + interval - 1) // interval + padding) * interval
    if low < 0 or high < low or lower < 0 or upper > 2**63 - 1:
        raise ValueError("Expanded strike range is outside valid nonnegative BIGINT prices")
    return lower, upper


def ingest_options(underlying, expiry, start_date, end_date, strike_interval_paise, strike_padding):
    expiry_date, start, stop = input_dates(expiry, start_date, end_date)
    underlying = underlying.upper()
    if underlying not in BREEZE_CODES:
        raise ValueError(f"No verified NFO Breeze stock-code mapping for {underlying}")
    interval = strike_interval_paise
    if type(interval) is not int or interval <= 0:
        raise ValueError("strike_interval_paise must be a positive integer")
    if type(strike_padding) is not int or strike_padding < 0:
        raise ValueError("strike_padding must be a nonnegative integer")

    PROGRESS_PATH.parent.mkdir(parents=True, exist_ok=True)
    fetched = inserted = skipped = successful_calls = 0
    with connect() as connection, closing(sqlite3.connect(PROGRESS_PATH)) as progress:
        # Scope checkpoints to the target database without persisting credentials.
        identity = f"{connection.info.host}:{connection.info.port}/{connection.info.dbname}"
        scope = hashlib.sha256(identity.encode()).hexdigest()
        progress.execute("""CREATE TABLE IF NOT EXISTS completed_requests (
            scope TEXT NOT NULL, contract_id INTEGER NOT NULL,
            start_time TEXT NOT NULL, stop_time TEXT NOT NULL, timeframe TEXT NOT NULL,
            PRIMARY KEY (scope, contract_id, start_time, stop_time, timeframe))""")
        progress.commit()

        underlying_id = get_underlying_id(connection, underlying)
        expiry_id = get_option_expiry_id(connection, underlying_id, expiry_date)
        low, high = get_underlying_range(connection, underlying_id, start, stop)
        lower, upper = strike_bounds(low, high, interval, strike_padding)
        connection.commit()
        logger.info("underlying=%s expiry=%s low_paise=%d high_paise=%d lower_strike_paise=%d upper_strike_paise=%d interval_paise=%d padding_intervals=%d generated_contracts=%d",
                    underlying, expiry_date, low, high, lower, upper, interval, strike_padding, ((upper - lower) // interval + 1) * 2)
        with connection.transaction():
            contracts = ensure_option_contracts(connection, underlying_id, expiry_id, lower, upper, interval)

        client = None  # An already completed rerun does not need Breeze authentication.
        for index, (contract_id, strike, option_type) in enumerate(contracts, 1):
            current = start
            while current < stop:
                window_stop = min(current + REQUEST_WINDOW, stop)
                key = (scope, contract_id, current.isoformat(), window_stop.isoformat(), "5m")
                if progress.execute("""SELECT 1 FROM completed_requests WHERE
                    scope=? AND contract_id=? AND start_time=? AND stop_time=? AND timeframe=?""", key).fetchone():
                    skipped += 1
                    current = window_stop
                    continue
                if client is None:
                    client = BreezeClient()
                request_time = datetime.now(MARKET_ZONE)
                try:
                    candles = client.historical_option(
                        BREEZE_CODES[underlying], expiry_date, strike, option_type,
                        current, window_stop - timedelta(seconds=1),
                    )
                    rows = [candle_row(candle, contract_id, "5m") for candle in candles]
                    if any(not current <= row[1] < window_stop for row in rows):
                        raise ValueError("Breeze returned option candles outside the requested date window")
                    rows.sort(key=lambda row: row[1])
                    batch_inserted = 0
                    with connection.transaction():
                        for offset in range(0, len(rows), BATCH_SIZE):
                            batch_inserted += insert_option_candles(connection, rows[offset:offset + BATCH_SIZE])
                    # Only checkpoint closed historical windows, after PostgreSQL
                    # commit. A crash before this marker safely causes a refetch.
                    if window_stop <= request_time - timedelta(minutes=5):
                        progress.execute("INSERT OR IGNORE INTO completed_requests VALUES (?, ?, ?, ?, ?)", key)
                        progress.commit()
                except Exception as exc:
                    # Preserve context without printing SDK/driver exception text.
                    reason = str(exc) if isinstance(exc, (ValueError, RuntimeError)) else type(exc).__name__
                    raise RuntimeError(
                        f"Options ingestion failed for contract={contract_id}, strike_paise={strike}, "
                        f"right={option_type}, start={current.isoformat()}, stop={window_stop.isoformat()} "
                        f"({reason}); completed windows remain safe to rerun"
                    ) from exc
                fetched += len(rows)
                inserted += batch_inserted
                successful_calls += 1
                logger.info("contract=%d progress=%d/%d strike_paise=%d right=%s start=%s stop=%s api_success=true fetched=%d inserted=%d",
                            contract_id, index, len(contracts), strike, option_type, current.isoformat(), window_stop.isoformat(), len(rows), batch_inserted)
                current = window_stop
    result = dict(contracts=len(contracts), successful_calls=successful_calls, skipped_windows=skipped, fetched=fetched, inserted=inserted)
    logger.info("Options ingestion complete: %s", result)
    return result
