from datetime import date, time

import pandas as pd

from backend.config.settings import StrategyConfig
from backend.services import market_data_service
from backend.strategies.short_strangle import ShortStrangleStrategy


def test_short_strangle_uses_scheduled_exit_without_stop_loss(monkeypatch):
    def fake_get_underlying_candle(**kwargs):
        return {"close": 25000}

    def fake_get_option_candle(**kwargs):
        if kwargs["candle_date"] == date(2025, 12, 12):
            return {"close": 100 if kwargs["right"] == "call" else 90}
        if kwargs["candle_date"] == date(2026, 1, 27):
            return {"close": 70 if kwargs["right"] == "call" else 60}
        return None

    monkeypatch.setattr(market_data_service, "get_underlying_candle", fake_get_underlying_candle)
    monkeypatch.setattr(market_data_service, "get_option_candle", fake_get_option_candle)

    config = StrategyConfig(
        entry_dte=46,
        entry_time=time(9, 30),
        exit_time=time(15, 30),
        strike_offset=0,
        lot_size=75,
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
    )

    result = ShortStrangleStrategy().run(config, date(2026, 1, 27))
    trades = result["trades"]

    assert result["skipped_expiries"].empty
    assert trades["exit_reason"].tolist() == ["scheduled_exit", "scheduled_exit"]
    assert trades["exit_date"].tolist() == ["2026-01-27", "2026-01-27"]
    assert trades["exit_time"].tolist() == ["15:30", "15:30"]
    assert trades["exit_price"].tolist() == [70.0, 60.0]


def test_short_strangle_total_stop_loss_exits_both_legs(monkeypatch):
    def fake_get_underlying_candle(**kwargs):
        return {"close": 25000}

    def fake_get_option_candle(**kwargs):
        key = (kwargs["right"], kwargs["candle_date"], kwargs["candle_time"])
        prices = {
            ("call", date(2025, 12, 12), time(9, 30)): 100,
            ("put", date(2025, 12, 12), time(9, 30)): 100,
        }
        close = prices.get(key)
        return None if close is None else {"close": close}

    def fake_get_option_5m_range(**kwargs):
        closes = [100, 250] if kwargs["right"] == "call" else [100, 160]
        return pd.DataFrame(
            [
                {"datetime": "2025-12-12 09:25:00", "close": closes[0]},
                {"datetime": "2025-12-12 09:30:00", "close": closes[1]},
            ]
        )

    monkeypatch.setattr(market_data_service, "get_underlying_candle", fake_get_underlying_candle)
    monkeypatch.setattr(market_data_service, "get_option_candle", fake_get_option_candle)
    monkeypatch.setattr(market_data_service, "get_option_5m_range", fake_get_option_5m_range)

    config = StrategyConfig(
        entry_dte=46,
        entry_time=time(9, 30),
        exit_time=time(15, 30),
        total_stop_loss_multiplier=2,
        strike_offset=0,
        lot_size=75,
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
    )

    result = ShortStrangleStrategy().run(config, date(2026, 1, 27))
    trades = result["trades"]

    assert result["skipped_expiries"].empty
    assert trades["exit_reason"].tolist() == ["total_stop_loss", "total_stop_loss"]
    assert trades["exit_date"].tolist() == ["2025-12-12", "2025-12-12"]
    assert trades["exit_time"].tolist() == ["09:35", "09:35"]
    assert trades["exit_price"].tolist() == [250.0, 160.0]
