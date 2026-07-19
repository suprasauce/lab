from datetime import date, time

import pandas as pd

from backend.services import market_data_service
from backend.services.mtm_service import build_daily_mtm


def test_build_daily_mtm_for_short_leg(monkeypatch):
    def fake_get_option_candle(**kwargs):
        prices = {
            (date(2026, 1, 2), time(9, 30)): 100.0,
            (date(2026, 1, 5), time(15, 30)): 80.0,
            (date(2026, 1, 6), time(15, 30)): 70.0,
        }
        close = prices.get((kwargs["candle_date"], kwargs["candle_time"]))
        return None if close is None else {"close": close}

    monkeypatch.setattr(market_data_service, "get_option_candle", fake_get_option_candle)
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-06",
                "entry_date": "2026-01-02",
                "exit_date": "2026-01-06",
                "leg_role": "short_call",
                "option_type": "call",
                "strike": 25000,
                "entry_price": 100.0,
                "exit_price": 70.0,
                "lot_size": 75,
                "entry_time": "09:30",
                "exit_time": "15:30",
            }
        ]
    )

    mtm = build_daily_mtm(trades)

    assert mtm["mtm_date"].tolist() == ["2026-01-02", "2026-01-05", "2026-01-06"]
    assert mtm["mtm_time"].tolist() == ["09:30", "15:30", "15:30"]
    assert mtm["current_price"].tolist() == [100.0, 80.0, 70.0]
    assert mtm["mtm"].tolist() == [0.0, 1500.0, 2250.0]
    assert mtm["reason"].tolist() == ["ok", "ok", "ok"]
