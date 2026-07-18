"""DuckDB storage for 5-minute market data."""

from __future__ import annotations

from datetime import date, datetime, time
from pathlib import Path

import duckdb
import pandas as pd

from common.settings import DB_PATH
from common.utils import bar_start_for_end_time


class DuckDBMarketDataStore:
    def __init__(self, path: Path = DB_PATH):
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._init_schema()

    def _connect(self) -> duckdb.DuckDBPyConnection:
        return duckdb.connect(str(self.path))

    def _init_schema(self) -> None:
        with self._connect() as con:
            con.execute(
                """
                CREATE TABLE IF NOT EXISTS underlying_5m (
                    symbol TEXT NOT NULL,
                    exchange TEXT NOT NULL,
                    datetime TIMESTAMP NOT NULL,
                    open DOUBLE,
                    high DOUBLE,
                    low DOUBLE,
                    close DOUBLE,
                    volume DOUBLE,
                    created_at TIMESTAMP NOT NULL,
                    updated_at TIMESTAMP NOT NULL
                )
                """
            )
            con.execute(
                """
                CREATE TABLE IF NOT EXISTS derivatives_5m (
                    underlying_symbol TEXT NOT NULL,
                    exchange TEXT NOT NULL,
                    instrument_type TEXT NOT NULL,
                    expiry DATE NOT NULL,
                    strike INTEGER,
                    option_right TEXT,
                    datetime TIMESTAMP NOT NULL,
                    open DOUBLE,
                    high DOUBLE,
                    low DOUBLE,
                    close DOUBLE,
                    volume DOUBLE,
                    open_interest DOUBLE,
                    created_at TIMESTAMP NOT NULL,
                    updated_at TIMESTAMP NOT NULL
                )
                """
            )

    def load_underlying_5m(self, *, symbol: str, exchange: str, start: date, end: date) -> pd.DataFrame:
        with self._connect() as con:
            return con.execute(
                """
                SELECT datetime, open, high, low, close, volume
                FROM underlying_5m
                WHERE symbol = ?
                  AND exchange = ?
                  AND CAST(datetime AS DATE) BETWEEN ? AND ?
                ORDER BY datetime
                """,
                [symbol, exchange, start, end],
            ).df()

    def load_underlying_candle(self, *, symbol: str, exchange: str, candle_date: date, candle_time: time) -> dict | None:
        candle_start = bar_start_for_end_time(candle_date, candle_time)
        with self._connect() as con:
            df = con.execute(
                """
                SELECT datetime, open, high, low, close, volume
                FROM underlying_5m
                WHERE symbol = ?
                  AND exchange = ?
                  AND datetime = ?
                LIMIT 1
                """,
                [symbol, exchange, candle_start],
            ).df()
        return None if df.empty else df.iloc[0].to_dict()

    def load_derivative_5m(
        self,
        *,
        underlying_symbol: str,
        exchange: str,
        instrument_type: str,
        expiry: date,
        strike: int,
        right: str,
        start: date,
        end: date,
    ) -> pd.DataFrame:
        with self._connect() as con:
            return con.execute(
                """
                SELECT datetime, open, high, low, close, volume, open_interest
                FROM derivatives_5m
                WHERE underlying_symbol = ?
                  AND exchange = ?
                  AND instrument_type = ?
                  AND expiry = ?
                  AND strike = ?
                  AND option_right = ?
                  AND CAST(datetime AS DATE) BETWEEN ? AND ?
                ORDER BY datetime
                """,
                [underlying_symbol, exchange, instrument_type, expiry, strike, right.lower(), start, end],
            ).df()

    def load_derivative_candle(
        self,
        *,
        underlying_symbol: str,
        exchange: str,
        instrument_type: str,
        expiry: date,
        strike: int,
        right: str,
        candle_date: date,
        candle_time: time,
    ) -> dict | None:
        candle_start = bar_start_for_end_time(candle_date, candle_time)
        with self._connect() as con:
            df = con.execute(
                """
                SELECT datetime, open, high, low, close, volume, open_interest
                FROM derivatives_5m
                WHERE underlying_symbol = ?
                  AND exchange = ?
                  AND instrument_type = ?
                  AND expiry = ?
                  AND strike = ?
                  AND option_right = ?
                  AND datetime = ?
                LIMIT 1
                """,
                [underlying_symbol, exchange, instrument_type, expiry, strike, right.lower(), candle_start],
            ).df()
        return None if df.empty else df.iloc[0].to_dict()

    def save_underlying_5m(self, *, symbol: str, exchange: str, df: pd.DataFrame) -> None:
        if df.empty:
            return
        rows = _prepare_common_columns(df)
        now = datetime.now()
        rows["symbol"] = symbol
        rows["exchange"] = exchange
        rows["created_at"] = now
        rows["updated_at"] = now

        with self._connect() as con:
            con.register("incoming_underlying", rows)
            con.execute(
                """
                DELETE FROM underlying_5m
                USING incoming_underlying
                WHERE underlying_5m.symbol = incoming_underlying.symbol
                  AND underlying_5m.exchange = incoming_underlying.exchange
                  AND underlying_5m.datetime = incoming_underlying.datetime
                """
            )
            con.execute(
                """
                INSERT INTO underlying_5m
                SELECT symbol, exchange, datetime, open, high, low, close, volume, created_at, updated_at
                FROM incoming_underlying
                """
            )

    def save_derivative_5m(
        self,
        *,
        underlying_symbol: str,
        exchange: str,
        instrument_type: str,
        expiry: date,
        strike: int,
        right: str,
        df: pd.DataFrame,
    ) -> None:
        if df.empty:
            return
        rows = _prepare_common_columns(df)
        now = datetime.now()
        rows["underlying_symbol"] = underlying_symbol
        rows["exchange"] = exchange
        rows["instrument_type"] = instrument_type
        rows["expiry"] = expiry
        rows["strike"] = strike
        rows["option_right"] = right.lower()
        rows["created_at"] = now
        rows["updated_at"] = now

        with self._connect() as con:
            con.register("incoming_derivatives", rows)
            con.execute(
                """
                DELETE FROM derivatives_5m
                USING incoming_derivatives
                WHERE derivatives_5m.underlying_symbol = incoming_derivatives.underlying_symbol
                  AND derivatives_5m.exchange = incoming_derivatives.exchange
                  AND derivatives_5m.instrument_type = incoming_derivatives.instrument_type
                  AND derivatives_5m.expiry = incoming_derivatives.expiry
                  AND derivatives_5m.strike = incoming_derivatives.strike
                  AND derivatives_5m.option_right = incoming_derivatives.option_right
                  AND derivatives_5m.datetime = incoming_derivatives.datetime
                """
            )
            con.execute(
                """
                INSERT INTO derivatives_5m
                SELECT
                    underlying_symbol, exchange, instrument_type, expiry, strike, option_right,
                    datetime, open, high, low, close, volume, open_interest, created_at, updated_at
                FROM incoming_derivatives
                """
            )


def _prepare_common_columns(df: pd.DataFrame) -> pd.DataFrame:
    rows = df.copy()
    for col in ("open", "high", "low", "close", "volume", "open_interest"):
        if col not in rows.columns:
            rows[col] = None
    rows["datetime"] = pd.to_datetime(rows["datetime"]).dt.tz_localize(None)
    return rows
