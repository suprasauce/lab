import pandas as pd

from backend.services.metrics_service import (
    build_average_mtm_by_expiry,
    build_backtest_metrics,
    build_equity_curve,
    build_expiry_pnl_curve,
    build_trade_metrics,
)


def test_build_backtest_metrics_derives_pnl_and_mtm():
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "entry_price": 100.0,
                "exit_price": 80.0,
                "lot_size": 75,
            },
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "entry_price": 90.0,
                "exit_price": 110.0,
                "lot_size": 75,
            },
        ]
    )
    skipped = pd.DataFrame(
        [
            {"expiry_date": "2026-02-24", "reason": "spot_no_price"},
            {"expiry_date": "2026-02-24", "reason": "spot_no_price"},
        ]
    )
    daily_mtm = pd.DataFrame(
        [
            {"mtm_date": "2026-01-01", "mtm": 0.0},
            {"mtm_date": "2026-01-02", "mtm": 1500.0},
            {"mtm_date": "2026-01-03", "mtm": -750.0},
        ]
    )

    metrics = build_backtest_metrics(
        trades=trades,
        skipped_expiries=skipped,
        daily_mtm=daily_mtm,
    )

    assert metrics["total_pnl"] == 0.0
    assert metrics["traded_expiries"] == 1
    assert metrics["skipped_expiries"] == 1
    assert "total_legs" not in metrics
    assert metrics["max_drawdown"] == -2250.0
    assert metrics["most_common_skip_reason"] == "spot_no_price"


def test_build_backtest_metrics_derives_rr_and_expectancy():
    trades = pd.DataFrame(
        [
            {
                "trade_id": "win-1",
                "expiry_date": "2026-01-27",
                "entry_price": 100.0,
                "exit_price": 80.0,
                "lot_size": 75,
            },
            {
                "trade_id": "win-2",
                "expiry_date": "2026-02-24",
                "entry_price": 120.0,
                "exit_price": 80.0,
                "lot_size": 75,
            },
            {
                "trade_id": "loss-1",
                "expiry_date": "2026-03-31",
                "entry_price": 100.0,
                "exit_price": 160.0,
                "lot_size": 75,
            },
        ]
    )

    metrics = build_backtest_metrics(
        trades=trades,
        skipped_expiries=pd.DataFrame(),
        daily_mtm=pd.DataFrame(),
        include_mtm=False,
    )

    assert metrics["average_profit"] == 2250.0
    assert metrics["average_loss"] == 4500.0
    assert metrics["risk_reward_ratio"] == 2.0
    assert metrics["expectancy"] == 0.0


def test_build_backtest_metrics_rr_is_none_without_profit():
    trades = pd.DataFrame(
        [
            {
                "trade_id": "loss-1",
                "expiry_date": "2026-01-27",
                "entry_price": 100.0,
                "exit_price": 110.0,
                "lot_size": 75,
            },
        ]
    )

    metrics = build_backtest_metrics(
        trades=trades,
        skipped_expiries=pd.DataFrame(),
        daily_mtm=pd.DataFrame(),
        include_mtm=False,
    )

    assert metrics["risk_reward_ratio"] is None
    assert metrics["expectancy"] == -750.0


def test_build_equity_curve_returns_realized_curve():
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-03",
                "exit_date": "2026-01-03",
                "entry_price": 100.0,
                "exit_price": 80.0,
                "lot_size": 75,
            },
            {
                "trade_id": "trade-2",
                "expiry_date": "2026-01-05",
                "exit_date": "2026-01-05",
                "entry_price": 50.0,
                "exit_price": 60.0,
                "lot_size": 75,
            },
        ]
    )
    curve = build_equity_curve(trades)

    assert curve == [
        {"date": "2026-01-03", "equity": 1500.0},
        {"date": "2026-01-05", "equity": 750.0},
    ]


def test_build_expiry_pnl_curve_returns_expiry_bars():
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "entry_price": 100.0,
                "exit_price": 80.0,
                "lot_size": 75,
            },
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "entry_price": 50.0,
                "exit_price": 70.0,
                "lot_size": 75,
            },
            {
                "trade_id": "trade-2",
                "expiry_date": "2026-02-24",
                "entry_price": 40.0,
                "exit_price": 50.0,
                "lot_size": 75,
            },
        ]
    )

    curve = build_expiry_pnl_curve(trades)

    assert curve == [
        {"date": "2026-01-27", "pnl": 0.0},
        {"date": "2026-02-24", "pnl": -750.0},
    ]


def test_build_average_mtm_by_expiry_returns_average_5m_mtm_pct_of_premium():
    trades = pd.DataFrame(
        [
            {"expiry_date": "2026-01-27", "entry_price": 10.0, "lot_size": 100},
            {"expiry_date": "2026-02-24", "entry_price": 20.0, "lot_size": 100},
        ]
    )
    daily_mtm = pd.DataFrame(
        [
            {"expiry_date": "2026-01-27", "mtm_date": "2026-01-01", "mtm_time": "09:30", "mtm": 100.0},
            {"expiry_date": "2026-01-27", "mtm_date": "2026-01-01", "mtm_time": "09:30", "mtm": -50.0},
            {"expiry_date": "2026-01-27", "mtm_date": "2026-01-01", "mtm_time": "09:35", "mtm": 200.0},
            {"expiry_date": "2026-02-24", "mtm_date": "2026-02-01", "mtm_time": "09:30", "mtm": -300.0},
        ]
    )

    curve = build_average_mtm_by_expiry(trades=trades, daily_mtm=daily_mtm)

    assert curve == [
        {"date": "2026-01-27", "average_mtm_pct_of_premium": 12.5},
        {"date": "2026-02-24", "average_mtm_pct_of_premium": -15.0},
    ]


def test_build_trade_metrics_mtm_volatility_pct_of_premium():
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "exit_date": "2026-01-20",
                "exit_reason": "scheduled_exit",
                "entry_price": 100,
                "lot_size": 10,
            },
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "exit_date": "2026-01-20",
                "exit_reason": "scheduled_exit",
                "entry_price": 50,
                "lot_size": 10,
            },
        ]
    )
    daily_mtm = pd.DataFrame(
        [
            {"trade_id": "trade-1", "mtm_date": "2026-01-01", "mtm": 0},
            {"trade_id": "trade-1", "mtm_date": "2026-01-02", "mtm": 150},
            {"trade_id": "trade-1", "mtm_date": "2026-01-03", "mtm": -150},
        ]
    )

    metrics = build_trade_metrics(trades=trades, daily_mtm=daily_mtm)

    assert metrics == [
        {
            "trade_id": "trade-1",
            "expiry_date": "2026-01-27",
            "exit_date": "2026-01-20",
            "exit_reason": "scheduled_exit",
            "premium_received": 1500.0,
            "maxMtm": 150.0,
            "minMtm": -150.0,
            "mtmVolatilityPctOfPremium": 15.0,
        }
    ]


def test_build_trade_metrics_handles_missing_or_short_data():
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "exit_date": "2026-01-27",
                "entry_price": 0,
                "lot_size": 10,
            }
        ]
    )
    one_mtm = pd.DataFrame([{"trade_id": "trade-1", "mtm_date": "2026-01-01", "mtm": 0}])

    metrics = build_trade_metrics(trades=trades, daily_mtm=one_mtm)

    assert metrics[0]["mtmVolatilityPctOfPremium"] is None
