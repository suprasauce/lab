"""Two synchronous ingestion operations, fixed at 5-minute candles."""

import logging
from datetime import date, datetime, time, timedelta
from decimal import Decimal, InvalidOperation

import database
from breeze_client import MARKET_ZONE, get_client

logger = logging.getLogger(__name__)
MAX_DTE = 45
REQUEST_WINDOW = timedelta(days=13)
BATCH_SIZE = 500
BREEZE_CODES = {"NIFTY": "NIFTY", "BANKNIFTY": "CNXBAN"}


def to_paise(value):
    try:
        paise = Decimal(str(value)) * 100
        if not paise.is_finite() or paise < 0 or paise != paise.to_integral_value():
            raise ValueError("Prices must be nonnegative with at most two decimals")
        result = int(paise)
        if result > 2**63 - 1:
            raise ValueError("Price exceeds PostgreSQL BIGINT")
        return result
    except InvalidOperation as exc:
        raise ValueError("Invalid price") from exc


def calendar_date(value):
    if not isinstance(value, str):
        raise ValueError("Dates must be YYYY-MM-DD strings")
    parsed = date.fromisoformat(value)
    if parsed.isoformat() != value:
        raise ValueError("Dates must use YYYY-MM-DD format")
    return parsed


def stock_code(underlying):
    if not isinstance(underlying, str) or underlying not in BREEZE_CODES:
        raise ValueError("Supported underlying symbols are NIFTY and BANKNIFTY")
    return BREEZE_CODES[underlying]


def candle_row(candle):
    timestamp = datetime.fromisoformat(candle["datetime"])
    if timestamp.tzinfo is None:
        timestamp = timestamp.replace(tzinfo=MARKET_ZONE)
    else:
        timestamp = timestamp.astimezone(MARKET_ZONE)
    volume = Decimal(str(candle["volume"]))
    if not volume.is_finite() or volume < 0 or volume != volume.to_integral_value() or volume > 2**63 - 1:
        raise ValueError("Candle volume must be a nonnegative BIGINT")
    return (timestamp, "5m", *(to_paise(candle[field]) for field in ("open", "high", "low", "close")), int(volume))


def fetch_candles(start, end, request_window):
    """Collect and validate all windows before any write transaction starts."""
    current = datetime.combine(start, time.min, MARKET_ZONE)
    stop = datetime.combine(end + timedelta(days=1), time.min, MARKET_ZONE)
    rows = {}
    while current < stop:
        window_stop = min(current + REQUEST_WINDOW, stop)
        candles = request_window(current, window_stop - timedelta(seconds=1))
        for candle in candles:
            try:
                row = candle_row(candle)
            except (KeyError, TypeError, ValueError, InvalidOperation) as exc:
                raise RuntimeError("Breeze returned invalid candle data") from exc
            if not current <= row[0] < window_stop:
                raise RuntimeError("Breeze returned a candle outside the requested local-time window")
            rows[row[0]] = row
        current = window_stop
    return [rows[timestamp] for timestamp in sorted(rows)]


def fetch_option(underlying, expiry, strike, option_type):
    symbol = stock_code(underlying)
    expiry_date = calendar_date(expiry)
    strike_paise = to_paise(strike)  # API strikes are rupees; PostgreSQL uses paise.
    if strike_paise == 0 or option_type not in ("CALL", "PUT"):
        raise ValueError("strike must be positive and option_type must be CALL or PUT")

    with database.connect() as connection:
        underlying_id = database.get_underlying_id(connection, underlying)
        expiry_id = database.get_option_expiry_id(connection, underlying_id, expiry_date)
        contract_id = database.get_option_contract_id(connection, underlying_id, expiry_id, strike_paise, option_type)
    if contract_id is not None:
        return {"status": "already_exists", "contract_id": contract_id, "fetched": 0, "inserted": 0}
    if expiry_date >= datetime.now(MARKET_ZONE).date():
        raise ValueError("New contracts must have an expiry before today so their historical data can be complete")

    client = get_client()
    rows = fetch_candles(
        expiry_date - timedelta(days=MAX_DTE), expiry_date,
        lambda start, end: client.historical_option(symbol, expiry_date, strike_paise, option_type, start, end),
    )
    if not rows:
        raise RuntimeError("Breeze returned no option candles; no contract was inserted")

    inserted = 0
    with database.connect() as connection:
        with connection.transaction():
            contract_id = database.insert_option_contract(connection, underlying_id, expiry_id, strike_paise, option_type)
            created = contract_id is not None
            if created:
                for offset in range(0, len(rows), BATCH_SIZE):
                    batch = [(contract_id, *row) for row in rows[offset:offset + BATCH_SIZE]]
                    inserted += database.insert_option_candles(connection, batch)
            else:
                # A concurrent request may have committed the complete contract
                # while we fetched. Its unique-key conflict waits for that commit.
                contract_id = database.get_option_contract_id(connection, underlying_id, expiry_id, strike_paise, option_type)
                if contract_id is None:
                    raise RuntimeError("Conflicting contract has an inconsistent underlying relationship")
    # Both transaction and connection have exited successfully before returning.
    logger.info("underlying=%s contract_id=%d fetched=%d inserted=%d", underlying, contract_id, len(rows), inserted)
    return {"status": "ingested" if created else "already_exists", "contract_id": contract_id, "fetched": len(rows), "inserted": inserted}


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
