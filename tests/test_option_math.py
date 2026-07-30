from backend.common.option_math import black_scholes_price, implied_volatility, option_delta


def test_implied_volatility_and_delta_round_trip_call():
    price = black_scholes_price(
        spot=100,
        strike=105,
        time_to_expiry=30 / 365,
        risk_free_rate=0.07,
        volatility=0.2,
        option_type="call",
    )

    iv = implied_volatility(
        spot=100,
        strike=105,
        option_price=price,
        time_to_expiry=30 / 365,
        risk_free_rate=0.07,
        option_type="call",
    )
    delta = option_delta(
        spot=100,
        strike=105,
        time_to_expiry=30 / 365,
        risk_free_rate=0.07,
        volatility=iv,
        option_type="call",
    )

    assert round(iv, 2) == 0.2
    assert 0 < delta < 1


def test_put_delta_is_negative():
    delta = option_delta(
        spot=100,
        strike=95,
        time_to_expiry=30 / 365,
        risk_free_rate=0.07,
        volatility=0.2,
        option_type="put",
    )

    assert -1 < delta < 0
