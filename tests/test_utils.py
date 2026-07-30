"""Tests for shared utils."""

from datetime import date

from backend.common.utils import day_session_breeze_range


def test_day_session_breeze_range_uses_market_clock():
    assert day_session_breeze_range(date(2026, 5, 26)) == (
        "2026-05-26T09:15:00.000Z",
        "2026-05-26T15:30:00.000Z",
    )
