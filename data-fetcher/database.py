"""SQL against the existing lab schema; callers own write transactions."""

import os

import psycopg


def connect():
    dsn = os.environ.get("DATABASE_URL")
    if not dsn:
        raise RuntimeError("DATABASE_URL is required in the OS environment")
    # Metadata reads must not open a transaction while Breeze is being called.
    return psycopg.connect(dsn, connect_timeout=10, autocommit=True)


def get_underlying_id(connection, symbol):
    with connection.cursor() as cursor:
        cursor.execute("SELECT id FROM lab.underlyings WHERE symbol = %s", (symbol,))
        row = cursor.fetchone()
    if row is None:
        raise ValueError(f"Underlying {symbol!r} does not exist in lab.underlyings")
    return row[0]


def get_option_expiry_id(connection, underlying_id, expiry):
    with connection.cursor() as cursor:
        cursor.execute(
            "SELECT id FROM lab.option_expiry WHERE underlying_id = %s AND expiry_date = %s",
            (underlying_id, expiry),
        )
        row = cursor.fetchone()
    if row is None:
        raise ValueError(f"Expiry {expiry} must already exist for this underlying in lab.option_expiry")
    return row[0]


def get_option_contract_id(connection, underlying_id, expiry_id, strike, option_type):
    with connection.cursor() as cursor:
        cursor.execute(
            """SELECT id FROM lab.option_contracts
               WHERE underlying_id = %s AND expiry_id = %s AND strike_price = %s AND option_type = %s""",
            (underlying_id, expiry_id, strike, option_type),
        )
        row = cursor.fetchone()
    return row[0] if row else None


def insert_option_contract(connection, underlying_id, expiry_id, strike, option_type):
    with connection.cursor() as cursor:
        cursor.execute(
            """INSERT INTO lab.option_contracts (underlying_id, expiry_id, strike_price, option_type)
               VALUES (%s, %s, %s, %s)
               ON CONFLICT (expiry_id, strike_price, option_type) DO NOTHING
               RETURNING id""",
            (underlying_id, expiry_id, strike, option_type),
        )
        row = cursor.fetchone()
    return row[0] if row else None


def insert_option_candles(connection, rows):
    with connection.cursor() as cursor:
        cursor.executemany(
            """INSERT INTO lab.option_candles
                   (option_contract_id, start_time, timeframe, open, high, low, close, volume)
               VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
               ON CONFLICT (option_contract_id, timeframe, start_time) DO NOTHING""", rows,
        )
        return cursor.rowcount


def insert_underlying_candles(connection, rows):
    with connection.cursor() as cursor:
        cursor.executemany(
            """INSERT INTO lab.underlying_candles
                   (underlying_id, start_time, timeframe, open, high, low, close, volume)
               VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
               ON CONFLICT (underlying_id, timeframe, start_time) DO NOTHING""", rows,
        )
        return cursor.rowcount
