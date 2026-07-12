"""Fetch and cache Breeze 5-minute data."""

from __future__ import annotations

import logging
from datetime import date

import pandas as pd

from breeze_client import BreezeClient
from nse_calendar import trading_days_between
from data_store import (
    has_nifty_day,
    has_option_day,
    load_nifty_day,
    load_option_day,
    save_nifty_day,
    save_option_day,
)
from utils import day_session_utc_range, expiry_to_breeze_iso, normalize_candle_df

logger = logging.getLogger(__name__)


class DataFetcher:
    def __init__(self, client: BreezeClient):
        self.client = client

    def ensure_nifty_day(self, d: date) -> pd.DataFrame:
        if has_nifty_day(d):
            return load_nifty_day(d)
        from_iso, to_iso = day_session_utc_range(d)
        raw = self.client.get_historical_5min(
            from_date=from_iso,
            to_date=to_iso,
            stock_code="NIFTY",
            exchange_code="NSE",
            product_type="cash",
        )
        df = normalize_candle_df(raw)
        if not df.empty:
            save_nifty_day(d, df)
        else:
            logger.warning("No Nifty cash data for %s", d)
        return df

    def ensure_option_day(
        self,
        d: date,
        expiry: date,
        strike: int,
        right: str,
    ) -> pd.DataFrame:
        if has_option_day(expiry, strike, right, d):
            return load_option_day(expiry, strike, right, d)
        from_iso, to_iso = day_session_utc_range(d)
        raw = self.client.get_historical_5min(
            from_date=from_iso,
            to_date=to_iso,
            stock_code="NIFTY",
            exchange_code="NFO",
            product_type="options",
            expiry_date=expiry_to_breeze_iso(expiry),
            right=right.lower(),
            strike_price=str(strike),
        )
        df = normalize_candle_df(raw)
        if not df.empty:
            save_option_day(expiry, strike, right, d, df)
        else:
            logger.debug("No option data %s %s %s on %s", expiry, strike, right, d)
        return df

    def fetch_hold_period(
        self,
        entry: date,
        expiry: date,
        ce_strike: int,
        pe_strike: int,
    ) -> None:
        """Cache all trading days from entry through expiry for spot + both legs."""
        days = trading_days_between(entry, expiry)
        total = len(days) * 3
        done = 0
        for d in days:
            self.ensure_nifty_day(d)
            done += 1
            logger.info("Fetch progress %d/%d — Nifty %s", done, total, d)
            self.ensure_option_day(d, expiry, ce_strike, "call")
            done += 1
            logger.info("Fetch progress %d/%d — CE %s@%s", done, total, ce_strike, d)
            self.ensure_option_day(d, expiry, pe_strike, "put")
            done += 1
            logger.info("Fetch progress %d/%d — PE %s@%s", done, total, pe_strike, d)
