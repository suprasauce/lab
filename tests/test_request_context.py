from datetime import date, time

import pandas as pd

from backend.services import market_data_service
from backend.services.request_context import (
    create_context,
    get_option_5m_range,
    get_option_candle,
    get_underlying_candle,
)


def test_context_caches_underlying_and_option_candles(monkeypatch):
    calls = {"underlying": 0, "option": 0}

    def fake_underlying(**kwargs):
        calls["underlying"] += 1
        return {"close": 25000}

    def fake_option(**kwargs):
        calls["option"] += 1
        return {"close": 100}

    monkeypatch.setattr(market_data_service, "get_underlying_candle", fake_underlying)
    monkeypatch.setattr(market_data_service, "get_option_candle", fake_option)

    context = create_context()
    for _ in range(2):
        assert get_underlying_candle(
            context,
            symbol="NIFTY",
            exchange="NSE",
            candle_date=date(2026, 1, 1),
            candle_time=time(9, 30),
        ) == {"close": 25000}
        assert get_option_candle(
            context,
            symbol="NIFTY",
            exchange="NFO",
            expiry=date(2026, 1, 27),
            strike=25000,
            right="call",
            candle_date=date(2026, 1, 1),
            candle_time=time(9, 30),
        ) == {"close": 100}

    assert calls == {"underlying": 1, "option": 1}


def test_context_serves_option_candle_from_cached_range(monkeypatch):
    calls = {"range": 0, "candle": 0}

    def fake_range(**kwargs):
        calls["range"] += 1
        return pd.DataFrame([{"datetime": "2026-01-01 09:25:00", "close": 123.45}])

    def fake_candle(**kwargs):
        calls["candle"] += 1
        return {"close": 999}

    monkeypatch.setattr(market_data_service, "get_option_5m_range", fake_range)
    monkeypatch.setattr(market_data_service, "get_option_candle", fake_candle)

    context = create_context()
    get_option_5m_range(
        context,
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=25000,
        right="call",
        start=date(2026, 1, 1),
        end=date(2026, 1, 27),
    )
    candle = get_option_candle(
        context,
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=25000,
        right="call",
        candle_date=date(2026, 1, 1),
        candle_time=time(9, 30),
    )

    assert candle["close"] == 123.45
    assert calls == {"range": 1, "candle": 0}
