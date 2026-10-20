"""Persistence against the Go application's existing lab schema."""

import os

import psycopg


def connect():
    dsn = os.environ.get("DATABASE_URL")
    if not dsn:
        raise ValueError("DATABASE_URL is required")
    return psycopg.connect(dsn, connect_timeout=10)


def get_underlying_id(connection, symbol):
    with connection.cursor() as cursor:
        cursor.execute("SELECT id FROM lab.underlyings WHERE symbol = %s", (symbol,))
        row = cursor.fetchone()
    if row is None:
        raise ValueError(f"Underlying {symbol!r} does not exist in lab.underlyings")
    return row[0]


def insert_candles(connection, rows):
    """Insert one bounded batch; return newly inserted rows, excluding duplicates."""
    if not rows:
        return 0
    with connection.cursor() as cursor:
        cursor.executemany(
            """
            INSERT INTO lab.underlying_candles
                (underlying_id, start_time, timeframe, open, high, low, close, volume)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (underlying_id, timeframe, start_time) DO NOTHING
            """,
            rows,
        )
        return cursor.rowcount


def get_option_expiry_id(connection, underlying_id, expiry):
    with connection.cursor() as cursor:
        cursor.execute(
            "SELECT id FROM lab.option_expiry WHERE underlying_id = %s AND expiry_date = %s",
            (underlying_id, expiry),
        )
        row = cursor.fetchone()
    if row is None:
        raise ValueError(f"Expiry {expiry} does not exist for underlying {underlying_id}")
    return row[0]


def get_underlying_range(connection, underlying_id, start, stop):
    with connection.cursor() as cursor:
        cursor.execute(
            """SELECT MIN(low), MAX(high) FROM lab.underlying_candles
               WHERE underlying_id = %s AND start_time >= %s AND start_time < %s""",
            (underlying_id, start, stop),
        )
        low, high = cursor.fetchone()
    if low is None or high is None:
        raise ValueError("Underlying candle data is unavailable for the requested range")
    return low, high


def ensure_option_contracts(connection, underlying_id, expiry_id, lower, upper, interval):
    with connection.cursor() as cursor:
        batch = []
        for strike in range(lower, upper + 1, interval):
            for option_type in ("CALL", "PUT"):
                batch.append((underlying_id, expiry_id, strike, option_type))
            if len(batch) >= 500:
                cursor.executemany(
                    """INSERT INTO lab.option_contracts (underlying_id, expiry_id, strike_price, option_type)
                       VALUES (%s, %s, %s, %s)
                       ON CONFLICT (expiry_id, strike_price, option_type) DO NOTHING""", batch,
                )
                batch.clear()
        if batch:
            cursor.executemany(
                """INSERT INTO lab.option_contracts (underlying_id, expiry_id, strike_price, option_type)
                   VALUES (%s, %s, %s, %s)
                   ON CONFLICT (expiry_id, strike_price, option_type) DO NOTHING""", batch,
            )
        cursor.execute(
            """SELECT id, strike_price, option_type FROM lab.option_contracts
               WHERE underlying_id = %s AND expiry_id = %s
                 AND strike_price BETWEEN %s AND %s AND MOD(strike_price, %s) = 0
               ORDER BY strike_price, option_type""",
            (underlying_id, expiry_id, lower, upper, interval),
        )
        contracts = cursor.fetchall()
    if len(contracts) != ((upper - lower) // interval + 1) * 2:
        raise ValueError("Could not resolve all generated contracts; check existing underlying/expiry relationships")
    return contracts


def insert_option_candles(connection, rows):
    if not rows:
        return 0
    with connection.cursor() as cursor:
        cursor.executemany(
            """INSERT INTO lab.option_candles
                   (option_contract_id, start_time, timeframe, open, high, low, close, volume)
               VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
               ON CONFLICT (option_contract_id, timeframe, start_time) DO NOTHING""", rows,
        )
        return cursor.rowcount
