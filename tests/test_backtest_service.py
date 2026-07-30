from datetime import date, time

import pandas as pd

import backend.services.backtest_service as backtest_service
from backend.services import market_data_service
from backend.services.backtest_service import PositionConfig, run_backtest


def test_engine_runs_each_position_for_each_expiry(monkeypatch):
    def fake_underlying(**kwargs):
        return {"close": 25000.0}

    def fake_option(**kwargs):
        prices = {
            ("call", 25300, date(2025, 12, 12)): 100.0,
            ("call", 25300, date(2026, 1, 27)): 80.0,
            ("put", 24700, date(2025, 12, 12)): 90.0,
            ("put", 24700, date(2026, 1, 27)): 110.0,
            ("call", 25300, date(2026, 1, 9)): 120.0,
            ("call", 25300, date(2026, 2, 24)): 70.0,
            ("put", 24700, date(2026, 1, 9)): 85.0,
            ("put", 24700, date(2026, 2, 24)): 95.0,
        }
        close = prices.get((kwargs["right"], kwargs["strike"], kwargs["candle_date"]))
        return None if close is None else {"close": close}

    monkeypatch.setattr(market_data_service, "get_underlying_candle", fake_underlying)
    monkeypatch.setattr(market_data_service, "get_option_candle", fake_option)

    positions = [
        PositionConfig(
            "short_call",
            "call",
            "sell",
            75,
            "offset",
            {"offset_points": 300},
            45,
            time(9, 30),
            0,
            time(15, 30),
        ),
        PositionConfig(
            "short_put",
            "put",
            "sell",
            75,
            "offset",
            {"offset_points": 300},
            45,
            time(9, 30),
            0,
            time(15, 30),
        ),
    ]

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 2, 28),
        positions=positions,
    )

    assert len(results["trades"]) == 4
    assert results["trades"]["expiry_date"].tolist() == [
        "2026-01-27",
        "2026-01-27",
        "2026-02-24",
        "2026-02-24",
    ]
    assert results["trades"]["strike"].tolist() == [25300, 24700, 25300, 24700]
    assert results["skipped_expiries"].empty


def test_run_backtest_for_strategy_can_skip_mtm(monkeypatch):
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "exit_date": "2026-01-27",
                "entry_price": 100.0,
                "exit_price": 80.0,
                "lot_size": 75,
                "side": "sell",
            }
        ]
    )

    def fake_run_backtest(**kwargs):
        return {"trades": trades, "skipped_expiries": pd.DataFrame()}

    def fail_build_daily_mtm(trades):
        raise AssertionError("MTM should not be calculated")

    monkeypatch.setattr(backtest_service, "run_backtest", fake_run_backtest)
    monkeypatch.setattr(backtest_service, "build_daily_mtm", fail_build_daily_mtm)
    monkeypatch.setattr(backtest_service, "save_run", lambda run_id, metadata, results: None)

    _, results = backtest_service.run_backtest_for_strategy(
        strategy_id="custom_multi_leg",
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[
            PositionConfig(
                "short_call",
                "call",
                "sell",
                75,
                "offset",
                {"offset_points": 300},
                45,
                time(9, 30),
                0,
                time(15, 30),
            )
        ],
        include_mtm=False,
    )

    assert results["daily_mtm"].empty
    assert results["trade_metrics"] == []
    assert results["vix_curve"] == []
    assert results["metrics"]["max_drawdown"] is None
