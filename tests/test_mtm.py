from datetime import date, time

import pandas as pd

from backend.services import market_data_service
from backend.services.mtm_service import build_daily_mtm


def test_build_daily_mtm_for_short_leg(monkeypatch):
    def fake_get_option_5m_range(**kwargs):
        return pd.DataFrame(
            [
                {"datetime": "2026-01-02 09:25:00", "close": 100.0},
                {"datetime": "2026-01-02 09:30:00", "close": 95.0},
                {"datetime": "2026-01-05 15:25:00", "close": 80.0},
                {"datetime": "2026-01-06 15:25:00", "close": 70.0},
                {"datetime": "2026-01-06 15:30:00", "close": 60.0},
            ]
        )

    monkeypatch.setattr(market_data_service, "get_option_5m_range", fake_get_option_5m_range)
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

    assert mtm["mtm_date"].tolist() == ["2026-01-02", "2026-01-02", "2026-01-05", "2026-01-06"]
    assert mtm["mtm_time"].tolist() == ["09:30", "09:35", "15:30", "15:30"]
    assert mtm["current_price"].tolist() == [100.0, 95.0, 80.0, 70.0]
    assert mtm["mtm"].tolist() == [0.0, 375.0, 1500.0, 2250.0]
    assert mtm["reason"].tolist() == ["ok", "ok", "ok", "ok"]
