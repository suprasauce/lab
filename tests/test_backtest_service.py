from datetime import date, time

import pandas as pd

from backend.common.utils import MARKET_CLOSE, bar_start_for_end_time
import backend.services.backtest_service as backtest_service
from backend.services import market_data_service
from backend.services.backtest_service import (
    ActionConfig,
    AdjustmentConfig,
    PositionConfig,
    StrikeSelectionConfig,
    TriggerConfig,
    run_backtest,
)


def test_engine_runs_each_position_for_each_expiry(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("call", 25300, date(2026, 1, 27), time(15, 30)): 80.0,
            ("put", 24700, date(2025, 12, 12), time(9, 30)): 90.0,
            ("put", 24700, date(2026, 1, 27), time(15, 30)): 110.0,
            ("call", 25300, date(2026, 1, 9), time(9, 30)): 120.0,
            ("call", 25300, date(2026, 2, 24), time(15, 30)): 70.0,
            ("put", 24700, date(2026, 1, 9), time(9, 30)): 85.0,
            ("put", 24700, date(2026, 2, 24), time(15, 30)): 95.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 2, 28),
        positions=[_position("short_call", "call", 300), _position("short_put", "put", 300)],
        include_mtm=False,
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
        return {"trades": trades, "skipped_expiries": pd.DataFrame(), "daily_mtm": pd.DataFrame()}

    monkeypatch.setattr(backtest_service, "run_backtest", fake_run_backtest)
    monkeypatch.setattr(backtest_service, "save_run", lambda run_id, metadata, results: None)

    _, results = backtest_service.run_backtest_for_strategy(
        strategy_id="custom_multi_leg",
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300)],
        include_mtm=False,
    )

    assert results["daily_mtm"].empty
    assert results["trade_metrics"] == []
    assert results["vix_curve"] == []
    assert results["metrics"]["max_drawdown"] is None


def test_engine_rolls_triggered_leg_on_premium_increase(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("call", 25300, date(2025, 12, 12), time(9, 35)): 160.0,
            ("call", 25500, date(2025, 12, 12), time(9, 35)): 50.0,
            ("call", 25500, date(2026, 1, 27), time(15, 30)): 30.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300)],
        adjustments=[_adjustment(50, 500)],
    )

    trades = results["trades"]

    assert trades["strike"].tolist() == [25300, 25500]
    assert trades["entry_time"].tolist() == ["09:30", "09:35"]
    assert trades["exit_time"].tolist() == ["09:35", "15:30"]
    assert trades["exit_price"].tolist() == [160.0, 30.0]
    assert trades["exit_reason"].tolist() == ["adjustment", "scheduled_exit"]


def test_replacement_leg_uses_own_entry_premium_and_can_roll_again(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("call", 25300, date(2025, 12, 12), time(9, 35)): 160.0,
            ("call", 25500, date(2025, 12, 12), time(9, 35)): 50.0,
            ("call", 25500, date(2025, 12, 12), time(9, 40)): 80.0,
            ("call", 25500, date(2026, 1, 27), time(15, 30)): 30.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300)],
        adjustments=[_adjustment(50, 500)],
    )

    trades = results["trades"]

    assert trades["entry_price"].tolist() == [100.0, 50.0, 80.0]
    assert trades["exit_price"].tolist() == [160.0, 80.0, 30.0]
    assert trades["exit_reason"].tolist() == ["adjustment", "adjustment", "scheduled_exit"]


def test_adjustment_max_adjustments_caps_roll_count(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("call", 25300, date(2025, 12, 12), time(9, 35)): 160.0,
            ("call", 25500, date(2025, 12, 12), time(9, 35)): 50.0,
            ("call", 25500, date(2025, 12, 12), time(9, 40)): 90.0,
            ("call", 25500, date(2026, 1, 27), time(15, 30)): 30.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300)],
        adjustments=[_adjustment(50, 500, max_adjustments=1)],
    )

    trades = results["trades"]

    assert trades["entry_price"].tolist() == [100.0, 50.0]
    assert trades["exit_price"].tolist() == [160.0, 30.0]
    assert trades["exit_reason"].tolist() == ["adjustment", "scheduled_exit"]


def test_adjustment_exits_strategy_when_net_credit_after_roll_is_not_positive(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("put", 24700, date(2025, 12, 12), time(9, 30)): 90.0,
            ("call", 25300, date(2025, 12, 12), time(9, 35)): 220.0,
            ("put", 24700, date(2025, 12, 12), time(9, 35)): 80.0,
            ("call", 25500, date(2025, 12, 12), time(9, 35)): 20.0,
            ("put", 24700, date(2026, 1, 27), time(15, 30)): 10.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300), _position("short_put", "put", 300)],
        adjustments=[_adjustment(50, 500, exit_if_net_credit_lte_zero=True)],
    )

    trades = results["trades"]

    assert trades["leg_role"].tolist() == ["short_call", "short_put"]
    assert trades["exit_reason"].tolist() == ["net_credit_exit", "net_credit_exit"]
    assert trades["exit_time"].tolist() == ["09:35", "09:35"]


def test_newly_opened_leg_is_not_evaluated_on_same_candle(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("call", 25300, date(2025, 12, 12), time(9, 35)): 160.0,
            ("call", 25500, date(2025, 12, 12), time(9, 35)): 100.0,
            ("call", 25500, date(2026, 1, 27), time(15, 30)): 80.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300)],
        adjustments=[_adjustment(50, 500)],
    )

    assert results["trades"]["exit_reason"].tolist() == ["adjustment", "scheduled_exit"]


def test_call_adjustment_does_not_affect_put_leg(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("call", 25300, date(2025, 12, 12), time(9, 35)): 160.0,
            ("call", 25500, date(2025, 12, 12), time(9, 35)): 50.0,
            ("call", 25500, date(2026, 1, 27), time(15, 30)): 30.0,
            ("put", 24700, date(2025, 12, 12), time(9, 30)): 90.0,
            ("put", 24700, date(2026, 1, 27), time(15, 30)): 70.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300), _position("short_put", "put", 300)],
        adjustments=[_adjustment(50, 500)],
    )

    trades = results["trades"]

    assert trades[trades["leg_role"] == "short_call"]["exit_reason"].tolist() == ["adjustment", "scheduled_exit"]
    assert trades[trades["leg_role"] == "short_put"]["exit_reason"].tolist() == ["scheduled_exit"]


def test_missing_replacement_price_ends_only_triggered_leg(monkeypatch):
    _mock_market(
        monkeypatch,
        {
            ("call", 25300, date(2025, 12, 12), time(9, 30)): 100.0,
            ("call", 25300, date(2025, 12, 12), time(9, 35)): 160.0,
            ("put", 24700, date(2025, 12, 12), time(9, 30)): 90.0,
            ("put", 24700, date(2026, 1, 27), time(15, 30)): 70.0,
        },
    )

    results = run_backtest(
        start_date=date(2026, 1, 1),
        end_date=date(2026, 1, 31),
        positions=[_position("short_call", "call", 300), _position("short_put", "put", 300)],
        adjustments=[_adjustment(50, 500)],
    )

    trades = results["trades"]

    assert trades[trades["leg_role"] == "short_call"]["exit_reason"].tolist() == ["adjustment_reentry_failed"]
    assert trades[trades["leg_role"] == "short_put"]["exit_reason"].tolist() == ["scheduled_exit"]


def _mock_market(monkeypatch, option_prices: dict):
    def fake_option_range(**kwargs):
        rows = []
        for key, close in option_prices.items():
            if len(key) == 4:
                right, strike, candle_date, candle_time = key
            else:
                right, strike, candle_date = key
                candle_time = MARKET_CLOSE
            if right != kwargs["right"] or strike != kwargs["strike"]:
                continue
            if kwargs["start"] <= candle_date <= kwargs["end"]:
                rows.append(
                    {
                        "datetime": bar_start_for_end_time(candle_date, candle_time),
                        "close": close,
                    }
                )
        return pd.DataFrame(rows)

    monkeypatch.setattr(market_data_service, "get_underlying_candle", lambda **kwargs: {"close": 25000.0})
    monkeypatch.setattr(market_data_service, "get_option_5m_range", fake_option_range)


def _position(role: str, option_type: str, offset: int) -> PositionConfig:
    return PositionConfig(
        role,
        option_type,
        "sell",
        75,
        "offset",
        {"offset_points": offset},
        45,
        time(9, 30),
        0,
        time(15, 30),
    )


def _adjustment(
    trigger_pct: float,
    offset: int,
    max_adjustments: int | None = None,
    exit_if_net_credit_lte_zero: bool = False,
) -> AdjustmentConfig:
    return AdjustmentConfig(
        trigger=TriggerConfig("premium_increase_pct", trigger_pct),
        action=ActionConfig(
            "roll_triggered_leg",
            StrikeSelectionConfig("offset", {"offset_points": offset}),
        ),
        max_adjustments=max_adjustments,
        exit_if_net_credit_lte_zero=exit_if_net_credit_lte_zero,
    )
