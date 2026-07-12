"""NSE calendar: holidays, monthly expiries, entry dates."""

from __future__ import annotations

from calendar import monthrange
from datetime import date, timedelta

EXPIRY_REGIME_CUTOFF = date(2025, 9, 1)

# NSE trading holidays (equity segment) — extend as needed
NSE_HOLIDAYS: set[date] = {
    # 2023
    date(2023, 1, 26),
    date(2023, 3, 7),
    date(2023, 3, 30),
    date(2023, 4, 4),
    date(2023, 4, 7),
    date(2023, 4, 14),
    date(2023, 5, 1),
    date(2023, 6, 29),
    date(2023, 8, 15),
    date(2023, 9, 19),
    date(2023, 10, 2),
    date(2023, 10, 24),
    date(2023, 11, 14),
    date(2023, 11, 27),
    date(2023, 12, 25),
    # 2024
    date(2024, 1, 26),
    date(2024, 3, 8),
    date(2024, 3, 25),
    date(2024, 3, 29),
    date(2024, 4, 11),
    date(2024, 4, 17),
    date(2024, 5, 1),
    date(2024, 6, 17),
    date(2024, 7, 17),
    date(2024, 8, 15),
    date(2024, 10, 2),
    date(2024, 11, 1),
    date(2024, 11, 15),
    date(2024, 12, 25),
    # 2025
    date(2025, 2, 26),
    date(2025, 3, 14),
    date(2025, 3, 31),
    date(2025, 4, 10),
    date(2025, 4, 14),
    date(2025, 4, 18),
    date(2025, 5, 1),
    date(2025, 8, 15),
    date(2025, 8, 27),
    date(2025, 10, 2),
    date(2025, 10, 21),
    date(2025, 10, 22),
    date(2025, 11, 5),
    date(2025, 12, 25),
    # 2026 (partial — update from NSE circular)
    date(2026, 1, 26),
    date(2026, 3, 3),
    date(2026, 3, 26),
    date(2026, 4, 3),
    date(2026, 4, 14),
    date(2026, 5, 1),
    date(2026, 5, 28),
    date(2026, 6, 26),
    date(2026, 8, 15),
    date(2026, 10, 2),
    date(2026, 10, 20),
    date(2026, 11, 9),
    date(2026, 11, 24),
    date(2026, 12, 25),
}


def is_trading_day(d: date) -> bool:
    return d.weekday() < 5 and d not in NSE_HOLIDAYS


def previous_trading_day(d: date) -> date:
    cur = d
    while not is_trading_day(cur):
        cur -= timedelta(days=1)
    return cur


def next_trading_day(d: date) -> date:
    cur = d
    while not is_trading_day(cur):
        cur += timedelta(days=1)
    return cur


def adjust_expiry_for_holiday(expiry: date) -> date:
    """Roll expiry to previous trading day if it falls on a holiday/weekend."""
    return previous_trading_day(expiry)


def _last_weekday_of_month(year: int, month: int, weekday: int) -> date:
    """weekday: Monday=0 … Sunday=6."""
    last_day = monthrange(year, month)[1]
    d = date(year, month, last_day)
    while d.weekday() != weekday:
        d -= timedelta(days=1)
    return d


def monthly_expiry(year: int, month: int) -> date:
    """
    Nifty monthly expiry: last Thursday before Sep 2025 regime;
    last Tuesday on/after Sep 2025 regime (based on expiry date itself).
    """
    tuesday = _last_weekday_of_month(year, month, weekday=1)
    thursday = _last_weekday_of_month(year, month, weekday=3)

    if tuesday >= EXPIRY_REGIME_CUTOFF:
        raw = tuesday
    else:
        raw = thursday

    return adjust_expiry_for_holiday(raw)


def iter_monthly_expiries(start: date, end: date) -> list[date]:
    expiries: list[date] = []
    y, m = start.year, start.month
    end_ym = end.year * 12 + end.month
    while y * 12 + m <= end_ym:
        exp = monthly_expiry(y, m)
        if start <= exp <= end:
            expiries.append(exp)
        m += 1
        if m > 12:
            m = 1
            y += 1
    return sorted(set(expiries))


def entry_date_for_expiry(expiry: date, entry_dte: int) -> date:
    """Calendar DTE entry; roll to previous trading day if holiday/weekend."""
    calendar_entry = expiry - timedelta(days=entry_dte)
    if is_trading_day(calendar_entry):
        return calendar_entry
    return previous_trading_day(calendar_entry)


def calendar_dte_at_entry(expiry: date, entry: date) -> int:
    return (expiry - entry).days


def trading_days_between(start: date, end: date) -> list[date]:
    days: list[date] = []
    cur = start
    while cur <= end:
        if is_trading_day(cur):
            days.append(cur)
        cur += timedelta(days=1)
    return days
