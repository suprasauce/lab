from datetime import date, time

import backend.services.strike_selection_service as strike_selection_service
from backend.services import market_data_service
from backend.services.strike_selection_service import StrikeSelectionContext, resolve_strike, round_atm


def test_round_atm():
    assert round_atm(24123.0) == 24100


def test_resolve_atm_strike():
    result = resolve_strike(
        StrikeSelectionContext(
            option_type="call",
            expiry=date(2026, 1, 27),
            entry_date=date(2025, 12, 12),
            entry_time=time(9, 30),
            spot=25025.0,
            atm=25000,
            method="atm",
            params={},
        )
    )

    assert result == {"strike": 25000}


def test_resolve_premium_uses_closest_scanned_candidate(monkeypatch):
    def fake_option(**kwargs):
        prices = {25000: 200.0, 25050: 140.0, 25100: 95.0, 25150: 60.0}
        close = prices.get(kwargs["strike"])
        return None if close is None else {"close": close, "volume": 1}

    monkeypatch.setattr(market_data_service, "get_option_candle", fake_option)

    result = resolve_strike(
        StrikeSelectionContext(
            option_type="call",
            expiry=date(2026, 1, 27),
            entry_date=date(2025, 12, 12),
            entry_time=time(9, 30),
            spot=25000.0,
            atm=25000,
            method="premium",
            params={"target_premium": 100.0},
        )
    )

    assert result["strike"] == 25100
    assert result["price"] == 95.0


def test_resolve_premium_skips_missing_candidates(monkeypatch):
    def fake_option(**kwargs):
        prices = {25000: 200.0, 25150: 60.0}
        close = prices.get(kwargs["strike"])
        return None if close is None else {"close": close, "volume": 1}

    monkeypatch.setattr(market_data_service, "get_option_candle", fake_option)

    result = resolve_strike(
        StrikeSelectionContext(
            option_type="call",
            expiry=date(2026, 1, 27),
            entry_date=date(2025, 12, 12),
            entry_time=time(9, 30),
            spot=25000.0,
            atm=25000,
            method="premium",
            params={"target_premium": 100.0},
        )
    )

    assert result["strike"] == 25150
    assert result["price"] == 60.0


def test_resolve_premium_skips_zero_volume_candidate(monkeypatch):
    def fake_option(**kwargs):
        candles = {
            25000: {"close": 200.0, "volume": 1},
            25050: {"close": 100.0, "volume": 0},
            25100: {"close": 90.0, "volume": 1},
        }
        return candles.get(kwargs["strike"])

    monkeypatch.setattr(market_data_service, "get_option_candle", fake_option)

    result = resolve_strike(
        StrikeSelectionContext(
            option_type="call",
            expiry=date(2026, 1, 27),
            entry_date=date(2025, 12, 12),
            entry_time=time(9, 30),
            spot=25000.0,
            atm=25000,
            method="premium",
            params={"target_premium": 100.0},
        )
    )

    assert result["strike"] == 25100
    assert result["price"] == 90.0


def test_delta_selection_uses_binary_search(monkeypatch):
    calls = []

    def fake_candidate_at_offset(context, time_to_expiry, offset):
        calls.append(offset)
        delta_by_offset = {
            20: 0.30,
            30: 0.15,
            25: 0.21,
            27: 0.18,
            26: 0.20,
        }
        delta = delta_by_offset.get(offset)
        if delta is None:
            return None
        return {"strike": context.atm + offset * 50, "price": 100.0, "delta": delta, "iv": 0.2, "offset": offset}

    monkeypatch.setattr(strike_selection_service, "_candidate_at_offset", fake_candidate_at_offset)

    context = StrikeSelectionContext(
        option_type="call",
        expiry=date(2026, 1, 27),
        entry_date=date(2025, 12, 12),
        entry_time=time(9, 30),
        spot=25000.0,
        atm=25000,
        method="delta",
        params={"delta": 0.20},
    )

    result = strike_selection_service._binary_search_delta(
        context=context,
        time_to_expiry=0.10,
        target=0.20,
    )

    assert result["offset"] == 26
    assert result["strike"] == 26300
    assert len(set(calls)) < 41


def test_delta_selection_uses_estimated_strike_neighbors(monkeypatch):
    monkeypatch.setattr(strike_selection_service, "_estimate_strike_from_delta", lambda **kwargs: 26000)

    def fake_candidate(context, strike, time_to_expiry):
        candidates = {
            25000: {"strike": 25000, "price": 200.0, "delta": 0.50, "iv": 0.20},
            25850: {"strike": 25850, "price": 90.0, "delta": 0.24, "iv": 0.20},
            25900: {"strike": 25900, "price": 80.0, "delta": 0.22, "iv": 0.20},
            25950: {"strike": 25950, "price": 70.0, "delta": 0.21, "iv": 0.20},
            26000: {"strike": 26000, "price": 60.0, "delta": 0.18, "iv": 0.20},
            26050: {"strike": 26050, "price": 50.0, "delta": 0.16, "iv": 0.20},
            26100: {"strike": 26100, "price": 40.0, "delta": 0.14, "iv": 0.20},
            26150: {"strike": 26150, "price": 30.0, "delta": 0.12, "iv": 0.20},
        }
        return candidates.get(strike)

    monkeypatch.setattr(strike_selection_service, "_candidate", fake_candidate)

    result = resolve_strike(
        StrikeSelectionContext(
            option_type="call",
            expiry=date(2026, 1, 27),
            entry_date=date(2025, 12, 12),
            entry_time=time(9, 30),
            spot=25000.0,
            atm=25000,
            method="delta",
            params={"delta": 0.20},
        )
    )

    assert result["strike"] == 25950
    assert result["delta"] == 0.21


def test_estimated_strike_neighbors_include_eight_strikes_each_side():
    context = StrikeSelectionContext(
        option_type="put",
        expiry=date(2025, 5, 29),
        entry_date=date(2025, 4, 11),
        entry_time=time(9, 30),
        spot=22762.15,
        atm=22750,
        method="delta",
        params={"delta": 0.20},
    )

    assert strike_selection_service._nearby_strikes(context, 21750) == [
        21350,
        21400,
        21450,
        21500,
        21550,
        21600,
        21650,
        21700,
        21750,
        21800,
        21850,
        21900,
        21950,
        22000,
        22050,
        22100,
        22150,
    ]


def test_estimated_delta_fixed_window_can_find_candidate_farther_from_estimate(monkeypatch):
    monkeypatch.setattr(strike_selection_service, "_estimate_strike_from_delta", lambda **kwargs: 24500)

    def fake_candidate(context, strike, time_to_expiry):
        candidates = {
            22750: {"strike": 22750, "price": 772.0, "delta": 0.56, "iv": 0.20},
            24200: {"strike": 24200, "price": 139.8, "delta": 0.1948, "iv": 0.16},
            24300: {"strike": 24300, "price": 119.0, "delta": 0.1730, "iv": 0.16},
            24350: {"strike": 24350, "price": 116.2, "delta": 0.1678, "iv": 0.16},
            24400: {"strike": 24400, "price": 107.55, "delta": 0.1582, "iv": 0.16},
            24500: {"strike": 24500, "price": 89.95, "delta": 0.1384, "iv": 0.16},
        }
        return candidates.get(strike)

    monkeypatch.setattr(strike_selection_service, "_candidate", fake_candidate)

    result = resolve_strike(
        StrikeSelectionContext(
            option_type="call",
            expiry=date(2025, 5, 29),
            entry_date=date(2025, 4, 11),
            entry_time=time(9, 30),
            spot=22762.15,
            atm=22750,
            method="delta",
            params={"delta": 0.20},
        )
    )

    assert result["strike"] == 24200
    assert result["delta"] == 0.1948


def test_delta_binary_search_skips_missing_mid(monkeypatch):
    def fake_candidate_at_offset(context, time_to_expiry, offset):
        if offset == 20:
            return None
        if offset == 21:
            return {"strike": context.atm + offset * 50, "price": 100.0, "delta": 0.20, "iv": 0.2, "offset": offset}
        return None

    monkeypatch.setattr(strike_selection_service, "_candidate_at_offset", fake_candidate_at_offset)

    context = StrikeSelectionContext(
        option_type="call",
        expiry=date(2026, 1, 27),
        entry_date=date(2025, 12, 12),
        entry_time=time(9, 30),
        spot=25000.0,
        atm=25000,
        method="delta",
        params={"delta": 0.20},
    )

    result = strike_selection_service._binary_search_delta(
        context=context,
        time_to_expiry=0.10,
        target=0.20,
    )

    assert result["offset"] == 21
