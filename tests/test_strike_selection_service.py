from datetime import date, time

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
        return None if close is None else {"close": close}

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
