"""Tests for price extraction and strike selection."""

from datetime import datetime
from datetime import date

import pandas as pd

from backend.common.strike_selector import select_strikes
from backend.common.utils import IST, day_session_breeze_range, get_price_at_time


def test_select_strikes():
    sel = select_strikes(24123.0, 6)
    assert sel.atm_strike == 24100
    assert sel.ce_strike == 24400
    assert sel.pe_strike == 23800


def test_get_price_at_time():
    rows = []
    for minute in (25, 30, 35):
        start = datetime(2025, 1, 2, 9, minute, tzinfo=IST)
        rows.append({"datetime": start, "close": float(minute)})
    df = pd.DataFrame(rows)
    assert get_price_at_time(df, datetime.strptime("09:30", "%H:%M").time()) == 25.0
    assert get_price_at_time(df, datetime.strptime("09:45", "%H:%M").time()) is None


def test_day_session_breeze_range_uses_market_clock():
    assert day_session_breeze_range(date(2026, 5, 26)) == (
        "2026-05-26T09:15:00.000Z",
        "2026-05-26T15:30:00.000Z",
    )
