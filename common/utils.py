"""Timezone and price extraction helpers."""

from __future__ import annotations

from datetime import date, datetime, time, timedelta
from zoneinfo import ZoneInfo

import pandas as pd

IST = ZoneInfo("Asia/Kolkata")

MARKET_OPEN = time(9, 15)
MARKET_CLOSE = time(15, 30)


def breeze_iso_at_market_time(d: date, t: time) -> str:
    """
    Breeze historical endpoints expect NSE clock times in this ISO shape.

    Despite the trailing "Z", sending UTC-converted values clips the returned
    data to the morning session. Keep the market clock time intact.
    """
    return datetime.combine(d, t).strftime("%Y-%m-%dT%H:%M:%S.000Z")


def day_session_breeze_range(d: date) -> tuple[str, str]:
    """Full NSE session for one day in the format Breeze expects."""
    return (
        breeze_iso_at_market_time(d, MARKET_OPEN),
        breeze_iso_at_market_time(d, MARKET_CLOSE),
    )


def expiry_to_breeze_iso(expiry: date) -> str:
    """Breeze expects expiry datetime; 07:00 UTC is the SDK convention."""
    return f"{expiry.isoformat()}T07:00:00.000Z"


def normalize_candle_df(raw: list[dict] | None) -> pd.DataFrame:
    if not raw:
        return pd.DataFrame()
    df = pd.DataFrame(raw)
    if df.empty:
        return df
    col = "datetime" if "datetime" in df.columns else "date_time"
    if col not in df.columns:
        return df
    df["datetime"] = pd.to_datetime(df[col])
    if df["datetime"].dt.tz is None:
        df["datetime"] = df["datetime"].dt.tz_localize(IST)
    else:
        df["datetime"] = df["datetime"].dt.tz_convert(IST)
    for c in ("open", "high", "low", "close", "volume", "open_interest"):
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    return df.sort_values("datetime").reset_index(drop=True)


def bar_end_time(bar_start: datetime, interval_minutes: int = 5) -> time:
    end = bar_start + timedelta(minutes=interval_minutes)
    return end.time().replace(microsecond=0)


def bar_start_for_end_time(d: date, target: time, interval_minutes: int = 5) -> datetime:
    """Return the candle timestamp for a bar ending at target time."""
    return datetime.combine(d, target) - timedelta(minutes=interval_minutes)


def get_price_at_time(df: pd.DataFrame, target: time) -> float | None:
    """
    Return close of the 5min bar ending at target time.

    For example, 09:30 uses the candle timestamped 09:25 if Breeze labels
    candles by start time.
    """
    if df.empty or "close" not in df.columns:
        return None
    for _, row in df.iterrows():
        end = bar_end_time(row["datetime"].to_pydatetime())
        if (
            end.hour == target.hour
            and end.minute == target.minute
            and end.second == target.second
        ):
            close = float(row["close"])
            if close > 0:
                return close
    return None
