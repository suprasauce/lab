"""Pluggable strike selection methods."""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import date, datetime, time
import math
from statistics import NormalDist
from typing import Callable

from backend.common.option_math import implied_volatility, option_delta
from backend.services import market_data_service

RISK_FREE_RATE = 0.07
DELTA_STRIKE_SCAN_RANGE = 40
ESTIMATED_STRIKE_NEIGHBORS = 8
STRIKE_STEP = 50


@dataclass(frozen=True)
class StrikeSelectionContext:
    option_type: str
    expiry: date
    entry_date: date
    entry_time: time
    spot: float
    atm: int
    method: str
    params: dict[str, float | None]


Resolver = Callable[[StrikeSelectionContext], dict | None]


def resolve_strike(context: StrikeSelectionContext) -> dict | None:
    resolver = _RESOLVERS.get(context.method)
    return None if resolver is None else resolver(context)


def _resolve_atm(context: StrikeSelectionContext) -> dict:
    return {"strike": context.atm}


def _resolve_offset(context: StrikeSelectionContext) -> dict | None:
    value = context.params.get("offset_points")
    if value is None:
        return None
    direction = 1 if context.option_type == "call" else -1
    raw_strike = context.atm + direction * float(value)
    return {"strike": _round_to_step(raw_strike)}


def _resolve_fixed_strike(context: StrikeSelectionContext) -> dict | None:
    value = context.params.get("fixed_strike")
    return None if value is None else {"strike": int(value)}


def _resolve_delta(context: StrikeSelectionContext) -> dict | None:
    value = context.params.get("delta")
    if value is None:
        return None
    time_to_expiry = _time_to_expiry_years(context.entry_date, context.entry_time, context.expiry)
    if time_to_expiry <= 0:
        return None
    target = abs(float(value))
    estimated = _estimated_delta_candidate(
        context=context,
        time_to_expiry=time_to_expiry,
        target=target,
    )
    if estimated is not None:
        return estimated
    return _binary_search_delta(
        context=context,
        time_to_expiry=time_to_expiry,
        target=target,
    )


def _resolve_premium(context: StrikeSelectionContext) -> dict | None:
    value = context.params.get("target_premium")
    if value is None:
        return None
    return _best_candidate(
        context=context,
        time_to_expiry=None,
        score=lambda candidate: abs(candidate["price"] - float(value)),
    )


def _best_candidate(
    *,
    context: StrikeSelectionContext,
    time_to_expiry: float | None,
    score,
) -> dict | None:
    candidates = _fetch_candidates(context, time_to_expiry)
    if not candidates:
        return None
    return min(candidates, key=score)


def _estimated_delta_candidate(
    *,
    context: StrikeSelectionContext,
    time_to_expiry: float,
    target: float,
) -> dict | None:
    atm_candidate = _candidate(context, context.atm, time_to_expiry)
    if atm_candidate is None or atm_candidate.get("iv") is None:
        return None
    estimated_strike = _estimate_strike_from_delta(
        context=context,
        time_to_expiry=time_to_expiry,
        target=target,
        volatility=float(atm_candidate["iv"]),
    )
    if estimated_strike is None:
        return None

    strikes = _nearby_strikes(context, estimated_strike)
    with ThreadPoolExecutor(max_workers=len(strikes) or 1) as executor:
        candidates = list(executor.map(lambda strike: _candidate(context, strike, time_to_expiry), strikes))
    valid = [candidate for candidate in candidates if candidate is not None]
    if not valid:
        return None
    return min(valid, key=lambda candidate: abs(abs(candidate["delta"]) - target))


def _estimate_strike_from_delta(
    *,
    context: StrikeSelectionContext,
    time_to_expiry: float,
    target: float,
    volatility: float,
) -> int | None:
    if not 0 < target < 1 or volatility <= 0:
        return None
    probability = target if context.option_type == "call" else 1 - target
    if not 0 < probability < 1:
        return None
    d1 = NormalDist().inv_cdf(probability)
    strike = context.spot * math.exp(
        (RISK_FREE_RATE + 0.5 * volatility**2) * time_to_expiry
        - d1 * volatility * math.sqrt(time_to_expiry)
    )
    return _round_to_step(strike)


def _nearby_strikes(context: StrikeSelectionContext, center: int) -> list[int]:
    center_offset = _strike_to_offset(context, center)
    if center_offset is None:
        return []
    low_offset = max(0, center_offset - ESTIMATED_STRIKE_NEIGHBORS)
    high_offset = min(DELTA_STRIKE_SCAN_RANGE, center_offset + ESTIMATED_STRIKE_NEIGHBORS)
    return sorted(_strike_at_offset(context, offset) for offset in range(low_offset, high_offset + 1))


def _delta_strike_bounds(context: StrikeSelectionContext) -> tuple[int, int]:
    distance = DELTA_STRIKE_SCAN_RANGE * STRIKE_STEP
    if context.option_type == "call":
        return context.atm, context.atm + distance
    return context.atm - distance, context.atm


def _strike_to_offset(context: StrikeSelectionContext, strike: int) -> int | None:
    direction = 1 if context.option_type == "call" else -1
    offset = round((strike - context.atm) / (STRIKE_STEP * direction))
    return offset if 0 <= offset <= DELTA_STRIKE_SCAN_RANGE else None


def _strike_at_offset(context: StrikeSelectionContext, offset: int) -> int:
    direction = 1 if context.option_type == "call" else -1
    return context.atm + offset * STRIKE_STEP * direction


def _binary_search_delta(
    *,
    context: StrikeSelectionContext,
    time_to_expiry: float,
    target: float,
) -> dict | None:
    low = 0
    high = DELTA_STRIKE_SCAN_RANGE
    best = None

    while low <= high:
        mid = (low + high) // 2
        candidate = _nearest_valid_candidate(context, time_to_expiry, mid, low, high)
        if candidate is None:
            break

        if best is None or abs(abs(candidate["delta"]) - target) < abs(abs(best["delta"]) - target):
            best = candidate

        if abs(candidate["delta"]) > target:
            low = candidate["offset"] + 1
        else:
            high = candidate["offset"] - 1

    return best


def _nearest_valid_candidate(
    context: StrikeSelectionContext,
    time_to_expiry: float,
    offset: int,
    low: int,
    high: int,
) -> dict | None:
    for distance in range(0, high - low + 1):
        for candidate_offset in (offset - distance, offset + distance):
            if candidate_offset < low or candidate_offset > high:
                continue
            candidate = _candidate_at_offset(context, time_to_expiry, candidate_offset)
            if candidate is not None:
                return candidate
    return None


def _fetch_candidates(
    context: StrikeSelectionContext,
    time_to_expiry: float | None,
) -> list[dict]:
    offsets = list(range(DELTA_STRIKE_SCAN_RANGE + 1))
    candidates = [_candidate_at_offset(context, time_to_expiry, offset) for offset in offsets]
    return [candidate for candidate in candidates if candidate is not None]


def _candidate_at_offset(context: StrikeSelectionContext, time_to_expiry: float | None, offset: int) -> dict | None:
    candidate = _candidate(context, _strike_at_offset(context, offset), time_to_expiry)
    if candidate is not None:
        candidate["offset"] = offset
    return candidate


def _candidate(
    context: StrikeSelectionContext,
    strike: int,
    time_to_expiry: float | None,
) -> dict | None:
    price = _close(
        market_data_service.get_option_candle(
            symbol="NIFTY",
            exchange="NFO",
            expiry=context.expiry,
            strike=strike,
            right=context.option_type,
            candle_date=context.entry_date,
            candle_time=context.entry_time,
        )
    )
    if price is None:
        return None
    result = {"strike": strike, "price": price}
    if time_to_expiry is None:
        return result
    iv = implied_volatility(
        spot=context.spot,
        strike=strike,
        option_price=price,
        time_to_expiry=time_to_expiry,
        risk_free_rate=RISK_FREE_RATE,
        option_type=context.option_type,
    )
    if iv is None:
        return None
    delta = option_delta(
        spot=context.spot,
        strike=strike,
        time_to_expiry=time_to_expiry,
        risk_free_rate=RISK_FREE_RATE,
        volatility=iv,
        option_type=context.option_type,
    )
    if delta is None:
        return None
    result.update({"iv": iv, "delta": delta})
    return result


def _time_to_expiry_years(entry_date: date, entry_time: time, expiry: date) -> float:
    entry_at = datetime.combine(entry_date, entry_time)
    expiry_at = datetime.combine(expiry, time(15, 30))
    seconds = (expiry_at - entry_at).total_seconds()
    return max(seconds / (365 * 24 * 60 * 60), 0)


def _round_to_step(value: float) -> int:
    return int(round(value / STRIKE_STEP) * STRIKE_STEP)


def round_atm(spot: float) -> int:
    return _round_to_step(spot)


def _close(candle: dict | None) -> float | None:
    if candle is None or candle.get("close") is None:
        return None
    if float(candle.get("volume") or 0) <= 0:
        return None
    close = float(candle["close"])
    return close if close > 0 else None


_RESOLVERS: dict[str, Resolver] = {
    "atm": _resolve_atm,
    "offset": _resolve_offset,
    "delta": _resolve_delta,
    "premium": _resolve_premium,
    "fixed_strike": _resolve_fixed_strike,
}
