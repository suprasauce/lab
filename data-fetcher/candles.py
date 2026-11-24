"""Shared date and 1-minute candle normalization for data providers."""

from datetime import date, datetime
from decimal import Decimal, InvalidOperation
from zoneinfo import ZoneInfo

MARKET_ZONE = ZoneInfo("Asia/Kolkata")
BATCH_SIZE = 500


def to_paise(value):
    try:
        paise = Decimal(str(value)) * 100
        if not paise.is_finite() or paise < 0 or paise != paise.to_integral_value():
            raise ValueError("Prices must be nonnegative with at most two decimals")
        result = int(paise)
        if result > 2**63 - 1:
            raise ValueError("Price exceeds PostgreSQL BIGINT")
        return result
    except InvalidOperation as exc:
        raise ValueError("Invalid price") from exc


def calendar_date(value):
    if not isinstance(value, str):
        raise ValueError("Dates must be YYYY-MM-DD strings")
    parsed = date.fromisoformat(value)
    if parsed.isoformat() != value:
        raise ValueError("Dates must use YYYY-MM-DD format")
    return parsed


def candle_row(candle):
    timestamp = datetime.fromisoformat(candle["datetime"])
    if timestamp.tzinfo is None:
        timestamp = timestamp.replace(tzinfo=MARKET_ZONE)
    else:
        timestamp = timestamp.astimezone(MARKET_ZONE)
    volume = Decimal(str(candle["volume"]))
    # TODO: Investigate negative historical volumes returned by providers.
    # Preserve the reported value for now rather than rejecting the candle.
    if not volume.is_finite() or volume != volume.to_integral_value() or not -(2**63) <= volume <= 2**63 - 1:
        raise ValueError("Candle volume must fit a BIGINT")
    return (timestamp, "1m", *(to_paise(candle[field]) for field in ("open", "high", "low", "close")), int(volume))


