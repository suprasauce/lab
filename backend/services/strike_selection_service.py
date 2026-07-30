"""Pluggable strike selection methods."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import date, datetime, time
from typing import Callable

from backend.common.option_math import implied_volatility, option_delta
from backend.services import market_data_service

RISK_FREE_RATE = 0.07
DELTA_STRIKE_SCAN_RANGE = 36
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
    target = abs(float(value)) if context.option_type == "call" else -abs(float(value))
    return _best_scanned_candidate(
        context=context,
        time_to_expiry=time_to_expiry,
        score=lambda candidate: abs(candidate["delta"] - target),
        stop=lambda candidate: abs(candidate["delta"]) <= abs(target),
    )


def _resolve_premium(context: StrikeSelectionContext) -> dict | None:
    value = context.params.get("target_premium")
    if value is None:
        return None
    return _best_scanned_candidate(
        context=context,
        time_to_expiry=None,
        score=lambda candidate: abs(candidate["price"] - float(value)),
        stop=lambda candidate: candidate["price"] <= float(value),
    )


def _best_scanned_candidate(
    *,
    context: StrikeSelectionContext,
    time_to_expiry: float | None,
    score,
    stop,
) -> dict | None:
    candidates = []
    offset = 0
    while offset <= DELTA_STRIKE_SCAN_RANGE:
        candidate = _candidate_at_offset(context, time_to_expiry, offset)
        if candidate is None and candidates:
            candidate = _next_valid_candidate(
                context,
                time_to_expiry,
                candidates[-1]["offset"] + 1,
                DELTA_STRIKE_SCAN_RANGE,
            )
        if candidate is None:
            offset += 4
            continue
        candidates.append(candidate)
        if stop(candidate):
            previous_offset = candidates[-2]["offset"] if len(candidates) > 1 else candidate["offset"]
            candidates.extend(
                _candidates_between_offsets(
                    context,
                    time_to_expiry,
                    previous_offset + 1,
                    candidate["offset"] - 1,
                )
            )
            return min(candidates, key=score)
        offset = candidate["offset"] + 4

    if not candidates:
        return None
    return min(candidates, key=score)


def _candidate_at_offset(
    context: StrikeSelectionContext,
    time_to_expiry: float | None,
    offset: int,
) -> dict | None:
    direction = 1 if context.option_type == "call" else -1
    strike = context.atm + offset * STRIKE_STEP * direction
    candidate = _candidate(context, strike, time_to_expiry)
    if candidate is not None:
        candidate["offset"] = offset
    return candidate


def _next_valid_candidate(
    context: StrikeSelectionContext,
    time_to_expiry: float | None,
    start_offset: int,
    max_range: int,
) -> dict | None:
    for offset in range(start_offset, max_range + 1):
        candidate = _candidate_at_offset(context, time_to_expiry, offset)
        if candidate is not None:
            return candidate
    return None


def _candidates_between_offsets(
    context: StrikeSelectionContext,
    time_to_expiry: float | None,
    start_offset: int,
    end_offset: int,
) -> list[dict]:
    if start_offset > end_offset:
        return []
    return [
        candidate
        for offset in range(start_offset, end_offset + 1)
        if (candidate := _candidate_at_offset(context, time_to_expiry, offset)) is not None
    ]


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
    close = float(candle["close"])
    return close if close > 0 else None


_RESOLVERS: dict[str, Resolver] = {
    "atm": _resolve_atm,
    "offset": _resolve_offset,
    "delta": _resolve_delta,
    "premium": _resolve_premium,
    "fixed_strike": _resolve_fixed_strike,
}
