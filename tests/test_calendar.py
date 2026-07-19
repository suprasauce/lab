"""Tests for calendar logic (no API required)."""

from datetime import date

from backend.common.nse_calendar import (
    entry_date_for_expiry,
    is_trading_day,
    iter_monthly_expiries,
    monthly_expiry,
    previous_trading_day,
)


def test_aug_2025_thursday_expiry():
    assert monthly_expiry(2025, 8) == date(2025, 8, 28)


def test_sep_2025_tuesday_expiry():
    assert monthly_expiry(2025, 9) == date(2025, 9, 30)


def test_oct_2025_tuesday_expiry():
    assert monthly_expiry(2025, 10) == date(2025, 10, 28)


def test_entry_holiday_rolls_prev():
    expiry = date(2025, 9, 30)
    entry = entry_date_for_expiry(expiry, 45)
    assert is_trading_day(entry)
    assert entry < expiry


def test_entry_on_weekend_rolls_prev():
    # Force a weekend calendar date by checking DTE math
    expiry = date(2024, 1, 25)  # last Thursday Jan 2024
    entry = entry_date_for_expiry(expiry, 45)
    assert entry.weekday() < 5
    assert entry not in {date(2023, 12, 25)} or is_trading_day(entry)


def test_iter_expiries_in_range():
    exps = iter_monthly_expiries(date(2025, 8, 1), date(2025, 10, 31))
    assert date(2025, 8, 28) in exps
    assert date(2025, 9, 30) in exps
    assert date(2025, 10, 28) in exps


def test_previous_trading_day_from_sunday():
    assert previous_trading_day(date(2025, 8, 17)) == date(2025, 8, 14)
