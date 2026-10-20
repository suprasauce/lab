"""Fetch underlying OHLCV without resampling or synthesizing candles."""

import logging
from datetime import date, datetime, time, timedelta
from decimal import Decimal, InvalidOperation

from breeze_client import BreezeClient, MARKET_ZONE
from database import connect, get_underlying_id, insert_candles


logger = logging.getLogger(__name__)
TIMEFRAMES = {"1minute": "1m", "5minute": "5m", "1day": "1d"}
REQUEST_WINDOW = timedelta(hours=12)
BATCH_SIZE = 500


def to_paise(value):
    try:
        paise = Decimal(str(value)) * 100
        if not paise.is_finite() or paise < 0 or paise != paise.to_integral_value():
            raise ValueError("OHLC price must be nonnegative with at most two decimals")
        result = int(paise)
        if result > 2**63 - 1:
            raise ValueError("OHLC price exceeds PostgreSQL BIGINT")
        return result
    except InvalidOperation as exc:
        raise ValueError("Invalid OHLC price") from exc


def candle_row(candle, underlying_id, timeframe):
    timestamp = datetime.fromisoformat(candle["datetime"])
    if timestamp.tzinfo is None:
        timestamp = timestamp.replace(tzinfo=MARKET_ZONE)
    else:
        timestamp = timestamp.astimezone(MARKET_ZONE)
    volume = Decimal(str(candle["volume"]))
    if not volume.is_finite() or volume < 0 or volume != volume.to_integral_value() or volume > 2**63 - 1:
        raise ValueError("Candle volume must be a nonnegative BIGINT")
    return (
        underlying_id, timestamp, timeframe,
        *(to_paise(candle[field]) for field in ("open", "high", "low", "close")),
        int(volume),
    )


def fetch_underlying(underlying="NIFTY", start_date="2026-09-01", end_date="2026-09-30", timeframe="5minute"):
    if timeframe not in TIMEFRAMES:
        raise ValueError(f"Supported Breeze timeframes: {', '.join(TIMEFRAMES)}; 15minute is not provided by V2")
    if underlying != "NIFTY":
        raise ValueError("This initial index pipeline supports NIFTY only")
    start = date.fromisoformat(start_date)
    end = date.fromisoformat(end_date)
    if start > end:
        raise ValueError("start_date must not be after end_date")
    current = datetime.combine(start, time.min, MARKET_ZONE)
    stop = datetime.combine(end + timedelta(days=1), time.min, MARKET_ZONE)
    fetched = inserted = 0

    with connect() as connection:
        underlying_id = get_underlying_id(connection, underlying)
        connection.commit()  # Do not hold a database transaction during API calls.
        client = BreezeClient()
        while current < stop:
            window_size = timedelta(days=1) if timeframe == "1day" else REQUEST_WINDOW
            window_stop = min(current + window_size, stop)
            # Half-open local windows, expressed as inclusive second boundaries
            # to Breeze. At 1m this covers at most 720 intervals, below 1000.
            candles = client.historical_underlying(
                underlying, current, window_stop - timedelta(seconds=1), timeframe,
            )
            rows = []
            for candle in candles:
                row = candle_row(candle, underlying_id, TIMEFRAMES[timeframe])
                if not current <= row[1] < window_stop:
                    raise ValueError("Breeze returned a candle outside the requested local-time window")
                rows.append(row)
            rows.sort(key=lambda row: row[1])
            batch_inserted = 0
            with connection.transaction():
                for offset in range(0, len(rows), BATCH_SIZE):
                    batch_inserted += insert_candles(connection, rows[offset:offset + BATCH_SIZE])
            fetched += len(candles)
            inserted += batch_inserted
            logger.info("underlying=%s start=%s end=%s fetched=%d inserted=%d",
                        underlying, current.isoformat(), window_stop.isoformat(), len(candles), batch_inserted)
            current = window_stop

    logger.info("underlying=%s timeframe=%s fetched=%d inserted=%d", underlying, timeframe, fetched, inserted)
    if fetched == 0:
        logger.warning("No candles returned; no synthetic data was generated")
    return {"fetched": fetched, "inserted": inserted}
