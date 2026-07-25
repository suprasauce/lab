from datetime import date

from backend.services import vix_service


def test_build_vix_curve_fetches_cached_market_data(monkeypatch):
    calls = []

    def fake_get_underlying_candle(*, symbol, exchange, candle_date, candle_time):
        calls.append((symbol, exchange, candle_date, candle_time))
        return {"close": 14.236}

    monkeypatch.setattr(
        vix_service.market_data_service,
        "get_underlying_candle",
        fake_get_underlying_candle,
    )

    curve = vix_service.build_vix_curve(
        equity_curve=[{"date": "2026-01-27", "equity": 100.0}],
        trade_metrics=[{"expiry_date": "2026-01-27"}],
    )

    assert curve == [{"date": "2026-01-27", "vix": 14.24}]
    assert calls == [("INDVIX", "NSE", date(2026, 1, 27), vix_service.VIX_CANDLE_TIME)]


def test_build_vix_curve_skips_fetch_failures(monkeypatch):
    def fake_get_underlying_candle(**kwargs):
        raise RuntimeError("session expired")

    monkeypatch.setattr(
        vix_service.market_data_service,
        "get_underlying_candle",
        fake_get_underlying_candle,
    )

    curve = vix_service.build_vix_curve(
        equity_curve=[{"date": "2026-01-27", "equity": 100.0}],
        trade_metrics=[],
    )

    assert curve == []
