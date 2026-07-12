"""ATM ± offset strike selection."""

from __future__ import annotations

from dataclasses import dataclass


STRIKE_STEP = 50


@dataclass
class StrikeSelection:
    spot: float
    atm_strike: int
    ce_strike: int
    pe_strike: int


def round_atm(spot: float, step: int = STRIKE_STEP) -> int:
    return int(round(spot / step) * step)


def select_strikes(spot: float, strike_offset: int) -> StrikeSelection:
    atm = round_atm(spot)
    return StrikeSelection(
        spot=spot,
        atm_strike=atm,
        ce_strike=atm + strike_offset,
        pe_strike=atm - strike_offset,
    )
