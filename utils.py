"""Timezone and price extraction helpers."""

from __future__ import annotations

from datetime import date, datetime, time, timedelta
from zoneinfo import ZoneInfo

import pandas as pd

IST = ZoneInfo("Asia/Kolkata")
UTC = ZoneInfo("UTC")

MARKET_OPEN = time(9, 15)
MARKET_CLOSE = time(15, 30)


def ist_to_utc_iso(d: date, t: time) -> str:
    dt = datetime.combine(d, t, tzinfo=IST)
    return dt.astimezone(UTC).strftime("%Y-%m-%dT%H:%M:%S.000Z")


def day_session_utc_range(d: date) -> tuple[str, str]:
    """Full NSE cash session for one day in Breeze ISO UTC."""
    return (
        ist_to_utc_iso(d, MARKET_OPEN),
        ist_to_utc_iso(d, MARKET_CLOSE),
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


def get_price_at_time(df: pd.DataFrame, target: time) -> float | None:
    """
    Return close of the 5min bar ending at target time (e.g. 09:30 → bar 09:25–09:30).
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
