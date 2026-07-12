"""Trade records for short strangle backtest."""

from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import date


@dataclass
class Trade:
    expiry_date: date
    entry_date: date
    exit_date: date
    actual_dte: int
    spot_at_entry: float
    atm_strike: int
    ce_strike: int
    pe_strike: int
    ce_entry: float
    pe_entry: float
    entry_premium: float
    ce_exit: float
    pe_exit: float
    exit_cost: float
    pnl: float
    lot_size: int
    entry_dte: int
    strike_offset: int
    entry_time: str
    exit_time: str

    def to_dict(self) -> dict:
        d = asdict(self)
        for k, v in d.items():
            if isinstance(v, date):
                d[k] = v.isoformat()
        return d


@dataclass
class SkippedTrade:
    expiry_date: date
    entry_date: date
    actual_dte: int | None
    ce_strike: int | None
    pe_strike: int | None
    skip_reason: str

    def to_dict(self) -> dict:
        d = asdict(self)
        for k, v in d.items():
            if isinstance(v, date):
                d[k] = v.isoformat()
        return d
