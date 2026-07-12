"""Tests for price extraction and strike selection."""

from datetime import datetime

import pandas as pd

from strike_selector import select_strikes
from utils import IST, get_price_at_time


def test_select_strikes():
    sel = select_strikes(24123.0, 18)
    assert sel.atm_strike == 24100
    assert sel.ce_strike == 24118
    assert sel.pe_strike == 24082


def test_get_price_at_time():
    rows = []
    for minute in (25, 30, 35):
        start = datetime(2025, 1, 2, 9, minute, tzinfo=IST)
        rows.append({"datetime": start, "close": float(minute)})
    df = pd.DataFrame(rows)
    assert get_price_at_time(df, datetime.strptime("09:30", "%H:%M").time()) == 25.0
    assert get_price_at_time(df, datetime.strptime("09:45", "%H:%M").time()) is None
