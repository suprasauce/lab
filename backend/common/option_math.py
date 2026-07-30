"""Black-Scholes option math."""

from __future__ import annotations

import math


def black_scholes_price(
    *,
    spot: float,
    strike: float,
    time_to_expiry: float,
    risk_free_rate: float,
    volatility: float,
    option_type: str,
) -> float | None:
    if spot <= 0 or strike <= 0 or time_to_expiry <= 0 or volatility <= 0:
        return None
    d1, d2 = _d1_d2(
        spot=spot,
        strike=strike,
        time_to_expiry=time_to_expiry,
        risk_free_rate=risk_free_rate,
        volatility=volatility,
    )
    discount = math.exp(-risk_free_rate * time_to_expiry)
    if option_type.lower() == "call":
        return spot * _norm_cdf(d1) - strike * discount * _norm_cdf(d2)
    if option_type.lower() == "put":
        return strike * discount * _norm_cdf(-d2) - spot * _norm_cdf(-d1)
    return None


def implied_volatility(
    *,
    spot: float,
    strike: float,
    option_price: float,
    time_to_expiry: float,
    risk_free_rate: float,
    option_type: str,
    max_iterations: int = 80,
    tolerance: float = 1e-4,
) -> float | None:
    if spot <= 0 or strike <= 0 or option_price <= 0 or time_to_expiry <= 0:
        return None

    low = 0.0001
    high = 5.0
    low_price = black_scholes_price(
        spot=spot,
        strike=strike,
        time_to_expiry=time_to_expiry,
        risk_free_rate=risk_free_rate,
        volatility=low,
        option_type=option_type,
    )
    high_price = black_scholes_price(
        spot=spot,
        strike=strike,
        time_to_expiry=time_to_expiry,
        risk_free_rate=risk_free_rate,
        volatility=high,
        option_type=option_type,
    )
    if low_price is None or high_price is None or not (low_price <= option_price <= high_price):
        return None

    for _ in range(max_iterations):
        mid = (low + high) / 2
        mid_price = black_scholes_price(
            spot=spot,
            strike=strike,
            time_to_expiry=time_to_expiry,
            risk_free_rate=risk_free_rate,
            volatility=mid,
            option_type=option_type,
        )
        if mid_price is None:
            return None
        if abs(mid_price - option_price) <= tolerance:
            return mid
        if mid_price < option_price:
            low = mid
        else:
            high = mid
    return (low + high) / 2


def option_delta(
    *,
    spot: float,
    strike: float,
    time_to_expiry: float,
    risk_free_rate: float,
    volatility: float,
    option_type: str,
) -> float | None:
    if spot <= 0 or strike <= 0 or time_to_expiry <= 0 or volatility <= 0:
        return None
    d1, _ = _d1_d2(
        spot=spot,
        strike=strike,
        time_to_expiry=time_to_expiry,
        risk_free_rate=risk_free_rate,
        volatility=volatility,
    )
    if option_type.lower() == "call":
        return _norm_cdf(d1)
    if option_type.lower() == "put":
        return _norm_cdf(d1) - 1
    return None


def _d1_d2(
    *,
    spot: float,
    strike: float,
    time_to_expiry: float,
    risk_free_rate: float,
    volatility: float,
) -> tuple[float, float]:
    vol_sqrt_t = volatility * math.sqrt(time_to_expiry)
    d1 = (math.log(spot / strike) + (risk_free_rate + 0.5 * volatility**2) * time_to_expiry) / vol_sqrt_t
    d2 = d1 - vol_sqrt_t
    return d1, d2


def _norm_cdf(value: float) -> float:
    return 0.5 * (1 + math.erf(value / math.sqrt(2)))
